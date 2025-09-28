package pool

import (
	"fmt"
	"net/http"

	"github.com/htchan/goclient"
)

func NewClientPoolRequester(
	pool ClientPool,
	recordRequest RequestRecorder,
) goclient.Requester {
	return func(req *http.Request) (*http.Response, error) {
		client := pool.GetClient(req)

		resp, err := client.Do(req)
		recordRequest(pool, client, req, resp, err)

		if err != nil {
			return nil, fmt.Errorf("client do request failed: %w", err)
		}

		return resp, nil
	}
}
