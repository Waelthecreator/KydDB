package storage

import (
	"time"
)

const (
	defaultMaxSize = 100
)

type CacheEntry struct {
	key              string
	value            []byte
	lastModifiedTime time.Time
}

type Storage interface {
	Set(key string, value []byte) error
	Delete(key string) error
	Get(key string) ([]byte, error)
	Len() int
	AddToRebalance(pairsToAdd []CacheEntry)
	RemoveKeyToRebalance(keysToRemove []string) []CacheEntry
	GetAllEntries() []CacheEntry
}
