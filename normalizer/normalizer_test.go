package normalizer

import (
	"encoding/json"
	"testing"
	"time"

	"equinox/adapters"
	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Will Bitcoin exceed $100,000?", "will bitcoin exceed 100000"},
		{"Fed Rate Cut - March 2026", "fed rate cut march 2026"},
		{"   Multiple   Spaces   Here   ", "multiple space here"},
		{"UPPERCASE TITLE", "uppercase title"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComputeRulesHash(t *testing.T) {
	h1 := ComputeRulesHash("This market resolves YES if X happens.")
	h2 := ComputeRulesHash("This market resolves YES if X happens.")
	h3 := ComputeRulesHash("Different description entirely.")

	assert.Equal(t, h1, h2, "same description should produce same hash")
	assert.NotEqual(t, h1, h3, "different descriptions should produce different hashes")
	assert.Len(t, h1, 64, "SHA-256 hex should be 64 chars")
}

func TestComputeRulesHash_NormalizesWhitespace(t *testing.T) {
	h1 := ComputeRulesHash("test  description")
	h2 := ComputeRulesHash("test description")
	assert.Equal(t, h1, h2)
}

func TestNormalizeKalshi(t *testing.T) {
	payload := map[string]interface{}{
		"ticker":        "KXBTC-100K",
		"title":         "Will Bitcoin exceed $100,000?",
		"subtitle":      "Bitcoin price milestone",
		"status":        "open",
		"yes_bid":       65,
		"yes_ask":       68,
		"no_bid":        32,
		"no_ask":        35,
		"volume_24h":    1500,
		"close_time":    "2026-04-01T00:00:00Z",
		"rules_primary": "Resolves YES if BTC > $100k on CoinGecko.",
		"liquidity":     25000.0,
	}
	rawPayload, _ := json.Marshal(payload)

	raw := adapters.RawMarket{
		NativeID:   "KXBTC-100K",
		Venue:      models.VenueKalshi,
		RawPayload: rawPayload,
		FetchedAt:  time.Now(),
	}

	norm := New()
	results, errs := norm.Normalize([]adapters.RawMarket{raw})

	assert.Empty(t, errs)
	require.Len(t, results, 1)

	cm := results[0]
	assert.Equal(t, "KALSHI:KXBTC-100K", cm.ID)
	assert.Equal(t, models.VenueKalshi, cm.Venue)
	assert.Equal(t, "Will Bitcoin exceed $100,000?", cm.Title)
	assert.NotEmpty(t, cm.NormalizedTitle)

	// Price normalization: (65+68)/200 = 0.665
	assert.InDelta(t, 0.665, cm.YesPrice, 0.001)
	assert.InDelta(t, 0.335, cm.NoPrice, 0.001)
	// Spread: (68-65)/100 = 0.03
	assert.InDelta(t, 0.03, cm.Spread, 0.001)

	assert.Equal(t, models.StatusOpen, cm.Status)
	assert.Equal(t, models.ContractBinary, cm.ContractType)
	assert.Equal(t, models.SettlementCFTC, cm.SettlementMechanism)
	assert.NotNil(t, cm.ResolutionTime)
	assert.NotNil(t, cm.ResolutionTimeUTC)
	assert.NotEmpty(t, cm.RulesHash)
	assert.Len(t, cm.Outcomes, 2)
}

func TestNormalizeKalshi_StatusMapping(t *testing.T) {
	tests := []struct {
		kalshiStatus string
		expected     models.MarketStatus
	}{
		{"open", models.StatusOpen},
		{"closed", models.StatusClosed},
		{"settled", models.StatusResolved},
		{"unknown", models.StatusSuspended},
	}

	for _, tt := range tests {
		t.Run(tt.kalshiStatus, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]interface{}{
				"ticker": "T1",
				"title":  "Test",
				"status": tt.kalshiStatus,
			})

			raw := adapters.RawMarket{
				NativeID:   "T1",
				Venue:      models.VenueKalshi,
				RawPayload: payload,
				FetchedAt:  time.Now(),
			}

			norm := New()
			results, _ := norm.Normalize([]adapters.RawMarket{raw})
			require.Len(t, results, 1)
			assert.Equal(t, tt.expected, results[0].Status)
		})
	}
}

func TestNormalizePolymarket(t *testing.T) {
	payload := map[string]interface{}{
		"gamma": map[string]interface{}{
			"id":            "pm-123",
			"question":      "Will Bitcoin exceed $100,000?",
			"description":   "Resolves YES if BTC > $100k on CoinGecko.",
			"conditionId":   "cond-abc",
			"endDate":       "2026-04-01T00:00:00Z",
			"liquidity":     75000.0,
			"volume24hr":    12000.0,
			"active":        true,
			"funded":        true,
			"outcomePrices": `["0.65","0.35"]`,
			"outcomes":      `["Yes","No"]`,
			"neg_risk":      false,
		},
	}
	rawPayload, _ := json.Marshal(payload)

	raw := adapters.RawMarket{
		NativeID:   "cond-abc",
		Venue:      models.VenuePolymarket,
		RawPayload: rawPayload,
		FetchedAt:  time.Now(),
	}

	norm := New()
	results, errs := norm.Normalize([]adapters.RawMarket{raw})

	assert.Empty(t, errs)
	require.Len(t, results, 1)

	cm := results[0]
	assert.Equal(t, "POLYMARKET:cond-abc", cm.ID)
	assert.Equal(t, models.VenuePolymarket, cm.Venue)
	assert.Equal(t, "Will Bitcoin exceed $100,000?", cm.Title)
	assert.InDelta(t, 0.65, cm.YesPrice, 0.001)
	assert.InDelta(t, 0.35, cm.NoPrice, 0.001)
	assert.Equal(t, models.StatusOpen, cm.Status)
	assert.Equal(t, models.ContractBinary, cm.ContractType)
	assert.Equal(t, models.SettlementOptimisticOracle, cm.SettlementMechanism)
	assert.NotNil(t, cm.SettlementNote)
	assert.NotNil(t, cm.ResolutionTime)
	assert.NotEmpty(t, cm.RulesHash)
	assert.Len(t, cm.Outcomes, 2)
	assert.Equal(t, "Yes", cm.Outcomes[0].Label)
}

func TestNormalizePolymarket_OutcomePricesParsing(t *testing.T) {
	payload := map[string]interface{}{
		"gamma": map[string]interface{}{
			"id":            "pm-price-test",
			"question":      "Price parsing test",
			"conditionId":   "cond-price",
			"active":        true,
			"funded":        true,
			"outcomePrices": `["0.8234","0.1766"]`,
			"outcomes":      `["Yes","No"]`,
		},
	}
	rawPayload, _ := json.Marshal(payload)

	raw := adapters.RawMarket{
		NativeID:   "cond-price",
		Venue:      models.VenuePolymarket,
		RawPayload: rawPayload,
		FetchedAt:  time.Now(),
	}

	norm := New()
	results, errs := norm.Normalize([]adapters.RawMarket{raw})

	assert.Empty(t, errs)
	require.Len(t, results, 1)
	assert.InDelta(t, 0.8234, results[0].YesPrice, 0.0001)
	assert.InDelta(t, 0.1766, results[0].NoPrice, 0.0001)
}

func TestNormalize_UnknownVenue(t *testing.T) {
	raw := adapters.RawMarket{
		NativeID: "unknown-1",
		Venue:    models.Venue("UNKNOWN"),
		FetchedAt: time.Now(),
	}

	norm := New()
	results, errs := norm.Normalize([]adapters.RawMarket{raw})

	assert.Empty(t, results)
	assert.Len(t, errs, 1)
}
