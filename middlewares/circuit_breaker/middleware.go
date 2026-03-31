package circuitbreaker

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/htchan/goclient"
)

// ErrCircuitOpen is returned when the circuit breaker is open and rejects the request.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker tracks the state of the circuit breaker.
type CircuitBreaker struct {
	mu sync.Mutex

	state            State
	failureThreshold int
	successThreshold int
	recoverDuration          time.Duration
	isFailure        goclient.ResultValidator

	failureCount int
	successCount int
	lastFailure  time.Time

	// now is a function that returns the current time, injectable for testing.
	now func() time.Time
}

// Option configures the circuit breaker.
type Option func(*CircuitBreaker)

// WithNowFunc overrides the time source (for testing).
func WithNowFunc(f func() time.Time) Option {
	return func(breaker *CircuitBreaker) {
		breaker.now = f
	}
}

// NewCircuitBreaker creates a new circuit breaker.
//
// failureThreshold: number of consecutive failures before opening the circuit.
// successThreshold: number of consecutive successes in half-open state before closing.
// recoverDuration: how long to wait in open state before transitioning to half-open.
// isFailure: determines whether a request result counts as a failure.
func NewCircuitBreaker(
	failureThreshold int,
	successThreshold int,
	recoverDuration time.Duration,
	isFailure goclient.ResultValidator,
	opts ...Option,
) *CircuitBreaker {
	if failureThreshold < 1 {
		failureThreshold = 1
	}
	if successThreshold < 1 {
		successThreshold = 1
	}

	breaker := &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		recoverDuration:          recoverDuration,
		isFailure:        isFailure,
		now:              time.Now,
	}

	for _, opt := range opts {
		opt(breaker)
	}

	return breaker
}

// State returns the current state of the circuit breaker.
// If the breaker is open and recoverDuration has elapsed, it transitions to half-open.
func (breaker *CircuitBreaker) State() State {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	if breaker.state == StateOpen && breaker.now().Sub(breaker.lastFailure) >= breaker.recoverDuration {
		breaker.state = StateHalfOpen
		breaker.successCount = 0
		breaker.failureCount = 0
	}
	return breaker.state
}

// recordSuccess records a successful request.
func (breaker *CircuitBreaker) recordSuccess() {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	breaker.failureCount = 0
	switch breaker.state {
	case StateHalfOpen:
		breaker.successCount++
		if breaker.successCount >= breaker.successThreshold {
			breaker.state = StateClosed
			breaker.successCount = 0
		}
	case StateClosed:
		// already closed, nothing to do
	}
}

// recordFailure records a failed request.
func (breaker *CircuitBreaker) recordFailure() {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	breaker.successCount = 0
	breaker.failureCount++
	breaker.lastFailure = breaker.now()

	switch breaker.state {
	case StateClosed:
		if breaker.failureCount >= breaker.failureThreshold {
			breaker.state = StateOpen
		}
	case StateHalfOpen:
		// any failure in half-open immediately reopens
		breaker.state = StateOpen
		breaker.failureCount = 0
	}
}

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
