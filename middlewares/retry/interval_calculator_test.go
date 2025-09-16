package retry

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type input struct {
	i    int
	req  *http.Request
	resp *http.Response
}

func TestStaticRetryInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		input    input
		want     time.Duration
	}{
		{
			name:     "happy flow: i = 0",
			interval: time.Second,
			input: input{
				i:    0,
				req:  nil,
				resp: nil,
			},
			want: time.Second,
		},
		{
			name:     "happy flow: i = 100",
			interval: time.Second,
			input: input{
				i:    100,
				req:  nil,
				resp: nil,
			},
			want: time.Second,
		},
		{
			name:     "happy flow: not affected by req",
			interval: time.Second,
			input: input{
				i: 1000,
				req: &http.Request{
					Method: "GET",
					URL:    nil,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				resp: nil,
			},
			want: time.Second,
		},
		{
			name:     "happy flow: not affected by resp",
			interval: time.Second,
			input: input{
				i:   1000,
				req: nil,
				resp: &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
			},
			want: time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c := StaticRetryInterval(test.interval)
			got := c(test.input.i, test.input.req, test.input.resp)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestLinearRetryInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		input    input
		want     time.Duration
	}{
		{
			name:     "happy flow: i = 0",
			interval: time.Second,
			input: input{
				i:    0,
				req:  nil,
				resp: nil,
			},
			want: 0 * time.Second,
		},
		{
			name:     "happy flow: i = 100",
			interval: time.Second,
			input: input{
				i:    100,
				req:  nil,
				resp: nil,
			},
			want: 100 * time.Second,
		},
		{
			name:     "happy flow: not affected by req",
			interval: time.Second,
			input: input{
				i: 5,
				req: &http.Request{
					Method: "GET",
					URL:    nil,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				resp: nil,
			},
			want: 5 * time.Second,
		},
		{
			name:     "happy flow: not affected by resp",
			interval: time.Second,
			input: input{
				i:   5,
				req: nil,
				resp: &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
			},
			want: 5 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c := LinearRetryInterval(test.interval)
			got := c(test.input.i, test.input.req, test.input.resp)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestExponentialRetryInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		input    input
		want     time.Duration
	}{
		{
			name:     "happy flow: i = 0",
			interval: time.Second,
			input: input{
				i:    0,
				req:  nil,
				resp: nil,
			},
			want: 0 * time.Second,
		},
		{
			name:     "happy flow: i = 100",
			interval: time.Second,
			input: input{
				i:    5,
				req:  nil,
				resp: nil,
			},
			want: 25 * time.Second,
		},
		{
			name:     "happy flow: not affected by req",
			interval: time.Second,
			input: input{
				i: 5,
				req: &http.Request{
					Method: "GET",
					URL:    nil,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				resp: nil,
			},
			want: 25 * time.Second,
		},
		{
			name:     "happy flow: not affected by resp",
			interval: time.Second,
			input: input{
				i:   5,
				req: nil,
				resp: &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
			},
			want: 25 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c := ExponentialRetryInterval(test.interval)
			got := c(test.input.i, test.input.req, test.input.resp)
			assert.Equal(t, test.want, got)
		})
	}
}
