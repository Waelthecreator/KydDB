package storage

import "container/list"

const (
	defaultMaxSize = 1000
)

type Storage interface {
	Set(key string, value []byte) error
	Delete(key string) error
	Get(key string) ([]byte, error)
	Len() int
	Has(key string) (*list.Element, bool)
	Rebalance(keysToAdd map[string][]byte)
}
