package pool

import (
	"net/http"
	"sync"
)

type ClientPool interface {
	AddClients(...*http.Client)
	GetClient(req *http.Request) *http.Client
}

type clientPoolImpl struct {
	mutex   *sync.Mutex
	cond    *sync.Cond
	clients []*http.Client
}

func NewClientPool() ClientPool {
	mutex := new(sync.Mutex)
	return &clientPoolImpl{
		mutex:   mutex,
		cond:    sync.NewCond(mutex),
		clients: []*http.Client{},
	}
}

func (pool *clientPoolImpl) AddClients(clients ...*http.Client) {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	for _, cli := range clients {
		pool.clients = append(pool.clients, cli)
		pool.cond.Signal()
	}
}

func (pool *clientPoolImpl) GetClient(req *http.Request) *http.Client {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	if len(pool.clients) == 0 {
		pool.cond.Wait()
	}

	cli := pool.clients[0]
	pool.clients = pool.clients[1:]

	return cli
}
