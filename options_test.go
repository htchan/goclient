package goclient

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithMiddlewares(t *testing.T) {
	t.Parallel()

	testMiddleware := func(next Requester) Requester {
		return func(req *http.Request) (*http.Response, error) {
			return next(req)
		}
	}

	tests := []struct {
		name        string
		middlewares []Middleware
		wantClient  *Client
	}{
		{
			name:        "happy flow: 1 middleware",
			middlewares: []Middleware{testMiddleware},
			wantClient: &Client{
				middlewares: []Middleware{testMiddleware},
			},
		},
		{
			name:        "happy flow: multiple middleware",
			middlewares: []Middleware{testMiddleware, testMiddleware},
			wantClient: &Client{
				middlewares: []Middleware{testMiddleware, testMiddleware},
			},
		},
		{
			name:        "edge case: no middlewares",
			middlewares: []Middleware{},
			wantClient: &Client{
				middlewares: nil,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := &Client{}
			WithMiddlewares(test.middlewares...)(client)
			assert.Equal(t, len(test.wantClient.middlewares), len(client.middlewares))
			for i, middleware := range client.middlewares {
				assert.Equal(t, reflect.ValueOf(test.wantClient.middlewares[i]).Pointer(), reflect.ValueOf(middleware).Pointer())
			}
		})
	}
}

func TestWithRequester(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		requester  Requester
		wantClient *Client
	}{
		{
			name:       "happy flow",
			requester:  defaultRequester,
			wantClient: &Client{requester: defaultRequester},
		},
		{
			name:       "edge case: nil requester",
			requester:  nil,
			wantClient: &Client{requester: nil},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := &Client{}
			WithRequester(test.requester)(client)
			assert.Equal(t, reflect.ValueOf(test.wantClient.requester).Pointer(), reflect.ValueOf(client.requester).Pointer())
		})
	}
}

func Test_defaultRequester(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantRespStatus int
		wantError      error
	}{
		{
			name: "happy flow",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantRespStatus: http.StatusOK,
			wantError:      nil,
		},
		{
			name: "edge case: non 200 response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantRespStatus: http.StatusInternalServerError,
			wantError:      nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(test.serverHandler)
			url := server.URL
			if test.name == "edge case: connection error" {
				server.Close() // Close server to cause connection error
			} else {
				defer server.Close()
			}

			req, reqErr := http.NewRequest(http.MethodGet, url, nil)
			assert.NoError(t, reqErr)
			resp, respErr := defaultRequester(req)

			if test.name == "edge case: connection error" {
				assert.Error(t, respErr)
				assert.Nil(t, resp)
			} else {
				if test.wantRespStatus > 0 {
					assert.Equal(t, test.wantRespStatus, resp.StatusCode)
				}
				assert.ErrorIs(t, respErr, test.wantError)
			}
		})
	}
}
