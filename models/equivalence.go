package models

import (
	"time"
)

// MatchMethod indicates how an equivalence group was formed.
type MatchMethod string

const (
	MatchRuleBased MatchMethod = "RULE_BASED"
	MatchEmbedding MatchMethod = "EMBEDDING"
	MatchHybrid    MatchMethod = "HYBRID"
)

// MatchFlag signals conditions that may affect routing or confidence.
type MatchFlag string

const (
	FlagResolutionTimeMismatch MatchFlag = "RESOLUTION_TIME_MISMATCH"
	FlagLowConfidence          MatchFlag = "LOW_CONFIDENCE"
	FlagCategoricalMismatch    MatchFlag = "CATEGORICAL_MISMATCH"
	FlagSettlementDivergence   MatchFlag = "SETTLEMENT_DIVERGENCE"
	FlagStalePricingData       MatchFlag = "STALE_PRICING_DATA"
	FlagSingleVenueOnly        MatchFlag = "SINGLE_VENUE_ONLY"
	FlagEmbeddingUnavailable   MatchFlag = "EMBEDDING_UNAVAILABLE"
	FlagResolutionTimeMissing  MatchFlag = "RESOLUTION_TIME_MISSING"
)

// EquivalenceGroup links markets across venues that represent the same real-world outcome.
type EquivalenceGroup struct {
	GroupID             string         `json:"group_id"`              // deterministic UUID from sorted member IDs
	Members             []CanonicalMarket `json:"members"`
	ConfidenceScore     float64        `json:"confidence_score"`      // 0.0–1.0
	MatchMethod         MatchMethod    `json:"match_method"`
	EmbeddingSimilarity *float64       `json:"embedding_similarity"`
	StringSimilarity    *float64       `json:"string_similarity"`
	ResolutionDelta     *time.Duration `json:"resolution_delta"`
	MatchRationale      string         `json:"match_rationale"`
	CreatedAt           time.Time      `json:"created_at"`
	Flags               []MatchFlag    `json:"flags"`
}
