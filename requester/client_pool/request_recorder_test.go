package pool

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/htchan/goclient"
	"github.com/stretchr/testify/assert"
)

func getClients(p *clientPoolImpl) []*http.Client {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.clients
}

func TestNewRequestRecorderAlwaysAddClientBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		cooldownInterval time.Duration
		wantClients      []*http.Client
	}{
		{
			name:             "happy path",
			cooldownInterval: 100 * time.Millisecond,
			wantClients:      []*http.Client{http.DefaultClient},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			requestRecorder := NewRequestRecorderAlwaysAddClientBack(tt.cooldownInterval)
			pool := clientPoolImpl{
				mutex:   new(sync.Mutex),
				cond:    sync.NewCond(new(sync.Mutex)),
				clients: []*http.Client{},
			}
			cli := http.DefaultClient

			requestRecorder(&pool, cli, nil, nil, nil)
			assert.Equal(t, []*http.Client{}, getClients(&pool)) // because of cooldown
			time.Sleep(tt.cooldownInterval + 50*time.Millisecond)
			assert.Equal(t, tt.wantClients, getClients(&pool))
		})
	}
}

func TestNewRequestRecorderDropClientForContinueFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		isRequestFail           goclient.ResultValidator
		failureThreshold        int
		failureCooldownInterval time.Duration
		successCooldownInterval time.Duration
		wantCooldownInterval    time.Duration
		wantClients             []*http.Client
	}{
		{
			name: "happy path: success request",
			isRequestFail: func(req *http.Request, resp *http.Response, err error) bool {
				return false
			},
			failureThreshold:        3,
			failureCooldownInterval: 10000 * time.Millisecond,
			successCooldownInterval: 100 * time.Millisecond,
			wantCooldownInterval:    150 * time.Millisecond,
			wantClients:             []*http.Client{http.DefaultClient},
		},
		{
			name: "happy path: failed request below threshold",
			isRequestFail: func(req *http.Request, resp *http.Response, err error) bool {
				return true
			},
			failureThreshold:        3,
			failureCooldownInterval: 100 * time.Millisecond,
			successCooldownInterval: 10000 * time.Millisecond,
			wantCooldownInterval:    150 * time.Millisecond,
			wantClients:             []*http.Client{http.DefaultClient},
		},
		{
			name: "happy path: failed request exceed threshold",
			isRequestFail: func(req *http.Request, resp *http.Response, err error) bool {
				return true
			},
			failureThreshold:        1,
			failureCooldownInterval: 50 * time.Millisecond,
			successCooldownInterval: 50 * time.Millisecond,
			wantCooldownInterval:    150 * time.Millisecond,
			wantClients:             []*http.Client{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			requestRecorder := NewRequestRecorderDropClientForContinueFailed(
				tt.isRequestFail,
				tt.failureThreshold,
				tt.failureCooldownInterval,
				tt.successCooldownInterval,
			)
			pool := clientPoolImpl{
				mutex:   new(sync.Mutex),
				cond:    sync.NewCond(new(sync.Mutex)),
				clients: []*http.Client{},
			}
			cli := http.DefaultClient

			requestRecorder(&pool, cli, nil, nil, nil)
			assert.Equal(t, []*http.Client{}, getClients(&pool)) // because of cooldown
			time.Sleep(tt.wantCooldownInterval)
			assert.Equal(t, tt.wantClients, getClients(&pool))
		})
	}
}
