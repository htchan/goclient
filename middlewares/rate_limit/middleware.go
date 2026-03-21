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
			// tPtr is shared with the queue. We set a large initial expiry
			// (100x interval) so the slot cannot be dequeued while the
			// request is in-flight. After the request completes, we update
			// to the real expiry. If the request crashes without updating,
			// the slot still self-heals after 100x interval.
			tPtr := new(time.Time)
			for {
				*tPtr = time.Now().UTC().Truncate(truncateInterval).Add(interval * 100)
				// try to dequeue expired items
				for item := queue.Item(0); item != nil && item.Before(time.Now()); item = queue.Item(0) {
					queue.Dequeue()
				}

				// enqueue new request time
				if queue.Enqueue(tPtr) == nil {
					break
				}

				// wait until the earliest item expires instead of a fixed 1s sleep
				earliest := queue.Item(0)
				if earliest != nil {
					waitDuration := time.Until(*earliest)
					if waitDuration > 0 {
						time.Sleep(waitDuration)
					}
				} else {
					time.Sleep(truncateInterval)
				}
			}

			resp, err := f(req)

			// Update to real expiry based on completion time.
			*tPtr = time.Now().UTC().Truncate(truncateInterval).Add(interval)

			return resp, err
		}
	}
}
