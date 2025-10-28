package ratelimit

import (
	"net/http"
	"time"

	"github.com/htchan/goclient"
)

const truncateInterval = 1000 * time.Millisecond

func NewRateLimitMiddleware(
	queue *Queue,
	interval time.Duration,
) goclient.Middleware {
	return func(f goclient.Requester) goclient.Requester {
		return func(req *http.Request) (*http.Response, error) {
			tPtr := new(time.Time)
			for true {
				*tPtr = time.Now().UTC().Truncate(truncateInterval).Add(interval)
				// try to dequeue
				for item := queue.Item(0); item != nil && item.Before(time.Now()); item = queue.Item(0) {
					queue.Dequeue()
				}

				// enqueue new request time
				if queue.Enqueue(tPtr) == nil {
					break
				}

				time.Sleep(time.Second)
			}

			resp, err := f(req)

			*tPtr = time.Now().UTC().Truncate(truncateInterval).Add(interval)

			return resp, err
		}
	}
}
