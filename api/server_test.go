package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"equinox/config"
	"equinox/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T) (*Server, *store.DB) {
	t.Helper()
	db, err := store.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{
		WeightPriceQuality:        0.40,
		WeightLiquidity:           0.35,
		WeightSpreadQuality:       0.15,
		WeightMarketStatus:        0.10,
		StalenessLiquidityHaircut: 0.20,
	}

	srv := NewServer(db, cfg, ":0")
	return srv, db
}

func seedCanonicalMarkets(t *testing.T, db *store.DB) {
	t.Helper()
	markets := []struct {
		id, venue, title, normTitle, status, contractType, settlement string
		yesPrice, noPrice, spread, liquidity                          float64
	}{
		{"KALSHI:test-1", "KALSHI", "Will BTC exceed 100k?", "will btc exceed 100k",
			"OPEN", "BINARY", "CFTC_REGULATED", 0.65, 0.35, 0.02, 50000},
		{"POLYMARKET:test-2", "POLYMARKET", "Will BTC exceed 100k?", "will btc exceed 100k",
			"OPEN", "BINARY", "OPTIMISTIC_ORACLE", 0.63, 0.37, 0.03, 80000},
		{"KALSHI:test-3", "KALSHI", "Will ETH hit 5k?", "will eth hit 5k",
			"CLOSED", "BINARY", "CFTC_REGULATED", 0.40, 0.60, 0.05, 20000},
	}

	for _, m := range markets {
		_, err := db.Conn().Exec(`
			INSERT INTO canonical_markets (id, venue, title, normalized_title, yes_price, no_price, 
			    spread, liquidity, status, contract_type, settlement_mechanism, outcomes, rules_hash,
			    data_staleness_flag, ingested_at, raw_payload)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', 'hash', 0, CURRENT_TIMESTAMP, '{}')`,
			m.id, m.venue, m.title, m.normTitle, m.yesPrice, m.noPrice,
			m.spread, m.liquidity, m.status, m.contractType, m.settlement)
		require.NoError(t, err)
	}
}

func seedEquivalenceGroup(t *testing.T, db *store.DB) {
	t.Helper()
	_, err := db.Conn().Exec(`
		INSERT INTO equivalence_groups (group_id, member_ids, confidence_score, match_method,
		    embedding_similarity, string_similarity, match_rationale, flags)
		VALUES ('grp-1', '["KALSHI:test-1","POLYMARKET:test-2"]', 0.95, 'EMBEDDING',
		    0.96, 0.88, 'High similarity across venues', '["SETTLEMENT_DIVERGENCE"]')
	`)
	require.NoError(t, err)
}

func TestGetMarkets_All(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/markets", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["count"])
}

func TestGetMarkets_FilterVenue(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/markets?venue=KALSHI", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["count"])
}

func TestGetMarkets_FilterStatus(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/markets?status=OPEN", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["count"])
}

func TestGetMarkets_FilterVenueAndStatus(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/markets?venue=KALSHI&status=OPEN", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["count"])
}

func TestGetMarkets_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/markets", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp["markets"])
	assert.Equal(t, float64(0), resp["count"])
}

func TestGetGroups_All(t *testing.T) {
	srv, db := setupTestServer(t)
	seedEquivalenceGroup(t, db)

	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["count"])
}

func TestGetGroups_MinConfidence(t *testing.T) {
	srv, db := setupTestServer(t)
	seedEquivalenceGroup(t, db)

	req := httptest.NewRequest(http.MethodGet, "/groups?min_confidence=0.99", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
}

func TestPostRoute_Success(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	body, _ := json.Marshal(map[string]interface{}{
		"market_id": "KALSHI:test-1",
		"side":      "YES",
		"size":      100,
	})

	req := httptest.NewRequest(http.MethodPost, "/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Logf("response body: %s", w.Body.String())
	}
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["selected_venue"])
	assert.NotEmpty(t, resp["routing_rationale"])
	assert.Equal(t, true, resp["simulated_only"])
}

func TestPostRoute_BadRequest(t *testing.T) {
	srv, _ := setupTestServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"market_id": "",
		"side":      "YES",
		"size":      0,
	})

	req := httptest.NewRequest(http.MethodPost, "/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPostRoute_MarketNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"market_id": "NONEXISTENT:xyz",
		"side":      "YES",
		"size":      100,
	})

	req := httptest.NewRequest(http.MethodPost, "/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetHealth_DatabaseCounts(t *testing.T) {
	srv, db := setupTestServer(t)
	// Point venue URLs at a closed port so health checks fail instantly
	srv.cfg.KalshiBaseURL = "http://127.0.0.1:1"
	srv.cfg.PolymarketGammaURL = "http://127.0.0.1:1"
	srv.cfg.PolymarketCLOBURL = "http://127.0.0.1:1"

	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotEmpty(t, resp["timestamp"])
	assert.Equal(t, "degraded", resp["status"])
	counts := resp["counts"].(map[string]interface{})
	assert.Equal(t, float64(3), counts["canonical_markets"])
	assert.Equal(t, float64(1), counts["equivalence_groups"])
}

func TestPostRouteBatch_Success(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	orders := []map[string]interface{}{
		{"market_id": "KALSHI:test-1", "side": "YES", "size": 100},
		{"market_id": "POLYMARKET:test-2", "side": "NO", "size": 50},
	}
	body, _ := json.Marshal(orders)

	req := httptest.NewRequest(http.MethodPost, "/route/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["successful_count"])
	assert.Equal(t, float64(0), resp["error_count"])
	assert.NotEmpty(t, resp["decisions"])
}

func TestPostRouteBatch_PartialErrors(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	orders := []map[string]interface{}{
		{"market_id": "KALSHI:test-1", "side": "YES", "size": 100},
		{"market_id": "NONEXISTENT:xyz", "side": "YES", "size": 50},
	}
	body, _ := json.Marshal(orders)

	req := httptest.NewRequest(http.MethodPost, "/route/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["successful_count"])
	assert.Equal(t, float64(1), resp["error_count"])
}

func TestGetDecisions_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/decisions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
}

func TestGetDecisions_WithVenueFilter(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	// Create a routing decision
	_, err := db.Conn().Exec(`
		INSERT INTO routing_decisions
			(decision_id, group_id, order_request, selected_venue, selected_market_id,
			 rejected_alternatives, scoring_breakdown, routing_rationale, simulated_only)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"dec-1", "grp-1", `{"market_id":"KALSHI:test-1","side":"YES","size":100}`,
		"KALSHI", "KALSHI:test-1", `[]`, `{}`, "Test decision", 1)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/decisions?venue=KALSHI", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["count"])
	decisions := resp["decisions"].([]interface{})
	assert.Len(t, decisions, 1)
}

func TestGetGroupHistory_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/groups/nonexistent-group/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
