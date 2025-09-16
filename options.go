package goclient

import "net/http"

type ClientOption func(*Client)

func defaultRequester(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func WithMiddlewares(middlewares ...Middleware) ClientOption {
	return func(client *Client) {
		client.middlewares = append(client.middlewares, middlewares...)
	}
}

func WithRequester(requester Requester) ClientOption {
	return func(client *Client) {
		client.requester = requester
	}
}
