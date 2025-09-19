package customclient

import (
	"net/http"

	"github.com/htchan/goclient"
)

func NewCustomClientRequester(cli *http.Client) goclient.Requester {
	return func(req *http.Request) (*http.Response, error) {
		return cli.Do(req)
	}
}
