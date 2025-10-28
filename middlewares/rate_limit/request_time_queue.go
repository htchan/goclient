package ratelimit

import (
	"errors"
	"time"
)

var (
	ErrFullQueue = errors.New("Queue is full")
)

type Queue struct {
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
	return q.count
}

func (q *Queue) Enqueue(t *time.Time) error {
	if q.count >= q.size {
		return ErrFullQueue
	}

	q.queue[(q.startIndex+q.count)%q.size] = t
	q.count += 1

	return nil
}

func (q *Queue) Dequeue() *time.Time {
	if q.count == 0 {
		return nil
	}

	item := q.queue[q.startIndex]
	q.startIndex = (q.startIndex + 1) % q.size
	q.count -= 1

	return item
}

func (q Queue) Item(i int) *time.Time {
	if i >= q.count || i < 0 {
		return nil
	}

	return q.queue[(q.startIndex+i)%q.size]
}
