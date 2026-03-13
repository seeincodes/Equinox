package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"equinox/models"
	"equinox/routing"
	"equinox/store"

	"github.com/spf13/cobra"
)

var (
	routeMarket string
	routeSide   string
	routeSize   int
)

var routeCmd = &cobra.Command{
	Use:   "route",
	Short: "Simulate a routing decision for a market",
	Long:  "Finds the equivalence group for a market and simulates a routing decision with scoring breakdown.",
	RunE:  runRoute,
}

func init() {
	routeCmd.Flags().StringVar(&routeMarket, "market", "", "Market ID to route (required)")
	routeCmd.Flags().StringVar(&routeSide, "side", "", "Order side: YES or NO (required)")
	routeCmd.Flags().IntVar(&routeSize, "size", 0, "Number of contracts (required)")
	routeCmd.MarkFlagRequired("market")
	routeCmd.MarkFlagRequired("side")
	routeCmd.MarkFlagRequired("size")
}

func runRoute(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	group, err := findGroupForMarket(db, routeMarket)
	if err != nil {
		return fmt.Errorf("find group: %w", err)
	}

	order := models.OrderRequest{
		MarketID: routeMarket,
		Side:     routeSide,
		Size:     routeSize,
	}

	routingCfg := routing.Config{
		WeightPriceQuality:        cfg.WeightPriceQuality,
		WeightLiquidity:           cfg.WeightLiquidity,
		WeightSpreadQuality:       cfg.WeightSpreadQuality,
		WeightMarketStatus:        cfg.WeightMarketStatus,
		StalenessLiquidityHaircut: cfg.StalenessLiquidityHaircut,
	}

	engine := routing.NewEngine(routingCfg)
	decision, err := engine.Route(order, *group)
	if err != nil {
		return fmt.Errorf("route: %w", err)
	}

	if err := persistRoutingDecision(db, decision); err != nil {
		slog.Warn("failed to persist routing decision", "error", err)
	}

	fmt.Println(decision.RoutingRationale)
	return nil
}

func findGroupForMarket(db *store.DB, marketID string) (*models.EquivalenceGroup, error) {
	rows, err := db.Conn().Query(`
		SELECT group_id, member_ids, confidence_score, match_method,
		       embedding_similarity, string_similarity, resolution_delta_seconds,
		       match_rationale, flags
		FROM equivalence_groups
	`)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var groupID, method, rationale string
		var memberIDsJSON, flagsJSON string
		var confidence float64
		var embSim, strSim *float64
		var resDeltaSecs *int64

		if err := rows.Scan(&groupID, &memberIDsJSON, &confidence, &method,
			&embSim, &strSim, &resDeltaSecs, &rationale, &flagsJSON); err != nil {
			continue
		}

		var memberIDs []string
		json.Unmarshal([]byte(memberIDsJSON), &memberIDs)

		found := false
		for _, id := range memberIDs {
			if id == marketID {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		canonicals, err := loadCanonicalMarkets(db)
		if err != nil {
			return nil, err
		}

		var members []models.CanonicalMarket
		for _, mid := range memberIDs {
			for _, cm := range canonicals {
				if cm.ID == mid {
					members = append(members, cm)
					break
				}
			}
		}

		var flags []models.MatchFlag
		json.Unmarshal([]byte(flagsJSON), &flags)

		return &models.EquivalenceGroup{
			GroupID:             groupID,
			Members:            members,
			ConfidenceScore:     confidence,
			MatchMethod:        models.MatchMethod(method),
			EmbeddingSimilarity: embSim,
			StringSimilarity:    strSim,
			MatchRationale:      rationale,
			Flags:               flags,
		}, nil
	}

	return nil, fmt.Errorf("no equivalence group found containing market %s", marketID)
}

func persistRoutingDecision(db *store.DB, d *models.RoutingDecision) error {
	orderJSON, _ := json.Marshal(d.OrderRequest)
	rejectedJSON, _ := json.Marshal(d.RejectedAlternatives)
	scoringJSON, _ := json.Marshal(d.ScoringBreakdown)

	_, err := db.Conn().Exec(`
		INSERT INTO routing_decisions
		(decision_id, group_id, order_request, selected_venue, selected_market_id,
		 rejected_alternatives, scoring_breakdown, routing_rationale, simulated_only, cache_mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
	`,
		d.DecisionID, d.EquivalenceGroup.GroupID,
		string(orderJSON), string(d.SelectedVenue), d.SelectedMarket.ID,
		string(rejectedJSON), string(scoringJSON), d.RoutingRationale,
		d.CacheMode,
	)
	return err
}
