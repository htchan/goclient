package retry

import (
	"net/http"
	"time"
)

type RetryIntervalCalculator func(i int, req *http.Request, resp *http.Response) time.Duration

func StaticRetryInterval(interval time.Duration) RetryIntervalCalculator {
	return func(_ int, _ *http.Request, _ *http.Response) time.Duration {
		return interval
	}
}

func LinearRetryInterval(interval time.Duration) RetryIntervalCalculator {
	return func(i int, _ *http.Request, _ *http.Response) time.Duration {
		return interval * time.Duration(i)
	}
}

func ExponentialRetryInterval(interval time.Duration) RetryIntervalCalculator {
	return func(i int, _ *http.Request, _ *http.Response) time.Duration {
		return interval * time.Duration(i*i)
	}
}
