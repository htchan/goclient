package retry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/htchan/goclient"
	"github.com/stretchr/testify/assert"
)

func TestNewRetryMiddleware(t *testing.T) {
	t.Parallel()

	retryForNotSuccessResp := func(_ *http.Request, resp *http.Response, _ error) bool {
		return resp.StatusCode != http.StatusOK
	}
	failFirstNRequest := func(n int) func(*int) http.HandlerFunc {
		return func(i *int) http.HandlerFunc {
			*i = 0
			return func(w http.ResponseWriter, r *http.Request) {
				if n > *i {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusOK)
				}
				*i++
			}
		}
	}
	tests := []struct {
		name              string
		maxRetries        int
		shouldRetry       goclient.ResultValidator
		calculator        RetryIntervalCalculator
		serverHandler     func(*int) http.HandlerFunc
		wantRespStatus    int
		wantCallCount     int
		wantLeastDuration time.Duration
	}{
		// normal testcase
		{
			name:              "happy flow: success at first trial",
			maxRetries:        10,
			shouldRetry:       retryForNotSuccessResp,
			calculator:        LinearRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(0),
			wantRespStatus:    http.StatusOK,
			wantCallCount:     1,
			wantLeastDuration: 0,
		},
		{
			name:              "happy flow: success before reaching max retries",
			maxRetries:        10,
			shouldRetry:       retryForNotSuccessResp,
			calculator:        LinearRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(9),
			wantRespStatus:    http.StatusOK,
			wantCallCount:     10,
			wantLeastDuration: 45 * time.Millisecond,
		},
		{
			name:              "error flow: fail until reaching limit",
			maxRetries:        10,
			shouldRetry:       retryForNotSuccessResp,
			calculator:        LinearRetryInterval(2 * time.Millisecond),
			serverHandler:     failFirstNRequest(10),
			wantRespStatus:    http.StatusInternalServerError,
			wantCallCount:     10,
			wantLeastDuration: 45 * time.Millisecond,
		},
		// should retry validator
		{
			name:              "happy flow: never retry",
			maxRetries:        0,
			shouldRetry:       func(_ *http.Request, _ *http.Response, _ error) bool { return false },
			calculator:        LinearRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(0),
			wantRespStatus:    http.StatusOK,
			wantCallCount:     1,
			wantLeastDuration: 0,
		},
		{
			name:              "happy flow: always retry",
			maxRetries:        10,
			shouldRetry:       func(_ *http.Request, _ *http.Response, _ error) bool { return true },
			calculator:        LinearRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(0),
			wantRespStatus:    http.StatusOK,
			wantCallCount:     10,
			wantLeastDuration: 45 * time.Millisecond,
		},
		// // sleep interval calculator
		{
			name:              "happy flow: static sleep interval",
			maxRetries:        10,
			shouldRetry:       retryForNotSuccessResp,
			calculator:        StaticRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(10),
			wantRespStatus:    http.StatusInternalServerError,
			wantCallCount:     10,
			wantLeastDuration: 10 * time.Millisecond,
		},
		{
			name:              "happy flow: linear sleep interval",
			maxRetries:        10,
			shouldRetry:       retryForNotSuccessResp,
			calculator:        LinearRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(10),
			wantRespStatus:    http.StatusInternalServerError,
			wantCallCount:     10,
			wantLeastDuration: 45 * time.Millisecond,
		},
		{
			name:              "happy flow: exponential sleep interval",
			maxRetries:        10,
			shouldRetry:       retryForNotSuccessResp,
			calculator:        ExponentialRetryInterval(1 * time.Millisecond),
			serverHandler:     failFirstNRequest(10),
			wantRespStatus:    http.StatusInternalServerError,
			wantCallCount:     10,
			wantLeastDuration: 285 * time.Millisecond,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			callCount := 0
			start := time.Now()

			serv := httptest.NewServer(test.serverHandler(&callCount))
			defer serv.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(
					NewRetryMiddleware(test.maxRetries, test.shouldRetry, test.calculator),
				),
			)

			req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
			assert.NoError(t, reqErr)
			resp, err := cli.Do(req)
			assert.Equal(t, test.wantRespStatus, resp.StatusCode)
			assert.NoError(t, err)
			assert.Equal(t, test.wantCallCount, callCount)
			assert.LessOrEqual(t, test.wantLeastDuration, time.Since(start))
		})
	}
}

func TestRetryForError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		result bool
	}{
		{
			name:   "happy flow: error is nil",
			err:    nil,
			result: false,
		},
		{
			name:   "happy flow: error is not nil",
			err:    errors.New("test error"),
			result: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := RetryForError(nil, nil, test.err)
			assert.Equal(t, test.result, result)
		})
	}
}

func TestNewRetryMiddleware_BodyReplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxRetries     int
		serverHandler  func(*int) http.HandlerFunc
		wantBody       string
		wantCallCount  int
		wantRespStatus int
	}{
		{
			name:       "POST body is replayed on retry",
			maxRetries: 3,
			serverHandler: func(count *int) http.HandlerFunc {
				*count = 0
				return func(w http.ResponseWriter, r *http.Request) {
					body := make([]byte, 1024)
					n, _ := r.Body.Read(body)
					*count++
					if *count < 3 {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusOK)
					w.Write(body[:n])
				}
			},
			wantBody:       "hello world",
			wantCallCount:  3,
			wantRespStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			callCount := 0
			serv := httptest.NewServer(tt.serverHandler(&callCount))
			defer serv.Close()

			cli := goclient.NewClient(
				goclient.WithMiddlewares(
					NewRetryMiddleware(
						tt.maxRetries,
						func(_ *http.Request, resp *http.Response, _ error) bool {
							return resp != nil && resp.StatusCode != http.StatusOK
						},
						StaticRetryInterval(1*time.Millisecond),
					),
				),
			)

			req, reqErr := http.NewRequest(http.MethodPost, serv.URL, strings.NewReader(tt.wantBody))
			assert.NoError(t, reqErr)

			resp, err := cli.Do(req)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantRespStatus, resp.StatusCode)
			assert.Equal(t, tt.wantCallCount, callCount)

			respBody := make([]byte, 1024)
			n, _ := resp.Body.Read(respBody)
			assert.Equal(t, tt.wantBody, string(respBody[:n]))
		})
	}
}
