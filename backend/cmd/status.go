package cmd

import (
	"context"
	"fmt"
	"time"

	"equinox/adapters/kalshi"
	"equinox/adapters/polymarket"
	"equinox/store"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check health of venue adapters and ingestion stats",
	Long:  "Reports adapter health status and market ingestion statistics.",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("=== Equinox Status ===")
	fmt.Println()

	// Health checks
	fmt.Println("Venue Health:")
	kalshiAdapter := kalshi.New(cfg.KalshiBaseURL, cfg.KalshiAPIKey)
	polyAdapter := polymarket.New(cfg.PolymarketGammaURL, cfg.PolymarketCLOBURL)

	kalshiErr := kalshiAdapter.HealthCheck(ctx)
	if kalshiErr != nil {
		fmt.Printf("  Kalshi:      UNHEALTHY (%v)\n", kalshiErr)
	} else {
		fmt.Println("  Kalshi:      HEALTHY")
	}

	polyErr := polyAdapter.HealthCheck(ctx)
	if polyErr != nil {
		fmt.Printf("  Polymarket:  UNHEALTHY (%v)\n", polyErr)
	} else {
		fmt.Println("  Polymarket:  HEALTHY")
	}

	fmt.Println()

	// Database stats
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		fmt.Printf("Database: UNAVAILABLE (%v)\n", err)
		return nil
	}
	defer db.Close()

	fmt.Println("Ingestion Stats:")
	printTableCount(db, "raw_markets", "  Raw Markets")
	printTableCount(db, "canonical_markets", "  Canonical Markets")
	printTableCount(db, "equivalence_groups", "  Equivalence Groups")
	printTableCount(db, "routing_decisions", "  Routing Decisions")
	printTableCount(db, "embedding_cache", "  Cached Embeddings")

	fmt.Println()
	printVenueBreakdown(db)
	printLatestIngest(db)

	return nil
}

func printTableCount(db *store.DB, table, label string) {
	var count int
	err := db.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	if err != nil {
		fmt.Printf("%s: error (%v)\n", label, err)
		return
	}
	fmt.Printf("%s: %d\n", label, count)
}

func printVenueBreakdown(db *store.DB) {
	rows, err := db.Conn().Query("SELECT venue, COUNT(*) FROM canonical_markets GROUP BY venue")
	if err != nil {
		return
	}
	defer rows.Close()

	fmt.Println("Markets by Venue:")
	for rows.Next() {
		var venue string
		var count int
		rows.Scan(&venue, &count)
		fmt.Printf("  %s: %d\n", venue, count)
	}
	fmt.Println()
}

func printLatestIngest(db *store.DB) {
	var latest *string
	err := db.Conn().QueryRow("SELECT MAX(ingested_at) FROM raw_markets").Scan(&latest)
	if err != nil || latest == nil {
		fmt.Println("Last Ingest: never")
		return
	}
	fmt.Printf("Last Ingest: %s\n", *latest)
}
