package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"equinox/adapters/kalshi"
	"equinox/adapters/polymarket"
	"equinox/config"
	"equinox/models"
	"equinox/routing"
	"equinox/store"
)

// Server implements the optional REST API.
type Server struct {
	db     *store.DB
	cfg    *config.Config
	mux    *http.ServeMux
	server *http.Server
}

func NewServer(db *store.DB, cfg *config.Config, addr string) *Server {
	s := &Server{db: db, cfg: cfg, mux: http.NewServeMux()}
	s.server = &http.Server{Addr: addr, Handler: s.mux}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /markets", s.handleGetMarkets)
	s.mux.HandleFunc("GET /groups", s.handleGetGroups)
	s.mux.HandleFunc("POST /route", s.handlePostRoute)
	s.mux.HandleFunc("GET /health", s.handleGetHealth)
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

// GET /markets?venue=&status=
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

// GET /groups?min_confidence=
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

	type groupSummary struct {
		GroupID             string          `json:"group_id"`
		MemberIDs           json.RawMessage `json:"member_ids"`
		ConfidenceScore     float64         `json:"confidence_score"`
		MatchMethod         string          `json:"match_method"`
		EmbeddingSimilarity *float64        `json:"embedding_similarity"`
		StringSimilarity    *float64        `json:"string_similarity"`
		MatchRationale      string          `json:"match_rationale"`
		Flags               json.RawMessage `json:"flags"`
	}

	var groups []groupSummary
	for rows.Next() {
		var g groupSummary
		var memberIDs, flags string
		if err := rows.Scan(&g.GroupID, &memberIDs, &g.ConfidenceScore, &g.MatchMethod,
			&g.EmbeddingSimilarity, &g.StringSimilarity, &g.MatchRationale, &flags); err != nil {
			continue
		}
		g.MemberIDs = json.RawMessage(memberIDs)
		g.Flags = json.RawMessage(flags)
		groups = append(groups, g)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"groups": groups,
		"count":  len(groups),
	})
}

// POST /route
func (s *Server) handlePostRoute(w http.ResponseWriter, r *http.Request) {
	var order models.OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if order.MarketID == "" || order.Side == "" || order.Size <= 0 {
		writeError(w, http.StatusBadRequest, "market_id, side, and size (>0) are required")
		return
	}

	group, err := s.findGroupForMarket(order.MarketID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	engine := routing.NewEngine(routing.Config{
		WeightPriceQuality:        s.cfg.WeightPriceQuality,
		WeightLiquidity:           s.cfg.WeightLiquidity,
		WeightSpreadQuality:       s.cfg.WeightSpreadQuality,
		WeightMarketStatus:        s.cfg.WeightMarketStatus,
		StalenessLiquidityHaircut: s.cfg.StalenessLiquidityHaircut,
	})

	decision, err := engine.Route(order, *group)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "routing failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, decision)
}

// GET /health
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
		"raw_markets":       rawCount,
		"canonical_markets": canonicalCount,
		"equivalence_groups": groupCount,
	}

	status := http.StatusOK
	if health["status"] == "degraded" {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, health)
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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
