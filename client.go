package goclient

import (
	"net/http"
)

type Requester func(*http.Request) (*http.Response, error)
type Middleware func(Requester) Requester
type ResultValidator func(req *http.Request, resp *http.Response, err error) bool

type Client struct {
	middlewares []Middleware
	requester   Requester
}

func NewClient(options ...ClientOption) *Client {
	client := &Client{}

	for _, option := range options {
		option(client)
	}

	if client.requester == nil {
		client.requester = defaultRequester
	}

	return client
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	f := c.requester
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		f = c.middlewares[i](f)
	}

	return f(req)
}
