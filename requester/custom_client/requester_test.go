package customclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCustomClientRequester(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		cli            *http.Client
		serverHandler  http.HandlerFunc
		wantRespStatue int
		wantErr        error
	}{
		{
			name: "happy flow: use specific client return timeout",
			cli: &http.Client{
				Timeout: 1 * time.Millisecond,
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(5 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
			wantRespStatue: 0,
			wantErr:        context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.serverHandler)
			defer srv.Close()

			req, err := http.NewRequest("GET", srv.URL, nil)
			assert.NoError(t, err)

			resp, err := NewCustomClientRequester(tt.cli)(req)
			if tt.wantRespStatue > 0 {
				assert.Equal(t, tt.wantRespStatue, resp.StatusCode)
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
