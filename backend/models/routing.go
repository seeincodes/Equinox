package models

import (
	"time"
)

// OrderRequest represents a simulated order to route.
type OrderRequest struct {
	MarketID string `json:"market_id"`
	Side     string `json:"side"` // YES or NO
	Size     int    `json:"size"` // number of contracts
}

// VenueScore holds the scoring breakdown for a single venue.
type VenueScore struct {
	PriceQuality  float64 `json:"price_quality"`
	Liquidity     float64 `json:"liquidity"`
	SpreadQuality float64 `json:"spread_quality"`
	MarketStatus  float64 `json:"market_status"`
	Total         float64 `json:"total"`
}

// RejectedVenue captures why an alternative venue was not selected.
type RejectedVenue struct {
	Venue           Venue      `json:"venue"`
	MarketID        string     `json:"market_id"`
	Score           VenueScore `json:"score"`
	RejectionReason string     `json:"rejection_reason"`
}

// RoutingDecision is the output of the routing engine.
// SimulatedOnly is ALWAYS true in prototype — type-enforced via simulatedOnlyTrue.
type RoutingDecision struct {
	DecisionID           string              `json:"decision_id"`
	OrderRequest         OrderRequest        `json:"order_request"`
	EquivalenceGroup     EquivalenceGroup    `json:"equivalence_group"`
	SelectedVenue        Venue               `json:"selected_venue"`
	SelectedMarket       CanonicalMarket     `json:"selected_market"`
	RejectedAlternatives []RejectedVenue     `json:"rejected_alternatives"`
	ScoringBreakdown     map[Venue]VenueScore `json:"scoring_breakdown"`
	RoutingRationale     string              `json:"routing_rationale"` // human-readable narrative
	Timestamp            time.Time           `json:"timestamp"`
	SimulatedOnly        simulatedOnlyTrue   `json:"simulated_only"`   // ALWAYS true; type-enforced
	CacheMode            bool                `json:"cache_mode"`       // true if served from stale data
}

// simulatedOnlyTrue is a type that always marshals to true.
// This enforces at the type level that SimulatedOnly cannot be set to false.
type simulatedOnlyTrue struct{}

func (s simulatedOnlyTrue) MarshalJSON() ([]byte, error) {
	return []byte("true"), nil
}

func (s *simulatedOnlyTrue) UnmarshalJSON(data []byte) error {
	return nil // always true regardless of input
}

// IsSimulated always returns true. Exists for code that needs a bool check.
func (s simulatedOnlyTrue) IsSimulated() bool {
	return true
}
