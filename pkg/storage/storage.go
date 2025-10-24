package storage

import (
	"time"
)

const (
	defaultMaxSize = 100
)

type CacheEntry struct {
	Key              string
	Value            []byte
	LastModifiedTime time.Time
}

type Storage interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, error)
	Len() int
	AddToRebalance(pairsToAdd []CacheEntry) error
	RemoveKeyToRebalance(keysToRemove []string) []CacheEntry
	GetAllEntries() []CacheEntry
}
