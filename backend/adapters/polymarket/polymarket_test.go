package polymarket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_VenueID(t *testing.T) {
	a := New("http://localhost", "http://localhost")
	assert.Equal(t, models.VenuePolymarket, a.VenueID())
}

func TestAdapter_FetchMarkets_Success(t *testing.T) {
	gammaMux := http.NewServeMux()
	gammaMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		markets := []gammaMarket{
			{
				ID:            "gamma-1",
				Question:      "Will Bitcoin exceed $100,000?",
				Description:   "Resolves YES if BTC > $100k",
				ConditionID:   "cond-abc",
				Active:        true,
				Funded:        true,
				OutcomePrices: `["0.65","0.35"]`,
				Outcomes:      `["Yes","No"]`,
				Liquidity:     50000,
			},
		}
		json.NewEncoder(w).Encode(markets)
	})
	gammaSrv := httptest.NewServer(gammaMux)
	defer gammaSrv.Close()

	clobMux := http.NewServeMux()
	clobMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		resp := clobMarketsResponse{
			Data: []clobMarket{
				{
					ConditionID: "cond-abc",
					Tokens: []clobToken{
						{TokenID: "token-yes", Outcome: "Yes"},
						{TokenID: "token-no", Outcome: "No"},
					},
					Active: true,
				},
			},
			NextCursor: "",
		}
		json.NewEncoder(w).Encode(resp)
	})
	clobSrv := httptest.NewServer(clobMux)
	defer clobSrv.Close()

	a := New(gammaSrv.URL, clobSrv.URL)
	markets, err := a.FetchMarkets(context.Background())

	require.NoError(t, err)
	assert.Len(t, markets, 1)
	assert.Equal(t, "cond-abc", markets[0].NativeID)
	assert.Equal(t, models.VenuePolymarket, markets[0].Venue)

	// Verify merged payload
	var merged mergedMarket
	err = json.Unmarshal(markets[0].RawPayload, &merged)
	require.NoError(t, err)
	assert.Equal(t, "gamma-1", merged.Gamma.ID)
	assert.NotNil(t, merged.CLOB)
	assert.Equal(t, "cond-abc", merged.CLOB.ConditionID)
}

func TestAdapter_FetchMarkets_GammaOnly(t *testing.T) {
	gammaMux := http.NewServeMux()
	gammaMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		markets := []gammaMarket{
			{
				ID:            "gamma-1",
				Question:      "Test market",
				ConditionID:   "cond-xyz",
				Active:        true,
				Funded:        true,
				OutcomePrices: `["0.50","0.50"]`,
				Outcomes:      `["Yes","No"]`,
			},
		}
		json.NewEncoder(w).Encode(markets)
	})
	gammaSrv := httptest.NewServer(gammaMux)
	defer gammaSrv.Close()

	clobMux := http.NewServeMux()
	clobMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	clobSrv := httptest.NewServer(clobMux)
	defer clobSrv.Close()

	a := New(gammaSrv.URL, clobSrv.URL)
	markets, err := a.FetchMarkets(context.Background())

	require.NoError(t, err)
	assert.Len(t, markets, 1)

	var merged mergedMarket
	json.Unmarshal(markets[0].RawPayload, &merged)
	assert.Nil(t, merged.CLOB)
}

func TestAdapter_FetchMarkets_SkipsInvalid(t *testing.T) {
	gammaMux := http.NewServeMux()
	gammaMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		markets := []gammaMarket{
			{ID: "valid", Question: "Valid?", ConditionID: "c1", Active: true},
			{ID: "", Question: "Missing ID"},
		}
		json.NewEncoder(w).Encode(markets)
	})
	gammaSrv := httptest.NewServer(gammaMux)
	defer gammaSrv.Close()

	clobMux := http.NewServeMux()
	clobMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(clobMarketsResponse{})
	})
	clobSrv := httptest.NewServer(clobMux)
	defer clobSrv.Close()

	a := New(gammaSrv.URL, clobSrv.URL)
	markets, err := a.FetchMarkets(context.Background())

	require.NoError(t, err)
	assert.Len(t, markets, 1)
}

func TestAdapter_FetchPricing_Success(t *testing.T) {
	clobMux := http.NewServeMux()
	clobMux.HandleFunc("/book", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token-123", r.URL.Query().Get("token_id"))
		w.Write([]byte(`{"bids": [{"price": "0.65", "size": "100"}], "asks": [{"price": "0.68", "size": "50"}]}`))
	})
	clobSrv := httptest.NewServer(clobMux)
	defer clobSrv.Close()

	a := New("http://unused", clobSrv.URL)
	pricing, err := a.FetchPricing(context.Background(), "token-123")

	require.NoError(t, err)
	assert.Equal(t, "token-123", pricing.NativeID)
	assert.Equal(t, models.VenuePolymarket, pricing.Venue)
}

func TestAdapter_HealthCheck_Success(t *testing.T) {
	gammaMux := http.NewServeMux()
	gammaMux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]gammaMarket{{ID: "1", Question: "Q"}})
	})
	gammaSrv := httptest.NewServer(gammaMux)
	defer gammaSrv.Close()

	a := New(gammaSrv.URL, "http://unused")
	err := a.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestValidateGammaMarket(t *testing.T) {
	tests := []struct {
		name    string
		market  gammaMarket
		wantErr bool
	}{
		{
			name:    "valid",
			market:  gammaMarket{ID: "1", Question: "Test?"},
			wantErr: false,
		},
		{
			name:    "missing ID",
			market:  gammaMarket{Question: "Test?"},
			wantErr: true,
		},
		{
			name:    "missing question",
			market:  gammaMarket{ID: "1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGammaMarket(tt.market)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
