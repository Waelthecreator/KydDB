package queue

import (
	"sync"
)

type RingQueue[T any] struct {
	data     []T
	head     int
	tail     int
	size     int
	capacity int
	mu       sync.Mutex
}

func NewRingQueue[T any](capacity int) *RingQueue[T] {
	return &RingQueue[T]{
		data:     make([]T, capacity),
		capacity: capacity,
	}
}

func (rq *RingQueue[T]) Enqueue(item T) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	if rq.size == rq.capacity {
		nextTail := (rq.tail + 1) % rq.capacity
		if nextTail == rq.head {
			if rq.head != 0 {
				rq.head = (rq.head + 1) % rq.capacity
				rq.size--
			} else {
				rq.expand()
			}
		}
	}
	rq.data[rq.tail] = item
	rq.tail = (rq.tail + 1) % rq.capacity
	rq.size++
}

func (rq *RingQueue[T]) Dequeue() (T, bool) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	var zero T
	if rq.size == 0 {
		return zero, false
	}
	item := rq.data[rq.head]
	rq.head = (rq.head + 1) % rq.capacity
	rq.size--
	return item, true
}

func (rq *RingQueue[T]) Peek() (T, bool) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	var zero T
	if rq.size == 0 {
		return zero, false
	}
	return rq.data[rq.head], true
}

func (rq *RingQueue[T]) Items() []T {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	items := make([]T, rq.size)
	for i := 0; i < rq.size; i++ {
		index := (rq.head + i) % rq.capacity
		items[i] = rq.data[index]
	}
	return items
}

func (rq *RingQueue[T]) expand() {
	newCapacity := rq.capacity * 2
	newData := make([]T, newCapacity)
	for i := 0; i < rq.size; i++ {
		newData[i] = rq.data[(rq.head+i)%rq.capacity]
	}
	rq.data = newData
	rq.head = 0
	rq.tail = rq.size
	rq.capacity = newCapacity
}
