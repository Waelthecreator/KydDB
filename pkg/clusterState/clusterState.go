package clusterState

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	pb "github.com/Waelthecreator/KydDB/api/proto"
	hr "github.com/Waelthecreator/KydDB/pkg/hashring"
	rq "github.com/Waelthecreator/KydDB/pkg/queue"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Deltas struct {
	Added     *rq.RingQueue[*pb.NodeInfo]
	Suspected *rq.RingQueue[*pb.NodeInfo]
	Removed   *rq.RingQueue[*pb.NodeInfo]
}

type ClusterState struct {
	nodeID     string
	mu         sync.RWMutex
	nodes      []*pb.NodeInfo
	nodesIndex map[string]int
	ring       *hr.HashRing
	deltas     Deltas
	size       int

	nodeDownThreshold time.Duration
	eventTTL          time.Duration
	stopCh            chan struct{}
}

func NewClusterState(vnode int, nodeID string) *ClusterState {
	clusterState := &ClusterState{
		nodeID:     nodeID,
		nodes:      make([]*pb.NodeInfo, 0),
		nodesIndex: make(map[string]int),
		ring:       hr.NewHashRing(vnode),
		deltas: Deltas{
			Added:     rq.NewRingQueue[*pb.NodeInfo](10),
			Suspected: rq.NewRingQueue[*pb.NodeInfo](10),
			Removed:   rq.NewRingQueue[*pb.NodeInfo](10),
		},
		size:              0,
		nodeDownThreshold: 10 * time.Second,
		eventTTL:          30 * time.Second,
		stopCh:            make(chan struct{}),
	}
	go clusterState.cleanupLoop()
	return clusterState
}

func (cs *ClusterState) Stop() {
	close(cs.stopCh)
}

func (cs *ClusterState) AddNode(node *pb.NodeInfo) {
	if node.Id == cs.nodeID {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.addNodeHelper(node)
}

func (cs *ClusterState) addNodeHelper(node *pb.NodeInfo) {
	if _, exists := cs.nodesIndex[node.Id]; !exists {
		cs.nodes = append(cs.nodes, node)
		cs.nodesIndex[node.Id] = len(cs.nodes) - 1
		cs.ring.AddNode(node.Id)
		node.LastSeen = timestamppb.Now()
		cs.deltas.Added.Enqueue(node)
		cs.size++
	}
}

func (cs *ClusterState) RemoveNode(nodeID string) {
	if nodeID == cs.nodeID {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.removeNodeHelper(nodeID)
}

func (cs *ClusterState) removeNodeHelper(nodeID string) {
	if index, exists := cs.nodesIndex[nodeID]; exists {
		node := cs.nodes[index]
		lastIdx := len(cs.nodes) - 1
		lastNode := cs.nodes[lastIdx]
		cs.nodes[index] = lastNode
		cs.nodes = cs.nodes[:lastIdx]
		cs.nodesIndex[lastNode.Id] = index
		delete(cs.nodesIndex, nodeID)
		cs.ring.RemoveNode(nodeID)
		node.Status = "REMOVED"
		node.LastSeen = timestamppb.Now()
		cs.deltas.Removed.Enqueue(node)
		cs.size--
	}
}

func (cs *ClusterState) suspectNode(nodeID string) {
	if index, exists := cs.nodesIndex[nodeID]; exists {
		node := cs.nodes[index]
		node.Status = "SUSPECTED"
		node.LastSeen = timestamppb.Now()
		cs.deltas.Suspected.Enqueue(node)
	}
}

func (cs *ClusterState) restoreNode(nodeID string) {
	if index, exists := cs.nodesIndex[nodeID]; exists {
		node := cs.nodes[index]
		node.Status = "ALIVE"
		node.LastSeen = timestamppb.Now()
		cs.deltas.Added.Enqueue(node)
	}
}

func (cs *ClusterState) updateNodeLastSeen(nodeID string) {
	if index, exists := cs.nodesIndex[nodeID]; exists {
		node := cs.nodes[index]
		node.Status = "ALIVE"
		node.LastSeen = timestamppb.Now()
	}
}

func (cs *ClusterState) GetSize() int {
	return cs.size
}

func (cs *ClusterState) GetNode(key string) (*pb.NodeInfo, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	nodeID, err := cs.ring.GetNode(key)
	if err != nil {
		return nil, err
	}
	index, exists := cs.nodesIndex[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}
	return cs.nodes[index], nil
}

func (cs *ClusterState) GetAllNodes() []*pb.NodeInfo {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.nodes
}

func (cs *ClusterState) GetDeltas() *pb.ClusterDelta {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return &pb.ClusterDelta{
		Added:     cs.deltas.Added.Items(),
		Suspected: cs.deltas.Suspected.Items(),
		Removed:   cs.deltas.Removed.Items(),
	}
}

func (cs *ClusterState) JoinCluster(nodes []*pb.NodeInfo) {
	for _, node := range nodes {
		cs.AddNode(node)
	}
}

func (cs *ClusterState) MergeDeltas(delta *pb.ClusterDelta) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, node := range delta.Added {
		cs.mergeNodeState(node)
	}
	for _, node := range delta.Suspected {
		cs.mergeNodeState(node)
	}
	for _, node := range delta.Removed {
		cs.mergeNodeState(node)
	}
}

func (cs *ClusterState) mergeNodeState(remoteNodeState *pb.NodeInfo) {
	if remoteNodeState.Id == cs.nodeID {
		return
	}
	localNodeStateIndex, exists := cs.nodesIndex[remoteNodeState.Id]
	if !exists {
		if remoteNodeState.Status != "REMOVED" {
			cs.addNodeHelper(remoteNodeState)
		}
		return
	}
	localNodeState := cs.nodes[localNodeStateIndex]
	if remoteNodeState.LastSeen.AsTime().After(localNodeState.LastSeen.AsTime()) {
		switch remoteNodeState.Status {
		case "ALIVE":
			if localNodeState.Status == "REMOVED" {
				return
			} else if localNodeState.Status == "ALIVE" {
				cs.updateNodeLastSeen(remoteNodeState.Id)
			} else {
				cs.restoreNode(remoteNodeState.Id)
			}
		case "SUSPECTED":
			if localNodeState.Status == "REMOVED" {
				return
			} else if localNodeState.Status == "ALIVE" {
				cs.suspectNode(remoteNodeState.Id)
			} else {
				cs.updateNodeLastSeen(remoteNodeState.Id)
			}
		case "REMOVED":
			cs.removeNodeHelper(remoteNodeState.Id)
		}
	}
}

func (cs *ClusterState) cleanupExpiredEvents(queue *rq.RingQueue[*pb.NodeInfo]) {
	for {
		node, ok := queue.Peek()
		if !ok {
			break
		}
		if time.Since(node.LastSeen.AsTime()) > cs.eventTTL {
			queue.Dequeue()
		} else {
			break
		}
	}
}

func (cs *ClusterState) HandleFailedHealthCheck(nodeID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	index := cs.nodesIndex[nodeID]
	node := cs.nodes[index]
	now := timestamppb.Now()
	switch node.Status {
	case "ALIVE":
		cs.suspectNode(nodeID)
	case "SUSPECTED":
		downDuration := now.AsTime().Sub(node.LastSeen.AsTime())
		if downDuration >= cs.nodeDownThreshold {
			cs.removeNodeHelper(nodeID)
		}
	}
}

func (cs *ClusterState) HandleSuccessfulHealthCheck(nodeID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	index := cs.nodesIndex[nodeID]
	node := cs.nodes[index]
	if node.Status != "ALIVE" {
		cs.restoreNode(nodeID)
	} else {
		cs.updateNodeLastSeen(nodeID)
	}
}

func (cs *ClusterState) GetRandomNodeInfo() *pb.NodeInfo {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.size == 0 {
		return nil
	}
	index := rand.Intn(cs.size)
	return cs.nodes[index]
}

func (cs *ClusterState) cleanupLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cs.cleanupExpiredEvents(cs.deltas.Added)
			cs.cleanupExpiredEvents(cs.deltas.Suspected)
			cs.cleanupExpiredEvents(cs.deltas.Removed)
		case <-cs.stopCh:
			return
		}

	}
}
