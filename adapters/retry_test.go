package adapters

import (
	"context"
	"fmt"
	"testing"
	"time"

	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryPolicy_SuccessOnFirstAttempt(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)
	calls := 0

	err := rp.Execute(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryPolicy_RetriesT1(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)
	calls := 0

	err := rp.Execute(context.Background(), "test", func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return &AdapterError{
				Venue:      models.VenueKalshi,
				Type:       T1ServerError,
				Attempts:   1,
				LastError:  fmt.Errorf("server error"),
				OccurredAt: time.Now(),
			}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetryPolicy_DoesNotRetryT3(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)
	calls := 0

	err := rp.Execute(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return &AdapterError{
			Venue:      models.VenueKalshi,
			Type:       T3Auth,
			Attempts:   1,
			LastError:  fmt.Errorf("auth error"),
			OccurredAt: time.Now(),
		}
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
	var adErr *AdapterError
	require.ErrorAs(t, err, &adErr)
	assert.Equal(t, T3Auth, adErr.Type)
}

func TestRetryPolicy_DoesNotRetryT4(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)
	calls := 0

	err := rp.Execute(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return &AdapterError{
			Venue:      models.VenueKalshi,
			Type:       T4SchemaChange,
			Attempts:   1,
			LastError:  fmt.Errorf("schema changed"),
			OccurredAt: time.Now(),
		}
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryPolicy_ExhaustsRetries(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)
	calls := 0

	err := rp.Execute(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return &AdapterError{
			Venue:      models.VenueKalshi,
			Type:       T1ServerError,
			Attempts:   1,
			LastError:  fmt.Errorf("persistent failure"),
			OccurredAt: time.Now(),
		}
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls) // 1 initial + 2 retries
	var adErr *AdapterError
	require.ErrorAs(t, err, &adErr)
	assert.Equal(t, 3, adErr.Attempts)
}

func TestRetryPolicy_CancelledContext(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := rp.Execute(ctx, "test", func(ctx context.Context) error {
		return nil
	})

	require.Error(t, err)
}

func TestRetryPolicy_BackoffAfterConsecutive429s(t *testing.T) {
	rp := NewRetryPolicy(models.VenueKalshi)

	for i := 0; i < 2; i++ {
		rp.Execute(context.Background(), "test", func(ctx context.Context) error {
			return &AdapterError{
				Venue:      models.VenueKalshi,
				Type:       T2RateLimit,
				Attempts:   1,
				LastError:  fmt.Errorf("rate limited"),
				OccurredAt: time.Now(),
			}
		})
	}

	assert.True(t, rp.IsBackedOff())
	assert.True(t, rp.BackoffRemaining() > 0)
}

func TestClassifyHTTPError(t *testing.T) {
	assert.Equal(t, T2RateLimit, ClassifyHTTPError(429))
	assert.Equal(t, T3Auth, ClassifyHTTPError(401))
	assert.Equal(t, T3Auth, ClassifyHTTPError(403))
	assert.Equal(t, T1ServerError, ClassifyHTTPError(500))
	assert.Equal(t, T1ServerError, ClassifyHTTPError(502))
	assert.Equal(t, T1ServerError, ClassifyHTTPError(503))
}

func TestParseRetryAfter(t *testing.T) {
	assert.Equal(t, 2*time.Second, ParseRetryAfter("2"))
	assert.Equal(t, 10*time.Second, ParseRetryAfter("10"))
	assert.Equal(t, time.Duration(0), ParseRetryAfter(""))
}

func TestComputeBackoff_HonoursRetryAfter(t *testing.T) {
	d := computeBackoff(1, 5*time.Second)
	assert.Equal(t, 5*time.Second, d)
}

func TestComputeBackoff_ExponentialGrowth(t *testing.T) {
	d1 := computeBackoff(1, 0)
	d2 := computeBackoff(2, 0)
	d3 := computeBackoff(3, 0)

	// With jitter, d2 should be roughly 2× d1, d3 roughly 4× d1
	assert.Greater(t, float64(d2), float64(d1)*1.5)
	assert.Greater(t, float64(d3), float64(d2)*1.5)
}
