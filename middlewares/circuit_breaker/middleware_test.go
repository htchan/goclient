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

func isError(_ *http.Request, _ *http.Response, err error) bool {
	return err != nil
}

func isServerError(_ *http.Request, resp *http.Response, err error) bool {
	return err != nil || (resp != nil && resp.StatusCode >= 500)
}

func closedBreaker() *CircuitBreaker {
	return NewCircuitBreaker(1, 1, 5*time.Second, isServerError)
}

func openBreaker() *CircuitBreaker {
	now := time.Now()
	breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
		WithNowFunc(func() time.Time { return now }),
	)
	// trigger open
	breaker.recordFailure()
	return breaker
}

func halfOpenBreaker() *CircuitBreaker {
	now := time.Now()
	mu := sync.Mutex{}
	breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
		WithNowFunc(func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		}),
	)
	// trigger open
	breaker.recordFailure()
	// advance past recoverDuration
	mu.Lock()
	now = now.Add(6 * time.Second)
	mu.Unlock()
	// force transition to half-open
	breaker.State()
	return breaker
}

func TestCircuitBreakerMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		breaker     *CircuitBreaker
		serverStatus int
		useErrorRequester bool
		wantState   State
		wantStatus  int
		wantErr     error
		wantNilResp bool
	}{
		{
			name:         "closed: failure opens circuit",
			breaker:      closedBreaker(),
			serverStatus: http.StatusInternalServerError,
			wantState:    StateOpen,
			wantStatus:   http.StatusInternalServerError,
		},
		{
			name:         "closed: success keeps closed",
			breaker:      closedBreaker(),
			serverStatus: http.StatusOK,
			wantState:    StateClosed,
			wantStatus:   http.StatusOK,
		},
		{
			name:         "closed: error opens circuit",
			breaker:      closedBreaker(),
			useErrorRequester: true,
			wantState:    StateOpen,
			wantNilResp:  true,
		},
		{
			name:        "open: rejects request immediately",
			breaker:     openBreaker(),
			wantState:   StateOpen,
			wantNilResp: true,
			wantErr:     ErrCircuitOpen,
		},
		{
			name:         "half-open: success closes circuit",
			breaker:      halfOpenBreaker(),
			serverStatus: http.StatusOK,
			wantState:    StateClosed,
			wantStatus:   http.StatusOK,
		},
		{
			name:         "half-open: failure reopens circuit",
			breaker:      halfOpenBreaker(),
			serverStatus: http.StatusInternalServerError,
			wantState:    StateOpen,
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cli *goclient.Client

			if tt.useErrorRequester {
				cli = goclient.NewClient(
					goclient.WithMiddlewares(NewCircuitBreakerMiddleware(tt.breaker)),
					goclient.WithRequester(func(_ *http.Request) (*http.Response, error) {
						return nil, errors.New("connection refused")
					}),
				)
			} else {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.serverStatus)
				}))
				defer server.Close()

				cli = goclient.NewClient(
					goclient.WithMiddlewares(NewCircuitBreakerMiddleware(tt.breaker)),
				)

				req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
				resp, err := cli.Do(req)

				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
				if tt.wantNilResp {
					assert.Nil(t, resp)
				} else if tt.wantStatus != 0 {
					assert.Equal(t, tt.wantStatus, resp.StatusCode)
				}

				assert.Equal(t, tt.wantState, tt.breaker.State())
				return
			}

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
			resp, err := cli.Do(req)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.Error(t, err)
			}
			if tt.wantNilResp {
				assert.Nil(t, resp)
			}

			assert.Equal(t, tt.wantState, tt.breaker.State())
		})
	}
}

func TestCircuitBreakerMiddleware_SuccessResetsFailureCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		failureThreshold int
		failsBefore      int
		successesBetween int
		failsAfter       int
		wantState        State
	}{
		{
			name:             "success resets count: failures after reset don't reach threshold",
			failureThreshold: 3,
			failsBefore:      2,
			successesBetween: 1,
			failsAfter:       2,
			wantState:        StateClosed,
		},
		{
			name:             "success resets count: failures after reset reach threshold",
			failureThreshold: 3,
			failsBefore:      2,
			successesBetween: 1,
			failsAfter:       3,
			wantState:        StateOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			breaker := NewCircuitBreaker(tt.failureThreshold, 1, time.Second, isServerError)

			failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer failServer.Close()

			successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer successServer.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(NewCircuitBreakerMiddleware(breaker)),
			)

			for range tt.failsBefore {
				req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
				cli.Do(req)
			}
			for range tt.successesBetween {
				req, _ := http.NewRequest(http.MethodGet, successServer.URL, nil)
				cli.Do(req)
			}
			for range tt.failsAfter {
				req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
				cli.Do(req)
			}

			assert.Equal(t, tt.wantState, breaker.State())
		})
	}
}

func TestCircuitBreakerMiddleware_ConcurrentAccess(t *testing.T) {
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

type stateTransition struct {
	from State
	to   State
}

func TestCircuitBreakerMiddleware_OnStateChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		breaker          func(cb func(from, to State)) *CircuitBreaker
		action           string // "request_success", "request_fail"
		wantTransitions  []stateTransition
	}{
		{
			name: "callback fires: closed to open",
			breaker: func(cb func(from, to State)) *CircuitBreaker {
				return NewCircuitBreaker(1, 1, 5*time.Second, isServerError, WithOnStateChange(cb))
			},
			action:          "request_fail",
			wantTransitions: []stateTransition{{from: StateClosed, to: StateOpen}},
		},
		{
			name: "callback fires: half-open to closed",
			breaker: func(cb func(from, to State)) *CircuitBreaker {
				now := time.Now()
				mu := sync.Mutex{}
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time {
						mu.Lock()
						defer mu.Unlock()
						return now
					}),
					WithOnStateChange(cb),
				)
				breaker.recordFailure()
				mu.Lock()
				now = now.Add(6 * time.Second)
				mu.Unlock()
				breaker.State()
				return breaker
			},
			action:          "request_success",
			wantTransitions: []stateTransition{
				{from: StateClosed, to: StateOpen},
				{from: StateOpen, to: StateHalfOpen},
				{from: StateHalfOpen, to: StateClosed},
			},
		},
		{
			name: "callback fires: half-open to open",
			breaker: func(cb func(from, to State)) *CircuitBreaker {
				now := time.Now()
				mu := sync.Mutex{}
				breaker := NewCircuitBreaker(1, 1, 5*time.Second, isServerError,
					WithNowFunc(func() time.Time {
						mu.Lock()
						defer mu.Unlock()
						return now
					}),
					WithOnStateChange(cb),
				)
				breaker.recordFailure()
				mu.Lock()
				now = now.Add(6 * time.Second)
				mu.Unlock()
				breaker.State()
				return breaker
			},
			action:          "request_fail",
			wantTransitions: []stateTransition{
				{from: StateClosed, to: StateOpen},
				{from: StateOpen, to: StateHalfOpen},
				{from: StateHalfOpen, to: StateOpen},
			},
		},
		{
			name: "no callback when state unchanged",
			breaker: func(cb func(from, to State)) *CircuitBreaker {
				return NewCircuitBreaker(5, 1, time.Second, isServerError, WithOnStateChange(cb))
			},
			action:          "request_fail",
			wantTransitions: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var transitions []stateTransition
			breaker := tt.breaker(func(from, to State) {
				transitions = append(transitions, stateTransition{from: from, to: to})
			})

			failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer failServer.Close()

			successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer successServer.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(NewCircuitBreakerMiddleware(breaker)),
			)

			switch tt.action {
			case "request_success":
				req, _ := http.NewRequest(http.MethodGet, successServer.URL, nil)
				cli.Do(req)
			case "request_fail":
				req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
				cli.Do(req)
			}

			assert.Equal(t, tt.wantTransitions, transitions)
		})
	}
}
