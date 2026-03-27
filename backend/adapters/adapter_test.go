package adapters

import (
	"equinox/models"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAdapterError_Error(t *testing.T) {
	err := &AdapterError{
		Venue:      models.VenueKalshi,
		Type:       T1ServerError,
		Attempts:   3,
		LastError:  fmt.Errorf("connection refused"),
		OccurredAt: time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
	}

	msg := err.Error()
	if !strings.Contains(msg, "KALSHI") {
		t.Errorf("error message should contain venue, got: %s", msg)
	}
	if !strings.Contains(msg, "T1") {
		t.Errorf("error message should contain failure type, got: %s", msg)
	}
	if !strings.Contains(msg, "attempts=3") {
		t.Errorf("error message should contain attempt count, got: %s", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("error message should contain underlying error, got: %s", msg)
	}
}

func TestAdapterError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	err := &AdapterError{
		Venue:      models.VenuePolymarket,
		Type:       T2RateLimit,
		Attempts:   1,
		LastError:  inner,
		OccurredAt: time.Now(),
	}

	if err.Unwrap() != inner {
		t.Error("Unwrap should return the inner error")
	}
}
