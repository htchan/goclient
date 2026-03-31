package circuitbreaker

import (
	"net/http"

	"github.com/htchan/goclient"
)

// NewCircuitBreakerMiddleware creates a middleware that wraps requests with
// circuit breaker logic.
func NewCircuitBreakerMiddleware(breaker *CircuitBreaker) goclient.Middleware {
	return func(f goclient.Requester) goclient.Requester {
		return func(req *http.Request) (*http.Response, error) {
			if breaker.State() == StateOpen {
				return nil, ErrCircuitOpen
			}

			resp, err := f(req)

			if breaker.isFailure(req, resp, err) {
				breaker.recordFailure()
			} else {
				breaker.recordSuccess()
			}

			return resp, err
		}
	}
}
