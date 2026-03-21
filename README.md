# goclient

A lightweight Go HTTP client library with middleware support for retry logic, rate limiting, and custom request handling.

## Features

- **Middleware Support**: Chain multiple middlewares for request / response processing
    - **Retry Logic**: Configurable retry with custom intervals (static, linear, exponential)
    - **Rate Limiting**: Token-bucket style rate limiter with configurable window and queue size

- **Requester Support**: Requester is the inner most function to send the request out.
    - **Client Pool**: Pick client from pool to process request, with configurable failure tracking and cooldown

## Usage

### Basic Client

```go
client := goclient.NewClient()
req, err := http.NewRequest(http.MethodGet, "url", nil)
resp, err := client.Do(req)
```

### With Custom Requester

```go
client := goclient.NewClient(
    goclient.WithRequester(func(req *http.Request) (*http.Response, error) {
        return customHTTPClient.Do(req)
    }),
)
```

### With Middlewares

```go
import (
    "github.com/htchan/goclient/middlewares/retry"
    ratelimit "github.com/htchan/goclient/middlewares/rate_limit"
)

client := goclient.NewClient(
    goclient.WithMiddlewares(
        retry.NewRetryMiddleware(3, retry.RetryForError, retry.LinearRetryInterval(time.Second)),
        ratelimit.NewRateLimitMiddleware(ratelimit.NewQueue(10), 5*time.Second),
    ),
)
```

## Middlewares

### Retry Middleware

Automatically retries failed requests with configurable intervals.

```go
retryMiddleware := retry.NewRetryMiddleware(
    maxRetries,
    shouldRetryValidator,
    intervalCalculator,
)
```

### Rate Limit Middleware

Limits request throughput using a fixed-size queue with configurable cooldown intervals.

```go
rateLimitMiddleware := ratelimit.NewRateLimitMiddleware(
    ratelimit.NewQueue(10), // max 10 concurrent slots
    5 * time.Second,        // cooldown interval per slot
)
```
