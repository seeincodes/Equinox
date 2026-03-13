package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"equinox/models"
)

const (
	maxRetryAttempts     = 3
	baseBackoff          = 1 * time.Second
	jitterFraction       = 0.20
	rateLimitBackoff     = 5 * time.Minute
	consecutiveRateLimit = 2
)

// RetryPolicy manages exponential backoff retry for adapter HTTP calls.
type RetryPolicy struct {
	venue              models.Venue
	consecutive429s    int
	backoffUntil       time.Time
}

func NewRetryPolicy(venue models.Venue) *RetryPolicy {
	return &RetryPolicy{venue: venue}
}

// IsBackedOff returns true if the venue is in a rate-limit backoff period.
func (rp *RetryPolicy) IsBackedOff() bool {
	return time.Now().Before(rp.backoffUntil)
}

// BackoffRemaining returns how long until backoff expires.
func (rp *RetryPolicy) BackoffRemaining() time.Duration {
	if rp.IsBackedOff() {
		return time.Until(rp.backoffUntil)
	}
	return 0
}

// Execute runs fn with retry logic. Only T1 and T2 errors are retried.
func (rp *RetryPolicy) Execute(ctx context.Context, operation string, fn func(ctx context.Context) error) error {
	if rp.IsBackedOff() {
		return &AdapterError{
			Venue:      rp.venue,
			Type:       T2RateLimit,
			Attempts:   0,
			LastError:  fmt.Errorf("venue backed off for %s", rp.BackoffRemaining().Round(time.Second)),
			OccurredAt: time.Now(),
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return &AdapterError{
				Venue:      rp.venue,
				Type:       T1ServerError,
				Attempts:   attempt - 1,
				LastError:  fmt.Errorf("context cancelled: %w", err),
				OccurredAt: time.Now(),
			}
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			rp.consecutive429s = 0
			return nil
		}

		adErr, ok := lastErr.(*AdapterError)
		if !ok {
			return lastErr
		}

		if !isRetryable(adErr.Type) {
			return adErr
		}

		if adErr.Type == T2RateLimit {
			rp.consecutive429s++
			if rp.consecutive429s >= consecutiveRateLimit {
				rp.backoffUntil = time.Now().Add(rateLimitBackoff)
				slog.Warn("rate limit backoff activated",
					"venue", rp.venue,
					"backoff_until", rp.backoffUntil.Format(time.RFC3339),
				)
			}
		}

		if attempt < maxRetryAttempts {
			delay := computeBackoff(attempt, adErr.RetryAfter)
			slog.Debug("retrying operation",
				"venue", rp.venue,
				"operation", operation,
				"attempt", attempt,
				"delay", delay.String(),
				"error_type", adErr.Type,
			)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return &AdapterError{
					Venue:      rp.venue,
					Type:       adErr.Type,
					Attempts:   attempt,
					LastError:  fmt.Errorf("context cancelled during retry: %w", ctx.Err()),
					OccurredAt: time.Now(),
				}
			}
		}
	}

	return &AdapterError{
		Venue:      rp.venue,
		Type:       lastErr.(*AdapterError).Type,
		Attempts:   maxRetryAttempts,
		LastError:  lastErr.(*AdapterError).LastError,
		OccurredAt: time.Now(),
	}
}

func isRetryable(ft FailureType) bool {
	return ft == T1ServerError || ft == T2RateLimit
}

func computeBackoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	base := float64(baseBackoff) * math.Pow(2, float64(attempt-1))
	jitter := base * jitterFraction * (2*rand.Float64() - 1) // ±20%
	return time.Duration(base + jitter)
}

// ClassifyHTTPError maps an HTTP status code to a FailureType.
func ClassifyHTTPError(statusCode int) FailureType {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return T2RateLimit
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return T3Auth
	case statusCode >= 500:
		return T1ServerError
	default:
		return T1ServerError
	}
}

// ParseRetryAfter extracts the Retry-After header as a duration.
func ParseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
