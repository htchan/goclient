package circuitbreaker

import (
	"errors"
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
	recoverDuration  time.Duration
	isFailure        goclient.ResultValidator

	failureCount int
	successCount int
	lastFailure  time.Time

	// now is a function that returns the current time, injectable for testing.
	now func() time.Time

	// onStateChange is called when the circuit breaker transitions between states.
	onStateChange func(from, to State)
}

// Option configures the circuit breaker.
type Option func(*CircuitBreaker)

// WithNowFunc overrides the time source (for testing).
func WithNowFunc(f func() time.Time) Option {
	return func(breaker *CircuitBreaker) {
		breaker.now = f
	}
}

// WithOnStateChange sets a callback that is invoked when the circuit breaker
// transitions between states. The callback receives the previous and new state.
func WithOnStateChange(f func(from, to State)) Option {
	return func(breaker *CircuitBreaker) {
		breaker.onStateChange = f
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
		recoverDuration:  recoverDuration,
		isFailure:        isFailure,
		now:              time.Now,
		onStateChange:    func(from, to State) {},
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
		breaker.setState(StateHalfOpen)
		breaker.successCount = 0
		breaker.failureCount = 0
	}
	return breaker.state
}

// setState transitions the breaker to a new state and fires the callback.
// Must be called with mu held.
func (breaker *CircuitBreaker) setState(to State) {
	from := breaker.state
	breaker.state = to
	if from != to {
		breaker.onStateChange(from, to)
	}
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
			breaker.setState(StateClosed)
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
			breaker.setState(StateOpen)
		}
	case StateHalfOpen:
		// any failure in half-open immediately reopens
		breaker.setState(StateOpen)
		breaker.failureCount = 0
	}
}
