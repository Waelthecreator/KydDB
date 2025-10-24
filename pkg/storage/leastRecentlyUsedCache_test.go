package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewLRU(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(10)
	assert.NotNil(t, lru)
	assert.Equal(t, 10, lru.maxSize)
	assert.NotNil(t, lru.storageIndex)
	assert.NotNil(t, lru.storageList)
}
func TestNewLRUwithSizeLessThanZero(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(-1)
	assert.NotNil(t, lru)
	assert.Equal(t, defaultMaxSize, lru.maxSize)
	assert.NotNil(t, lru.storageIndex)
	assert.NotNil(t, lru.storageList)
}

func TestEviction(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	err := lru.Set("key1", []byte("value1"))
	assert.NoError(t, err)
	err = lru.evict()
	assert.NoError(t, err)
	value, err := lru.Get("key1")
	assert.NoError(t, err)
	assert.Nil(t, value)
}
func TestEvictionWithNoElementsError(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	err := lru.evict()
	assert.Error(t, err)
}
func TestEvictionWithNonCacheEntryError(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	lru.storageList.PushFront("String Entry")
	err := lru.evict()
	assert.Error(t, err)
}
func TestSetAndGet(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	err := lru.Set("key1", []byte("value1"))
	assert.NoError(t, err)
	value, err := lru.Get("key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("value1"), value)
}
func TestSetMostRecentVal(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	err := lru.Set("key1", []byte("value1"))
	assert.NoError(t, err)
	err = lru.Set("key1", []byte("value2"))
	assert.NoError(t, err)
	value, err := lru.Get("key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("value2"), value)
}
func TestSetWithEviction(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	err := lru.Set("key1", []byte("value1"))
	assert.NoError(t, err)
	err = lru.Set("key2", []byte("value2"))
	assert.NoError(t, err)
	err = lru.Set("key3", []byte("value3"))
	assert.NoError(t, err)
	value, err := lru.Get("key1")
	assert.NoError(t, err)
	assert.Nil(t, value)
	value, err = lru.Get("key2")
	assert.NoError(t, err)
	assert.Equal(t, []byte("value2"), value)
	value, err = lru.Get("key3")
	assert.NoError(t, err)
	assert.Equal(t, []byte("value3"), value)
}
func TestLRULen(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	assert.Equal(t, 0, lru.Len())
	err := lru.Set("key1", []byte("value1"))
	assert.NoError(t, err)
	assert.Equal(t, 1, lru.Len())
	err = lru.Set("key2", []byte("value2"))
	assert.NoError(t, err)
	assert.Equal(t, 2, lru.Len())
	err = lru.Set("key3", []byte("value3"))
	assert.NoError(t, err)
	assert.Equal(t, 2, lru.Len())
}
func TestGetAllEntries(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(2)
	err := lru.Set("key1", []byte("value1"))
	assert.NoError(t, err)
	err = lru.Set("key2", []byte("value2"))
	assert.NoError(t, err)
	entries := lru.GetAllEntries()
	assert.Len(t, entries, 2)
	assert.Equal(t, "key2", entries[0].Key)
	assert.Equal(t, "key1", entries[1].Key)
}

func makeEntry(key string, value string, modTime time.Time) CacheEntry {
	return CacheEntry{
		Key:              key,
		Value:            []byte(value),
		LastModifiedTime: modTime,
	}
}

func TestAddToRebalanceEmptyCache(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(3)
	entries := []CacheEntry{
		makeEntry("key1", "value1", time.Now()),
		makeEntry("key2", "value2", time.Now().Add(-10*time.Second)),
	}
	err := lru.AddToRebalance(entries)
	assert.NoError(t, err)
	assert.Equal(t, 2, lru.Len())
	assert.Equal(t, "key1", lru.storageList.Front().Value.(CacheEntry).Key)
}

func TestAddToRebalanceItemIsNewer(t *testing.T) {
	oldTime := time.Now()
	newTime := oldTime.Add(10 * time.Second)
	lru := NewLeastRecentlyUsedCache(3)
	elem := lru.storageList.PushFront(makeEntry("key1", "value1", oldTime))
	lru.storageIndex["key1"] = elem
	err := lru.AddToRebalance([]CacheEntry{makeEntry("key2", "value2", newTime)})
	assert.NoError(t, err)
	assert.Equal(t, 2, lru.Len())
	first := lru.storageList.Front().Value.(CacheEntry)
	assert.Equal(t, "key2", first.Key)
}

func TestAddToRebalanceWhereItemIsOlder(t *testing.T) {
	oldTime := time.Now()
	newTime := oldTime.Add(10 * time.Second)
	lru := NewLeastRecentlyUsedCache(3)
	elem := lru.storageList.PushFront(makeEntry("key1", "value1", newTime))
	lru.storageIndex["key1"] = elem
	err := lru.AddToRebalance([]CacheEntry{makeEntry("key2", "value2", oldTime)})
	assert.NoError(t, err)
	assert.Equal(t, 2, lru.Len())
	assert.Equal(t, "key1", lru.storageList.Front().Value.(CacheEntry).Key)
}

func TestAddToRebalanceWithEviction(t *testing.T) {
	oldTime := time.Now()
	newTime := oldTime.Add(10 * time.Second)
	lru := NewLeastRecentlyUsedCache(1)
	elem := lru.storageList.PushFront(makeEntry("key1", "value1", oldTime))
	lru.storageIndex["key1"] = elem
	err := lru.AddToRebalance([]CacheEntry{makeEntry("key2", "value2", newTime)})
	assert.NoError(t, err)
	assert.Equal(t, 1, lru.Len())
	assert.Equal(t, "key2", lru.storageList.Front().Value.(CacheEntry).Key)
}
func TestRemoveKeyToRebalanceNoSort(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(3)
	elem1 := lru.storageList.PushFront(makeEntry("key1", "value1", time.Now().Add(10*time.Minute)))
	lru.storageIndex["key1"] = elem1
	elem2 := lru.storageList.PushFront(makeEntry("key2", "value2", time.Now().Add(5*time.Minute)))
	lru.storageIndex["key2"] = elem2
	removed := lru.RemoveKeyToRebalance([]string{"key1"})
	assert.Equal(t, 1, lru.Len())
	assert.Equal(t, 1, len(removed))
	assert.Equal(t, "key1", removed[0].Key)
}
func TestRemoveKeyToRebalanceWithSort(t *testing.T) {
	lru := NewLeastRecentlyUsedCache(3)
	elem1 := lru.storageList.PushFront(makeEntry("key1", "value1", time.Now().Add(10*time.Minute)))
	lru.storageIndex["key1"] = elem1
	elem2 := lru.storageList.PushFront(makeEntry("key2", "value2", time.Now().Add(5*time.Minute)))
	lru.storageIndex["key2"] = elem2
	elem3 := lru.storageList.PushFront(makeEntry("key3", "value3", time.Now().Add(15*time.Minute)))
	lru.storageIndex["key3"] = elem3
	removed := lru.RemoveKeyToRebalance([]string{"key1", "key2", "key3"})
	assert.Equal(t, 0, lru.Len())
	assert.Equal(t, 3, len(removed))
	assert.Equal(t, "key3", removed[0].Key)
	assert.Equal(t, "key1", removed[1].Key)
	assert.Equal(t, "key2", removed[2].Key)
}
