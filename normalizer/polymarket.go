package normalizer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"equinox/adapters"
	"equinox/models"
)

type polymergedPayload struct {
	Gamma polyGamma  `json:"gamma"`
	CLOB  *polyCLOB  `json:"clob,omitempty"`
}

type polyGamma struct {
	ID              string  `json:"id"`
	Question        string  `json:"question"`
	Description     string  `json:"description"`
	ConditionID     string  `json:"conditionId"`
	EndDate         string  `json:"endDate"`
	Liquidity       float64 `json:"liquidity"`
	Volume24h       float64 `json:"volume24hr"`
	Active          bool    `json:"active"`
	Closed          bool    `json:"closed"`
	Funded          bool    `json:"funded"`
	OutcomePrices   string  `json:"outcomePrices"` // JSON-encoded: "[\"0.65\",\"0.35\"]"
	Outcomes        string  `json:"outcomes"`       // JSON-encoded: "[\"Yes\",\"No\"]"
	NegRisk         bool    `json:"neg_risk"`
	NegRiskMarketID string  `json:"neg_risk_market_id"`
}

type polyCLOB struct {
	ConditionID string          `json:"condition_id"`
	Tokens      []polyCLOBToken `json:"tokens"`
	Active      bool            `json:"active"`
}

type polyCLOBToken struct {
	TokenID string `json:"token_id"`
	Outcome string `json:"outcome"`
}

func normalizePolymarket(raw adapters.RawMarket) (*models.CanonicalMarket, error) {
	var p polymergedPayload
	if err := json.Unmarshal(raw.RawPayload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal polymarket payload: %w", err)
	}

	// Parse outcomePrices: JSON-encoded string → float64 slice
	yesPrice, noPrice, err := parseOutcomePrices(p.Gamma.OutcomePrices)
	if err != nil {
		return nil, fmt.Errorf("parse outcome prices: %w", err)
	}

	outcomes, err := parseOutcomeLabels(p.Gamma.Outcomes)
	if err != nil {
		outcomes = []string{"Yes", "No"}
	}

	var modelOutcomes []models.Outcome
	for i, label := range outcomes {
		price := 0.0
		if i == 0 {
			price = yesPrice
		} else if i == 1 {
			price = noPrice
		}
		modelOutcomes = append(modelOutcomes, models.Outcome{
			Label: label,
			Price: price,
		})
	}

	spread := 0.0
	// ASSUMPTION A1: USDC = USD 1:1 for price normalization.

	status := mapPolymarketStatus(p.Gamma)

	var resolutionTime *time.Time
	var resolutionTimeUTC *time.Time
	if p.Gamma.EndDate != "" {
		if t, err := time.Parse(time.RFC3339, p.Gamma.EndDate); err == nil {
			resolutionTime = &t
			utc := t.UTC()
			resolutionTimeUTC = &utc
		} else if t, err := time.Parse("2006-01-02T15:04:05Z", p.Gamma.EndDate); err == nil {
			resolutionTime = &t
			utc := t.UTC()
			resolutionTimeUTC = &utc
		}
	}

	var vol24h *float64
	if p.Gamma.Volume24h > 0 {
		v := p.Gamma.Volume24h
		vol24h = &v
	}

	contractType := models.ContractBinary
	if len(outcomes) > 2 {
		contractType = models.ContractCategorical
	}

	nativeID := p.Gamma.ConditionID
	if nativeID == "" {
		nativeID = p.Gamma.ID
	}

	description := p.Gamma.Description
	normalizedTitle := NormalizeTitle(p.Gamma.Question)

	settlementNote := "OPTIMISTIC_ORACLE: Resolution via UMA optimistic oracle. USDC/USD assumed 1:1."

	cm := &models.CanonicalMarket{
		ID:                  fmt.Sprintf("POLYMARKET:%s", nativeID),
		Venue:               models.VenuePolymarket,
		Title:               p.Gamma.Question,
		NormalizedTitle:      normalizedTitle,
		Description:         description,
		Outcomes:            modelOutcomes,
		ResolutionTime:      resolutionTime,
		ResolutionTimeUTC:   resolutionTimeUTC,
		YesPrice:            yesPrice,
		NoPrice:             noPrice,
		Spread:              spread,
		Liquidity:           p.Gamma.Liquidity,
		Volume24h:           vol24h,
		Status:              status,
		ContractType:        contractType,
		SettlementMechanism: models.SettlementOptimisticOracle,
		SettlementNote:      &settlementNote,
		RulesHash:           ComputeRulesHash(description),
		IngestedAt:          raw.FetchedAt,
		RawPayload:          raw.RawPayload,
	}

	return cm, nil
}

func parseOutcomePrices(s string) (yesPrice, noPrice float64, err error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty outcomePrices")
	}

	var prices []string
	if err := json.Unmarshal([]byte(s), &prices); err != nil {
		return 0, 0, fmt.Errorf("unmarshal outcomePrices %q: %w", s, err)
	}

	if len(prices) < 2 {
		return 0, 0, fmt.Errorf("expected at least 2 prices, got %d", len(prices))
	}

	yesPrice, err = strconv.ParseFloat(prices[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse yes price %q: %w", prices[0], err)
	}

	noPrice, err = strconv.ParseFloat(prices[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse no price %q: %w", prices[1], err)
	}

	return yesPrice, noPrice, nil
}

func parseOutcomeLabels(s string) ([]string, error) {
	if s == "" {
		return nil, fmt.Errorf("empty outcomes")
	}

	var labels []string
	if err := json.Unmarshal([]byte(s), &labels); err != nil {
		return nil, fmt.Errorf("unmarshal outcomes: %w", err)
	}

	return labels, nil
}

func mapPolymarketStatus(g polyGamma) models.MarketStatus {
	if g.Closed {
		return models.StatusClosed
	}
	if g.Active && g.Funded {
		return models.StatusOpen
	}
	if g.Active {
		return models.StatusSuspended
	}
	return models.StatusClosed
}
