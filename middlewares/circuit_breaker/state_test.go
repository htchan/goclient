package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestState_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.state.String())
		})
	}
}

func TestNewCircuitBreaker_Defaults(t *testing.T) {
	t.Parallel()

	breaker := NewCircuitBreaker(0, 0, time.Second, isError)
	assert.Equal(t, 1, breaker.failureThreshold)
	assert.Equal(t, 1, breaker.successThreshold)
	assert.Equal(t, StateClosed, breaker.State())
}
