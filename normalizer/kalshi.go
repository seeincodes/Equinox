package normalizer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"equinox/adapters"
	"equinox/models"
)

type kalshiPayload struct {
	Ticker             string `json:"ticker"`
	Title              string `json:"title"`
	Subtitle           string `json:"subtitle"`
	Status             string `json:"status"`
	YesBidDollars      string `json:"yes_bid_dollars"`
	YesAskDollars      string `json:"yes_ask_dollars"`
	NoBidDollars       string `json:"no_bid_dollars"`
	NoAskDollars       string `json:"no_ask_dollars"`
	Volume24hFP        string `json:"volume_24h_fp"`
	CloseTime          string `json:"close_time"`
	ExpirationTime     string `json:"expiration_time"`
	RulesPrimary       string `json:"rules_primary"`
	LiquidityDollars   string `json:"liquidity_dollars"`
	MarketType         string `json:"market_type"`
}

func parseDollarString(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func normalizeKalshi(raw adapters.RawMarket) (*models.CanonicalMarket, error) {
	var p kalshiPayload
	if err := json.Unmarshal(raw.RawPayload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal kalshi payload: %w", err)
	}

	yesBid := parseDollarString(p.YesBidDollars)
	yesAsk := parseDollarString(p.YesAskDollars)
	yesPrice := (yesBid + yesAsk) / 2.0
	noPrice := 1.0 - yesPrice
	spread := yesAsk - yesBid
	if spread < 0 {
		spread = 0
	}

	liquidity := parseDollarString(p.LiquidityDollars)

	status := mapKalshiStatus(p.Status)

	var resolutionTime *time.Time
	var resolutionTimeUTC *time.Time
	closeTimeStr := p.CloseTime
	if closeTimeStr == "" {
		closeTimeStr = p.ExpirationTime
	}
	if closeTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, closeTimeStr); err == nil {
			resolutionTime = &t
			utc := t.UTC()
			resolutionTimeUTC = &utc
		}
	}

	description := p.RulesPrimary
	if description == "" {
		description = p.Subtitle
	}

	vol24h := parseDollarString(p.Volume24hFP)
	var vol24hPtr *float64
	if vol24h > 0 {
		vol24hPtr = &vol24h
	}

	contractType := models.ContractBinary
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
		Liquidity:           liquidity,
		Volume24h:           vol24hPtr,
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
	case "open", "active":
		return models.StatusOpen
	case "closed":
		return models.StatusClosed
	case "settled":
		return models.StatusResolved
	default:
		return models.StatusSuspended
	}
}
