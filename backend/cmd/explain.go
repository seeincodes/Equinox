package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"equinox/models"
	"equinox/store"

	"github.com/spf13/cobra"
)

var explainGroupID string

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Show human-readable breakdown of an equivalence group",
	Long:  "Displays detailed information about an equivalence group, its members, confidence scores, and flags.",
	RunE:  runExplain,
}

func init() {
	explainCmd.Flags().StringVar(&explainGroupID, "group", "", "Group ID to explain (required)")
	explainCmd.MarkFlagRequired("group")
}

func runExplain(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	group, err := loadGroup(db, explainGroupID)
	if err != nil {
		return fmt.Errorf("load group: %w", err)
	}

	printGroupExplanation(group)
	return nil
}

func loadGroup(db *store.DB, groupID string) (*models.EquivalenceGroup, error) {
	var memberIDsJSON, method, rationale, flagsJSON string
	var confidence float64
	var embSim, strSim *float64
	var resDeltaSecs *int64

	err := db.Conn().QueryRow(`
		SELECT group_id, member_ids, confidence_score, match_method,
		       embedding_similarity, string_similarity, resolution_delta_seconds,
		       match_rationale, flags
		FROM equivalence_groups WHERE group_id = ?
	`, groupID).Scan(
		&groupID, &memberIDsJSON, &confidence, &method,
		&embSim, &strSim, &resDeltaSecs, &rationale, &flagsJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("group %s not found: %w", groupID, err)
	}

	var memberIDs []string
	json.Unmarshal([]byte(memberIDsJSON), &memberIDs)

	var flags []models.MatchFlag
	json.Unmarshal([]byte(flagsJSON), &flags)

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

	var resDelta *time.Duration
	if resDeltaSecs != nil {
		d := time.Duration(*resDeltaSecs) * time.Second
		resDelta = &d
	}

	return &models.EquivalenceGroup{
		GroupID:             groupID,
		Members:            members,
		ConfidenceScore:     confidence,
		MatchMethod:        models.MatchMethod(method),
		EmbeddingSimilarity: embSim,
		StringSimilarity:    strSim,
		ResolutionDelta:     resDelta,
		MatchRationale:      rationale,
		Flags:               flags,
	}, nil
}

func printGroupExplanation(g *models.EquivalenceGroup) {
	fmt.Printf("=== Equivalence Group: %s ===\n\n", g.GroupID)
	fmt.Printf("Confidence:  %.4f\n", g.ConfidenceScore)
	fmt.Printf("Method:      %s\n", g.MatchMethod)

	if g.EmbeddingSimilarity != nil {
		fmt.Printf("Embedding:   %.4f\n", *g.EmbeddingSimilarity)
	}
	if g.StringSimilarity != nil {
		fmt.Printf("String Sim:  %.4f\n", *g.StringSimilarity)
	}
	if g.ResolutionDelta != nil {
		fmt.Printf("Res. Delta:  %s\n", g.ResolutionDelta)
	}
	fmt.Println()

	if len(g.Flags) > 0 {
		flagStrs := make([]string, len(g.Flags))
		for i, f := range g.Flags {
			flagStrs[i] = string(f)
		}
		fmt.Printf("Flags: %s\n\n", strings.Join(flagStrs, ", "))
	}

	fmt.Printf("Rationale: %s\n\n", g.MatchRationale)

	fmt.Println("Members:")
	for _, m := range g.Members {
		fmt.Printf("  [%s] %s\n", m.Venue, m.ID)
		fmt.Printf("    Title:      %s\n", m.Title)
		fmt.Printf("    YesPrice:   %.4f\n", m.YesPrice)
		fmt.Printf("    Spread:     %.4f\n", m.Spread)
		fmt.Printf("    Liquidity:  $%.2f\n", m.Liquidity)
		fmt.Printf("    Status:     %s\n", m.Status)
		fmt.Printf("    Settlement: %s\n", m.SettlementMechanism)
		if m.ResolutionTime != nil {
			fmt.Printf("    Resolves:   %s\n", m.ResolutionTime.Format(time.RFC3339))
		}
		fmt.Println()
	}
}
