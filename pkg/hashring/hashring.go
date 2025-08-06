package hashring

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

const (
	defaultVirtualNodes = 100
)

type HashRing struct {
	mu         sync.RWMutex
	ring       map[uint32]string
	sortedKeys []uint32
	nodes      map[string]bool
}

func HashKey(key string) uint32 {
	hashWriter := fnv.New32a()
	hashWriter.Write([]byte(key))
	return hashWriter.Sum32()
}

func (hr *HashRing) AddNode(nodeID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	if hr.nodes[nodeID] {
		return
	}
	hr.nodes[nodeID] = true
	for i := 0; i < defaultVirtualNodes; i++ {
		virtualNodeKey := fmt.Sprintf("%s-%d", nodeID, i)
		virtualNodeKeyHash := HashKey(virtualNodeKey)
		hr.ring[virtualNodeKeyHash] = nodeID
		hr.sortedKeys = append(hr.sortedKeys, virtualNodeKeyHash)
	}
	sort.Slice(hr.sortedKeys, func(i, j int) bool {
		return hr.sortedKeys[i] < hr.sortedKeys[j]
	})
}

func (hr *HashRing) RemoveNode(nodeID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	if !hr.nodes[nodeID] {
		return
	}
	delete(hr.nodes, nodeID)
	newSortedKeys := make([]uint32, 0, len(hr.sortedKeys))
	for _, hash := range hr.sortedKeys {
		if hr.ring[hash] != nodeID {
			newSortedKeys = append(newSortedKeys, hash)
		} else {
			delete(hr.ring, hash)
		}
	}
	hr.sortedKeys = newSortedKeys
}

func (hr *HashRing) GetNode(key string) (string, error) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	if len(hr.nodes) == 0 {
		return "", fmt.Errorf("no nodes available")
	}
	keyHash := HashKey(key)
	idx := sort.Search(len(hr.sortedKeys), func(i int) bool {
		return hr.sortedKeys[i] >= keyHash
	})
	if idx == len(hr.sortedKeys) {
		idx = 0
	}
	nodeID := hr.ring[hr.sortedKeys[idx]]
	return nodeID, nil
}

type VirtualNode struct {
	Hash   uint32
	NodeID string
}

func (hr *HashRing) GetVirtualNodes() []VirtualNode {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	virtualNodes := make([]VirtualNode, 0, len(hr.ring))
	for hash, nodeID := range hr.ring {
		virtualNodes = append(virtualNodes, VirtualNode{
			Hash:   hash,
			NodeID: nodeID,
		})
	}
	return virtualNodes
}

func NewHashRing() *HashRing {
	return &HashRing{
		ring:       make(map[uint32]string),
		sortedKeys: make([]uint32, 0),
		nodes:      make(map[string]bool),
	}
}
