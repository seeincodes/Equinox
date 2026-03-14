package kalshi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"equinox/adapters"
	"equinox/models"

	"github.com/go-resty/resty/v2"
)

// flexFloat64 handles JSON fields that may arrive as either a number or a string.
type flexFloat64 float64

func (f *flexFloat64) UnmarshalJSON(b []byte) error {
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexFloat64(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("flexFloat64: cannot parse %s", string(b))
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("flexFloat64: cannot parse string %q: %w", s, err)
	}
	*f = flexFloat64(n)
	return nil
}

// ASSUMPTION A2: Kalshi read-only endpoints require no authentication for market data.
// ASSUMPTION A4: Kalshi sandbox is representative of production schema.
// ASSUMPTION A8: Venue APIs will generally be available during ingest cycles.
// ASSUMPTION A9: Kalshi sandbox accessible for development.

// Adapter implements adapters.VenueAdapter for the Kalshi exchange.
type Adapter struct {
	client  *resty.Client
	cb      *adapters.CircuitBreaker
	rp      *adapters.RetryPolicy
	baseURL string
}

func New(baseURL, apiKey string) *Adapter {
	client := adapters.NewHTTPClient(baseURL)
	if apiKey != "" {
		client.SetHeader("Authorization", "Bearer "+apiKey)
	}
	return &Adapter{
		client:  client,
		cb:      adapters.NewCircuitBreaker(models.VenueKalshi),
		rp:      adapters.NewRetryPolicy(models.VenueKalshi),
		baseURL: baseURL,
	}
}

func (a *Adapter) VenueID() models.Venue {
	return models.VenueKalshi
}

// FetchMarkets retrieves all open markets from Kalshi with cursor-based pagination.
func (a *Adapter) FetchMarkets(ctx context.Context) ([]adapters.RawMarket, error) {
	var allMarkets []adapters.RawMarket
	cursor := ""
	now := time.Now()

	for {
		params := map[string]string{
			"status": "open",
			"limit":  "100",
		}
		if cursor != "" {
			params["cursor"] = cursor
		}

		body, err := adapters.DoGet(ctx, a.client, models.VenueKalshi, "/markets", params, a.cb, a.rp)
		if err != nil {
			return nil, err
		}

		var resp marketsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, &adapters.AdapterError{
				Venue:      models.VenueKalshi,
				Type:       adapters.T4SchemaChange,
				Attempts:   1,
				LastError:  fmt.Errorf("unmarshal markets response: %w", err),
				OccurredAt: time.Now(),
			}
		}

		if len(resp.Markets) == 0 && len(allMarkets) == 0 {
			return nil, &adapters.AdapterError{
				Venue:      models.VenueKalshi,
				Type:       adapters.T5PartialData,
				Attempts:   1,
				LastError:  fmt.Errorf("empty market list from Kalshi"),
				OccurredAt: time.Now(),
			}
		}

		for _, m := range resp.Markets {
			if err := validateKalshiMarket(m); err != nil {
				slog.Warn("skipping invalid Kalshi market",
					"ticker", m.Ticker,
					"error", err,
				)
				continue
			}

			payload, _ := json.Marshal(m)
			allMarkets = append(allMarkets, adapters.RawMarket{
				NativeID:   m.Ticker,
				Venue:      models.VenueKalshi,
				RawPayload: payload,
				FetchedAt:  now,
			})
		}

		if resp.Cursor == "" {
			break
		}
		cursor = resp.Cursor

		slog.Debug("kalshi pagination",
			"fetched", len(allMarkets),
			"cursor", cursor,
		)
	}

	slog.Info("kalshi markets fetched", "count", len(allMarkets))
	return allMarkets, nil
}

// FetchPricing retrieves the orderbook for a single market.
func (a *Adapter) FetchPricing(ctx context.Context, ticker string) (*adapters.RawPricing, error) {
	params := map[string]string{"depth": "10"}
	path := fmt.Sprintf("/markets/%s/orderbook", ticker)

	body, err := adapters.DoGet(ctx, a.client, models.VenueKalshi, path, params, a.cb, a.rp)
	if err != nil {
		return nil, err
	}

	var resp orderbookResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &adapters.AdapterError{
			Venue:      models.VenueKalshi,
			Type:       adapters.T4SchemaChange,
			Attempts:   1,
			LastError:  fmt.Errorf("unmarshal orderbook: %w", err),
			OccurredAt: time.Now(),
		}
	}

	return &adapters.RawPricing{
		NativeID:   ticker,
		Venue:      models.VenueKalshi,
		RawPayload: body,
		FetchedAt:  time.Now(),
	}, nil
}

// HealthCheck verifies the Kalshi exchange is operational.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	_, err := adapters.DoGet(ctx, a.client, models.VenueKalshi, "/exchange/status", nil, a.cb, a.rp)
	return err
}

// validateKalshiMarket checks required fields for T4 schema validation.
func validateKalshiMarket(m kalshiMarket) error {
	if m.Ticker == "" {
		return fmt.Errorf("missing ticker")
	}
	if m.Title == "" {
		return fmt.Errorf("missing title")
	}
	if m.Status == "" {
		return fmt.Errorf("missing status")
	}
	return nil
}

// API response structs

type marketsResponse struct {
	Markets []kalshiMarket `json:"markets"`
	Cursor  string         `json:"cursor"`
}

type kalshiMarket struct {
	Ticker             string `json:"ticker"`
	EventTicker        string `json:"event_ticker"`
	Title              string `json:"title"`
	Subtitle           string `json:"subtitle"`
	Status             string `json:"status"`
	YesBidDollars      string `json:"yes_bid_dollars"`
	YesAskDollars      string `json:"yes_ask_dollars"`
	NoBidDollars       string `json:"no_bid_dollars"`
	NoAskDollars       string `json:"no_ask_dollars"`
	LastPriceDollars   string `json:"last_price_dollars"`
	VolumeFP           string `json:"volume_fp"`
	Volume24hFP        string `json:"volume_24h_fp"`
	OpenInterestFP     string `json:"open_interest_fp"`
	LiquidityDollars   string `json:"liquidity_dollars"`
	CloseTime          string `json:"close_time"`
	ExpirationTime     string `json:"expiration_time"`
	RulesPrimary       string `json:"rules_primary"`
	RulesSecondary     string `json:"rules_secondary"`
	Result             string `json:"result"`
	CanCloseEarly      bool   `json:"can_close_early"`
	ExpirationValue    string `json:"expiration_value"`
	MarketType         string `json:"market_type"`
	OpenTime           string `json:"open_time"`
	NotionalValue      string `json:"notional_value_dollars"`
}

type orderbookResponse struct {
	Orderbook orderbook `json:"orderbook"`
}

type orderbook struct {
	Yes [][]int `json:"yes"` // [[price_cents, quantity], ...]
	No  [][]int `json:"no"`
}
