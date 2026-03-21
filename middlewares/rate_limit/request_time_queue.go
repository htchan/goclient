package ratelimit

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrFullQueue = errors.New("Queue is full")
)

type Queue struct {
	mu         sync.Mutex
	queue      []*time.Time
	startIndex int
	count      int
	size       int
}

func NewQueue(length int) *Queue {
	return &Queue{
		queue:      make([]*time.Time, length),
		startIndex: 0,
		count:      0,
		size:       length,
	}
}

func (q *Queue) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.count
}

func (q *Queue) Enqueue(t *time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.count >= q.size {
		return ErrFullQueue
	}

	q.queue[(q.startIndex+q.count)%q.size] = t
	q.count += 1

	return nil
}

func (q *Queue) Dequeue() *time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.count == 0 {
		return nil
	}

	item := q.queue[q.startIndex]
	q.startIndex = (q.startIndex + 1) % q.size
	q.count -= 1

	return item
}

func (q *Queue) Item(i int) *time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()

	if i >= q.count || i < 0 {
		return nil
	}

	return q.queue[(q.startIndex+i)%q.size]
}
