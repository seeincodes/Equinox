package equivalence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeMarket(id string, venue models.Venue, title string, yesPrice float64, settlement models.SettlementType, resTime *time.Time) models.CanonicalMarket {
	cm := models.CanonicalMarket{
		ID:                  id,
		Venue:               venue,
		Title:               title,
		NormalizedTitle:      title,
		Description:         "Test description for " + title,
		Outcomes:            []models.Outcome{{Label: "Yes", Price: yesPrice}, {Label: "No", Price: 1 - yesPrice}},
		YesPrice:            yesPrice,
		NoPrice:             1 - yesPrice,
		Spread:              0.03,
		Liquidity:           10000,
		Status:              models.StatusOpen,
		ContractType:        models.ContractBinary,
		SettlementMechanism: settlement,
		RulesHash:           "testhash",
		IngestedAt:          time.Now(),
		RawPayload:          json.RawMessage(`{}`),
	}
	if resTime != nil {
		utc := resTime.UTC()
		cm.ResolutionTime = resTime
		cm.ResolutionTimeUTC = &utc
	}
	return cm
}

func TestStage1Filter_MatchesCrossVenuePairs(t *testing.T) {
	cfg := Config{
		JaccardThreshold:      0.25,
		LevenshteinThreshold:  0.40,
		ResolutionWindowHours: 48,
	}
	engine := NewEngine(cfg, nil)

	resTime := time.Now().Add(24 * time.Hour)
	markets := []models.CanonicalMarket{
		makeMarket("KALSHI:BTC100K", models.VenueKalshi, "will bitcoin exceed 100k", 0.65, models.SettlementCFTC, &resTime),
		makeMarket("POLYMARKET:BTC100K", models.VenuePolymarket, "will bitcoin exceed 100k", 0.63, models.SettlementOptimisticOracle, &resTime),
	}

	candidates := engine.stage1Filter(markets)
	assert.Len(t, candidates, 1)
}

func TestStage1Filter_RejectsSameVenue(t *testing.T) {
	cfg := Config{
		JaccardThreshold:      0.25,
		LevenshteinThreshold:  0.40,
		ResolutionWindowHours: 48,
	}
	engine := NewEngine(cfg, nil)

	markets := []models.CanonicalMarket{
		makeMarket("KALSHI:A", models.VenueKalshi, "same title", 0.65, models.SettlementCFTC, nil),
		makeMarket("KALSHI:B", models.VenueKalshi, "same title", 0.63, models.SettlementCFTC, nil),
	}

	candidates := engine.stage1Filter(markets)
	assert.Empty(t, candidates)
}

func TestStage1Filter_RejectsContractTypeMismatch(t *testing.T) {
	cfg := Config{
		JaccardThreshold:      0.25,
		LevenshteinThreshold:  0.40,
		ResolutionWindowHours: 48,
	}
	engine := NewEngine(cfg, nil)

	a := makeMarket("KALSHI:A", models.VenueKalshi, "bitcoin exceeds 100k", 0.65, models.SettlementCFTC, nil)
	b := makeMarket("POLYMARKET:B", models.VenuePolymarket, "bitcoin exceeds 100k", 0.63, models.SettlementOptimisticOracle, nil)
	b.ContractType = models.ContractCategorical

	candidates := engine.stage1Filter([]models.CanonicalMarket{a, b})
	assert.Empty(t, candidates)
}

func TestStage1Filter_RejectsLowJaccard(t *testing.T) {
	cfg := Config{
		JaccardThreshold:      0.25,
		LevenshteinThreshold:  0.40,
		ResolutionWindowHours: 48,
	}
	engine := NewEngine(cfg, nil)

	markets := []models.CanonicalMarket{
		makeMarket("KALSHI:A", models.VenueKalshi, "bitcoin price prediction", 0.65, models.SettlementCFTC, nil),
		makeMarket("POLYMARKET:B", models.VenuePolymarket, "fed rate cut interest", 0.63, models.SettlementOptimisticOracle, nil),
	}

	candidates := engine.stage1Filter(markets)
	assert.Empty(t, candidates)
}

func TestStage1Filter_FlagsResolutionTimeMismatch(t *testing.T) {
	cfg := Config{
		JaccardThreshold:      0.25,
		LevenshteinThreshold:  0.40,
		ResolutionWindowHours: 48,
	}
	engine := NewEngine(cfg, nil)

	resTimeA := time.Now().Add(24 * time.Hour)
	resTimeB := time.Now().Add(96 * time.Hour) // 4 days apart

	markets := []models.CanonicalMarket{
		makeMarket("KALSHI:A", models.VenueKalshi, "will bitcoin exceed 100k", 0.65, models.SettlementCFTC, &resTimeA),
		makeMarket("POLYMARKET:B", models.VenuePolymarket, "will bitcoin exceed 100k", 0.63, models.SettlementOptimisticOracle, &resTimeB),
	}

	candidates := engine.stage1Filter(markets)
	require.Len(t, candidates, 1)
	assert.Contains(t, candidates[0].Flags, models.FlagResolutionTimeMismatch)
}

func TestDetectGroups_Stage1Only(t *testing.T) {
	cfg := Config{
		EmbeddingSimilarityHigh: 0.92,
		EmbeddingSimilarityLow:  0.78,
		JaccardThreshold:        0.25,
		LevenshteinThreshold:    0.40,
		ResolutionWindowHours:   48,
	}
	engine := NewEngine(cfg, nil)

	resTime := time.Now().Add(24 * time.Hour)
	markets := []models.CanonicalMarket{
		makeMarket("KALSHI:BTC100K", models.VenueKalshi, "will bitcoin exceed 100k", 0.65, models.SettlementCFTC, &resTime),
		makeMarket("POLYMARKET:BTC100K", models.VenuePolymarket, "will bitcoin exceed 100k", 0.63, models.SettlementOptimisticOracle, &resTime),
	}

	groups, err := engine.DetectGroups(context.Background(), markets)
	require.NoError(t, err)
	require.Len(t, groups, 1)

	g := groups[0]
	assert.Equal(t, models.MatchRuleBased, g.MatchMethod)
	assert.Contains(t, g.Flags, models.FlagLowConfidence)
	assert.Contains(t, g.Flags, models.FlagEmbeddingUnavailable)
	assert.Contains(t, g.Flags, models.FlagSettlementDivergence)
	assert.Len(t, g.Members, 2)
}

func TestCheckSettlementDivergence(t *testing.T) {
	g := models.EquivalenceGroup{
		Members: []models.CanonicalMarket{
			{SettlementMechanism: models.SettlementCFTC},
			{SettlementMechanism: models.SettlementOptimisticOracle},
		},
	}

	checkSettlementDivergence(&g)
	assert.Contains(t, g.Flags, models.FlagSettlementDivergence)
}

func TestCheckSettlementDivergence_NoFlag(t *testing.T) {
	g := models.EquivalenceGroup{
		Members: []models.CanonicalMarket{
			{SettlementMechanism: models.SettlementCFTC},
			{SettlementMechanism: models.SettlementCFTC},
		},
	}

	checkSettlementDivergence(&g)
	assert.NotContains(t, g.Flags, models.FlagSettlementDivergence)
}

func TestCosineSimilarity(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	assert.InDelta(t, 1.0, CosineSimilarity(a, b), 0.001)

	c := []float64{0, 1, 0}
	assert.InDelta(t, 0.0, CosineSimilarity(a, c), 0.001)

	d := []float64{1, 1, 0}
	assert.InDelta(t, 0.707, CosineSimilarity(a, d), 0.01)
}

func TestDeterministicGroupID(t *testing.T) {
	a := models.CanonicalMarket{ID: "KALSHI:A"}
	b := models.CanonicalMarket{ID: "POLYMARKET:B"}

	id1 := deterministicGroupID([]models.CanonicalMarket{a, b})
	id2 := deterministicGroupID([]models.CanonicalMarket{b, a})

	assert.Equal(t, id1, id2, "group ID should be deterministic regardless of member order")
	assert.Len(t, id1, 32, "group ID should be 32 hex chars (16 bytes)")
}
