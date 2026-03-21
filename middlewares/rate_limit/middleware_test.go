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

	t.Run("happy flow/empty queue", func(t *testing.T) {
		t.Parallel()

		refTimeNow := time.Now().UTC().Truncate(truncateInterval)
		refTimeResult := refTimeNow.Add(10 * time.Minute)
		queue := NewQueue(5)

		start := time.Now()

		serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer serv.Close()

		cli := goclient.NewClient(
			goclient.WithMiddlewares(
				NewRateLimitMiddleware(queue, 10*time.Minute),
			),
		)

		req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
		assert.NoError(t, reqErr)
		cli.Do(req)

		elapsed := time.Since(start)
		assert.LessOrEqual(t, time.Duration(0), elapsed)

		wantQueue := &Queue{
			queue:      []*time.Time{&refTimeResult, nil, nil, nil, nil},
			startIndex: 0,
			count:      1,
			size:       5,
		}
		if !assert.Equal(t, wantQueue, queue) {
			t.Error(queue.queue)
			t.Error(wantQueue.queue)
		}
	})

	t.Run("happy flow/full queue with expired item", func(t *testing.T) {
		t.Parallel()

		refTimeNow := time.Now().UTC().Truncate(truncateInterval)
		refTimeResult := refTimeNow.Add(10 * time.Minute)
		refTimeExpired := refTimeNow.AddDate(-1, 0, 0)
		refTimeFuture := refTimeNow.AddDate(1, 0, 0)

		queue := &Queue{
			queue:      []*time.Time{&refTimeExpired, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
			startIndex: 0,
			count:      5,
			size:       5,
		}

		start := time.Now()

		serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer serv.Close()

		cli := goclient.NewClient(
			goclient.WithMiddlewares(
				NewRateLimitMiddleware(queue, 10*time.Minute),
			),
		)

		req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
		assert.NoError(t, reqErr)
		cli.Do(req)

		elapsed := time.Since(start)
		assert.LessOrEqual(t, time.Duration(0), elapsed)

		wantQueue := &Queue{
			queue:      []*time.Time{&refTimeResult, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
			startIndex: 2,
			count:      4,
			size:       5,
		}
		if !assert.Equal(t, wantQueue, queue) {
			t.Error(queue.queue)
			t.Error(wantQueue.queue)
		}
	})

	t.Run("happy flow/full queue with not expired item", func(t *testing.T) {
		t.Parallel()

		refTimeNow := time.Now().UTC().Truncate(truncateInterval)
		refTimeFuture := refTimeNow.AddDate(1, 0, 0)
		// Item expires 2s from now — gives plenty of headroom
		refTimeAlmostExpired := refTimeNow.Add(2 * truncateInterval)
		refTimeResultLater := refTimeAlmostExpired.Add(10 * time.Minute)

		queue := &Queue{
			queue:      []*time.Time{&refTimeAlmostExpired, &refTimeAlmostExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
			startIndex: 0,
			count:      5,
			size:       5,
		}

		start := time.Now()

		serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer serv.Close()

		cli := goclient.NewClient(
			goclient.WithMiddlewares(
				NewRateLimitMiddleware(queue, 10*time.Minute),
			),
		)

		req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
		assert.NoError(t, reqErr)
		cli.Do(req)

		elapsed := time.Since(start)
		// Should have waited for the almost-expired item — at least 1s
		assert.LessOrEqual(t, 1*time.Second, elapsed)

		wantQueue := &Queue{
			queue:      []*time.Time{&refTimeResultLater, &refTimeAlmostExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
			startIndex: 2,
			count:      4,
			size:       5,
		}
		if !assert.Equal(t, wantQueue, queue) {
			t.Error(queue.queue)
			t.Error(wantQueue.queue)
		}
	})

	t.Run("happy flow/full queue with expired item/long processing request", func(t *testing.T) {
		t.Parallel()

		refTimeNow := time.Now().UTC().Truncate(truncateInterval)
		refTimeExpired := refTimeNow.AddDate(-1, 0, 0)
		refTimeFuture := refTimeNow.AddDate(1, 0, 0)
		// Server sleeps 1s, so result time ≈ now + 1s + interval
		refTimeLongProcessing := refTimeNow.Add(1 * truncateInterval)
		refTimeResultLongProcessing := refTimeLongProcessing.Add(10 * time.Minute)

		queue := &Queue{
			queue:      []*time.Time{&refTimeExpired, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
			startIndex: 0,
			count:      5,
			size:       5,
		}

		start := time.Now()

		serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1000 * time.Millisecond)
		}))
		defer serv.Close()

		cli := goclient.NewClient(
			goclient.WithMiddlewares(
				NewRateLimitMiddleware(queue, 10*time.Minute),
			),
		)

		req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
		assert.NoError(t, reqErr)
		cli.Do(req)

		elapsed := time.Since(start)
		assert.LessOrEqual(t, 1*time.Second, elapsed)

		wantQueue := &Queue{
			queue:      []*time.Time{&refTimeResultLongProcessing, &refTimeExpired, &refTimeFuture, &refTimeFuture, &refTimeFuture},
			startIndex: 2,
			count:      4,
			size:       5,
		}
		if !assert.Equal(t, wantQueue, queue) {
			t.Error(queue.queue)
			t.Error(wantQueue.queue)
		}
	})
}
