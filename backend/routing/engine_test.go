package routing

import (
	"encoding/json"
	"testing"
	"time"

	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultConfig() Config {
	return Config{
		WeightPriceQuality:        0.40,
		WeightLiquidity:           0.35,
		WeightSpreadQuality:       0.15,
		WeightMarketStatus:        0.10,
		StalenessLiquidityHaircut: 0.20,
	}
}

func makeTestMarket(id string, venue models.Venue, yesPrice, spread, liquidity float64, status models.MarketStatus) models.CanonicalMarket {
	return models.CanonicalMarket{
		ID:                  id,
		Venue:               venue,
		Title:               "Test Market",
		NormalizedTitle:      "test market",
		YesPrice:            yesPrice,
		NoPrice:             1 - yesPrice,
		Spread:              spread,
		Liquidity:           liquidity,
		Status:              status,
		ContractType:        models.ContractBinary,
		SettlementMechanism: models.SettlementCFTC,
		Outcomes:            []models.Outcome{{Label: "Yes", Price: yesPrice}, {Label: "No", Price: 1 - yesPrice}},
		IngestedAt:          time.Now(),
		RawPayload:          json.RawMessage(`{}`),
	}
}

func TestRoute_SelectsBestVenue(t *testing.T) {
	engine := NewEngine(defaultConfig())

	kalshiMarket := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.02, 50000, models.StatusOpen)
	polyMarket := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.64, 0.05, 30000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{kalshiMarket, polyMarket},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
		Flags:          []models.MatchFlag{},
	}

	order := models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}
	decision, err := engine.Route(order, group)

	require.NoError(t, err)
	assert.NotEmpty(t, decision.DecisionID)
	assert.Equal(t, order, decision.OrderRequest)
	assert.True(t, decision.SimulatedOnly.IsSimulated())
	assert.NotEmpty(t, decision.RoutingRationale)
	assert.Len(t, decision.ScoringBreakdown, 2)
}

func TestRoute_SelectsHigherLiquidity(t *testing.T) {
	engine := NewEngine(defaultConfig())

	low := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.03, 10000, models.StatusOpen)
	high := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.65, 0.03, 100000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{low, high},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)

	require.NoError(t, err)
	assert.Equal(t, models.VenuePolymarket, decision.SelectedVenue)
}

func TestRoute_TiebreaksByVenueOrdering(t *testing.T) {
	engine := NewEngine(defaultConfig())

	a := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.03, 50000, models.StatusOpen)
	b := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.65, 0.03, 50000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)

	require.NoError(t, err)
	// KALSHI < POLYMARKET alphabetically, so KALSHI wins tiebreak
	assert.Equal(t, models.VenueKalshi, decision.SelectedVenue)
}

func TestRoute_PenalizesSuspendedMarket(t *testing.T) {
	engine := NewEngine(defaultConfig())

	open := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.03, 50000, models.StatusOpen)
	suspended := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.65, 0.03, 50000, models.StatusSuspended)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{open, suspended},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)

	require.NoError(t, err)
	assert.Equal(t, models.VenueKalshi, decision.SelectedVenue)

	// Verify the suspended market has a lower market status score
	assert.Greater(t, decision.ScoringBreakdown[models.VenueKalshi].MarketStatus,
		decision.ScoringBreakdown[models.VenuePolymarket].MarketStatus)
}

func TestRoute_AppliesStaleHaircut(t *testing.T) {
	engine := NewEngine(defaultConfig())

	fresh := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.03, 50000, models.StatusOpen)
	stale := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.65, 0.03, 55000, models.StatusOpen)
	stale.DataStalenessFlag = true

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{fresh, stale},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)

	require.NoError(t, err)
	// Stale market gets 20% liquidity haircut: 55000 * 0.8 = 44000 < 50000
	assert.Equal(t, models.VenueKalshi, decision.SelectedVenue)
}

func TestRoute_EmptyGroup(t *testing.T) {
	engine := NewEngine(defaultConfig())
	group := models.EquivalenceGroup{GroupID: "empty-group-1234567890"}

	_, err := engine.Route(models.OrderRequest{}, group)
	require.Error(t, err)
}

func TestRoute_RejectedAlternatives(t *testing.T) {
	engine := NewEngine(defaultConfig())

	a := makeTestMarket("KALSHI:A", models.VenueKalshi, 0.65, 0.02, 50000, models.StatusOpen)
	b := makeTestMarket("POLYMARKET:A", models.VenuePolymarket, 0.60, 0.05, 30000, models.StatusOpen)

	group := models.EquivalenceGroup{
		GroupID:         "test-group-123456789012",
		Members:        []models.CanonicalMarket{a, b},
		ConfidenceScore: 0.95,
		MatchMethod:    models.MatchHybrid,
	}

	decision, err := engine.Route(models.OrderRequest{MarketID: "KALSHI:A", Side: "YES", Size: 100}, group)

	require.NoError(t, err)
	assert.Len(t, decision.RejectedAlternatives, 1)
	assert.NotEmpty(t, decision.RejectedAlternatives[0].RejectionReason)
}

func TestMarketStatusScore(t *testing.T) {
	assert.Equal(t, 1.0, marketStatusScore(models.StatusOpen))
	assert.Equal(t, 0.3, marketStatusScore(models.StatusSuspended))
	assert.Equal(t, 0.0, marketStatusScore(models.StatusClosed))
	assert.Equal(t, 0.0, marketStatusScore(models.StatusResolved))
}

func TestMinmaxNormalize(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50}
	result := minmaxNormalize(values)

	assert.InDelta(t, 0.0, result[0], 0.001)
	assert.InDelta(t, 0.25, result[1], 0.001)
	assert.InDelta(t, 0.5, result[2], 0.001)
	assert.InDelta(t, 0.75, result[3], 0.001)
	assert.InDelta(t, 1.0, result[4], 0.001)
}

func TestMinmaxNormalize_AllSame(t *testing.T) {
	values := []float64{42, 42, 42}
	result := minmaxNormalize(values)

	for _, v := range result {
		assert.InDelta(t, 0.5, v, 0.001)
	}
}

func TestComputeFairValue(t *testing.T) {
	markets := []models.CanonicalMarket{
		{YesPrice: 0.60},
		{YesPrice: 0.70},
	}

	fv := computeFairValue(markets)
	assert.InDelta(t, 0.65, fv, 0.001)
}
