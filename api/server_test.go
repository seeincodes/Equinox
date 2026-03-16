package api

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func seedTestUser(t *testing.T, srv *Server) {
	t.Helper()
	_, err := srv.auth.CreateUser("test@example.com", "testpass", "admin")
	require.NoError(t, err)
}

func addAuthCookie(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()
	session, err := srv.auth.Login("test@example.com", "testpass")
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "equinox_session", Value: session.ID})
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

// --- Auth tests ---

func TestAuthLogin_Success(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	body, _ := json.Marshal(map[string]string{
		"email": "test@example.com", "password": "testpass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "equinox_session" {
			found = true
			assert.True(t, c.HttpOnly)
		}
	}
	assert.True(t, found, "session cookie should be set")
}

func TestAuthLogin_BadCredentials(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	body, _ := json.Marshal(map[string]string{
		"email": "test@example.com", "password": "wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMe(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "test@example.com", resp["email"])
	assert.Equal(t, "admin", resp["role"])
}

func TestAuthRequired_NoSession(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/markets", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHealthNoAuth(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.cfg.KalshiBaseURL = "http://127.0.0.1:1"
	srv.cfg.PolymarketGammaURL = "http://127.0.0.1:1"
	srv.cfg.PolymarketCLOBURL = "http://127.0.0.1:1"

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

// --- Market tests ---

func TestGetMarkets_All(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/markets", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["count"])
}

func TestGetMarkets_FilterVenue(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/markets?venue=KALSHI", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["count"])
}

// --- Group tests ---

func TestGetGroups_WithMembers(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	groups := raw["groups"].([]interface{})
	require.Len(t, groups, 1)
	group := groups[0].(map[string]interface{})
	members := group["members"].([]interface{})
	assert.Len(t, members, 2)
}

func TestGetGroups_MinConfidence(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedEquivalenceGroup(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/groups?min_confidence=0.99", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
}

// --- Route tests ---

func TestPostRoute_Success(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	body, _ := json.Marshal(map[string]interface{}{
		"market_id": "KALSHI:test-1",
		"side":      "YES",
		"size":      100,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthCookie(t, srv, req)
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
}

func TestPostRoute_WithGroupID(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	body, _ := json.Marshal(map[string]interface{}{
		"group_id": "grp-1",
		"side":     "YES",
		"size":     100,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPostRoute_ViewerForbidden(t *testing.T) {
	srv, db := setupTestServer(t)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	_, err := srv.auth.CreateUser("viewer@example.com", "pass", "viewer")
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]interface{}{
		"market_id": "KALSHI:test-1", "side": "YES", "size": 100,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	session, _ := srv.auth.Login("viewer@example.com", "pass")
	req.AddCookie(&http.Cookie{Name: "equinox_session", Value: session.ID})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPostRoute_BadRequest(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	body, _ := json.Marshal(map[string]interface{}{
		"market_id": "",
		"side":      "YES",
		"size":      0,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Batch route tests ---

func TestPostRouteBatch_Success(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	orders := []map[string]interface{}{
		{"market_id": "KALSHI:test-1", "side": "YES", "size": 100},
		{"market_id": "POLYMARKET:test-2", "side": "NO", "size": 50},
	}
	body, _ := json.Marshal(orders)

	req := httptest.NewRequest(http.MethodPost, "/api/route/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["successful_count"])
}

// --- Decisions tests ---

func TestGetDecisions_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/decisions", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
	assert.NotNil(t, resp["total_count"])
	assert.NotNil(t, resp["page"])
}

func TestGetDecisions_WithPagination(t *testing.T) {
	srv, db := setupTestServer(t)
	seedTestUser(t, srv)
	seedCanonicalMarkets(t, db)
	seedEquivalenceGroup(t, db)

	// Create 3 decisions
	for i := 0; i < 3; i++ {
		db.Conn().Exec(`
			INSERT INTO routing_decisions
				(decision_id, group_id, order_request, selected_venue, selected_market_id,
				 rejected_alternatives, scoring_breakdown, routing_rationale, simulated_only, user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, fmt.Sprintf("dec-%d", i), "grp-1", `{"market_id":"KALSHI:test-1","side":"YES","size":100}`,
			"KALSHI", "KALSHI:test-1", `[]`, `{}`, "Test decision", 1, "")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/decisions?page=1&per_page=2", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["count"])
	assert.Equal(t, float64(3), resp["total_count"])
}

// --- Config tests ---

func TestGetConfig(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "0.40", resp["weight_price_quality"])
	assert.Equal(t, "0.35", resp["weight_liquidity"])
}

func TestPutConfig_Success(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	body, _ := json.Marshal(map[string]string{
		"weight_price_quality": "0.50",
		"weight_liquidity":     "0.25",
	})

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "0.50", resp["weight_price_quality"])
	assert.Equal(t, "0.25", resp["weight_liquidity"])
}

func TestPutConfig_WeightsSumValidation(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	body, _ := json.Marshal(map[string]string{
		"weight_price_quality": "0.90",
	})

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutConfig_ViewerForbidden(t *testing.T) {
	srv, _ := setupTestServer(t)
	_, _ = srv.auth.CreateUser("viewer@example.com", "pass", "viewer")
	session, _ := srv.auth.Login("viewer@example.com", "pass")

	body, _ := json.Marshal(map[string]string{"weight_price_quality": "0.50"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "equinox_session", Value: session.ID})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- History tests ---

func TestGetGroupHistory_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	seedTestUser(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/groups/nonexistent-group/history", nil)
	addAuthCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
