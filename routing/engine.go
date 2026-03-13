package routing

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"equinox/models"
)

// Config holds configurable scoring weights.
type Config struct {
	WeightPriceQuality        float64
	WeightLiquidity           float64
	WeightSpreadQuality       float64
	WeightMarketStatus        float64
	StalenessLiquidityHaircut float64
}

// Engine implements the routing decision logic.
// IMPORTANT: Zero imports from adapters/ — enforced by architecture.
type Engine struct {
	cfg Config
}

func NewEngine(cfg Config) *Engine {
	return &Engine{cfg: cfg}
}

// Route produces a RoutingDecision for the given order and equivalence group.
func (e *Engine) Route(order models.OrderRequest, group models.EquivalenceGroup) (*models.RoutingDecision, error) {
	if len(group.Members) == 0 {
		return nil, fmt.Errorf("equivalence group has no members")
	}

	fairValue := computeFairValue(group.Members)
	scores := e.scoreAll(group.Members, fairValue)

	type scored struct {
		market models.CanonicalMarket
		score  models.VenueScore
	}

	var scoredMarkets []scored
	for i, m := range group.Members {
		scoredMarkets = append(scoredMarkets, scored{market: m, score: scores[i]})
	}

	sort.SliceStable(scoredMarkets, func(i, j int) bool {
		if scoredMarkets[i].score.Total != scoredMarkets[j].score.Total {
			return scoredMarkets[i].score.Total > scoredMarkets[j].score.Total
		}
		return scoredMarkets[i].market.Venue < scoredMarkets[j].market.Venue
	})

	selected := scoredMarkets[0]
	var rejected []models.RejectedVenue
	breakdown := make(map[models.Venue]models.VenueScore)

	for _, sm := range scoredMarkets {
		breakdown[sm.market.Venue] = sm.score
	}

	for _, sm := range scoredMarkets[1:] {
		reason := buildRejectionReason(sm.score, selected.score)
		rejected = append(rejected, models.RejectedVenue{
			Venue:           sm.market.Venue,
			MarketID:        sm.market.ID,
			Score:           sm.score,
			RejectionReason: reason,
		})
	}

	rationale := buildRationale(order, selected.market, selected.score, group, rejected)

	decision := &models.RoutingDecision{
		DecisionID:           fmt.Sprintf("rd-%d", time.Now().UnixNano()),
		OrderRequest:         order,
		EquivalenceGroup:     group,
		SelectedVenue:        selected.market.Venue,
		SelectedMarket:       selected.market,
		RejectedAlternatives: rejected,
		ScoringBreakdown:     breakdown,
		RoutingRationale:     rationale,
		Timestamp:            time.Now(),
	}

	slog.Info("routing decision",
		"decision_id", decision.DecisionID,
		"selected_venue", decision.SelectedVenue,
		"selected_market", decision.SelectedMarket.ID,
		"score", selected.score.Total,
	)

	return decision, nil
}

func (e *Engine) scoreAll(markets []models.CanonicalMarket, fairValue float64) []models.VenueScore {
	liqValues := make([]float64, len(markets))
	spreadValues := make([]float64, len(markets))

	for i, m := range markets {
		liq := m.Liquidity
		if m.DataStalenessFlag {
			liq *= (1.0 - e.cfg.StalenessLiquidityHaircut)
		}
		liqValues[i] = liq
		spreadValues[i] = m.Spread
	}

	normLiq := minmaxNormalize(liqValues)
	normSpread := minmaxNormalize(spreadValues)

	scores := make([]models.VenueScore, len(markets))
	for i, m := range markets {
		pq := 1.0 - math.Abs(m.YesPrice-fairValue)
		lq := normLiq[i]
		sq := 1.0 - normSpread[i]
		ms := marketStatusScore(m.Status)

		total := e.cfg.WeightPriceQuality*pq +
			e.cfg.WeightLiquidity*lq +
			e.cfg.WeightSpreadQuality*sq +
			e.cfg.WeightMarketStatus*ms

		scores[i] = models.VenueScore{
			PriceQuality:  pq,
			Liquidity:     lq,
			SpreadQuality: sq,
			MarketStatus:  ms,
			Total:         total,
		}
	}

	return scores
}

func computeFairValue(markets []models.CanonicalMarket) float64 {
	if len(markets) == 0 {
		return 0
	}
	sum := 0.0
	for _, m := range markets {
		sum += m.YesPrice
	}
	return sum / float64(len(markets))
}

func marketStatusScore(status models.MarketStatus) float64 {
	switch status {
	case models.StatusOpen:
		return 1.0
	case models.StatusSuspended:
		return 0.3
	default:
		return 0.0
	}
}

func minmaxNormalize(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}

	minVal := values[0]
	maxVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	result := make([]float64, len(values))
	rangeVal := maxVal - minVal
	if rangeVal == 0 {
		for i := range result {
			result[i] = 0.5
		}
		return result
	}

	for i, v := range values {
		result[i] = (v - minVal) / rangeVal
	}
	return result
}

func buildRejectionReason(rejected, selected models.VenueScore) string {
	var reasons []string
	if rejected.PriceQuality < selected.PriceQuality {
		reasons = append(reasons, "worse price quality")
	}
	if rejected.Liquidity < selected.Liquidity {
		reasons = append(reasons, "lower liquidity")
	}
	if rejected.SpreadQuality < selected.SpreadQuality {
		reasons = append(reasons, "wider spread")
	}
	if rejected.MarketStatus < selected.MarketStatus {
		reasons = append(reasons, "unfavorable market status")
	}
	if len(reasons) == 0 {
		return "lower composite score"
	}
	return strings.Join(reasons, "; ")
}

func buildRationale(order models.OrderRequest, selected models.CanonicalMarket, score models.VenueScore, group models.EquivalenceGroup, rejected []models.RejectedVenue) string {
	var b strings.Builder

	fmt.Fprintf(&b, "ROUTING DECISION [%s]\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Order: BUY %d %s on \"%s\"\n", order.Size, order.Side, selected.Title)
	fmt.Fprintf(&b, "Equivalence Group: %s (%s, confidence: %.3f)\n", group.GroupID[:12], group.MatchMethod, group.ConfidenceScore)

	var warnings []string
	for _, f := range group.Flags {
		if f == models.FlagSettlementDivergence || f == models.FlagStalePricingData || f == models.FlagLowConfidence {
			warnings = append(warnings, string(f))
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintf(&b, "[WARNINGS: %s]\n", strings.Join(warnings, " | "))
	}

	fmt.Fprintf(&b, "\nSELECTED: %s — %s (score: %.4f)\n", selected.Venue, selected.ID, score.Total)
	fmt.Fprintf(&b, "  Price Quality:  %.4f (YesPrice %.4f)\n", score.PriceQuality, selected.YesPrice)
	fmt.Fprintf(&b, "  Liquidity:      %.4f ($%.0f)\n", score.Liquidity, selected.Liquidity)
	fmt.Fprintf(&b, "  Spread Quality: %.4f (spread: %.4f)\n", score.SpreadQuality, selected.Spread)
	fmt.Fprintf(&b, "  Market Status:  %.4f (%s)\n", score.MarketStatus, selected.Status)

	for _, r := range rejected {
		fmt.Fprintf(&b, "\nREJECTED: %s — %s (score: %.4f)\n", r.Venue, r.MarketID, r.Score.Total)
		fmt.Fprintf(&b, "  %s\n", r.RejectionReason)
	}

	fmt.Fprintf(&b, "\nNOTE: USDC/USD assumed 1:1. SimulatedOnly=true.\n")

	return b.String()
}
