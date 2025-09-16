package goclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	mockMiddleware := func(f Requester) Requester { return f }
	mockRequester := func(req *http.Request) (*http.Response, error) { return nil, nil }
	tests := []struct {
		name    string
		options []ClientOption
		want    *Client
	}{
		{
			name: "happy flow",
			options: []ClientOption{
				func(c *Client) { c.requester = mockRequester },
				func(c *Client) { c.middlewares = append(c.middlewares, mockMiddleware) },
			},
			want: &Client{
				requester: mockRequester,
				middlewares: []Middleware{
					mockMiddleware,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := NewClient(test.options...)
			if got == nil {
				t.Errorf("NewClient() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClientDo(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		client         *Client
		wantRespStatus int
		wantError      error
	}{
		{
			name: "happy flow: basic request",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			client:         &Client{requester: defaultRequester},
			wantRespStatus: 200,
			wantError:      nil,
		},
		{
			name: "happy flow: affected by middlewares",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			client: &Client{
				requester: defaultRequester,
				middlewares: []Middleware{
					func(f Requester) Requester {
						return func(req *http.Request) (*http.Response, error) {
							return nil, testErr
						}
					},
				},
			},
			wantRespStatus: -1,
			wantError:      testErr,
		},
		{
			name: "happy flow: affected by requester",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			client: &Client{
				requester: func(r *http.Request) (*http.Response, error) {
					return nil, testErr
				},
			},
			wantRespStatus: -1,
			wantError:      testErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(test.serverHandler)
			defer server.Close()

			req, reqErr := http.NewRequest(http.MethodGet, server.URL, nil)
			assert.NoError(t, reqErr)

			resp, respErr := test.client.Do(req)

			if test.wantRespStatus != -1 {
				assert.Equal(t, test.wantRespStatus, resp.StatusCode)
			}

			assert.ErrorIs(t, test.wantError, respErr)
		})
	}
}
