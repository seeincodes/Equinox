package kalshi

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
	a := New("http://localhost", "")
	assert.Equal(t, models.VenueKalshi, a.VenueID())
}

func TestAdapter_FetchMarkets_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		resp := marketsResponse{
			Markets: []kalshiMarket{
				{
					Ticker: "KXBTC-25MAR",
					Title:  "Will Bitcoin exceed $100,000?",
					Status: "open",
					YesBid: 65,
					YesAsk: 68,
					NoBid:  32,
					NoAsk:  35,
				},
				{
					Ticker: "KXFED-APR2026",
					Title:  "Will the Fed cut rates in April?",
					Status: "open",
					YesBid: 45,
					YesAsk: 48,
				},
			},
			Cursor: "",
		}
		json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	markets, err := a.FetchMarkets(context.Background())

	require.NoError(t, err)
	assert.Len(t, markets, 2)
	assert.Equal(t, "KXBTC-25MAR", markets[0].NativeID)
	assert.Equal(t, models.VenueKalshi, markets[0].Venue)
}

func TestAdapter_FetchMarkets_Pagination(t *testing.T) {
	page := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		page++
		cursor := r.URL.Query().Get("cursor")

		var resp marketsResponse
		if cursor == "" {
			resp = marketsResponse{
				Markets: []kalshiMarket{
					{Ticker: "MKT1", Title: "Market 1", Status: "open"},
				},
				Cursor: "page2",
			}
		} else {
			resp = marketsResponse{
				Markets: []kalshiMarket{
					{Ticker: "MKT2", Title: "Market 2", Status: "open"},
				},
				Cursor: "",
			}
		}
		json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	markets, err := a.FetchMarkets(context.Background())

	require.NoError(t, err)
	assert.Len(t, markets, 2)
	assert.Equal(t, 2, page)
}

func TestAdapter_FetchMarkets_SkipsInvalidMarkets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		resp := marketsResponse{
			Markets: []kalshiMarket{
				{Ticker: "VALID", Title: "Valid Market", Status: "open"},
				{Ticker: "", Title: "Missing ticker", Status: "open"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	markets, err := a.FetchMarkets(context.Background())

	require.NoError(t, err)
	assert.Len(t, markets, 1)
}

func TestAdapter_FetchMarkets_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	_, err := a.FetchMarkets(context.Background())
	require.Error(t, err)
}

func TestAdapter_FetchPricing_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/markets/KXBTC/orderbook", func(w http.ResponseWriter, r *http.Request) {
		resp := orderbookResponse{
			Orderbook: orderbook{
				Yes: [][]int{{65, 100}, {64, 200}},
				No:  [][]int{{35, 150}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	pricing, err := a.FetchPricing(context.Background(), "KXBTC")

	require.NoError(t, err)
	assert.Equal(t, "KXBTC", pricing.NativeID)
	assert.Equal(t, models.VenueKalshi, pricing.Venue)
}

func TestAdapter_HealthCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/exchange/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"exchange_active": true}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	err := a.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestAdapter_HealthCheck_Failure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/exchange/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(srv.URL, "")
	err := a.HealthCheck(context.Background())
	require.Error(t, err)
}

func TestValidateKalshiMarket(t *testing.T) {
	tests := []struct {
		name    string
		market  kalshiMarket
		wantErr bool
	}{
		{
			name:    "valid",
			market:  kalshiMarket{Ticker: "T1", Title: "Test", Status: "open"},
			wantErr: false,
		},
		{
			name:    "missing ticker",
			market:  kalshiMarket{Title: "Test", Status: "open"},
			wantErr: true,
		},
		{
			name:    "missing title",
			market:  kalshiMarket{Ticker: "T1", Status: "open"},
			wantErr: true,
		},
		{
			name:    "missing status",
			market:  kalshiMarket{Ticker: "T1", Title: "Test"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKalshiMarket(tt.market)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
