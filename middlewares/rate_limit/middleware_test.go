package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/htchan/goclient"
	"github.com/stretchr/testify/assert"
)

func TestNewRateLimitMiddleware(t *testing.T) {
	t.Parallel()

	refTimeNow := time.Now().UTC().Truncate(1000 * time.Millisecond)
	refTimeResult := refTimeNow.Add(10 * time.Minute)
	refTimeExpired := refTimeNow.AddDate(-1, 0, 0)
	refTimeFuture := refTimeNow.AddDate(1, 0, 0)
	refTimeAlmostExpired := refTimeNow.Add(1000 * time.Millisecond)
	refTimeResultFromAlmostExpired := refTimeAlmostExpired.Add(10 * time.Minute)

	tests := []struct {
		name              string
		queue             *Queue
		interval          time.Duration
		serverHandler     http.HandlerFunc
		wantQueue         *Queue
		wantLeastDuration time.Duration
	}{
		{
			name:          "happy flow/empty queue",
			queue:         NewQueue(5),
			interval:      10 * time.Minute,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {},
			wantQueue: &Queue{
				queue:      []*time.Time{&refTimeResult, nil, nil, nil, nil},
				startIndex: 0,
				count:      1,
				size:       5,
			},
			wantLeastDuration: 0 * time.Millisecond,
		},
		{
			name: "happy flow/full queue with expired item",
			queue: &Queue{
				queue:      []*time.Time{&refTimeExpired, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
				startIndex: 0,
				count:      5,
				size:       5,
			},
			interval:      10 * time.Minute,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {},
			wantQueue: &Queue{
				queue:      []*time.Time{&refTimeResult, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
				startIndex: 2,
				count:      4,
				size:       5,
			},
			wantLeastDuration: 0 * time.Millisecond,
		},
		{
			name: "happy flow/full queue with not expired item",
			queue: &Queue{
				queue:      []*time.Time{&refTimeAlmostExpired, &refTimeAlmostExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
				startIndex: 0,
				count:      5,
				size:       5,
			},
			interval:      10 * time.Minute,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {},
			wantQueue: &Queue{
				queue:      []*time.Time{&refTimeResultFromAlmostExpired, &refTimeAlmostExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
				startIndex: 2,
				count:      4,
				size:       5,
			},
			wantLeastDuration: 1000 * time.Millisecond,
		},
		{
			name: "happy flow/full queue with expired item/long processing request",
			queue: &Queue{
				queue:      []*time.Time{&refTimeExpired, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
				startIndex: 0,
				count:      5,
				size:       5,
			},
			interval: 10 * time.Minute,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(1000 * time.Millisecond)
			},
			wantQueue: &Queue{
				queue:      []*time.Time{&refTimeResultFromAlmostExpired, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
				startIndex: 2,
				count:      4,
				size:       5,
			},
			wantLeastDuration: 1000 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start := time.Now()

			serv := httptest.NewServer(tt.serverHandler)
			defer serv.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(
					NewRateLimitMiddleware(tt.queue, tt.interval),
				),
			)

			req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
			assert.NoError(t, reqErr)
			cli.Do(req)
			assert.LessOrEqual(t, tt.wantLeastDuration, time.Since(start))
			if !assert.Equal(t, tt.wantQueue, tt.queue) {
				t.Error(tt.queue.queue)
				t.Error(tt.wantQueue.queue)
			}
		})
	}
}
