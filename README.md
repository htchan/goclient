# goclient

A lightweight Go HTTP client library with middleware support for retry logic, circuit breakers, and custom request handling.

## Features

- **Middleware Support**: Chain multiple middlewares for request / response processing
    - **Retry Logic**: Configurable retry with custom intervals
    - **Circuit Breaker**: Prevent cascading failures

- **Requester Support**: Requester is the inner most function to send the request out. 
    - **client pool**: pick client from pool to process request

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
    "github.com/htchan/goclient/middlewares/circuitbreaker"
)

client := goclient.NewClient(
    goclient.WithMiddlewares(
        retry.RetryMiddleware(3, shouldRetryFunc, intervalCalculator),
        circuitbreaker.CircuitBreakerMiddleware(breaker),
    ),
)
```

## Middlewares

### Retry Middleware

Automatically retries failed requests with configurable intervals.

```go
retryMiddleware := retry.RetryMiddleware(
    maxRetries,
    shouldRetryValidator,
    intervalCalculator,
)
```
