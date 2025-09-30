package pool

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClientPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantClientPool clientPoolImpl
	}{
		{
			name: "happy flow",
			wantClientPool: clientPoolImpl{
				mutex:   new(sync.Mutex),
				cond:    sync.NewCond(new(sync.Mutex)),
				clients: []*http.Client{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := NewClientPool()
			assert.Equal(t, &tt.wantClientPool, pool)
		})
	}
}

func TestClientPoolImpl_AddClients(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		clientsToAdd   []*http.Client
		initialClients []*http.Client
		wantClients    []*http.Client
	}{
		{
			name:           "happy flow: add one client to empty pool",
			clientsToAdd:   []*http.Client{http.DefaultClient},
			initialClients: []*http.Client{},
			wantClients:    []*http.Client{http.DefaultClient},
		},
		{
			name:           "happy flow: add multiple clients to empty pool",
			clientsToAdd:   []*http.Client{http.DefaultClient, http.DefaultClient},
			initialClients: []*http.Client{},
			wantClients:    []*http.Client{http.DefaultClient, http.DefaultClient},
		},
		{
			name:           "happy flow: add one client to non-empty pool",
			clientsToAdd:   []*http.Client{http.DefaultClient},
			initialClients: []*http.Client{http.DefaultClient},
			wantClients:    []*http.Client{http.DefaultClient, http.DefaultClient},
		},
		{
			name:           "happy flow: add multiple clients to non-empty pool",
			clientsToAdd:   []*http.Client{http.DefaultClient, http.DefaultClient},
			initialClients: []*http.Client{http.DefaultClient},
			wantClients:    []*http.Client{http.DefaultClient, http.DefaultClient, http.DefaultClient},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := &clientPoolImpl{
				mutex:   new(sync.Mutex),
				cond:    sync.NewCond(new(sync.Mutex)),
				clients: tt.initialClients,
			}
			pool.AddClients(tt.clientsToAdd...)
			assert.Equal(t, tt.wantClients, pool.clients)
		})
	}
}

func TestClientPoolImpl_GetClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		addClientsFunc     func(pool *clientPoolImpl)
		wantClient         *http.Client
		wantPoolClients    []*http.Client
		wantWithinDuration time.Duration
	}{
		{
			name: "happy flow: get first client from pool",
			addClientsFunc: func(pool *clientPoolImpl) {
				pool.AddClients(http.DefaultClient, &http.Client{})
			},
			wantClient:         http.DefaultClient,
			wantPoolClients:    []*http.Client{{}},
			wantWithinDuration: 100 * time.Millisecond,
		},
		{
			name: "edge case: wait until client available",
			addClientsFunc: func(pool *clientPoolImpl) {
				go func() {
					time.Sleep(100 * time.Millisecond)
					pool.AddClients(http.DefaultClient)
				}()
			},
			wantClient:         http.DefaultClient,
			wantPoolClients:    []*http.Client{},
			wantWithinDuration: 200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start := time.Now()

			mu := new(sync.Mutex)
			pool := &clientPoolImpl{
				mutex: mu,
				cond:  sync.NewCond(mu),
			}
			tt.addClientsFunc(pool)

			gotClient := pool.GetClient(&http.Request{})
			assert.Equal(t, tt.wantClient, gotClient)
			assert.Equal(t, tt.wantPoolClients, pool.clients)
			assert.GreaterOrEqual(t, tt.wantWithinDuration, time.Since(start))
		})
	}
}
