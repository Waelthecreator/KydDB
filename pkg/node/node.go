package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	pb "github.com/Waelthecreator/KydDB/api/proto"
	cs "github.com/Waelthecreator/KydDB/pkg/clusterState"
	"github.com/Waelthecreator/KydDB/pkg/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type clientConn struct {
	client pb.NodeServiceClient
	conn   *grpc.ClientConn
}

type Node struct {
	pb.UnimplementedNodeServiceServer
	ID               string
	Address          string
	Port             int
	cache            storage.Storage
	cluster          *cs.ClusterState
	grpcServer       *grpc.Server
	listener         net.Listener
	clients          map[string]*clientConn
	clientsMu        sync.RWMutex
	healthCheckTimer time.Duration
	nodeTimeout      time.Duration
	grpcTimeout      time.Duration
	isRunning        bool
	runningMu        sync.Mutex
	stopChan         chan struct{}
	wg               sync.WaitGroup
	logger           *log.Logger
}

type NodeOptions struct {
	ID                  string
	Address             string
	Port                int
	CacheSize           int
	HealthCheckInterval time.Duration
	NodeTimeout         time.Duration
	GRPCTimeout         time.Duration
	VirtualNodes        int
}

func NewNode(opts NodeOptions) (*Node, error) {
	if opts.ID == "" {
		return nil, fmt.Errorf("node ID cannot be empty")
	}
	if opts.Address == "" {
		return nil, fmt.Errorf("node address cannot be empty")
	}
	if opts.Port <= 0 || opts.Port > 65535 {
		return nil, fmt.Errorf("invalid port number: %d", opts.Port)
	}
	if opts.HealthCheckInterval <= 0 {
		opts.HealthCheckInterval = 10 * time.Second
	}
	if opts.NodeTimeout <= 0 {
		opts.NodeTimeout = 30 * time.Second
	}
	if opts.GRPCTimeout <= 0 {
		opts.GRPCTimeout = 5 * time.Second
	}
	n := &Node{
		ID:               opts.ID,
		Address:          opts.Address,
		Port:             opts.Port,
		cache:            storage.NewLeastRecentlyUsedCache(opts.CacheSize),
		cluster:          cs.NewClusterState(opts.VirtualNodes, opts.ID),
		grpcServer:       nil,
		listener:         nil,
		clients:          make(map[string]*clientConn),
		clientsMu:        sync.RWMutex{},
		healthCheckTimer: opts.HealthCheckInterval,
		grpcTimeout:      opts.GRPCTimeout,
		isRunning:        false,
		runningMu:        sync.Mutex{},
		stopChan:         make(chan struct{}),
		wg:               sync.WaitGroup{},
		logger:           log.New(log.Writer(), fmt.Sprintf("[Node %s] ", opts.ID), log.LstdFlags),
	}

	selfInfo := &pb.NodeInfo{
		Id:       opts.ID,
		Address:  opts.Address,
		Port:     int32(opts.Port),
		LastSeen: timestamppb.Now(),
	}
	n.cluster.AddNode(selfInfo)
	return n, nil
}
func (n *Node) Start(bootstrapAddress string) error {
	n.runningMu.Lock()
	defer n.runningMu.Unlock()

	if n.isRunning {
		return fmt.Errorf("node is already running")
	}
	n.logger.Printf("Starting node on %s:%d", n.Address, n.Port)
	var err error
	n.listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", n.Address, n.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	n.grpcServer = grpc.NewServer()
	pb.RegisterNodeServiceServer(n.grpcServer, n)
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		if err := n.grpcServer.Serve(n.listener); err != nil {
			n.logger.Printf("gRPC server error: %v", err)
		}
	}()
	n.wg.Add(1)
	go n.healthCheckLoop()
	n.isRunning = true
	if bootstrapAddress != "" {
		if err := n.joinCluster(bootstrapAddress); err != nil {
			n.logger.Printf("Failed to join cluster: %v", err)
		}
	}
	n.logger.Printf("Node started successfully. Cluster size: %d", n.cluster.GetSize())
	return nil
}

func (n *Node) getOrCreateClient(nodeID, address string, port int32) (pb.NodeServiceClient, error) {
	n.clientsMu.RLock()
	if client, exists := n.clients[nodeID]; exists {
		n.clientsMu.RUnlock()
		return client.client, nil
	}
	n.clientsMu.RUnlock()
	n.clientsMu.Lock()
	defer n.clientsMu.Unlock()
	if client, exists := n.clients[nodeID]; exists {
		return client.client, nil
	}
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", address, port),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
	}
	client := &clientConn{
		client: pb.NewNodeServiceClient(conn),
		conn:   conn,
	}
	n.clients[nodeID] = client
	return client.client, nil
}

func (n *Node) removeClient(nodeID string) {
	n.clientsMu.Lock()
	defer n.clientsMu.Unlock()
	if client, exists := n.clients[nodeID]; exists {
		client.conn.Close()
		delete(n.clients, nodeID)
		n.logger.Printf("Removed connection to node %s", nodeID)
	}
}

func (n *Node) joinCluster(bootstrapAddress string) error {
	n.logger.Printf("Joining cluster via bootstrap node at %s", bootstrapAddress)
	ctx, cancel := context.WithTimeout(context.Background(), n.grpcTimeout)
	defer cancel()
	conn, err := grpc.NewClient(bootstrapAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to bootstrap node: %w", err)
	}
	client := pb.NewNodeServiceClient(conn)
	resp, err := client.Join(ctx, &pb.JoinRequest{
		NodeInfo: &pb.NodeInfo{
			Id:       n.ID,
			Address:  n.Address,
			Port:     int32(n.Port),
			Status:   "ALIVE",
			LastSeen: timestamppb.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to join cluster: %w", err)
	}
	n.clientsMu.Lock()
	n.clients[resp.BootstrapNode.Id] = &clientConn{
		client: client,
		conn:   conn,
	}
	n.clientsMu.Unlock()
	n.cluster.AddNode(resp.BootstrapNode)
	n.cluster.JoinCluster(resp.Nodes)
	n.logger.Printf("Joined cluster successfully. Current cluster size: %d", n.cluster.GetSize())
	return nil
}

func (n *Node) healthCheck() {
	targetNode := n.cluster.GetRandomNodeInfo()
	if targetNode == nil {
		return
	}
	client, err := n.getOrCreateClient(targetNode.Id, targetNode.Address, targetNode.Port)
	if err != nil {
		n.logger.Printf("Health check: failed to get client for node %s: %v", targetNode.Id, err)
		n.cluster.HandleFailedHealthCheck(targetNode.Id)
		n.removeClient(targetNode.Id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.grpcTimeout)
	defer cancel()
	resp, err := client.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		n.logger.Printf("Health check: node %s is unresponsive: %v", targetNode.Id, err)
		n.cluster.HandleFailedHealthCheck(targetNode.Id)
		n.removeClient(targetNode.Id)
		return
	}
	n.cluster.HandleSuccessfulHealthCheck(targetNode.Id)
	n.logger.Printf("Health check: node %s is healthy. Status: %s", targetNode.Id, resp.Status)
	if resp.Delta != nil {
		n.cluster.MergeDeltas(resp.Delta)
	}
}

func (n *Node) healthCheckLoop() {
	defer n.wg.Done()
	ticker := time.NewTicker(n.healthCheckTimer)
	defer ticker.Stop()
	for {
		select {
		case <-n.stopChan:
			n.logger.Printf("Health check loop stopped")
			return
		case <-ticker.C:
			n.healthCheck()
		}
	}
}
