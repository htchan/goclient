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

	cb := NewCircuitBreaker(0, 0, time.Second, isError)
	assert.Equal(t, 1, cb.failureThreshold)
	assert.Equal(t, 1, cb.successThreshold)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(3, 1, time.Second, isServerError)

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	cli := goclient.NewClient(
		goclient.WithMiddlewares(NewCircuitBreakerMiddleware(cb)),
	)

	// First 2 failures: circuit stays closed
	for i := range 2 {
		req, _ := http.NewRequest(http.MethodGet, failingServer.URL, nil)
		resp, err := cli.Do(req)
		assert.NoError(t, err, "request %d", i)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "request %d", i)
		assert.Equal(t, StateClosed, cb.State(), "request %d", i)
	}

	// 3rd failure: circuit opens
	req, _ := http.NewRequest(http.MethodGet, failingServer.URL, nil)
	resp, err := cli.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, StateOpen, cb.State())

	// 4th request: rejected immediately
	req, _ = http.NewRequest(http.MethodGet, failingServer.URL, nil)
	resp, err = cli.Do(req)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
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

	cb := NewCircuitBreaker(1, 1, 5*time.Second, isError, WithNowFunc(mockNow))

	failingRequester := func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}

	middleware := NewCircuitBreakerMiddleware(cb)
	wrapped := middleware(failingRequester)

	// Trigger failure → open
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	_, err := wrapped(req)
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.State())

	// Still open before timeout
	advanceTime(3 * time.Second)
	assert.Equal(t, StateOpen, cb.State())

	// After timeout → half-open
	advanceTime(3 * time.Second)
	assert.Equal(t, StateHalfOpen, cb.State())
}

func TestCircuitBreaker_HalfOpenToClose(t *testing.T) {
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

	cb := NewCircuitBreaker(1, 2, 5*time.Second, isServerError, WithNowFunc(mockNow))

	callCount := 0
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	cli := goclient.NewClient(
		goclient.WithMiddlewares(NewCircuitBreakerMiddleware(cb)),
	)

	// Trigger open
	req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
	cli.Do(req)
	assert.Equal(t, StateOpen, cb.State())

	// Advance past timeout → half-open
	advanceTime(6 * time.Second)
	assert.Equal(t, StateHalfOpen, cb.State())

	// First success in half-open (need 2 to close)
	req, _ = http.NewRequest(http.MethodGet, successServer.URL, nil)
	resp, err := cli.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, StateHalfOpen, cb.State())

	// Second success → closes circuit
	req, _ = http.NewRequest(http.MethodGet, successServer.URL, nil)
	resp, err = cli.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, StateClosed, cb.State())
	assert.Equal(t, 2, callCount)
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
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

	cb := NewCircuitBreaker(1, 3, 5*time.Second, isServerError, WithNowFunc(mockNow))

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	cli := goclient.NewClient(
		goclient.WithMiddlewares(NewCircuitBreakerMiddleware(cb)),
	)

	// Trigger open
	req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
	cli.Do(req)
	assert.Equal(t, StateOpen, cb.State())

	// Advance past timeout → half-open
	advanceTime(6 * time.Second)
	assert.Equal(t, StateHalfOpen, cb.State())

	// One success in half-open
	req, _ = http.NewRequest(http.MethodGet, successServer.URL, nil)
	cli.Do(req)
	assert.Equal(t, StateHalfOpen, cb.State())

	// Then a failure → back to open
	req, _ = http.NewRequest(http.MethodGet, failServer.URL, nil)
	cli.Do(req)
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(3, 1, time.Second, isServerError)

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	cli := goclient.NewClient(
		goclient.WithMiddlewares(NewCircuitBreakerMiddleware(cb)),
	)

	// 2 failures
	for range 2 {
		req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
		cli.Do(req)
	}
	assert.Equal(t, StateClosed, cb.State())

	// 1 success resets failure count
	req, _ := http.NewRequest(http.MethodGet, successServer.URL, nil)
	cli.Do(req)
	assert.Equal(t, StateClosed, cb.State())

	// 2 more failures should NOT open (count was reset)
	for range 2 {
		req, _ := http.NewRequest(http.MethodGet, failServer.URL, nil)
		cli.Do(req)
	}
	assert.Equal(t, StateClosed, cb.State())

	// 3rd consecutive failure opens
	req, _ = http.NewRequest(http.MethodGet, failServer.URL, nil)
	cli.Do(req)
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(100, 1, time.Second, isServerError)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cli := goclient.NewClient(
		goclient.WithMiddlewares(NewCircuitBreakerMiddleware(cb)),
	)

	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
			cli.Do(req)
		})
	}
	wg.Wait()

	// Should be open after 100+ failures
	assert.Equal(t, StateOpen, cb.State())
}

func TestNewCircuitBreakerMiddleware_Integration(t *testing.T) {
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

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cb := NewCircuitBreaker(3, 1, 2*time.Second, isServerError, WithNowFunc(mockNow))
	cli := goclient.NewClient(
		goclient.WithMiddlewares(NewCircuitBreakerMiddleware(cb)),
	)

	// Phase 1: 3 failures → open
	for range 3 {
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := cli.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	}
	assert.Equal(t, StateOpen, cb.State())
	assert.Equal(t, 3, requestCount)

	// Phase 2: requests rejected while open
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	_, err := cli.Do(req)
	assert.ErrorIs(t, err, ErrCircuitOpen)
	assert.Equal(t, 3, requestCount) // no additional server calls

	// Phase 3: advance time → half-open → success → closed
	advanceTime(3 * time.Second)
	req, _ = http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := cli.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, StateClosed, cb.State())
	assert.Equal(t, 4, requestCount)
}
