package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"equinox/adapters"
	"equinox/models"
	"equinox/normalizer"
	"equinox/store"

	"github.com/spf13/cobra"
)

var normalizeCmd = &cobra.Command{
	Use:   "normalize",
	Short: "Transform raw market data into canonical form",
	Long:  "Reads raw markets from SQLite and produces CanonicalMarket records. Idempotent — safe to run multiple times.",
	RunE:  runNormalize,
}

func runNormalize(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	raws, err := loadLatestRawMarkets(db)
	if err != nil {
		return fmt.Errorf("load raw markets: %w", err)
	}

	if len(raws) == 0 {
		fmt.Println("No raw markets found. Run 'equinox ingest' first.")
		return nil
	}

	norm := normalizer.New()
	canonicals, errs := norm.Normalize(raws)

	for _, e := range errs {
		slog.Warn("normalization error", "error", e)
	}

	persisted, err := persistCanonicalMarkets(db, canonicals)
	if err != nil {
		return fmt.Errorf("persist canonical markets: %w", err)
	}

	fmt.Printf("Normalized %d markets (%d errors, %d persisted)\n", len(canonicals), len(errs), persisted)
	return nil
}

func loadLatestRawMarkets(db *store.DB) ([]adapters.RawMarket, error) {
	rows, err := db.Conn().Query(`
		SELECT venue, native_id, raw_payload, ingested_at 
		FROM raw_markets 
		WHERE (venue, native_id, ingested_at) IN (
			SELECT venue, native_id, MAX(ingested_at) 
			FROM raw_markets 
			GROUP BY venue, native_id
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("query raw markets: %w", err)
	}
	defer rows.Close()

	var markets []adapters.RawMarket
	for rows.Next() {
		var venueStr, nativeID, ingestedAtStr string
		var payload []byte
		if err := rows.Scan(&venueStr, &nativeID, &payload, &ingestedAtStr); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		fetchedAt, _ := time.Parse(time.RFC3339, ingestedAtStr)
		markets = append(markets, adapters.RawMarket{
			NativeID:   nativeID,
			Venue:      models.Venue(venueStr),
			RawPayload: json.RawMessage(payload),
			FetchedAt:  fetchedAt,
		})
	}

	return markets, rows.Err()
}

func persistCanonicalMarkets(db *store.DB, markets []models.CanonicalMarket) (int, error) {
	tx, err := db.Conn().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO canonical_markets 
		(id, venue, title, normalized_title, description, outcomes, 
		 resolution_time, resolution_time_utc, yes_price, no_price, spread, 
		 liquidity, volume_24h, status, contract_type, settlement_mechanism,
		 settlement_note, rules_hash, data_staleness_flag, ingested_at, raw_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	persisted := 0
	for _, m := range markets {
		outcomesJSON, _ := json.Marshal(m.Outcomes)
		rawJSON, _ := json.Marshal(json.RawMessage(m.RawPayload))

		var resTime, resTimeUTC *string
		if m.ResolutionTime != nil {
			s := m.ResolutionTime.Format(time.RFC3339)
			resTime = &s
		}
		if m.ResolutionTimeUTC != nil {
			s := m.ResolutionTimeUTC.Format(time.RFC3339)
			resTimeUTC = &s
		}

		staleness := 0
		if m.DataStalenessFlag {
			staleness = 1
		}

		if _, err := stmt.Exec(
			m.ID, string(m.Venue), m.Title, m.NormalizedTitle, m.Description,
			string(outcomesJSON), resTime, resTimeUTC,
			m.YesPrice, m.NoPrice, m.Spread, m.Liquidity, m.Volume24h,
			string(m.Status), string(m.ContractType), string(m.SettlementMechanism),
			m.SettlementNote, m.RulesHash, staleness,
			m.IngestedAt.Format(time.RFC3339), string(rawJSON),
		); err != nil {
			slog.Warn("insert canonical market failed",
				"id", m.ID,
				"error", err,
			)
			continue
		}
		persisted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return persisted, nil
}
