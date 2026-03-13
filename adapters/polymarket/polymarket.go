package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"equinox/adapters"
	"equinox/models"

	"github.com/go-resty/resty/v2"
)

// Adapter implements adapters.VenueAdapter for the Polymarket exchange.
type Adapter struct {
	gammaClient *resty.Client
	clobClient  *resty.Client
	cb          *adapters.CircuitBreaker
	rp          *adapters.RetryPolicy
}

// New creates a Polymarket adapter with injectable base URLs for testing.
func New(gammaBaseURL, clobBaseURL string) *Adapter {
	return &Adapter{
		gammaClient: adapters.NewHTTPClient(gammaBaseURL),
		clobClient:  adapters.NewHTTPClient(clobBaseURL),
		cb:          adapters.NewCircuitBreaker(models.VenuePolymarket),
		rp:          adapters.NewRetryPolicy(models.VenuePolymarket),
	}
}

func (a *Adapter) VenueID() models.Venue {
	return models.VenuePolymarket
}

// FetchMarkets retrieves active markets from the Gamma API and enriches with CLOB data.
func (a *Adapter) FetchMarkets(ctx context.Context) ([]adapters.RawMarket, error) {
	gammaMarkets, err := a.fetchGammaMarkets(ctx)
	if err != nil {
		return nil, err
	}

	clobMarkets, err := a.fetchCLOBMarkets(ctx)
	if err != nil {
		slog.Warn("CLOB fetch failed, using Gamma-only data", "error", err)
		clobMarkets = make(map[string]clobMarket)
	}

	var allMarkets []adapters.RawMarket
	now := time.Now()

	for _, gm := range gammaMarkets {
		if err := validateGammaMarket(gm); err != nil {
			slog.Warn("skipping invalid Polymarket market",
				"id", gm.ID,
				"error", err,
			)
			continue
		}

		merged := mergedMarket{
			Gamma: gm,
		}

		if cm, ok := clobMarkets[gm.ConditionID]; ok {
			merged.CLOB = &cm
		}

		payload, _ := json.Marshal(merged)
		nativeID := gm.ConditionID
		if nativeID == "" {
			nativeID = gm.ID
		}

		allMarkets = append(allMarkets, adapters.RawMarket{
			NativeID:   nativeID,
			Venue:      models.VenuePolymarket,
			RawPayload: payload,
			FetchedAt:  now,
		})
	}

	slog.Info("polymarket markets fetched", "count", len(allMarkets))
	return allMarkets, nil
}

// FetchPricing retrieves the orderbook for a Polymarket market.
func (a *Adapter) FetchPricing(ctx context.Context, tokenID string) (*adapters.RawPricing, error) {
	params := map[string]string{"token_id": tokenID}

	body, err := adapters.DoGet(ctx, a.clobClient, models.VenuePolymarket, "/book", params, a.cb, a.rp)
	if err != nil {
		return nil, err
	}

	return &adapters.RawPricing{
		NativeID:   tokenID,
		Venue:      models.VenuePolymarket,
		RawPayload: body,
		FetchedAt:  time.Now(),
	}, nil
}

// HealthCheck verifies Polymarket APIs are reachable.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	params := map[string]string{"limit": "1", "active": "true"}
	_, err := adapters.DoGet(ctx, a.gammaClient, models.VenuePolymarket, "/markets", params, a.cb, a.rp)
	return err
}

func (a *Adapter) fetchGammaMarkets(ctx context.Context) ([]gammaMarket, error) {
	var allMarkets []gammaMarket
	offset := 0
	limit := 100

	for {
		params := map[string]string{
			"active": "true",
			"closed": "false",
			"limit":  fmt.Sprintf("%d", limit),
			"offset": fmt.Sprintf("%d", offset),
		}

		body, err := adapters.DoGet(ctx, a.gammaClient, models.VenuePolymarket, "/markets", params, a.cb, a.rp)
		if err != nil {
			return nil, err
		}

		var markets []gammaMarket
		if err := json.Unmarshal(body, &markets); err != nil {
			return nil, &adapters.AdapterError{
				Venue:      models.VenuePolymarket,
				Type:       adapters.T4SchemaChange,
				Attempts:   1,
				LastError:  fmt.Errorf("unmarshal gamma markets: %w", err),
				OccurredAt: time.Now(),
			}
		}

		if len(markets) == 0 {
			break
		}

		allMarkets = append(allMarkets, markets...)

		if len(markets) < limit {
			break
		}
		offset += limit
	}

	if len(allMarkets) == 0 {
		return nil, &adapters.AdapterError{
			Venue:      models.VenuePolymarket,
			Type:       adapters.T5PartialData,
			Attempts:   1,
			LastError:  fmt.Errorf("empty market list from Polymarket Gamma"),
			OccurredAt: time.Now(),
		}
	}

	return allMarkets, nil
}

func (a *Adapter) fetchCLOBMarkets(ctx context.Context) (map[string]clobMarket, error) {
	result := make(map[string]clobMarket)
	cursor := ""

	for {
		params := map[string]string{}
		if cursor != "" {
			params["next_cursor"] = cursor
		}

		body, err := adapters.DoGet(ctx, a.clobClient, models.VenuePolymarket, "/markets", params, a.cb, a.rp)
		if err != nil {
			return nil, err
		}

		var resp clobMarketsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, &adapters.AdapterError{
				Venue:      models.VenuePolymarket,
				Type:       adapters.T4SchemaChange,
				Attempts:   1,
				LastError:  fmt.Errorf("unmarshal CLOB markets: %w", err),
				OccurredAt: time.Now(),
			}
		}

		for _, m := range resp.Data {
			result[m.ConditionID] = m
		}

		if resp.NextCursor == "" || resp.NextCursor == "LTE=" {
			break
		}
		cursor = resp.NextCursor
	}

	return result, nil
}

func validateGammaMarket(m gammaMarket) error {
	if m.ID == "" {
		return fmt.Errorf("missing id")
	}
	if m.Question == "" {
		return fmt.Errorf("missing question")
	}
	return nil
}

// API response structs

type gammaMarket struct {
	ID              string  `json:"id"`
	Question        string  `json:"question"`
	Description     string  `json:"description"`
	ConditionID     string  `json:"conditionId"`
	Slug            string  `json:"slug"`
	EndDate         string  `json:"endDate"`
	Liquidity       float64 `json:"liquidity"`
	Volume          float64 `json:"volume"`
	Volume24h       float64 `json:"volume24hr"`
	Active          bool    `json:"active"`
	Closed          bool    `json:"closed"`
	Funded          bool    `json:"funded"`
	Archived        bool    `json:"archived"`
	New             bool    `json:"new"`
	Featured        bool    `json:"featured"`
	Restricted      bool    `json:"restricted"`
	GroupItemTitle  string  `json:"groupItemTitle"`
	OutcomePrices   string  `json:"outcomePrices"` // JSON-encoded string: "[\"0.65\",\"0.35\"]"
	Outcomes        string  `json:"outcomes"`       // JSON-encoded string: "[\"Yes\",\"No\"]"
	ClobTokenIDs    string  `json:"clobTokenIds"`   // JSON-encoded string
	NegRisk         bool    `json:"neg_risk"`
	NegRiskMarketID string  `json:"neg_risk_market_id"`
}

type clobMarketsResponse struct {
	Data       []clobMarket `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

type clobMarket struct {
	ConditionID string      `json:"condition_id"`
	Tokens      []clobToken `json:"tokens"`
	Active      bool        `json:"active"`
}

type clobToken struct {
	TokenID string `json:"token_id"`
	Outcome string `json:"outcome"`
}

// mergedMarket combines Gamma metadata with CLOB trading data.
type mergedMarket struct {
	Gamma gammaMarket `json:"gamma"`
	CLOB  *clobMarket `json:"clob,omitempty"`
}
