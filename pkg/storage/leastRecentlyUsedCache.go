package storage

import (
	"container/list"
	"errors"
	"sync"

	"go.uber.org/zap"
)

type LeastRecentlyUsedCache struct {
	storageIndex map[string]*list.Element
	storageList  *list.List
	logger       *zap.Logger
	mu           sync.RWMutex
}

type linkedListEntry struct {
	key   string
	value []byte
}

func NewLeastRecentlyUsedCache(logger *zap.Logger) *LeastRecentlyUsedCache {
	logger.Debug("creating least recently used cache")
	return &LeastRecentlyUsedCache{
		storageIndex: make(map[string]*list.Element),
		storageList:  list.New(),
		logger:       logger,
	}
}

func (lru *LeastRecentlyUsedCache) evict() error {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	lru.logger.Debug("evicting least recently used entry")
	element := lru.storageList.Back()
	if element == nil {
		lru.logger.Error("evict function is called when list is already empty")
		return errors.New("eviction error")
	}
	entry, ok := element.Value.(linkedListEntry)
	if !ok {
		lru.logger.Error("cache element has wrong type")
		return errors.New("eviction error")
	}
	delete(lru.storageIndex, entry.key)
	lru.storageList.Remove(element)
	lru.logger.Info("evicted least recently used key", zap.String("key", entry.key))
	return nil
}
func (lru *LeastRecentlyUsedCache) Has(key string) (*list.Element, bool) {
	lru.mu.RLock()
	defer lru.mu.RUnlock()
	element, exists := lru.storageIndex[key]
	if !exists {
		lru.logger.Info("cache miss", zap.String("key", key))
	} else {
		lru.logger.Info("cache hit", zap.String("key", key))
	}
	return element, exists
}

func (lru *LeastRecentlyUsedCache) Set(key string, value []byte) error {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	lru.logger.Debug("setting LRU cache value on key", zap.String("key", key), zap.String("value", string(value)))
	if element, exists := lru.Has(key); exists {
		entry, ok := element.Value.(linkedListEntry)
		if !ok {
			lru.logger.Error("cache element has wrong type for key", zap.String("key", key))
			return errors.New("key set error")
		}
		entry.value = value
		element.Value = entry
		lru.storageList.MoveToFront(element)
		return nil
	}
	if lru.storageList.Len() >= defaultMaxSize {
		err := lru.evict()
		if err != nil {
			lru.logger.Warn("an error has occured while trying to evict an element", zap.Error(err))
			return errors.New("key set error")
		}
	}
	element := lru.storageList.PushFront(linkedListEntry{
		key:   key,
		value: value,
	})
	lru.storageIndex[key] = element
	lru.logger.Info("set key value in LRU", zap.String("key", key))
	return nil
}
func (lru *LeastRecentlyUsedCache) Get(key string) ([]byte, error) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	lru.logger.Debug("finding value for key in LRU cache", zap.String("key", key))
	element, exists := lru.Has(key)
	if !exists {
		lru.logger.Info("cache miss for key", zap.String("key", key))
		return nil, nil
	}
	if entry, ok := element.Value.(linkedListEntry); !ok {
		lru.logger.Error("key returned list node which is of wrong type", zap.String("key", key))
		return nil, errors.New("cache get error")
	} else {
		lru.storageList.MoveToFront(element)
		lru.logger.Info("cache hit for key", zap.String("key", key))
		return entry.value, nil
	}
}
func (lru *LeastRecentlyUsedCache) Len() int {
	return len(lru.storageIndex)
}
