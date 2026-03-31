package circuitbreaker

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func isServerError(_ *http.Request, resp *http.Response, err error) bool {
	return err != nil || (resp != nil && resp.StatusCode >= 500)
}

func TestNewCircuitBreaker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		failureThreshold     int
		successThreshold     int
		recoverDuration      time.Duration
		wantFailureThreshold int
		wantSuccessThreshold int
		wantRecoverDuration  time.Duration
		wantState            State
		wantFailureCount     int
		wantSuccessCount     int
		wantLastFailure      time.Time
	}{
		{
			name:                 "defaults: zero thresholds clamped to 1",
			failureThreshold:     0,
			successThreshold:     0,
			recoverDuration:      time.Second,
			wantFailureThreshold: 1,
			wantSuccessThreshold: 1,
			wantRecoverDuration:  time.Second,
			wantState:            StateClosed,
			wantFailureCount:     0,
			wantSuccessCount:     0,
			wantLastFailure:      time.Time{},
		},
		{
			name:                 "defaults: negative thresholds clamped to 1",
			failureThreshold:     -5,
			successThreshold:     -3,
			recoverDuration:      time.Second,
			wantFailureThreshold: 1,
			wantSuccessThreshold: 1,
			wantRecoverDuration:  time.Second,
			wantState:            StateClosed,
			wantFailureCount:     0,
			wantSuccessCount:     0,
			wantLastFailure:      time.Time{},
		},
		{
			name:                 "custom thresholds preserved",
			failureThreshold:     5,
			successThreshold:     3,
			recoverDuration:      30 * time.Second,
			wantFailureThreshold: 5,
			wantSuccessThreshold: 3,
			wantRecoverDuration:  30 * time.Second,
			wantState:            StateClosed,
			wantFailureCount:     0,
			wantSuccessCount:     0,
			wantLastFailure:      time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := NewCircuitBreaker(tt.failureThreshold, tt.successThreshold, tt.recoverDuration, isServerError)
			assert.Equal(t, tt.wantFailureThreshold, breaker.failureThreshold)
			assert.Equal(t, tt.wantSuccessThreshold, breaker.successThreshold)
			assert.Equal(t, tt.wantRecoverDuration, breaker.recoverDuration)
			assert.Equal(t, tt.wantState, breaker.State())
			assert.Equal(t, tt.wantFailureCount, breaker.failureCount)
			assert.Equal(t, tt.wantSuccessCount, breaker.successCount)
			assert.Equal(t, tt.wantLastFailure, breaker.lastFailure)
			assert.NotNil(t, breaker.now)
			assert.NotNil(t, breaker.isFailure)
			assert.NotNil(t, breaker.onStateChange)
		})
	}
}

func TestCircuitBreaker_State(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func() *CircuitBreaker
		wantState State
	}{
		{
			name: "closed breaker returns closed",
			setup: func() *CircuitBreaker {
				return NewCircuitBreaker(3, 1, 5*time.Second, isServerError)
			},
			wantState: StateClosed,
		},
		{
			name: "open breaker before recover duration returns open",
			setup: func() *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
				)
				breaker.recordFailure()
				return breaker
			},
			wantState: StateOpen,
		},
		{
			name: "open breaker after recover duration returns half-open",
			setup: func() *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
				)
				breaker.recordFailure()
				now = now.Add(6 * time.Second)
				return breaker
			},
			wantState: StateHalfOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := tt.setup()
			assert.Equal(t, tt.wantState, breaker.State())
		})
	}
}

func TestCircuitBreaker_recordSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setup            func() *CircuitBreaker
		wantState        State
		wantFailureCount int
	}{
		{
			name: "closed: stays closed",
			setup: func() *CircuitBreaker {
				return NewCircuitBreaker(3, 1, 5*time.Second, isServerError)
			},
			wantState:        StateClosed,
			wantFailureCount: 0,
		},
		{
			name: "closed: resets failure count",
			setup: func() *CircuitBreaker {
				breaker := NewCircuitBreaker(3, 1, 5*time.Second, isServerError)
				breaker.recordFailure()
				breaker.recordFailure()
				return breaker
			},
			wantState:        StateClosed,
			wantFailureCount: 0,
		},
		{
			name: "half-open: closes after reaching success threshold",
			setup: func() *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
				)
				breaker.recordFailure()
				now = now.Add(6 * time.Second)
				breaker.State() // transition to half-open
				return breaker
			},
			wantState:        StateClosed,
			wantFailureCount: 0,
		},
		{
			name: "half-open: stays half-open before reaching success threshold",
			setup: func() *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 3, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
				)
				breaker.recordFailure()
				now = now.Add(6 * time.Second)
				breaker.State() // transition to half-open
				return breaker
			},
			wantState:        StateHalfOpen,
			wantFailureCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := tt.setup()
			breaker.recordSuccess()
			assert.Equal(t, tt.wantState, breaker.State())
			assert.Equal(t, tt.wantFailureCount, breaker.failureCount)
		})
	}
}

func TestCircuitBreaker_recordFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func() *CircuitBreaker
		wantState State
	}{
		{
			name: "closed: stays closed before reaching threshold",
			setup: func() *CircuitBreaker {
				return NewCircuitBreaker(3, 1, 5*time.Second, isServerError)
			},
			wantState: StateClosed,
		},
		{
			name: "closed: opens after reaching threshold",
			setup: func() *CircuitBreaker {
				breaker := NewCircuitBreaker(2, 1, 5*time.Second, isServerError)
				breaker.recordFailure()
				return breaker
			},
			wantState: StateOpen,
		},
		{
			name: "half-open: reopens immediately",
			setup: func() *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 3, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
				)
				breaker.recordFailure()
				now = now.Add(6 * time.Second)
				breaker.State() // transition to half-open
				return breaker
			},
			wantState: StateOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := tt.setup()
			breaker.recordFailure()
			assert.Equal(t, tt.wantState, breaker.State())
		})
	}
}

func TestCircuitBreaker_onStateChange(t *testing.T) {
	t.Parallel()

	type transition struct {
		from State
		to   State
	}

	tests := []struct {
		name            string
		setup           func(cb func(from, to State)) *CircuitBreaker
		action          func(breaker *CircuitBreaker)
		wantTransitions []transition
	}{
		{
			name: "closed to open fires callback",
			setup: func(cb func(from, to State)) *CircuitBreaker {
				return NewCircuitBreaker(1, 1, 5*time.Second, isServerError, WithOnStateChange(cb))
			},
			action:          func(breaker *CircuitBreaker) { breaker.recordFailure() },
			wantTransitions: []transition{{from: StateClosed, to: StateOpen}},
		},
		{
			name: "open to half-open fires callback",
			setup: func(cb func(from, to State)) *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
					WithOnStateChange(cb),
				)
				breaker.recordFailure()
				now = now.Add(6 * time.Second)
				return breaker
			},
			action:          func(breaker *CircuitBreaker) { breaker.State() },
			wantTransitions: []transition{{from: StateClosed, to: StateOpen}, {from: StateOpen, to: StateHalfOpen}},
		},
		{
			name: "half-open to closed fires callback",
			setup: func(cb func(from, to State)) *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
					WithOnStateChange(cb),
				)
				breaker.recordFailure()
				now = now.Add(6 * time.Second)
				breaker.State() // transition to half-open
				return breaker
			},
			action:          func(breaker *CircuitBreaker) { breaker.recordSuccess() },
			wantTransitions: []transition{{from: StateClosed, to: StateOpen}, {from: StateOpen, to: StateHalfOpen}, {from: StateHalfOpen, to: StateClosed}},
		},
		{
			name: "no callback when state unchanged",
			setup: func(cb func(from, to State)) *CircuitBreaker {
				return NewCircuitBreaker(5, 1, time.Second, isServerError, WithOnStateChange(cb))
			},
			action:          func(breaker *CircuitBreaker) { breaker.recordFailure() },
			wantTransitions: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var transitions []transition
			breaker := tt.setup(func(from, to State) {
				transitions = append(transitions, transition{from: from, to: to})
			})

			tt.action(breaker)

			assert.Equal(t, tt.wantTransitions, transitions)
		})
	}
}
