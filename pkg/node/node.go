package node

import (
	"fmt"
	"time"
)

type NodeInfo struct {
	ID       string
	Address  string
	Port     int
	LastSeen time.Time
}

func (ni *NodeInfo) FullAddress() string {
	return fmt.Sprintf("http://%s:%d", ni.Address, ni.Port)
}
