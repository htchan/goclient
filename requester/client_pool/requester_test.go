package pool

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClientPoolRequester(t *testing.T) {
	t.Parallel()

	successHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	failureHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	addClientBack := func(pool ClientPool, cli *http.Client, req *http.Request, resp *http.Response, err error) {
		pool.AddClients(cli)
	}

	tests := []struct {
		name               string
		addClientsFunc     func(pool ClientPool)
		recordRequest      RequestRecorder
		serverHandler      http.HandlerFunc
		wantRespStatus     int
		wantErr            error
		wantWithinDuration time.Duration
	}{
		{
			name: "happy flow: success with available client",
			addClientsFunc: func(pool ClientPool) {
				pool.AddClients(http.DefaultClient)
			},
			recordRequest:      addClientBack,
			serverHandler:      successHandler,
			wantRespStatus:     http.StatusOK,
			wantErr:            nil,
			wantWithinDuration: 10 * time.Millisecond,
		},
		{
			name: "happy flow: success when new client were added",
			addClientsFunc: func(pool ClientPool) {
				go func() {
					time.Sleep(100 * time.Millisecond)
					pool.AddClients(http.DefaultClient)
				}()
			},
			recordRequest:      addClientBack,
			serverHandler:      successHandler,
			wantRespStatus:     http.StatusOK,
			wantErr:            nil,
			wantWithinDuration: 110 * time.Millisecond,
		},
		{
			name: "happy flow: got 404 error",
			addClientsFunc: func(pool ClientPool) {
				pool.AddClients(http.DefaultClient)
			},
			recordRequest:      addClientBack,
			serverHandler:      failureHandler,
			wantRespStatus:     http.StatusNotFound,
			wantErr:            nil,
			wantWithinDuration: 10 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serv := httptest.NewServer(tt.serverHandler)
			defer serv.Close()

			start := time.Now()
			pool := NewClientPool()
			tt.addClientsFunc(pool)

			requester := NewClientPoolRequester(
				pool,
				tt.recordRequest,
			)

			req, reqErr := http.NewRequest(http.MethodGet, serv.URL, nil)
			assert.NoError(t, reqErr)

			resp, err := requester(req)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantRespStatus, resp.StatusCode)
			assert.GreaterOrEqual(t, tt.wantWithinDuration, time.Since(start))
		})
	}
}
