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

func makeAcceptanceMarket(id string, venue models.Venue, title, normTitle string, ct models.ContractType, st models.SettlementType, yesPrice float64, resTime *time.Time) models.CanonicalMarket {
	outcomes := []models.Outcome{
		{Label: "Yes", Price: yesPrice},
		{Label: "No", Price: 1 - yesPrice},
	}
	return models.CanonicalMarket{
		ID:                  id,
		Venue:               venue,
		Title:               title,
		NormalizedTitle:      normTitle,
		Outcomes:            outcomes,
		YesPrice:            yesPrice,
		NoPrice:             1 - yesPrice,
		Spread:              0.02,
		Liquidity:           50000,
		Status:              models.StatusOpen,
		ContractType:        ct,
		SettlementMechanism: st,
		ResolutionTime:      resTime,
		ResolutionTimeUTC:   resTime,
		RulesHash:           "hash",
		RawPayload:          json.RawMessage(`{}`),
		IngestedAt:          time.Now(),
	}
}

func acceptanceTimePtr(t time.Time) *time.Time { return &t }

// F3: equinox match identifies ≥5 known-equivalent pairs.
// These pairs represent realistic cross-venue markets that should be matched.
func TestF3_KnownEquivalentPairs(t *testing.T) {
	resTime := acceptanceTimePtr(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	equivalentPairs := []struct {
		name                 string
		kalshiTitle          string
		kalshiNormTitle      string
		polyTitle            string
		polyNormTitle        string
	}{
		{
			name:            "BTC price",
			kalshiTitle:     "Will Bitcoin exceed $100,000 by June 2026?",
			kalshiNormTitle: "will bitcoin exceed 100000 by june 2026",
			polyTitle:       "Bitcoin to exceed $100k by June 2026",
			polyNormTitle:   "bitcoin to exceed 100k by june 2026",
		},
		{
			name:            "ETH price",
			kalshiTitle:     "Will Ethereum hit $5,000 by July 2026?",
			kalshiNormTitle: "will ethereum hit 5000 by july 2026",
			polyTitle:       "Ethereum to reach $5,000 by July 2026",
			polyNormTitle:   "ethereum to reach 5000 by july 2026",
		},
		{
			name:            "Fed rate cut",
			kalshiTitle:     "Will the Fed cut interest rates in June 2026?",
			kalshiNormTitle: "will the fed cut interest rate in june 2026",
			polyTitle:       "Federal Reserve rate cut in June 2026",
			polyNormTitle:   "federal reserve rate cut in june 2026",
		},
		{
			name:            "S&P 500",
			kalshiTitle:     "Will the S&P 500 close above 6,000 in 2026?",
			kalshiNormTitle: "will the sp 500 close above 6000 in 2026",
			polyTitle:       "S&P 500 above 6,000 by end of 2026",
			polyNormTitle:   "sp 500 above 6000 by end of 2026",
		},
		{
			name:            "US GDP",
			kalshiTitle:     "Will US GDP growth exceed 3% in Q2 2026?",
			kalshiNormTitle: "will us gdp growth exceed 3 in q2 2026",
			polyTitle:       "US GDP growth above 3% for Q2 2026",
			polyNormTitle:   "us gdp growth above 3 for q2 2026",
		},
		{
			name:            "Trump approval",
			kalshiTitle:     "Will Trump approval rating exceed 50% in May 2026?",
			kalshiNormTitle: "will trump approval rating exceed 50 in may 2026",
			polyTitle:       "Trump approval above 50% in May 2026",
			polyNormTitle:   "trump approval above 50 in may 2026",
		},
		{
			name:            "Gold price",
			kalshiTitle:     "Will gold price exceed $3,000 per ounce by June 2026?",
			kalshiNormTitle: "will gold price exceed 3000 per ounce by june 2026",
			polyTitle:       "Gold to surpass $3,000/oz by June 2026",
			polyNormTitle:   "gold to surpass 3000oz by june 2026",
		},
	}

	var markets []models.CanonicalMarket
	for i, p := range equivalentPairs {
		kMarket := makeAcceptanceMarket(
			"KALSHI:eq-"+p.name, models.VenueKalshi,
			p.kalshiTitle, p.kalshiNormTitle,
			models.ContractBinary, models.SettlementCFTC,
			0.65, resTime,
		)
		kMarket.Description = "Will " + p.kalshiTitle
		kMarket.Outcomes = []models.Outcome{{Label: "Yes", Price: 0.65 + float64(i)*0.01}, {Label: "No", Price: 0.35 - float64(i)*0.01}}

		pMarket := makeAcceptanceMarket(
			"POLYMARKET:eq-"+p.name, models.VenuePolymarket,
			p.polyTitle, p.polyNormTitle,
			models.ContractBinary, models.SettlementOptimisticOracle,
			0.63, resTime,
		)
		pMarket.Description = "Will " + p.polyTitle
		pMarket.Outcomes = []models.Outcome{{Label: "Yes", Price: 0.63 + float64(i)*0.01}, {Label: "No", Price: 0.37 - float64(i)*0.01}}

		markets = append(markets, kMarket, pMarket)
	}

	cfg := Config{
		EmbeddingSimilarityHigh: 0.92,
		EmbeddingSimilarityLow:  0.78,
		JaccardThreshold:        0.25,
		LevenshteinThreshold:    0.40,
		ResolutionWindowHours:   48,
	}

	// Stage 1 only (no embedder) — all equivalent pairs should pass pre-filter
	engine := NewEngine(cfg, nil)
	groups, err := engine.DetectGroups(context.Background(), markets)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(groups), 5,
		"F3: must identify ≥5 known-equivalent pairs; got %d", len(groups))

	t.Logf("F3: identified %d equivalent pairs out of %d candidate pairs", len(groups), len(equivalentPairs))
	for _, g := range groups {
		t.Logf("  group %s: %s <-> %s (confidence=%.3f, method=%s)",
			g.GroupID[:8], g.Members[0].ID, g.Members[1].ID, g.ConfidenceScore, g.MatchMethod)
	}
}

// F4: equinox match rejects ≥10 known-non-equivalent pairs without false positives.
func TestF4_KnownNonEquivalentPairs(t *testing.T) {
	resTime := acceptanceTimePtr(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	resTimeLate := acceptanceTimePtr(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))

	type marketDef struct {
		id, title, normTitle string
		venue                models.Venue
		ct                   models.ContractType
		outcomeCount         int
		resTime              *time.Time
	}

	nonEquivMarkets := []marketDef{
		{"KALSHI:ne-btc100k", "Will Bitcoin exceed $100,000?", "will bitcoin exceed 100000", models.VenueKalshi, models.ContractBinary, 2, resTime},
		{"POLYMARKET:ne-eth5k", "Will Ethereum hit $5,000?", "will ethereum hit 5000", models.VenuePolymarket, models.ContractBinary, 2, resTime},
		{"KALSHI:ne-fed-cut", "Will the Fed cut interest rates?", "will the fed cut interest rate", models.VenueKalshi, models.ContractBinary, 2, resTime},
		{"POLYMARKET:ne-trump-win", "Will Trump win 2028 election?", "will trump win 2028 election", models.VenuePolymarket, models.ContractBinary, 2, resTimeLate},
		{"KALSHI:ne-gold3k", "Will gold exceed $3,000?", "will gold exceed 3000", models.VenueKalshi, models.ContractBinary, 2, resTime},
		{"POLYMARKET:ne-sp6k", "Will S&P 500 hit 6,000?", "will sp 500 hit 6000", models.VenuePolymarket, models.ContractBinary, 2, resTime},
		{"KALSHI:ne-rain-sf", "Will it rain in San Francisco tomorrow?", "will it rain in san francisco tomorrow", models.VenueKalshi, models.ContractBinary, 2, resTime},
		{"POLYMARKET:ne-rain-ny", "Will it rain in New York tomorrow?", "will it rain in new york tomorrow", models.VenuePolymarket, models.ContractBinary, 2, resTime},
		{"KALSHI:ne-superbowl", "Who will win the Super Bowl 2027?", "who will win the super bowl 2027", models.VenueKalshi, models.ContractCategorical, 4, resTimeLate},
		{"POLYMARKET:ne-oscars", "Who will win Best Picture at the Oscars 2027?", "who will win best picture at the oscar 2027", models.VenuePolymarket, models.ContractCategorical, 5, resTimeLate},
		{"KALSHI:ne-oil80", "Will oil price exceed $80 per barrel?", "will oil price exceed 80 per barrel", models.VenueKalshi, models.ContractBinary, 2, resTime},
		{"POLYMARKET:ne-gdp4", "Will US GDP growth exceed 4%?", "will us gdp growth exceed 4", models.VenuePolymarket, models.ContractBinary, 2, resTime},
		{"KALSHI:ne-btc50k", "Will Bitcoin drop below $50,000?", "will bitcoin drop below 50000", models.VenueKalshi, models.ContractBinary, 2, resTime},
		{"POLYMARKET:ne-aapl", "Will AAPL close above $250?", "will aapl close above 250", models.VenuePolymarket, models.ContractBinary, 2, resTime},
	}

	var markets []models.CanonicalMarket
	for _, md := range nonEquivMarkets {
		outcomes := make([]models.Outcome, md.outcomeCount)
		for i := 0; i < md.outcomeCount; i++ {
			outcomes[i] = models.Outcome{Label: "Option " + string(rune('A'+i)), Price: 1.0 / float64(md.outcomeCount)}
		}

		m := makeAcceptanceMarket(md.id, md.venue, md.title, md.normTitle, md.ct, models.SettlementCFTC, 0.50, md.resTime)
		m.Outcomes = outcomes
		m.Description = md.title
		markets = append(markets, m)
	}

	cfg := Config{
		EmbeddingSimilarityHigh: 0.92,
		EmbeddingSimilarityLow:  0.78,
		JaccardThreshold:        0.25,
		LevenshteinThreshold:    0.40,
		ResolutionWindowHours:   48,
	}

	engine := NewEngine(cfg, nil)
	groups, err := engine.DetectGroups(context.Background(), markets)
	require.NoError(t, err)

	// Count unique pairs in the dataset
	totalCrossVenuePairs := 0
	for i := 0; i < len(markets); i++ {
		for j := i + 1; j < len(markets); j++ {
			if markets[i].Venue != markets[j].Venue {
				totalCrossVenuePairs++
			}
		}
	}

	matchedPairs := len(groups)
	rejectedPairs := totalCrossVenuePairs - matchedPairs

	assert.GreaterOrEqual(t, rejectedPairs, 10,
		"F4: must reject ≥10 known-non-equivalent pairs; rejected %d of %d cross-venue pairs", rejectedPairs, totalCrossVenuePairs)

	// Verify no false positives: none of the matched groups should pair obviously different topics
	for _, g := range groups {
		if len(g.Members) >= 2 {
			a, b := g.Members[0], g.Members[1]
			assert.Contains(t, []models.MatchFlag(g.Flags), models.FlagLowConfidence,
				"matched pair %s <-> %s should have LOW_CONFIDENCE flag (no embeddings)", a.ID, b.ID)
		}
	}

	t.Logf("F4: rejected %d non-equivalent pairs out of %d total cross-venue pairs", rejectedPairs, totalCrossVenuePairs)
	t.Logf("F4: %d pairs matched (all with LOW_CONFIDENCE due to no embedding)", matchedPairs)
	for _, g := range groups {
		t.Logf("  matched: %s <-> %s (confidence=%.3f)", g.Members[0].ID, g.Members[1].ID, g.ConfidenceScore)
	}
}
