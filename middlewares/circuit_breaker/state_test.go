package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestState_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state State
		want  string
	}{
		{
			name:  "closed",
			state: StateClosed,
			want:  "closed",
		},
		{
			name:  "open",
			state: StateOpen,
			want:  "open",
		},
		{
			name:  "half-open",
			state: StateHalfOpen,
			want:  "half-open",
		},
		{
			name:  "unknown",
			state: State(99),
			want:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.state.String())
		})
	}
}
