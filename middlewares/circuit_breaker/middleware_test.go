package circuitbreaker

import (
	"errors"
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

	tests := []struct {
		name              string
		setup             func() *CircuitBreaker
		serverStatus      int
		useErrorRequester bool
		wantState         State
		wantStatus        int
		wantErr           error
		wantNilResp       bool
	}{
		{
			name: "passes request through when closed",
			setup: func() *CircuitBreaker {
				return NewCircuitBreaker(3, 1, 5*time.Second, isServerError)
			},
			serverStatus: http.StatusOK,
			wantState:    StateClosed,
			wantStatus:   http.StatusOK,
		},
		{
			name: "rejects request when open",
			setup: func() *CircuitBreaker {
				now := time.Now()
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time { return now }),
				)
				breaker.recordFailure()
				return breaker
			},
			serverStatus: http.StatusOK,
			wantState:    StateOpen,
			wantNilResp:  true,
			wantErr:      ErrCircuitOpen,
		},
		{
			name: "records failure from server error",
			setup: func() *CircuitBreaker {
				return NewCircuitBreaker(1, 1, 5*time.Second, isServerError)
			},
			serverStatus: http.StatusInternalServerError,
			wantState:    StateOpen,
			wantStatus:   http.StatusInternalServerError,
		},
		{
			name: "records failure from transport error",
			setup: func() *CircuitBreaker {
				return NewCircuitBreaker(1, 1, 5*time.Second, isServerError)
			},
			useErrorRequester: true,
			wantState:         StateOpen,
			wantNilResp:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := tt.setup()
			middleware := NewCircuitBreakerMiddleware(breaker)

			var resp *http.Response
			var err error

			if tt.useErrorRequester {
				requester := middleware(func(_ *http.Request) (*http.Response, error) {
					return nil, errors.New("connection refused")
				})
				req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
				resp, err = requester(req)
			} else {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.serverStatus)
				}))
				defer server.Close()

				requester := middleware(func(req *http.Request) (*http.Response, error) {
					return http.DefaultClient.Do(req)
				})
				req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
				resp, err = requester(req)
			}

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantNilResp {
				assert.Nil(t, resp)
			} else if tt.wantStatus != 0 {
				assert.Equal(t, tt.wantStatus, resp.StatusCode)
			}
			assert.Equal(t, tt.wantState, breaker.State())
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
