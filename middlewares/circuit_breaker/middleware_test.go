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

// step describes a single action in a circuit breaker test scenario.
type step struct {
	// action to perform
	action string // "request_success", "request_fail", "request_error", "advance_time"

	// for advance_time
	duration time.Duration

	// expected state after this step
	wantState State

	// expected response (for request steps)
	wantStatus    int  // 0 means don't check
	wantErr       error
	wantNilResp   bool
}

func TestCircuitBreakerMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		failureThreshold int
		successThreshold int
		recoverDuration  time.Duration
		isFailure        goclient.ResultValidator
		useMockTime      bool
		steps            []step
	}{
		{
			name:             "closed to open after reaching failure threshold",
			failureThreshold: 3,
			successThreshold: 1,
			recoverDuration:  time.Second,
			isFailure:        isServerError,
			steps: []step{
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateOpen, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateOpen, wantNilResp: true, wantErr: ErrCircuitOpen},
			},
		},
		{
			name:             "open to half-open after recover duration",
			failureThreshold: 1,
			successThreshold: 1,
			recoverDuration:  5 * time.Second,
			isFailure:        isError,
			useMockTime:      true,
			steps: []step{
				{action: "request_error", wantState: StateOpen, wantNilResp: true},
				{action: "advance_time", duration: 3 * time.Second, wantState: StateOpen},
				{action: "advance_time", duration: 3 * time.Second, wantState: StateHalfOpen},
			},
		},
		{
			name:             "half-open to closed after success threshold",
			failureThreshold: 1,
			successThreshold: 2,
			recoverDuration:  5 * time.Second,
			isFailure:        isServerError,
			useMockTime:      true,
			steps: []step{
				{action: "request_fail", wantState: StateOpen, wantStatus: http.StatusInternalServerError},
				{action: "advance_time", duration: 6 * time.Second, wantState: StateHalfOpen},
				{action: "request_success", wantState: StateHalfOpen, wantStatus: http.StatusOK},
				{action: "request_success", wantState: StateClosed, wantStatus: http.StatusOK},
			},
		},
		{
			name:             "half-open to open on failure",
			failureThreshold: 1,
			successThreshold: 3,
			recoverDuration:  5 * time.Second,
			isFailure:        isServerError,
			useMockTime:      true,
			steps: []step{
				{action: "request_fail", wantState: StateOpen, wantStatus: http.StatusInternalServerError},
				{action: "advance_time", duration: 6 * time.Second, wantState: StateHalfOpen},
				{action: "request_success", wantState: StateHalfOpen, wantStatus: http.StatusOK},
				{action: "request_fail", wantState: StateOpen, wantStatus: http.StatusInternalServerError},
			},
		},
		{
			name:             "success resets failure count",
			failureThreshold: 3,
			successThreshold: 1,
			recoverDuration:  time.Second,
			isFailure:        isServerError,
			steps: []step{
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_success", wantState: StateClosed, wantStatus: http.StatusOK},
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateOpen, wantStatus: http.StatusInternalServerError},
			},
		},
		{
			name:             "full lifecycle: closed → open → half-open → closed",
			failureThreshold: 3,
			successThreshold: 1,
			recoverDuration:  2 * time.Second,
			isFailure:        isServerError,
			useMockTime:      true,
			steps: []step{
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateClosed, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateOpen, wantStatus: http.StatusInternalServerError},
				{action: "request_fail", wantState: StateOpen, wantNilResp: true, wantErr: ErrCircuitOpen},
				{action: "advance_time", duration: 3 * time.Second, wantState: StateHalfOpen},
				{action: "request_success", wantState: StateClosed, wantStatus: http.StatusOK},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Now()
			mu := sync.Mutex{}
			mockNow := func() time.Time {
				mu.Lock()
				defer mu.Unlock()
				return now
			}
			advanceTime := func(d time.Duration) {
				mu.Lock()
				defer mu.Unlock()
				now = now.Add(d)
			}

			var opts []Option
			if tt.useMockTime {
				opts = append(opts, WithNowFunc(mockNow))
			}

			breaker := NewCircuitBreaker(
				tt.failureThreshold,
				tt.successThreshold,
				tt.recoverDuration,
				tt.isFailure,
				opts...,
			)

			successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer successServer.Close()

			failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer failServer.Close()

			errorRequester := func(_ *http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}

			cli := goclient.NewClient(
				goclient.WithMiddlewares(NewCircuitBreakerMiddleware(breaker)),
			)
			errorCli := goclient.NewClient(
				goclient.WithMiddlewares(NewCircuitBreakerMiddleware(breaker)),
				goclient.WithRequester(errorRequester),
			)

			for i, s := range tt.steps {
				switch s.action {
				case "request_success":
					req, _ := http.NewRequest(http.MethodGet, successServer.URL, nil)
					resp, err := cli.Do(req)
					if s.wantErr != nil {
						assert.ErrorIs(t, err, s.wantErr, "step %d", i)
					} else {
						assert.NoError(t, err, "step %d", i)
					}
					if s.wantNilResp {
						assert.Nil(t, resp, "step %d", i)
					} else if s.wantStatus != 0 {
						assert.Equal(t, s.wantStatus, resp.StatusCode, "step %d", i)
					}

				case "request_fail":
					req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
					resp, err := cli.Do(req)
					if s.wantErr != nil {
						assert.ErrorIs(t, err, s.wantErr, "step %d", i)
					} else {
						assert.NoError(t, err, "step %d", i)
					}
					if s.wantNilResp {
						assert.Nil(t, resp, "step %d", i)
					} else if s.wantStatus != 0 {
						assert.Equal(t, s.wantStatus, resp.StatusCode, "step %d", i)
					}

				case "request_error":
					req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
					resp, err := errorCli.Do(req)
					assert.Error(t, err, "step %d", i)
					if s.wantNilResp {
						assert.Nil(t, resp, "step %d", i)
					}

				case "advance_time":
					advanceTime(s.duration)
				}

				assert.Equal(t, s.wantState, breaker.State(), "step %d: expected state %s", i, s.wantState)
			}
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
