package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const stalenessWindowCycles = 5

// DetectStalePrices checks for markets with unchanged YesPrice across
// the last N ingest cycles and sets DataStalenessFlag=true.
func (db *DB) DetectStalePrices() (int, error) {
	rows, err := db.conn.Query(`
		SELECT cm.id, cm.yes_price, cm.venue
		FROM canonical_markets cm
		WHERE cm.data_staleness_flag = 0
		  AND cm.status = 'OPEN'
	`)
	if err != nil {
		return 0, fmt.Errorf("query canonical markets: %w", err)
	}
	defer rows.Close()

	flagged := 0
	for rows.Next() {
		var id string
		var currentPrice float64
		var venue string
		if err := rows.Scan(&id, &currentPrice, &venue); err != nil {
			continue
		}

		nativeID := id
		if len(id) > len(venue)+1 {
			nativeID = id[len(venue)+1:]
		}

		stale, err := db.isPriceUnchanged(venue, nativeID, stalenessWindowCycles)
		if err != nil {
			slog.Debug("staleness check skipped", "id", id, "error", err)
			continue
		}

		if stale {
			if _, err := db.conn.Exec(
				"UPDATE canonical_markets SET data_staleness_flag = 1 WHERE id = ?", id,
			); err != nil {
				slog.Warn("failed to flag stale market", "id", id, "error", err)
				continue
			}
			flagged++
			slog.Warn("stale price detected",
				"market_id", id,
				"venue", venue,
				"yes_price", currentPrice,
				"unchanged_cycles", stalenessWindowCycles,
			)
		}
	}

	return flagged, rows.Err()
}

// isPriceUnchanged checks if the raw_markets table has N consecutive entries
// with the same yes_price for a given market.
func (db *DB) isPriceUnchanged(venue, nativeID string, cycles int) (bool, error) {
	rows, err := db.conn.Query(`
		SELECT raw_payload
		FROM raw_markets
		WHERE venue = ? AND native_id = ?
		ORDER BY ingested_at DESC
		LIMIT ?
	`, venue, nativeID, cycles)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var payloads []string
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return false, err
		}
		payloads = append(payloads, payload)
	}

	if len(payloads) < cycles {
		return false, nil
	}

	// All payloads identical means no price movement
	first := payloads[0]
	for _, p := range payloads[1:] {
		if p != first {
			return false, nil
		}
	}

	return true, nil
}

// DetectVenueStaleness checks if a venue has not produced new raw_markets
// entries for longer than 2× the poll interval.
func (db *DB) DetectVenueStaleness(venue string, pollInterval time.Duration) bool {
	var latest sql.NullString
	err := db.conn.QueryRow(
		"SELECT MAX(ingested_at) FROM raw_markets WHERE venue = ?", venue,
	).Scan(&latest)
	if err != nil || !latest.Valid {
		return true
	}

	latestTime, err := time.Parse(time.RFC3339, latest.String)
	if err != nil {
		return true
	}

	threshold := 2 * pollInterval
	if time.Since(latestTime) > threshold {
		slog.Warn("venue appears stale",
			"venue", venue,
			"last_ingest", latestTime.Format(time.RFC3339),
			"threshold", threshold.String(),
		)
		return true
	}

	return false
}
