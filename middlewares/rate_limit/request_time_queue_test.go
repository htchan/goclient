package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var refTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func TestNewQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		length int
		want   *Queue
	}{
		{
			name:   "happy flow",
			length: 5,
			want: &Queue{
				queue: make([]*time.Time, 5),
				size:  5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NewQueue(tt.length)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQueue_Count(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		queue *Queue
		want  int
	}{
		{
			name:  "happy flow",
			queue: &Queue{startIndex: 0, count: 5, size: 10},
			want:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.queue.Count()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQueue_Enqueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queue       *Queue
		enqueueItem *time.Time
		wantErr     error
		wantQueue   *Queue
	}{
		{
			name:        "happy flow/empty queue",
			queue:       &Queue{size: 5, queue: make([]*time.Time, 5)},
			enqueueItem: &refTime,
			wantErr:     nil,
			wantQueue: &Queue{
				queue: []*time.Time{&refTime, nil, nil, nil, nil},
				count: 1,
				size:  5,
			},
		},
		{
			name:        "happy flow/non empty queue",
			queue:       &Queue{size: 5, count: 4, queue: make([]*time.Time, 5)},
			enqueueItem: &refTime,
			wantErr:     nil,
			wantQueue: &Queue{
				queue: []*time.Time{nil, nil, nil, nil, &refTime},
				count: 5,
				size:  5,
			},
		},
		{
			name:        "happy flow/full queue",
			queue:       &Queue{size: 5, count: 5, queue: make([]*time.Time, 5)},
			enqueueItem: &refTime,
			wantErr:     ErrFullQueue,
			wantQueue: &Queue{
				queue: []*time.Time{nil, nil, nil, nil, nil},
				count: 5,
				size:  5,
			},
		},
		{
			name:        "happy flow/non zero index/empty queue",
			queue:       &Queue{size: 5, startIndex: 4, queue: make([]*time.Time, 5)},
			enqueueItem: &refTime,
			wantErr:     nil,
			wantQueue: &Queue{
				queue:      []*time.Time{nil, nil, nil, nil, &refTime},
				startIndex: 4,
				count:      1,
				size:       5,
			},
		},
		{
			name:        "happy flow/non zero index/non empty queue",
			queue:       &Queue{size: 5, startIndex: 4, count: 4, queue: make([]*time.Time, 5)},
			enqueueItem: &refTime,
			wantErr:     nil,
			wantQueue: &Queue{
				queue:      []*time.Time{nil, nil, nil, &refTime, nil},
				startIndex: 4,
				count:      5,
				size:       5,
			},
		},
		{
			name:        "happy flow/non zero index/full queue",
			queue:       &Queue{size: 5, startIndex: 4, count: 5, queue: make([]*time.Time, 5)},
			enqueueItem: &refTime,
			wantErr:     ErrFullQueue,
			wantQueue: &Queue{
				queue:      []*time.Time{nil, nil, nil, nil, nil},
				startIndex: 4,
				count:      5,
				size:       5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.queue.Enqueue(tt.enqueueItem)
			assert.Equal(t, tt.wantQueue, tt.queue)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestQueue_Dequeue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queue     *Queue
		want      *time.Time
		wantQueue *Queue
	}{
		{
			name:      "happy flow/empty queue",
			queue:     &Queue{queue: make([]*time.Time, 5), size: 5},
			want:      nil,
			wantQueue: &Queue{queue: make([]*time.Time, 5), size: 5},
		},
		{
			name: "happy flow/non empty queue",
			queue: &Queue{
				queue: []*time.Time{&refTime, nil, nil, nil, nil},
				count: 1,
				size:  5,
			},
			want: &refTime,
			wantQueue: &Queue{
				queue:      []*time.Time{&refTime, nil, nil, nil, nil},
				startIndex: 1,
				count:      0,
				size:       5,
			},
		},
		{
			name:      "happy flow/non zero index/empty queue",
			queue:     &Queue{queue: make([]*time.Time, 5), startIndex: 4, size: 5},
			want:      nil,
			wantQueue: &Queue{queue: make([]*time.Time, 5), startIndex: 4, size: 5},
		},
		{
			name: "happy flow/non zero index/non empty queue",
			queue: &Queue{
				queue:      []*time.Time{nil, nil, nil, nil, &refTime},
				startIndex: 4,
				count:      1,
				size:       5,
			},
			want: &refTime,
			wantQueue: &Queue{
				queue:      []*time.Time{nil, nil, nil, nil, &refTime},
				startIndex: 0,
				count:      0,
				size:       5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.queue.Dequeue()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantQueue, tt.queue)
		})
	}
}

func TestQueue_Item(t *testing.T) {
	t.Parallel()

	refTime1 := refTime.AddDate(1, 0, 0)
	refTime2 := refTime.AddDate(2, 0, 0)
	refTime3 := refTime.AddDate(3, 0, 0)
	refTime4 := refTime.AddDate(4, 0, 0)

	tests := []struct {
		name  string
		queue *Queue
		i     int
		want  *time.Time
	}{
		{
			name: "error flow/empty queue",
			queue: &Queue{
				queue:      []*time.Time{&refTime, &refTime1, &refTime2, &refTime3, &refTime4},
				startIndex: 0,
				count:      0,
				size:       5,
			},
			i:    0,
			want: nil,
		},
		{
			name: "happy flow/1st item",
			queue: &Queue{
				queue:      []*time.Time{&refTime, &refTime1, &refTime2, &refTime3, &refTime4},
				startIndex: 0,
				count:      3,
				size:       5,
			},
			i:    0,
			want: &refTime,
		},
		{
			name: "happy flow/2nd item",
			queue: &Queue{
				queue:      []*time.Time{&refTime, &refTime1, &refTime2, &refTime3, &refTime4},
				startIndex: 0,
				count:      3,
				size:       5,
			},
			i:    1,
			want: &refTime1,
		},
		{
			name: "error flow/i = count",
			queue: &Queue{
				queue:      []*time.Time{&refTime, &refTime1, &refTime2, &refTime3, &refTime4},
				startIndex: 0,
				count:      3,
				size:       5,
			},
			i:    3,
			want: nil,
		},
		{
			name: "happy flow/non zero index/1st item",
			queue: &Queue{
				queue:      []*time.Time{&refTime, &refTime1, &refTime2, &refTime3, &refTime4},
				startIndex: 4,
				count:      3,
				size:       5,
			},
			i:    0,
			want: &refTime4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.queue.Item(tt.i)
			assert.Equal(t, tt.want, got)
		})
	}
}
