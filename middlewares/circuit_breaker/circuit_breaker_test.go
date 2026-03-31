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

	now := time.Now()

	tests := []struct {
		name      string
		breaker   *CircuitBreaker
		wantState State
	}{
		{
			name: "closed breaker returns closed",
			breaker: &CircuitBreaker{
				state:            StateClosed,
				failureThreshold: 3,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState: StateClosed,
		},
		{
			name: "open breaker before recover duration returns open",
			breaker: &CircuitBreaker{
				state:            StateOpen,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				lastFailure:      now,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState: StateOpen,
		},
		{
			name: "open breaker after recover duration returns half-open",
			breaker: &CircuitBreaker{
				state:            StateOpen,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				lastFailure:      now.Add(-6 * time.Second),
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState: StateHalfOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantState, tt.breaker.State())
		})
	}
}

func TestCircuitBreaker_recordSuccess(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name             string
		breaker          *CircuitBreaker
		wantState        State
		wantFailureCount int
	}{
		{
			name: "closed: stays closed",
			breaker: &CircuitBreaker{
				state:            StateClosed,
				failureThreshold: 3,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState:        StateClosed,
			wantFailureCount: 0,
		},
		{
			name: "closed: resets failure count",
			breaker: &CircuitBreaker{
				state:            StateClosed,
				failureThreshold: 3,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				failureCount:     2,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState:        StateClosed,
			wantFailureCount: 0,
		},
		{
			name: "half-open: closes after reaching success threshold",
			breaker: &CircuitBreaker{
				state:            StateHalfOpen,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState:        StateClosed,
			wantFailureCount: 0,
		},
		{
			name: "half-open: stays half-open before reaching success threshold",
			breaker: &CircuitBreaker{
				state:            StateHalfOpen,
				failureThreshold: 1,
				successThreshold: 3,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState:        StateHalfOpen,
			wantFailureCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.breaker.recordSuccess()
			assert.Equal(t, tt.wantState, tt.breaker.State())
			assert.Equal(t, tt.wantFailureCount, tt.breaker.failureCount)
		})
	}
}

func TestCircuitBreaker_recordFailure(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name      string
		breaker   *CircuitBreaker
		wantState State
	}{
		{
			name: "closed: stays closed before reaching threshold",
			breaker: &CircuitBreaker{
				state:            StateClosed,
				failureThreshold: 3,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState: StateClosed,
		},
		{
			name: "closed: opens after reaching threshold",
			breaker: &CircuitBreaker{
				state:            StateClosed,
				failureThreshold: 2,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				failureCount:     1,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState: StateOpen,
		},
		{
			name: "half-open: reopens immediately",
			breaker: &CircuitBreaker{
				state:            StateHalfOpen,
				failureThreshold: 1,
				successThreshold: 3,
				recoverDuration:  5 * time.Second,
				isFailure:        isServerError,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			wantState: StateOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.breaker.recordFailure()
			assert.Equal(t, tt.wantState, tt.breaker.State())
		})
	}
}

func TestCircuitBreaker_setState(t *testing.T) {
	t.Parallel()

	now := time.Now()

	type transition struct {
		from State
		to   State
	}

	tests := []struct {
		name            string
		breaker         *CircuitBreaker
		to              State
		wantState       State
		wantTransitions []transition
	}{
		{
			name: "closed to open fires callback",
			breaker: &CircuitBreaker{
				state:     StateClosed,
				isFailure: isServerError,
				now:       func() time.Time { return now },
			},
			to:              StateOpen,
			wantState:       StateOpen,
			wantTransitions: []transition{{from: StateClosed, to: StateOpen}},
		},
		{
			name: "open to half-open fires callback",
			breaker: &CircuitBreaker{
				state:     StateOpen,
				isFailure: isServerError,
				now:       func() time.Time { return now },
			},
			to:              StateHalfOpen,
			wantState:       StateHalfOpen,
			wantTransitions: []transition{{from: StateOpen, to: StateHalfOpen}},
		},
		{
			name: "half-open to closed fires callback",
			breaker: &CircuitBreaker{
				state:     StateHalfOpen,
				isFailure: isServerError,
				now:       func() time.Time { return now },
			},
			to:              StateClosed,
			wantState:       StateClosed,
			wantTransitions: []transition{{from: StateHalfOpen, to: StateClosed}},
		},
		{
			name: "half-open to open fires callback",
			breaker: &CircuitBreaker{
				state:     StateHalfOpen,
				isFailure: isServerError,
				now:       func() time.Time { return now },
			},
			to:              StateOpen,
			wantState:       StateOpen,
			wantTransitions: []transition{{from: StateHalfOpen, to: StateOpen}},
		},
		{
			name: "same state does not fire callback",
			breaker: &CircuitBreaker{
				state:     StateClosed,
				isFailure: isServerError,
				now:       func() time.Time { return now },
			},
			to:              StateClosed,
			wantState:       StateClosed,
			wantTransitions: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var transitions []transition
			tt.breaker.onStateChange = func(from, to State) {
				transitions = append(transitions, transition{from: from, to: to})
			}

			tt.breaker.setState(tt.to)

			assert.Equal(t, tt.wantState, tt.breaker.state)
			assert.Equal(t, tt.wantTransitions, transitions)
		})
	}
}
