package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"equinox/adapters"
	"equinox/adapters/kalshi"
	"equinox/adapters/polymarket"
	"equinox/store"

	"github.com/spf13/cobra"
)

var ingestVenue string
var ingestLimit int

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Fetch raw market data from venue APIs",
	Long:  "Fetches market metadata and pricing from Kalshi and/or Polymarket and stores raw responses in SQLite.",
	RunE:  runIngest,
}

func init() {
	ingestCmd.Flags().StringVar(&ingestVenue, "venue", "", "Filter by venue: kalshi or polymarket (default: both)")
	ingestCmd.Flags().IntVar(&ingestLimit, "limit", 0, "Max markets to fetch per venue (0 = unlimited)")
}

func runIngest(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	var adaptersToRun []adapters.VenueAdapter

	venue := strings.ToLower(ingestVenue)
	switch venue {
	case "kalshi":
		ka := kalshi.New(cfg.KalshiBaseURL, cfg.KalshiAPIKey)
		ka.MaxMarkets = ingestLimit
		adaptersToRun = append(adaptersToRun, ka)
	case "polymarket":
		pa := polymarket.New(cfg.PolymarketGammaURL, cfg.PolymarketCLOBURL)
		pa.MaxMarkets = ingestLimit
		adaptersToRun = append(adaptersToRun, pa)
	case "":
		ka := kalshi.New(cfg.KalshiBaseURL, cfg.KalshiAPIKey)
		ka.MaxMarkets = ingestLimit
		pa := polymarket.New(cfg.PolymarketGammaURL, cfg.PolymarketCLOBURL)
		pa.MaxMarkets = ingestLimit
		adaptersToRun = append(adaptersToRun, ka, pa)
	default:
		return fmt.Errorf("unknown venue %q: use 'kalshi' or 'polymarket'", ingestVenue)
	}

	type result struct {
		venue   string
		markets []adapters.RawMarket
		err     error
	}

	results := make(chan result, len(adaptersToRun))
	for _, adapter := range adaptersToRun {
		go func(a adapters.VenueAdapter) {
			markets, err := a.FetchMarkets(ctx)
			results <- result{venue: string(a.VenueID()), markets: markets, err: err}
		}(adapter)
	}

	var totalInserted int
	var failures []string

	for i := 0; i < len(adaptersToRun); i++ {
		r := <-results
		if r.err != nil {
			slog.Error("ingest failed",
				"venue", r.venue,
				"error", r.err,
			)
			failures = append(failures, r.venue)
			continue
		}

		inserted, err := persistRawMarkets(db, r.markets)
		if err != nil {
			slog.Error("persist failed",
				"venue", r.venue,
				"error", err,
			)
			failures = append(failures, r.venue)
			continue
		}

		totalInserted += inserted
		slog.Info("ingest complete",
			"venue", r.venue,
			"markets_fetched", len(r.markets),
			"markets_inserted", inserted,
		)
	}

	if len(failures) == len(adaptersToRun) {
		return fmt.Errorf("all venue ingests failed: %s", strings.Join(failures, ", "))
	}

	if len(failures) > 0 {
		slog.Warn("partial ingest failure",
			"failed_venues", failures,
			"successful_inserts", totalInserted,
		)
	}

	fmt.Printf("Ingested %d markets (%d venue failures)\n", totalInserted, len(failures))
	return nil
}

func persistRawMarkets(db *store.DB, markets []adapters.RawMarket) (int, error) {
	tx, err := db.Conn().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO raw_markets (venue, native_id, raw_payload, ingested_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, m := range markets {
		payload, err := json.Marshal(json.RawMessage(m.RawPayload))
		if err != nil {
			slog.Warn("skip malformed raw payload",
				"venue", m.Venue,
				"native_id", m.NativeID,
			)
			continue
		}

		if _, err := stmt.Exec(string(m.Venue), m.NativeID, payload, m.FetchedAt.UTC().Format(time.RFC3339)); err != nil {
			slog.Warn("insert raw market failed",
				"venue", m.Venue,
				"native_id", m.NativeID,
				"error", err,
			)
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return inserted, nil
}
