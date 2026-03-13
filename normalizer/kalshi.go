package normalizer

import (
	"encoding/json"
	"fmt"
	"time"

	"equinox/adapters"
	"equinox/models"
)

type kalshiPayload struct {
	Ticker          string  `json:"ticker"`
	Title           string  `json:"title"`
	Subtitle        string  `json:"subtitle"`
	Status          string  `json:"status"`
	YesBid          int     `json:"yes_bid"`
	YesAsk          int     `json:"yes_ask"`
	NoBid           int     `json:"no_bid"`
	NoAsk           int     `json:"no_ask"`
	Volume24h       int     `json:"volume_24h"`
	CloseTime       string  `json:"close_time"`
	RulesPrimary    string  `json:"rules_primary"`
	Liquidity       float64 `json:"liquidity"`
	MarketType      string  `json:"market_type"`
}

func normalizeKalshi(raw adapters.RawMarket) (*models.CanonicalMarket, error) {
	var p kalshiPayload
	if err := json.Unmarshal(raw.RawPayload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal kalshi payload: %w", err)
	}

	// Price normalization: cents (1-99) → 0.0-1.0
	yesPrice := (float64(p.YesBid) + float64(p.YesAsk)) / 200.0
	noPrice := 1.0 - yesPrice
	spread := float64(p.YesAsk-p.YesBid) / 100.0

	status := mapKalshiStatus(p.Status)

	var resolutionTime *time.Time
	var resolutionTimeUTC *time.Time
	if p.CloseTime != "" {
		if t, err := time.Parse(time.RFC3339, p.CloseTime); err == nil {
			resolutionTime = &t
			utc := t.UTC()
			resolutionTimeUTC = &utc
		}
	}

	description := p.RulesPrimary
	if description == "" {
		description = p.Subtitle
	}

	vol := float64(p.Volume24h)
	var vol24h *float64
	if p.Volume24h > 0 {
		vol24h = &vol
	}

	contractType := models.ContractBinary
	// ASSUMPTION A3: Kalshi markets default to BINARY
	if p.MarketType == "categorical" {
		contractType = models.ContractCategorical
	} else if p.MarketType == "scalar" {
		contractType = models.ContractScalar
	}

	normalizedTitle := NormalizeTitle(p.Title)

	cm := &models.CanonicalMarket{
		ID:                  fmt.Sprintf("KALSHI:%s", p.Ticker),
		Venue:               models.VenueKalshi,
		Title:               p.Title,
		NormalizedTitle:      normalizedTitle,
		Description:         description,
		Outcomes: []models.Outcome{
			{Label: "Yes", Price: yesPrice},
			{Label: "No", Price: noPrice},
		},
		ResolutionTime:      resolutionTime,
		ResolutionTimeUTC:   resolutionTimeUTC,
		YesPrice:            yesPrice,
		NoPrice:             noPrice,
		Spread:              spread,
		Liquidity:           p.Liquidity,
		Volume24h:           vol24h,
		Status:              status,
		ContractType:        contractType,
		SettlementMechanism: models.SettlementCFTC,
		RulesHash:           ComputeRulesHash(description),
		IngestedAt:          raw.FetchedAt,
		RawPayload:          raw.RawPayload,
	}

	return cm, nil
}

func mapKalshiStatus(s string) models.MarketStatus {
	switch s {
	case "open":
		return models.StatusOpen
	case "closed":
		return models.StatusClosed
	case "settled":
		return models.StatusResolved
	default:
		return models.StatusSuspended
	}
}
