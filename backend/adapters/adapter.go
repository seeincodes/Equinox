package adapters

import (
	"context"
	"encoding/json"
	"equinox/models"
	"fmt"
	"time"
)

// VenueAdapter is the interface all venue implementations must satisfy.
// Accepts an optional baseURL override so httptest.NewServer() can be injected in tests.
type VenueAdapter interface {
	FetchMarkets(ctx context.Context) ([]RawMarket, error)
	FetchPricing(ctx context.Context, marketID string) (*RawPricing, error)
	VenueID() models.Venue
	HealthCheck(ctx context.Context) error
}

// RawMarket holds the unparsed market data from a venue API response.
type RawMarket struct {
	NativeID   string          `json:"native_id"`
	Venue      models.Venue    `json:"venue"`
	RawPayload json.RawMessage `json:"raw_payload"`
	FetchedAt  time.Time       `json:"fetched_at"`
}

// RawPricing holds the unparsed pricing/orderbook data from a venue API response.
type RawPricing struct {
	NativeID   string          `json:"native_id"`
	Venue      models.Venue    `json:"venue"`
	RawPayload json.RawMessage `json:"raw_payload"`
	FetchedAt  time.Time       `json:"fetched_at"`
}

// FailureType classifies API failures for distinct handling.
type FailureType string

const (
	T1ServerError   FailureType = "T1" // 5xx, timeout, DNS failure
	T2RateLimit     FailureType = "T2" // HTTP 429
	T3Auth          FailureType = "T3" // HTTP 401/403
	T4SchemaChange  FailureType = "T4" // Missing/renamed field, type mismatch
	T5PartialData   FailureType = "T5" // HTTP 200 but empty or truncated
	T6Suspension    FailureType = "T6" // All requests fail, regulatory shutdown
	T7StaleData     FailureType = "T7" // HTTP 200 with unchanged prices
)

// AdapterError is a typed error returned by adapters.
type AdapterError struct {
	Venue      models.Venue
	Type       FailureType
	Attempts   int
	LastError  error
	OccurredAt time.Time
	RetryAfter time.Duration // from Retry-After header on 429 responses
}

func (e *AdapterError) Error() string {
	return fmt.Sprintf("[%s] %s adapter error (type=%s, attempts=%d): %v",
		e.OccurredAt.Format(time.RFC3339), e.Venue, e.Type, e.Attempts, e.LastError)
}

func (e *AdapterError) Unwrap() error {
	return e.LastError
}
