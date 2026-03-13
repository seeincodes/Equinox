package adapters

import (
	"context"
	"fmt"
	"time"

	"equinox/models"

	"github.com/go-resty/resty/v2"
)

const defaultHTTPTimeout = 15 * time.Second

// NewHTTPClient creates a resty client configured for venue API calls.
// baseURL is injectable for testing via httptest.NewServer().
func NewHTTPClient(baseURL string) *resty.Client {
	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(defaultHTTPTimeout).
		SetHeader("Accept", "application/json")
	return client
}

// DoGet performs a GET request with circuit breaker and retry integration.
// Returns the response body on success or an AdapterError on failure.
func DoGet(ctx context.Context, client *resty.Client, venue models.Venue, path string, queryParams map[string]string, cb *CircuitBreaker, rp *RetryPolicy) ([]byte, error) {
	var body []byte

	err := rp.Execute(ctx, fmt.Sprintf("GET %s", path), func(ctx context.Context) error {
		if err := cb.Allow(); err != nil {
			return err
		}

		req := client.R().SetContext(ctx)
		if queryParams != nil {
			req.SetQueryParams(queryParams)
		}

		resp, err := req.Get(path)
		if err != nil {
			adErr := &AdapterError{
				Venue:      venue,
				Type:       T1ServerError,
				Attempts:   1,
				LastError:  fmt.Errorf("HTTP request failed: %w", err),
				OccurredAt: time.Now(),
			}
			cb.RecordFailure(adErr)
			return adErr
		}

		if resp.StatusCode() >= 400 {
			failType := ClassifyHTTPError(resp.StatusCode())
			retryAfter := ParseRetryAfter(resp.Header().Get("Retry-After"))
			adErr := &AdapterError{
				Venue:      venue,
				Type:       failType,
				Attempts:   1,
				LastError:  fmt.Errorf("HTTP %d: %s", resp.StatusCode(), string(resp.Body())),
				OccurredAt: time.Now(),
				RetryAfter: retryAfter,
			}
			cb.RecordFailure(adErr)
			return adErr
		}

		cb.RecordSuccess()
		body = resp.Body()
		return nil
	})

	if err != nil {
		return nil, err
	}
	return body, nil
}
