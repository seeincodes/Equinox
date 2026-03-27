package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew_CreatesAndMigrates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}

	// Verify all tables exist
	tables := []string{"raw_markets", "canonical_markets", "equivalence_groups", "embedding_cache", "routing_decisions", "users", "sessions", "config", "canonical_market_snapshots"}
	for _, table := range tables {
		var name string
		err := db.Conn().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestNew_MigrationsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Run migrations twice — should not fail
	db1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New() error: %v", err)
	}
	db1.Close()

	db2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New() error: %v", err)
	}
	db2.Close()
}

func TestNew_WALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer db.Close()

	var journalMode string
	err = db.Conn().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
}

func TestNew_SimulatedOnlyConstraint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer db.Close()

	// Insert a valid equivalence group first (foreign key)
	_, err = db.Conn().Exec(`INSERT INTO equivalence_groups (group_id, member_ids, confidence_score, match_method, match_rationale, flags) VALUES ('g1', '[]', 0.95, 'HYBRID', 'test', '[]')`)
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}

	// Attempting to insert simulated_only=0 should violate CHECK constraint
	_, err = db.Conn().Exec(`INSERT INTO routing_decisions (decision_id, group_id, order_request, selected_venue, selected_market_id, rejected_alternatives, scoring_breakdown, routing_rationale, simulated_only, cache_mode) VALUES ('d1', 'g1', '{}', 'KALSHI', 'm1', '[]', '{}', 'test', 0, 0)`)
	if err == nil {
		t.Error("expected CHECK constraint violation for simulated_only=0, got nil")
	}
}

func TestNew_VenueConstraint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer db.Close()

	// Invalid venue should violate CHECK constraint
	_, err = db.Conn().Exec(`INSERT INTO raw_markets (venue, native_id, raw_payload) VALUES ('INVALID', 'test', '{}')`)
	if err == nil {
		t.Error("expected CHECK constraint violation for invalid venue, got nil")
	}

	// Valid venue should succeed
	_, err = db.Conn().Exec(`INSERT INTO raw_markets (venue, native_id, raw_payload) VALUES ('KALSHI', 'test', '{}')`)
	if err != nil {
		t.Errorf("valid insert failed: %v", err)
	}
}
