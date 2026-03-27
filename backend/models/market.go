package models

import (
	"encoding/json"
	"time"
)

// Venue identifies a prediction market platform.
type Venue string

const (
	VenueKalshi      Venue = "KALSHI"
	VenuePolymarket  Venue = "POLYMARKET"
)

// MarketStatus represents the lifecycle state of a market.
type MarketStatus string

const (
	StatusOpen      MarketStatus = "OPEN"
	StatusClosed    MarketStatus = "CLOSED"
	StatusResolved  MarketStatus = "RESOLVED"
	StatusSuspended MarketStatus = "SUSPENDED"
)

// ContractType categorizes the market structure.
type ContractType string

const (
	ContractBinary      ContractType = "BINARY"
	ContractCategorical ContractType = "CATEGORICAL"
	ContractScalar      ContractType = "SCALAR"
)

// SettlementType identifies how a market resolves.
type SettlementType string

const (
	SettlementCFTC             SettlementType = "CFTC_REGULATED"
	SettlementOptimisticOracle SettlementType = "OPTIMISTIC_ORACLE"
	SettlementUnknown          SettlementType = "UNKNOWN"
)

// Outcome represents a single outcome in a market.
type Outcome struct {
	Label string  `json:"label"`
	Price float64 `json:"price"` // 0.0–1.0 implied probability
}

// CanonicalMarket is the venue-agnostic market representation.
// Design rule: contains only venue-agnostic fields. Venue-specific fields live in the adapter layer.
// Optional fields are pointers.
type CanonicalMarket struct {
	ID                  string           `json:"id"`                    // "{venue}:{native_id}"
	Venue               Venue            `json:"venue"`
	Title               string           `json:"title"`                 // as-returned by venue API
	NormalizedTitle     string           `json:"normalized_title"`      // lowercase, punctuation stripped, stemmed
	Description         string           `json:"description"`           // full resolution criteria
	Outcomes            []Outcome        `json:"outcomes"`
	ResolutionTime      *time.Time       `json:"resolution_time"`       // nil if venue doesn't specify
	ResolutionTimeUTC   *time.Time       `json:"resolution_time_utc"`   // derived; always UTC
	YesPrice            float64          `json:"yes_price"`             // 0.0–1.0 implied probability
	NoPrice             float64          `json:"no_price"`              // should ≈ 1 - YesPrice
	Spread              float64          `json:"spread"`                // YesAsk - YesBid
	Liquidity           float64          `json:"liquidity"`             // USD equivalent
	Volume24h           *float64         `json:"volume_24h"`            // optional
	Status              MarketStatus     `json:"status"`
	ContractType        ContractType     `json:"contract_type"`
	SettlementMechanism SettlementType   `json:"settlement_mechanism"`
	SettlementNote      *string          `json:"settlement_note"`       // venue-specific resolution quirks
	RulesHash           string           `json:"rules_hash"`            // SHA-256 of normalized resolution criteria
	DataStalenessFlag   bool             `json:"data_staleness_flag"`   // true if from cache/fallback
	IngestedAt          time.Time        `json:"ingested_at"`
	RawPayload          json.RawMessage  `json:"raw_payload"`           // verbatim original; never modified
}
