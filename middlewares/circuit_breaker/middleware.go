package circuitbreaker

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/htchan/goclient"
)

// State represents the current state of the circuit breaker.
type State int

const (
	// StateClosed allows requests to pass through normally.
	StateClosed State = iota
	// StateOpen rejects requests immediately.
	StateOpen
	// StateHalfOpen allows a limited number of probe requests.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open and rejects the request.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker tracks the state of the circuit breaker.
type CircuitBreaker struct {
	mu sync.Mutex

	state            State
	failureThreshold int
	successThreshold int
	timeout          time.Duration
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
	return func(cb *CircuitBreaker) {
		cb.now = f
	}
}

// NewCircuitBreaker creates a new circuit breaker.
//
// failureThreshold: number of consecutive failures before opening the circuit.
// successThreshold: number of consecutive successes in half-open state before closing.
// timeout: how long to wait in open state before transitioning to half-open.
// isFailure: determines whether a request result counts as a failure.
func NewCircuitBreaker(
	failureThreshold int,
	successThreshold int,
	timeout time.Duration,
	isFailure goclient.ResultValidator,
	opts ...Option,
) *CircuitBreaker {
	if failureThreshold < 1 {
		failureThreshold = 1
	}
	if successThreshold < 1 {
		successThreshold = 1
	}

	cb := &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		isFailure:        isFailure,
		now:              time.Now,
	}

	for _, opt := range opts {
		opt(cb)
	}

	return cb
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// currentState returns the effective state, transitioning from open to half-open
// if the timeout has elapsed. Must be called with mu held.
func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen && cb.now().Sub(cb.lastFailure) >= cb.timeout {
		cb.state = StateHalfOpen
		cb.successCount = 0
		cb.failureCount = 0
	}
	return cb.state
}

// recordSuccess records a successful request. Must be called with mu held.
func (cb *CircuitBreaker) recordSuccess() {
	cb.failureCount = 0
	switch cb.state {
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = StateClosed
			cb.successCount = 0
		}
	case StateClosed:
		// already closed, nothing to do
	}
}

// recordFailure records a failed request. Must be called with mu held.
func (cb *CircuitBreaker) recordFailure() {
	cb.successCount = 0
	cb.failureCount++
	cb.lastFailure = cb.now()

	switch cb.state {
	case StateClosed:
		if cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// any failure in half-open immediately reopens
		cb.state = StateOpen
		cb.failureCount = 0
	}
}

// NewCircuitBreakerMiddleware creates a middleware that wraps requests with
// circuit breaker logic.
func NewCircuitBreakerMiddleware(cb *CircuitBreaker) goclient.Middleware {
	return func(f goclient.Requester) goclient.Requester {
		return func(req *http.Request) (*http.Response, error) {
			cb.mu.Lock()
			state := cb.currentState()
			if state == StateOpen {
				cb.mu.Unlock()
				return nil, ErrCircuitOpen
			}
			cb.mu.Unlock()

			resp, err := f(req)

			cb.mu.Lock()
			if cb.isFailure(req, resp, err) {
				cb.recordFailure()
			} else {
				cb.recordSuccess()
			}
			cb.mu.Unlock()

			return resp, err
		}
	}
}
