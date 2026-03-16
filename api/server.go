package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"equinox/adapters/kalshi"
	"equinox/adapters/polymarket"
	"equinox/auth"
	"equinox/config"
	"equinox/models"
	"equinox/routing"
	"equinox/store"
)

// Server implements the REST API.
type Server struct {
	db          *store.DB
	cfg         *config.Config
	auth        *auth.Auth
	mux         *http.ServeMux
	server      *http.Server
	configCache map[string]string
	configMu    sync.RWMutex
}

func NewServer(db *store.DB, cfg *config.Config, addr string) *Server {
	s := &Server{
		db:          db,
		cfg:         cfg,
		auth:        auth.New(db),
		mux:         http.NewServeMux(),
		configCache: make(map[string]string),
	}
	s.server = &http.Server{Addr: addr, Handler: s.mux}
	s.loadConfigCache()
	s.routes()
	return s
}

func (s *Server) routes() {
	// Public — no auth
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/health", s.handleGetHealth)

	// Authenticated — any role
	s.mux.Handle("POST /api/auth/logout", s.auth.RequireAuth(http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("GET /api/auth/me", s.auth.RequireAuth(http.HandlerFunc(s.handleMe)))
	s.mux.Handle("GET /api/markets", s.auth.RequireAuth(http.HandlerFunc(s.handleGetMarkets)))
	s.mux.Handle("GET /api/groups", s.auth.RequireAuth(http.HandlerFunc(s.handleGetGroups)))
	s.mux.Handle("GET /api/groups/{id}/history", s.auth.RequireAuth(http.HandlerFunc(s.handleGetGroupHistory)))
	s.mux.Handle("GET /api/decisions", s.auth.RequireAuth(http.HandlerFunc(s.handleGetDecisions)))

	// Analyst+ only
	s.mux.Handle("POST /api/route", s.auth.RequireAuth(s.auth.RequireRole("analyst", http.HandlerFunc(s.handlePostRoute))))
	s.mux.Handle("POST /api/route/batch", s.auth.RequireAuth(s.auth.RequireRole("analyst", http.HandlerFunc(s.handlePostRouteBatch))))

	// Admin only
	s.mux.Handle("GET /api/config", s.auth.RequireAuth(s.auth.RequireRole("admin", http.HandlerFunc(s.handleGetConfig))))
	s.mux.Handle("PUT /api/config", s.auth.RequireAuth(s.auth.RequireRole("admin", http.HandlerFunc(s.handlePutConfig))))
}

func (s *Server) ListenAndServe() error {
	slog.Info("API server starting", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Handler returns the underlying http.Handler for testing.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// --- Auth handlers ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	session, err := s.auth.Login(req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(auth.CookieName)
	if cookie != nil {
		s.auth.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, user)
}

// --- Config handlers ---

var configDefaults = map[string]string{
	"weight_price_quality":         "0.40",
	"weight_liquidity":             "0.35",
	"weight_spread_quality":        "0.15",
	"weight_market_status":         "0.10",
	"staleness_liquidity_haircut":  "0.20",
	"threshold_high_confidence":    "0.92",
	"threshold_medium_confidence":  "0.78",
}

func (s *Server) loadConfigCache() {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	// Start with defaults
	for k, v := range configDefaults {
		s.configCache[k] = v
	}

	// Override with DB values
	rows, err := s.db.Conn().Query("SELECT key, value FROM config")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			s.configCache[k] = v
		}
	}
}

func (s *Server) getConfigValue(key string) float64 {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	if v, ok := s.configCache[key]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.configMu.RLock()
	result := make(map[string]string, len(s.configCache))
	for k, v := range s.configCache {
		result[k] = v
	}
	s.configMu.RUnlock()
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Merge with current config for validation
	s.configMu.RLock()
	merged := make(map[string]string, len(s.configCache))
	for k, v := range s.configCache {
		merged[k] = v
	}
	s.configMu.RUnlock()
	for k, v := range updates {
		merged[k] = v
	}

	// Validate weights sum to 1.0
	weightKeys := []string{"weight_price_quality", "weight_liquidity", "weight_spread_quality", "weight_market_status"}
	var sum float64
	for _, k := range weightKeys {
		v, err := strconv.ParseFloat(merged[k], 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid value for %s", k))
			return
		}
		sum += v
	}
	if math.Abs(sum-1.0) > 0.001 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("routing weights must sum to 1.0 (got %.3f)", sum))
		return
	}

	// Validate HIGH > MEDIUM thresholds
	high, _ := strconv.ParseFloat(merged["threshold_high_confidence"], 64)
	med, _ := strconv.ParseFloat(merged["threshold_medium_confidence"], 64)
	if high <= med {
		writeError(w, http.StatusBadRequest, "threshold_high_confidence must be greater than threshold_medium_confidence")
		return
	}

	// Persist to DB
	user := auth.UserFromContext(r.Context())
	for k, v := range updates {
		_, err := s.db.Conn().Exec(`
			INSERT INTO config (key, value, updated_by, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = CURRENT_TIMESTAMP
		`, k, v, user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
			return
		}
	}

	// Invalidate cache
	s.loadConfigCache()

	slog.Info("config updated", "user", user.Email, "keys", len(updates))

	s.configMu.RLock()
	result := make(map[string]string, len(s.configCache))
	for k, v := range s.configCache {
		result[k] = v
	}
	s.configMu.RUnlock()
	writeJSON(w, http.StatusOK, result)
}

// --- Market handlers ---

// GET /api/markets?venue=&status=
func (s *Server) handleGetMarkets(w http.ResponseWriter, r *http.Request) {
	venueFilter := r.URL.Query().Get("venue")
	statusFilter := r.URL.Query().Get("status")

	query := "SELECT id, venue, title, normalized_title, yes_price, no_price, spread, liquidity, status, contract_type, settlement_mechanism FROM canonical_markets WHERE 1=1"
	var args []interface{}

	if venueFilter != "" {
		query += " AND venue = ?"
		args = append(args, venueFilter)
	}
	if statusFilter != "" {
		query += " AND status = ?"
		args = append(args, statusFilter)
	}
	query += " ORDER BY venue, title"

	rows, err := s.db.Conn().Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}
	defer rows.Close()

	type marketSummary struct {
		ID                  string  `json:"id"`
		Venue               string  `json:"venue"`
		Title               string  `json:"title"`
		NormalizedTitle     string  `json:"normalized_title"`
		YesPrice            float64 `json:"yes_price"`
		NoPrice             float64 `json:"no_price"`
		Spread              float64 `json:"spread"`
		Liquidity           float64 `json:"liquidity"`
		Status              string  `json:"status"`
		ContractType        string  `json:"contract_type"`
		SettlementMechanism string  `json:"settlement_mechanism"`
	}

	var markets []marketSummary
	for rows.Next() {
		var m marketSummary
		if err := rows.Scan(&m.ID, &m.Venue, &m.Title, &m.NormalizedTitle, &m.YesPrice, &m.NoPrice, &m.Spread, &m.Liquidity, &m.Status, &m.ContractType, &m.SettlementMechanism); err != nil {
			continue
		}
		markets = append(markets, m)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"markets": markets,
		"count":   len(markets),
	})
}

// --- Group handlers ---

type memberMarket struct {
	ID                  string  `json:"id"`
	Venue               string  `json:"venue"`
	Title               string  `json:"title"`
	YesPrice            float64 `json:"yes_price"`
	NoPrice             float64 `json:"no_price"`
	Spread              float64 `json:"spread"`
	Liquidity           float64 `json:"liquidity"`
	Status              string  `json:"status"`
	ContractType        string  `json:"contract_type"`
	SettlementMechanism string  `json:"settlement_mechanism"`
	DataStalenessFlag   bool    `json:"data_staleness_flag"`
}

// GET /api/groups?min_confidence=
func (s *Server) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	minConfidence := 0.0
	if mc := r.URL.Query().Get("min_confidence"); mc != "" {
		if v, err := strconv.ParseFloat(mc, 64); err == nil {
			minConfidence = v
		}
	}

	rows, err := s.db.Conn().Query(`
		SELECT group_id, member_ids, confidence_score, match_method,
		       embedding_similarity, string_similarity, match_rationale, flags
		FROM equivalence_groups
		WHERE confidence_score >= ?
		ORDER BY confidence_score DESC
	`, minConfidence)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}
	defer rows.Close()

	type groupResponse struct {
		GroupID             string          `json:"group_id"`
		MemberIDs           json.RawMessage `json:"member_ids"`
		Members             []memberMarket  `json:"members"`
		ConfidenceScore     float64         `json:"confidence_score"`
		MatchMethod         string          `json:"match_method"`
		EmbeddingSimilarity *float64        `json:"embedding_similarity"`
		StringSimilarity    *float64        `json:"string_similarity"`
		MatchRationale      string          `json:"match_rationale"`
		Flags               json.RawMessage `json:"flags"`
	}

	var groups []groupResponse
	for rows.Next() {
		var g groupResponse
		var memberIDsStr, flagsStr string
		if err := rows.Scan(&g.GroupID, &memberIDsStr, &g.ConfidenceScore, &g.MatchMethod,
			&g.EmbeddingSimilarity, &g.StringSimilarity, &g.MatchRationale, &flagsStr); err != nil {
			continue
		}
		g.MemberIDs = json.RawMessage(memberIDsStr)
		g.Flags = json.RawMessage(flagsStr)

		// Embed member market data
		var memberIDs []string
		json.Unmarshal([]byte(memberIDsStr), &memberIDs)
		g.Members = s.loadMemberSummaries(memberIDs)

		groups = append(groups, g)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"groups": groups,
		"count":  len(groups),
	})
}

func (s *Server) loadMemberSummaries(ids []string) []memberMarket {
	var members []memberMarket
	for _, id := range ids {
		var m memberMarket
		err := s.db.Conn().QueryRow(`
			SELECT id, venue, title, yes_price, no_price, spread, liquidity, status,
			       contract_type, settlement_mechanism,
			       CASE WHEN data_staleness_flag THEN 1 ELSE 0 END
			FROM canonical_markets WHERE id = ?
		`, id).Scan(&m.ID, &m.Venue, &m.Title, &m.YesPrice, &m.NoPrice, &m.Spread,
			&m.Liquidity, &m.Status, &m.ContractType, &m.SettlementMechanism, &m.DataStalenessFlag)
		if err != nil {
			continue
		}
		members = append(members, m)
	}
	return members
}

// GET /api/groups/{id}/history
func (s *Server) handleGetGroupHistory(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("id")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "group id required")
		return
	}

	// Verify group exists
	var memberIDsJSON string
	err := s.db.Conn().QueryRow(
		"SELECT member_ids FROM equivalence_groups WHERE group_id = ?", groupID,
	).Scan(&memberIDsJSON)
	if err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}

	var memberIDs []string
	if err := json.Unmarshal([]byte(memberIDsJSON), &memberIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse member ids")
		return
	}

	if len(memberIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"group_id": groupID, "history": []interface{}{}, "count": 0,
		})
		return
	}

	// Query snapshots for member markets
	placeholders := strings.Repeat("?,", len(memberIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(memberIDs))
	for i, id := range memberIDs {
		args[i] = id
	}

	rows, err := s.db.Conn().Query(fmt.Sprintf(`
		SELECT market_id, venue, yes_price, no_price, spread, liquidity, snapshot_at
		FROM canonical_market_snapshots
		WHERE market_id IN (%s)
		ORDER BY snapshot_at DESC
		LIMIT 1000
	`, placeholders), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}
	defer rows.Close()

	type snapshot struct {
		Timestamp string  `json:"timestamp"`
		Venue     string  `json:"venue"`
		YesPrice  float64 `json:"yes_price"`
		NoPrice   float64 `json:"no_price"`
		Spread    float64 `json:"spread"`
		Liquidity float64 `json:"liquidity"`
	}

	var history []snapshot
	for rows.Next() {
		var sn snapshot
		var marketID string
		var ts time.Time
		if err := rows.Scan(&marketID, &sn.Venue, &sn.YesPrice, &sn.NoPrice, &sn.Spread, &sn.Liquidity, &ts); err != nil {
			continue
		}
		sn.Timestamp = ts.Format(time.RFC3339)
		history = append(history, sn)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"group_id": groupID,
		"history":  history,
		"count":    len(history),
	})
}

// --- Route handlers ---

// POST /api/route
func (s *Server) handlePostRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MarketID string `json:"market_id"`
		GroupID  string `json:"group_id"`
		Side     string `json:"side"`
		Size     int    `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Side == "" || req.Size <= 0 {
		writeError(w, http.StatusBadRequest, "side and size (>0) are required")
		return
	}

	// Resolve group: either by market_id or group_id
	var group *models.EquivalenceGroup
	var err error

	if req.GroupID != "" {
		group, err = s.findGroupByID(req.GroupID)
	} else if req.MarketID != "" {
		group, err = s.findGroupForMarket(req.MarketID)
	} else {
		writeError(w, http.StatusBadRequest, "market_id or group_id is required")
		return
	}

	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	order := models.OrderRequest{
		MarketID: req.MarketID,
		Side:     req.Side,
		Size:     req.Size,
	}
	if order.MarketID == "" && len(group.Members) > 0 {
		order.MarketID = group.Members[0].ID
	}

	engine := routing.NewEngine(routing.Config{
		WeightPriceQuality:        s.getConfigValue("weight_price_quality"),
		WeightLiquidity:           s.getConfigValue("weight_liquidity"),
		WeightSpreadQuality:       s.getConfigValue("weight_spread_quality"),
		WeightMarketStatus:        s.getConfigValue("weight_market_status"),
		StalenessLiquidityHaircut: s.getConfigValue("staleness_liquidity_haircut"),
	})

	decision, err := engine.Route(order, *group)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "routing failed: "+err.Error())
		return
	}

	// Persist decision with user tracking
	user := auth.UserFromContext(r.Context())
	if err := s.persistDecision(*decision, user.ID); err != nil {
		slog.Warn("failed to persist routing decision", "decision_id", decision.DecisionID, "error", err)
	}

	writeJSON(w, http.StatusOK, decision)
}

// POST /api/route/batch
func (s *Server) handlePostRouteBatch(w http.ResponseWriter, r *http.Request) {
	var orders []models.OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&orders); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if len(orders) == 0 {
		writeError(w, http.StatusBadRequest, "orders array cannot be empty")
		return
	}

	engine := routing.NewEngine(routing.Config{
		WeightPriceQuality:        s.getConfigValue("weight_price_quality"),
		WeightLiquidity:           s.getConfigValue("weight_liquidity"),
		WeightSpreadQuality:       s.getConfigValue("weight_spread_quality"),
		WeightMarketStatus:        s.getConfigValue("weight_market_status"),
		StalenessLiquidityHaircut: s.getConfigValue("staleness_liquidity_haircut"),
	})

	user := auth.UserFromContext(r.Context())
	var decisions []models.RoutingDecision
	var errors []map[string]interface{}

	for i, order := range orders {
		if order.MarketID == "" || order.Side == "" || order.Size <= 0 {
			errors = append(errors, map[string]interface{}{
				"index":   i,
				"error":   "market_id, side, and size (>0) are required",
				"request": order,
			})
			continue
		}

		group, err := s.findGroupForMarket(order.MarketID)
		if err != nil {
			errors = append(errors, map[string]interface{}{
				"index": i,
				"error": err.Error(),
			})
			continue
		}

		decision, err := engine.Route(order, *group)
		if err != nil {
			errors = append(errors, map[string]interface{}{
				"index": i,
				"error": "routing failed: " + err.Error(),
			})
			continue
		}

		if err := s.persistDecision(*decision, user.ID); err != nil {
			slog.Warn("failed to persist routing decision", "decision_id", decision.DecisionID, "error", err)
		}

		decisions = append(decisions, *decision)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"decisions":        decisions,
		"successful_count": len(decisions),
		"errors":           errors,
		"error_count":      len(errors),
	})
}

// --- Decisions handler ---

// GET /api/decisions?venue=&after=&page=&per_page=&user=
func (s *Server) handleGetDecisions(w http.ResponseWriter, r *http.Request) {
	venueFilter := r.URL.Query().Get("venue")
	afterStr := r.URL.Query().Get("after")
	userFilter := r.URL.Query().Get("user")

	page := 1
	perPage := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}

	where := "WHERE 1=1"
	var args []interface{}

	if venueFilter != "" {
		where += " AND selected_venue = ?"
		args = append(args, venueFilter)
	}
	if userFilter != "" {
		where += " AND user_id = (SELECT id FROM users WHERE email = ?)"
		args = append(args, userFilter)
	}
	if afterStr != "" {
		afterTime, err := time.Parse(time.RFC3339, afterStr)
		if err != nil {
			afterTime, err = time.Parse("2006-01-02", afterStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid 'after' date format: use ISO 8601")
				return
			}
		}
		where += " AND created_at >= ?"
		args = append(args, afterTime)
	}

	// Get total count
	var totalCount int
	countQuery := "SELECT COUNT(*) FROM routing_decisions " + where
	s.db.Conn().QueryRow(countQuery, args...).Scan(&totalCount)

	// Get page
	offset := (page - 1) * perPage
	query := fmt.Sprintf(`
		SELECT decision_id, group_id, order_request, selected_venue, selected_market_id,
		       rejected_alternatives, scoring_breakdown, routing_rationale, cache_mode, created_at, user_id
		FROM routing_decisions
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, where)
	args = append(args, perPage, offset)

	rows, err := s.db.Conn().Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}
	defer rows.Close()

	type decisionSummary struct {
		DecisionID           string          `json:"decision_id"`
		GroupID              string          `json:"group_id"`
		OrderRequest         json.RawMessage `json:"order_request"`
		SelectedVenue        string          `json:"selected_venue"`
		SelectedMarketID     string          `json:"selected_market_id"`
		RejectedAlternatives json.RawMessage `json:"rejected_alternatives"`
		ScoringBreakdown     json.RawMessage `json:"scoring_breakdown"`
		RoutingRationale     string          `json:"routing_rationale"`
		CacheMode            bool            `json:"cache_mode"`
		CreatedAt            string          `json:"created_at"`
		UserID               string          `json:"user_id"`
	}

	var decisions []decisionSummary
	for rows.Next() {
		var d decisionSummary
		var orderReqJSON, rejectedJSON, scoringJSON string

		if err := rows.Scan(&d.DecisionID, &d.GroupID, &orderReqJSON, &d.SelectedVenue,
			&d.SelectedMarketID, &rejectedJSON, &scoringJSON, &d.RoutingRationale,
			&d.CacheMode, &d.CreatedAt, &d.UserID); err != nil {
			continue
		}

		d.OrderRequest = json.RawMessage(orderReqJSON)
		d.RejectedAlternatives = json.RawMessage(rejectedJSON)
		d.ScoringBreakdown = json.RawMessage(scoringJSON)
		decisions = append(decisions, d)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"decisions":   decisions,
		"count":       len(decisions),
		"total_count": totalCount,
		"page":        page,
		"per_page":    perPage,
	})
}

// --- Health handler ---

// GET /api/health
func (s *Server) handleGetHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	kalshiAdapter := kalshi.New(s.cfg.KalshiBaseURL, s.cfg.KalshiAPIKey)
	polyAdapter := polymarket.New(s.cfg.PolymarketGammaURL, s.cfg.PolymarketCLOBURL)

	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	venues := map[string]string{}

	if err := kalshiAdapter.HealthCheck(ctx); err != nil {
		venues["kalshi"] = fmt.Sprintf("unhealthy: %v", err)
		health["status"] = "degraded"
	} else {
		venues["kalshi"] = "healthy"
	}

	if err := polyAdapter.HealthCheck(ctx); err != nil {
		venues["polymarket"] = fmt.Sprintf("unhealthy: %v", err)
		health["status"] = "degraded"
	} else {
		venues["polymarket"] = "healthy"
	}

	health["venues"] = venues

	var rawCount, canonicalCount, groupCount int
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM raw_markets").Scan(&rawCount)
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM canonical_markets").Scan(&canonicalCount)
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM equivalence_groups").Scan(&groupCount)

	health["counts"] = map[string]int{
		"raw_markets":        rawCount,
		"canonical_markets":  canonicalCount,
		"equivalence_groups": groupCount,
	}

	status := http.StatusOK
	if health["status"] == "degraded" {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, health)
}

// --- Helpers ---

func (s *Server) findGroupByID(groupID string) (*models.EquivalenceGroup, error) {
	var memberIDsJSON, method, rationale, flagsJSON string
	var confidence float64

	err := s.db.Conn().QueryRow(`
		SELECT group_id, member_ids, confidence_score, match_method, match_rationale, flags
		FROM equivalence_groups WHERE group_id = ?
	`, groupID).Scan(&groupID, &memberIDsJSON, &confidence, &method, &rationale, &flagsJSON)
	if err != nil {
		return nil, fmt.Errorf("group not found: %s", groupID)
	}

	var memberIDs []string
	json.Unmarshal([]byte(memberIDsJSON), &memberIDs)

	members, _ := s.loadMembers(memberIDs)
	var flags []models.MatchFlag
	json.Unmarshal([]byte(flagsJSON), &flags)

	return &models.EquivalenceGroup{
		GroupID:         groupID,
		Members:         members,
		ConfidenceScore: confidence,
		MatchMethod:     models.MatchMethod(method),
		MatchRationale:  rationale,
		Flags:           flags,
	}, nil
}

func (s *Server) findGroupForMarket(marketID string) (*models.EquivalenceGroup, error) {
	type groupRow struct {
		groupID, method, rationale, flagsJSON string
		memberIDs                             []string
		confidence                            float64
	}

	rows, err := s.db.Conn().Query(`
		SELECT group_id, member_ids, confidence_score, match_method, match_rationale, flags
		FROM equivalence_groups
	`)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}

	var match *groupRow
	for rows.Next() {
		var gr groupRow
		var memberIDsJSON string
		if err := rows.Scan(&gr.groupID, &memberIDsJSON, &gr.confidence, &gr.method, &gr.rationale, &gr.flagsJSON); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(memberIDsJSON), &gr.memberIDs); err != nil {
			continue
		}
		for _, id := range gr.memberIDs {
			if id == marketID {
				match = &gr
				break
			}
		}
		if match != nil {
			break
		}
	}
	rows.Close()

	if match == nil {
		return nil, fmt.Errorf("no equivalence group found containing market %s", marketID)
	}

	members, _ := s.loadMembers(match.memberIDs)
	var flags []models.MatchFlag
	json.Unmarshal([]byte(match.flagsJSON), &flags)

	return &models.EquivalenceGroup{
		GroupID:         match.groupID,
		Members:         members,
		ConfidenceScore: match.confidence,
		MatchMethod:     models.MatchMethod(match.method),
		MatchRationale:  match.rationale,
		Flags:           flags,
	}, nil
}

func (s *Server) loadMembers(ids []string) ([]models.CanonicalMarket, error) {
	var members []models.CanonicalMarket
	for _, id := range ids {
		var cm models.CanonicalMarket
		var venue, status, contractType, settlement, outcomesJSON string

		err := s.db.Conn().QueryRow(`
			SELECT id, venue, title, normalized_title, yes_price, no_price, spread,
			       liquidity, status, contract_type, settlement_mechanism, outcomes, data_staleness_flag
			FROM canonical_markets WHERE id = ?
		`, id).Scan(&cm.ID, &venue, &cm.Title, &cm.NormalizedTitle,
			&cm.YesPrice, &cm.NoPrice, &cm.Spread, &cm.Liquidity,
			&status, &contractType, &settlement, &outcomesJSON, &cm.DataStalenessFlag)
		if err != nil {
			continue
		}

		cm.Venue = models.Venue(venue)
		cm.Status = models.MarketStatus(status)
		cm.ContractType = models.ContractType(contractType)
		cm.SettlementMechanism = models.SettlementType(settlement)
		json.Unmarshal([]byte(outcomesJSON), &cm.Outcomes)

		members = append(members, cm)
	}
	return members, nil
}

func (s *Server) persistDecision(decision models.RoutingDecision, userID string) error {
	orderReqJSON, _ := json.Marshal(decision.OrderRequest)
	rejectedJSON, _ := json.Marshal(decision.RejectedAlternatives)
	scoringJSON, _ := json.Marshal(decision.ScoringBreakdown)

	_, err := s.db.Conn().Exec(`
		INSERT INTO routing_decisions
			(decision_id, group_id, order_request, selected_venue, selected_market_id,
			 rejected_alternatives, scoring_breakdown, routing_rationale, simulated_only, cache_mode, user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		decision.DecisionID, decision.EquivalenceGroup.GroupID, string(orderReqJSON),
		string(decision.SelectedVenue), decision.SelectedMarket.ID,
		string(rejectedJSON), string(scoringJSON), decision.RoutingRationale,
		1, decision.CacheMode, userID)

	return err
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
