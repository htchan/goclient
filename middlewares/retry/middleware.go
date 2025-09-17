package retry

import (
	"net/http"
	"time"

	"github.com/htchan/goclient"
)

func NewRetryMiddleware(
	maxRetries int,
	shouldRetry goclient.ResultValidator,
	sleepDuration RetryIntervalCalculator,
) goclient.Middleware {
	if maxRetries < 1 {
		maxRetries = 1
	}

	return func(f goclient.Requester) goclient.Requester {
		return func(req *http.Request) (*http.Response, error) {
			var (
				resp *http.Response
				err  error
			)

			for i := range maxRetries {
				resp, err = f(req)
				shouldRetry := shouldRetry(req, resp, err)
				if !shouldRetry || i == maxRetries-1 { // no need to sleep for last trial
					break
				}

				time.Sleep(sleepDuration(i, req, resp))
			}

			return resp, err
		}
	}
}

func RetryForError(_ *http.Request, _ *http.Response, err error) bool {
	return err != nil
}
