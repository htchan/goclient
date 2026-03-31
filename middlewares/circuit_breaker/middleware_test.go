package circuitbreaker

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/htchan/goclient"
	"github.com/stretchr/testify/assert"
)

func TestNewCircuitBreakerMiddleware(t *testing.T) {
	t.Parallel()

	now := time.Now()
	dummyResp := &http.Response{StatusCode: http.StatusOK}
	alwaysFail := func(_ *http.Request, _ *http.Response, _ error) bool { return true }
	neverFail := func(_ *http.Request, _ *http.Response, _ error) bool { return false }

	tests := []struct {
		name        string
		breaker     *CircuitBreaker
		requester   goclient.Requester
		wantState   State
		wantResp    *http.Response
		wantErr     error
	}{
		{
			name: "passes request through when closed and records success",
			breaker: &CircuitBreaker{
				state:         StateClosed,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        neverFail,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			requester: func(_ *http.Request) (*http.Response, error) { return dummyResp, nil },
			wantState: StateClosed,
			wantResp:  dummyResp,
		},
		{
			name: "rejects request when open",
			breaker: &CircuitBreaker{
				state:         StateOpen,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        neverFail,
				lastFailure:      now,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			requester: func(_ *http.Request) (*http.Response, error) { return dummyResp, nil },
			wantState: StateOpen,
			wantErr:   ErrCircuitOpen,
		},
		{
			name: "records failure when isFailure returns true",
			breaker: &CircuitBreaker{
				state:         StateClosed,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        alwaysFail,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			requester: func(_ *http.Request) (*http.Response, error) { return dummyResp, nil },
			wantState: StateOpen,
			wantResp:  dummyResp,
		},
		{
			name: "records success when isFailure returns false",
			breaker: &CircuitBreaker{
				state:         StateHalfOpen,
				failureThreshold: 1,
				successThreshold: 1,
				recoverDuration:  5 * time.Second,
				isFailure:        neverFail,
				now:              func() time.Time { return now },
				onStateChange:    func(from, to State) {},
			},
			requester: func(_ *http.Request) (*http.Response, error) { return dummyResp, nil },
			wantState: StateClosed,
			wantResp:  dummyResp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			middleware := NewCircuitBreakerMiddleware(tt.breaker)
			wrapped := middleware(tt.requester)

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
			resp, err := wrapped(req)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantResp, resp)
			}
			assert.Equal(t, tt.wantState, tt.breaker.State())
		})
	}
}

func TestNewCircuitBreakerMiddleware_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		failureThreshold int
		goroutines       int
		serverStatus     int
		wantState        State
	}{
		{
			name:             "opens after threshold with concurrent failures",
			failureThreshold: 100,
			goroutines:       200,
			serverStatus:     http.StatusInternalServerError,
			wantState:        StateOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := NewCircuitBreaker(tt.failureThreshold, 1, time.Second, isServerError)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(NewCircuitBreakerMiddleware(breaker)),
			)

			var wg sync.WaitGroup
			for range tt.goroutines {
				wg.Go(func() {
					req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
					cli.Do(req)
				})
			}
			wg.Wait()

			assert.Equal(t, tt.wantState, breaker.State())
		})
	}
}
