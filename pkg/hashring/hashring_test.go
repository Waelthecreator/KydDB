package hashring

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const key = "testKey"
const nodeID1 = "node1"
const nodeID2 = "node2"

func TestNewHashRing(t *testing.T) {
	hr := NewHashRing()
	assert.NotNil(t, hr)
	assert.NotNil(t, hr.ring)
	assert.NotNil(t, hr.sortedKeys)
	assert.NotNil(t, hr.nodes)
}
func TestAddNode(t *testing.T) {
	hr := NewHashRing()
	hr.AddNode(nodeID1)
	assert.True(t, hr.nodes[nodeID1])
	assert.Equal(t, 100, len(hr.ring))
	assert.Equal(t, 100, len(hr.sortedKeys))
}
func TestRemoveNode(t *testing.T) {
	hr := NewHashRing()
	hr.AddNode(nodeID1)
	hr.RemoveNode(nodeID1)
	assert.False(t, hr.nodes[nodeID1])
	assert.Equal(t, 0, len(hr.ring))
	assert.Equal(t, 0, len(hr.sortedKeys))
}
func TestGetNode(t *testing.T) {
	hr := NewHashRing()
	hr.AddNode(nodeID1)
	hr.AddNode(nodeID2)
	nodeForKey, err := hr.GetNode(key)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodeForKey)
	assert.Contains(t, []string{nodeID1, nodeID2}, nodeForKey)
	hr.RemoveNode(nodeID1)
	nodeForKeyAfterRemoval, err := hr.GetNode(key)
	assert.NoError(t, err)
	assert.Equal(t, nodeID2, nodeForKeyAfterRemoval)
}
func TestGetNodeWithNoNodes(t *testing.T) {
	hr := NewHashRing()
	_, err := hr.GetNode(key)
	assert.Error(t, err)
}
func TestAddNodeTwice(t *testing.T) {
	hr := NewHashRing()
	hr.AddNode(nodeID1)
	initialRingSize := len(hr.ring)
	hr.AddNode(nodeID1)
	assert.Equal(t, initialRingSize, len(hr.ring))
	assert.True(t, hr.nodes[nodeID1])
}
func TestRemoveNonExistentNode(t *testing.T) {
	hr := NewHashRing()
	hr.AddNode(nodeID1)
	hr.RemoveNode(nodeID2)
	assert.True(t, hr.nodes[nodeID1])
	assert.False(t, hr.nodes[nodeID2])
	assert.Equal(t, 100, len(hr.ring))
	assert.Equal(t, 100, len(hr.sortedKeys))
}
