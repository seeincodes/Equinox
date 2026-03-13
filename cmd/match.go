package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"equinox/equivalence"
	"equinox/models"
	"equinox/store"

	"github.com/spf13/cobra"
)

var matchDryRun bool

var matchCmd = &cobra.Command{
	Use:   "match",
	Short: "Detect equivalent markets across venues",
	Long:  "Runs equivalence detection on canonical markets using rule-based and embedding-based matching.",
	RunE:  runMatch,
}

func init() {
	matchCmd.Flags().BoolVar(&matchDryRun, "dry-run", false, "Preview matches without persisting to database")
}

func runMatch(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	canonicals, err := loadCanonicalMarkets(db)
	if err != nil {
		return fmt.Errorf("load canonical markets: %w", err)
	}

	if len(canonicals) == 0 {
		fmt.Println("No canonical markets found. Run 'equinox normalize' first.")
		return nil
	}

	equivCfg := equivalence.Config{
		EmbeddingSimilarityHigh: cfg.EmbeddingSimilarityHigh,
		EmbeddingSimilarityLow:  cfg.EmbeddingSimilarityLow,
		JaccardThreshold:        cfg.JaccardThreshold,
		LevenshteinThreshold:    cfg.LevenshteinThreshold,
		ResolutionWindowHours:   cfg.ResolutionWindowHours,
	}

	var embedder *equivalence.EmbeddingClient
	if cfg.OpenAIAPIKey != "" {
		embedder = equivalence.NewEmbeddingClient(cfg.OpenAIAPIKey, db.Conn())
	}

	engine := equivalence.NewEngine(equivCfg, embedder)
	groups, err := engine.DetectGroups(ctx, canonicals)
	if err != nil {
		return fmt.Errorf("detect groups: %w", err)
	}

	if matchDryRun {
		fmt.Printf("Dry run: found %d equivalence groups\n", len(groups))
		for _, g := range groups {
			fmt.Printf("  Group %s (confidence=%.3f, method=%s)\n", g.GroupID[:12], g.ConfidenceScore, g.MatchMethod)
			for _, m := range g.Members {
				fmt.Printf("    - %s: %s\n", m.ID, m.Title)
			}
			if len(g.Flags) > 0 {
				fmt.Printf("    Flags: %v\n", g.Flags)
			}
		}
		return nil
	}

	persisted, err := persistEquivalenceGroups(db, groups)
	if err != nil {
		return fmt.Errorf("persist groups: %w", err)
	}

	fmt.Printf("Matched %d equivalence groups (%d persisted)\n", len(groups), persisted)
	return nil
}

func loadCanonicalMarkets(db *store.DB) ([]models.CanonicalMarket, error) {
	rows, err := db.Conn().Query(`
		SELECT id, venue, title, normalized_title, description, outcomes,
		       resolution_time, resolution_time_utc, yes_price, no_price, spread,
		       liquidity, volume_24h, status, contract_type, settlement_mechanism,
		       settlement_note, rules_hash, data_staleness_flag, ingested_at, raw_payload
		FROM canonical_markets
	`)
	if err != nil {
		return nil, fmt.Errorf("query canonical markets: %w", err)
	}
	defer rows.Close()

	var markets []models.CanonicalMarket
	for rows.Next() {
		var cm models.CanonicalMarket
		var outcomesJSON, rawJSON string
		var resTime, resTimeUTC *string
		var venue, status, contractType, settlement string
		var staleness int
		var ingestedAtStr string

		if err := rows.Scan(
			&cm.ID, &venue, &cm.Title, &cm.NormalizedTitle, &cm.Description,
			&outcomesJSON, &resTime, &resTimeUTC,
			&cm.YesPrice, &cm.NoPrice, &cm.Spread, &cm.Liquidity, &cm.Volume24h,
			&status, &contractType, &settlement,
			&cm.SettlementNote, &cm.RulesHash, &staleness, &ingestedAtStr, &rawJSON,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		cm.Venue = models.Venue(venue)
		cm.Status = models.MarketStatus(status)
		cm.ContractType = models.ContractType(contractType)
		cm.SettlementMechanism = models.SettlementType(settlement)
		cm.DataStalenessFlag = staleness != 0

		if resTime != nil {
			if t, err := time.Parse(time.RFC3339, *resTime); err == nil {
				cm.ResolutionTime = &t
			}
		}
		if resTimeUTC != nil {
			if t, err := time.Parse(time.RFC3339, *resTimeUTC); err == nil {
				cm.ResolutionTimeUTC = &t
			}
		}

		cm.IngestedAt, _ = time.Parse(time.RFC3339, ingestedAtStr)
		json.Unmarshal([]byte(outcomesJSON), &cm.Outcomes)
		cm.RawPayload = json.RawMessage(rawJSON)

		markets = append(markets, cm)
	}

	return markets, rows.Err()
}

func persistEquivalenceGroups(db *store.DB, groups []models.EquivalenceGroup) (int, error) {
	tx, err := db.Conn().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO equivalence_groups
		(group_id, member_ids, confidence_score, match_method,
		 embedding_similarity, string_similarity, resolution_delta_seconds,
		 match_rationale, flags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	persisted := 0
	for _, g := range groups {
		memberIDs := make([]string, len(g.Members))
		for i, m := range g.Members {
			memberIDs[i] = m.ID
		}
		memberIDsJSON, _ := json.Marshal(memberIDs)
		flagsJSON, _ := json.Marshal(g.Flags)

		var resDeltaSecs *int64
		if g.ResolutionDelta != nil {
			secs := int64(g.ResolutionDelta.Seconds())
			resDeltaSecs = &secs
		}

		if _, err := stmt.Exec(
			g.GroupID, string(memberIDsJSON), g.ConfidenceScore, string(g.MatchMethod),
			g.EmbeddingSimilarity, g.StringSimilarity, resDeltaSecs,
			g.MatchRationale, string(flagsJSON),
		); err != nil {
			slog.Warn("insert equivalence group failed", "group_id", g.GroupID, "error", err)
			continue
		}
		persisted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return persisted, nil
}
