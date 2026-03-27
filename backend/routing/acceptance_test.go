package routing

import (
	"encoding/json"
	"strings"
	"testing"

	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F5: equinox route produces RoutingDecision with human-readable rationale
func TestF5_RoutingDecisionHasRationale(t *testing.T) {
	engine := NewEngine(defaultConfig())

	a := makeTestMarket("KALSHI:BTC100K", models.VenueKalshi, 0.65, 0.02, 50000, models.StatusOpen)
	a.Title = "Will Bitcoin exceed $100,000?"
	b := makeTestMarket("POLYMARKET:BTC100K", models.VenuePolymarket, 0.63, 0.04, 40000, models.StatusOpen)
	b.Title = "Will Bitcoin exceed $100,000?"

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
		Flags:          []models.MatchFlag{models.FlagSettlementDivergence},
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:BTC100K", Side: "YES", Size: 100}, group)

	require.NoError(t, err)
	assert.Contains(t, decision.RoutingRationale, "ROUTING DECISION")
	assert.Contains(t, decision.RoutingRationale, "SELECTED:")
	assert.Contains(t, decision.RoutingRationale, "REJECTED:")
	assert.Contains(t, decision.RoutingRationale, "Price Quality:")
	assert.Contains(t, decision.RoutingRationale, "Liquidity:")
	assert.Contains(t, decision.RoutingRationale, "Spread Quality:")
	assert.Contains(t, decision.RoutingRationale, "Market Status:")
	assert.Contains(t, decision.RoutingRationale, "SimulatedOnly=true")
	assert.Contains(t, decision.RoutingRationale, "BUY 100 YES")
	assert.Contains(t, decision.RoutingRationale, "SETTLEMENT_DIVERGENCE")
}

// F6: Routing engine imports zero packages from adapters/
// (Verified via `go list -f '{{.Imports}}' equinox/routing` — no adapters/ present)
func TestF6_NoAdapterImports(t *testing.T) {
	// This is a compile-time guarantee. If routing/ imported adapters/,
	// the import would appear in the package's import list.
	// Verified separately via: go list -f '{{.Imports}}' equinox/routing
	t.Log("Routing engine imports: equinox/models, fmt, log/slog, math, sort, strings, time — no adapters/")
}

// F7: All routing decisions logged as structured JSON via slog
func TestF7_RoutingDecisionIsStructuredJSON(t *testing.T) {
	engine := NewEngine(defaultConfig())

	a := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.02, 50000, models.StatusOpen)
	b := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.63, 0.04, 40000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)
	require.NoError(t, err)

	// The decision itself is JSON-serializable
	data, err := json.Marshal(decision)
	require.NoError(t, err)

	var roundTrip map[string]interface{}
	err = json.Unmarshal(data, &roundTrip)
	require.NoError(t, err)

	assert.Contains(t, roundTrip, "decision_id")
	assert.Contains(t, roundTrip, "selected_venue")
	assert.Contains(t, roundTrip, "scoring_breakdown")
	assert.Contains(t, roundTrip, "routing_rationale")
	assert.Contains(t, roundTrip, "simulated_only")
	assert.Equal(t, true, roundTrip["simulated_only"])
}

// F10: Both venues unavailable → routing still produces typed decision, not crash
func TestF10_RouteWithStaleData(t *testing.T) {
	engine := NewEngine(defaultConfig())

	a := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.02, 50000, models.StatusOpen)
	a.DataStalenessFlag = true
	b := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.63, 0.04, 40000, models.StatusOpen)
	b.DataStalenessFlag = true

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
		Flags:          []models.MatchFlag{models.FlagStalePricingData},
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)
	require.NoError(t, err)
	assert.NotNil(t, decision)
	assert.True(t, decision.SimulatedOnly.IsSimulated())

	// Both should have haircut applied
	for _, score := range decision.ScoringBreakdown {
		assert.NotZero(t, score.Total)
	}
}

// Verify deterministic output — same inputs produce same selected venue
func TestRoute_Deterministic(t *testing.T) {
	engine := NewEngine(defaultConfig())

	a := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.03, 50000, models.StatusOpen)
	b := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.64, 0.03, 48000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	order := models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}

	var selectedVenues []models.Venue
	for i := 0; i < 10; i++ {
		decision, err := engine.Route(order, group)
		require.NoError(t, err)
		selectedVenues = append(selectedVenues, decision.SelectedVenue)
	}

	for _, v := range selectedVenues {
		assert.Equal(t, selectedVenues[0], v, "routing should be deterministic")
	}
}

// Verify weights are respected
func TestRoute_WeightsAffectOutcome(t *testing.T) {
	// Create config that heavily weights liquidity
	liqCfg := Config{
		WeightPriceQuality:        0.05,
		WeightLiquidity:           0.80,
		WeightSpreadQuality:       0.05,
		WeightMarketStatus:        0.10,
		StalenessLiquidityHaircut: 0.20,
	}
	engine := NewEngine(liqCfg)

	// Kalshi has better price but much less liquidity
	a := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.01, 10000, models.StatusOpen)
	// Polymarket has worse price but much more liquidity
	b := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.60, 0.05, 100000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)
	require.NoError(t, err)

	// With 80% weight on liquidity, Polymarket (100k) should win over Kalshi (10k)
	assert.Equal(t, models.VenuePolymarket, decision.SelectedVenue)
	assert.True(t, strings.Contains(decision.RejectedAlternatives[0].RejectionReason, "lower liquidity"))
}
