package storage

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

type LeastRecentlyUsedCache struct {
	storageIndex map[string]*list.Element
	storageList  *list.List
	mu           sync.RWMutex
}

func NewLeastRecentlyUsedCache() *LeastRecentlyUsedCache {
	return &LeastRecentlyUsedCache{
		storageIndex: make(map[string]*list.Element),
		storageList:  list.New(),
	}
}

func (lru *LeastRecentlyUsedCache) evict() error {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	element := lru.storageList.Back()
	if element == nil {
		return errors.New("eviction error")
	}
	entry, ok := element.Value.(CacheEntry)
	if !ok {
		return errors.New("eviction error")
	}
	delete(lru.storageIndex, entry.key)
	lru.storageList.Remove(element)
	return nil
}

func (lru *LeastRecentlyUsedCache) Set(key string, value []byte) error {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	if element, exists := lru.storageIndex[key]; exists {
		entry, ok := element.Value.(CacheEntry)
		if !ok {
			return errors.New("key set error")
		}
		entry.value = value
		entry.lastModifiedTime = time.Now()
		element.Value = entry
		lru.storageList.MoveToFront(element)
		return nil
	}
	if lru.storageList.Len() >= defaultMaxSize {
		err := lru.evict()
		if err != nil {
			return errors.New("key set error")
		}
	}
	element := lru.storageList.PushFront(CacheEntry{
		key:              key,
		value:            value,
		lastModifiedTime: time.Now(),
	})
	lru.storageIndex[key] = element
	return nil
}
func (lru *LeastRecentlyUsedCache) Get(key string) ([]byte, error) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	element, exists := lru.storageIndex[key]
	if !exists {
		return nil, nil
	}
	if entry, ok := element.Value.(CacheEntry); !ok {
		return nil, errors.New("cache get error")
	} else {
		lru.storageList.MoveToFront(element)
		return entry.value, nil
	}
}
func (lru *LeastRecentlyUsedCache) Len() int {
	lru.mu.RLock()
	defer lru.mu.RUnlock()
	return len(lru.storageIndex)
}
func (lru *LeastRecentlyUsedCache) AddToRebalance(pairsToAdd []CacheEntry) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	index := 0
	for element := lru.storageList.Front(); element != nil && index < len(pairsToAdd); element = element.Next() {
		if entry, ok := element.Value.(CacheEntry); ok {
			if entry.lastModifiedTime.Before(pairsToAdd[index].lastModifiedTime) {
				newElement := lru.storageList.InsertBefore(pairsToAdd[index], element)
				lru.storageIndex[pairsToAdd[index].key] = newElement
				index++
				if lru.Len() > defaultMaxSize {
					lru.evict()
				}
			}
		}
	}
	for index < len(pairsToAdd) && lru.Len() < defaultMaxSize {
		newElement := lru.storageList.PushBack(pairsToAdd[index])
		lru.storageIndex[pairsToAdd[index].key] = newElement
		index++
	}

}
func (lru *LeastRecentlyUsedCache) RemoveKeyToRebalance(keysToRemove []string) []CacheEntry {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	output := make([]CacheEntry, 0, len(keysToRemove))
	for _, value := range keysToRemove {
		if element, exists := lru.storageIndex[value]; exists {
			if entry, ok := element.Value.(CacheEntry); ok {
				output = append(output, entry)
				lru.storageList.Remove(element)
				delete(lru.storageIndex, entry.key)
			}
		}
	}
	return output
}

func (lru *LeastRecentlyUsedCache) GetAllEntries() []CacheEntry {
	lru.mu.RLock()
	defer lru.mu.RUnlock()
	entries := make([]CacheEntry, 0, len(lru.storageIndex))
	for element := lru.storageList.Front(); element != nil; element = element.Next() {
		if entry, ok := element.Value.(CacheEntry); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}
