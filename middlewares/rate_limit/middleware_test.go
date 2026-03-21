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

	// Each test case builds its own reference times at execution time
	// (inside setup) to avoid timing drift in parallel subtests.
	type testFixture struct {
		queue             *Queue
		interval          time.Duration
		serverHandler     http.HandlerFunc
		wantQueue         *Queue
		wantLeastDuration time.Duration
	}

	tests := []struct {
		name  string
		setup func() testFixture
	}{
		{
			name: "happy flow/empty queue",
			setup: func() testFixture {
				refTimeNow := time.Now().UTC().Truncate(truncateInterval)
				refTimeResult := refTimeNow.Add(10 * time.Minute)
				return testFixture{
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
				}
			},
		},
		{
			name: "happy flow/full queue with expired item",
			setup: func() testFixture {
				refTimeNow := time.Now().UTC().Truncate(truncateInterval)
				refTimeResult := refTimeNow.Add(10 * time.Minute)
				refTimeExpired := refTimeNow.AddDate(-1, 0, 0)
				refTimeFuture := refTimeNow.AddDate(1, 0, 0)
				return testFixture{
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
				}
			},
		},
		{
			name: "happy flow/full queue with not expired item",
			setup: func() testFixture {
				refTimeNow := time.Now().UTC().Truncate(truncateInterval)
				refTimeFuture := refTimeNow.AddDate(1, 0, 0)
				// Item expires 2s from now — gives plenty of headroom
				refTimeAlmostExpired := refTimeNow.Add(2 * truncateInterval)
				refTimeResultLater := refTimeAlmostExpired.Add(10 * time.Minute)
				return testFixture{
					queue: &Queue{
						queue:      []*time.Time{&refTimeAlmostExpired, &refTimeAlmostExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
						startIndex: 0,
						count:      5,
						size:       5,
					},
					interval:      10 * time.Minute,
					serverHandler: func(w http.ResponseWriter, r *http.Request) {},
					wantQueue: &Queue{
						queue:      []*time.Time{&refTimeResultLater, &refTimeAlmostExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
						startIndex: 2,
						count:      4,
						size:       5,
					},
					wantLeastDuration: 1000 * time.Millisecond,
				}
			},
		},
		{
			name: "happy flow/full queue with expired item/long processing request",
			setup: func() testFixture {
				refTimeNow := time.Now().UTC().Truncate(truncateInterval)
				refTimeExpired := refTimeNow.AddDate(-1, 0, 0)
				refTimeFuture := refTimeNow.AddDate(1, 0, 0)
				// Server sleeps 1s, so result time ≈ now + 1s + interval
				refTimeLongProcessing := refTimeNow.Add(1 * truncateInterval)
				refTimeResultLongProcessing := refTimeLongProcessing.Add(10 * time.Minute)
				return testFixture{
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
						queue:      []*time.Time{&refTimeResultLongProcessing, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
						startIndex: 2,
						count:      4,
						size:       5,
					},
					wantLeastDuration: 1000 * time.Millisecond,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixture := tt.setup()

			start := time.Now()

			serv := httptest.NewServer(fixture.serverHandler)
			defer serv.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(
					NewRateLimitMiddleware(fixture.queue, fixture.interval),
				),
			)

			req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
			assert.NoError(t, reqErr)
			cli.Do(req)
			assert.LessOrEqual(t, fixture.wantLeastDuration, time.Since(start))
			if !assert.Equal(t, fixture.wantQueue, fixture.queue) {
				t.Error(fixture.queue.queue)
				t.Error(fixture.wantQueue.queue)
			}
		})
	}
}
