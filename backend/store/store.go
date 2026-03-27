package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection.
type DB struct {
	conn *sql.DB
}

// New opens a SQLite database and runs migrations.
func New(dbPath string) (*DB, error) {
	dsn := dbPath + "?_journal_mode=WAL&_foreign_keys=ON"
	if dbPath == ":memory:" {
		// Use shared cache for in-memory DBs so all connections see the same data
		dsn = "file::memory:?mode=memory&cache=shared&_journal_mode=WAL&_foreign_keys=ON"
	}
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Verify connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	slog.Info("database initialized", "path", dbPath)
	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying sql.DB for direct queries.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) migrate() error {
	migrations := []string{
		migrationRawMarkets,
		migrationCanonicalMarkets,
		migrationEquivalenceGroups,
		migrationEmbeddingCache,
		migrationRoutingDecisions,
		migrationUsers,
		migrationSessions,
		migrationConfig,
		migrationCanonicalMarketSnapshots,
	}

	for i, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}

	// ALTER TABLE migrations — may fail if column already exists, that's OK
	if _, err := db.conn.Exec("ALTER TABLE routing_decisions ADD COLUMN user_id TEXT DEFAULT ''"); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migration add user_id: %w", err)
		}
	}

	slog.Debug("migrations applied", "count", len(migrations)+1)
	return nil
}

const migrationRawMarkets = `
CREATE TABLE IF NOT EXISTS raw_markets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    venue TEXT NOT NULL CHECK(venue IN ('KALSHI', 'POLYMARKET')),
    native_id TEXT NOT NULL,
    raw_payload JSON NOT NULL,
    ingested_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(venue, native_id, ingested_at)
);
CREATE INDEX IF NOT EXISTS idx_raw_markets_venue ON raw_markets(venue);
CREATE INDEX IF NOT EXISTS idx_raw_markets_ingested ON raw_markets(ingested_at);
`

const migrationCanonicalMarkets = `
CREATE TABLE IF NOT EXISTS canonical_markets (
    id TEXT PRIMARY KEY,
    venue TEXT NOT NULL,
    title TEXT NOT NULL,
    normalized_title TEXT NOT NULL,
    description TEXT,
    outcomes JSON NOT NULL,
    resolution_time TIMESTAMP,
    resolution_time_utc TIMESTAMP,
    yes_price REAL NOT NULL,
    no_price REAL NOT NULL,
    spread REAL NOT NULL,
    liquidity REAL NOT NULL,
    volume_24h REAL,
    status TEXT NOT NULL CHECK(status IN ('OPEN', 'CLOSED', 'RESOLVED', 'SUSPENDED')),
    contract_type TEXT NOT NULL CHECK(contract_type IN ('BINARY', 'CATEGORICAL', 'SCALAR')),
    settlement_mechanism TEXT NOT NULL CHECK(settlement_mechanism IN ('CFTC_REGULATED', 'OPTIMISTIC_ORACLE', 'UNKNOWN')),
    settlement_note TEXT,
    rules_hash TEXT NOT NULL,
    data_staleness_flag BOOLEAN NOT NULL DEFAULT 0,
    ingested_at TIMESTAMP NOT NULL,
    raw_payload JSON NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_canonical_venue ON canonical_markets(venue);
CREATE INDEX IF NOT EXISTS idx_canonical_status ON canonical_markets(status);
CREATE INDEX IF NOT EXISTS idx_canonical_normalized_title ON canonical_markets(normalized_title);
`

const migrationEquivalenceGroups = `
CREATE TABLE IF NOT EXISTS equivalence_groups (
    group_id TEXT PRIMARY KEY,
    member_ids JSON NOT NULL,
    confidence_score REAL NOT NULL,
    match_method TEXT NOT NULL CHECK(match_method IN ('RULE_BASED', 'EMBEDDING', 'HYBRID')),
    embedding_similarity REAL,
    string_similarity REAL,
    resolution_delta_seconds INTEGER,
    match_rationale TEXT NOT NULL,
    flags JSON NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_groups_confidence ON equivalence_groups(confidence_score);
`

const migrationEmbeddingCache = `
CREATE TABLE IF NOT EXISTS embedding_cache (
    title_hash TEXT PRIMARY KEY,
    embedding BLOB NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const migrationRoutingDecisions = `
CREATE TABLE IF NOT EXISTS routing_decisions (
    decision_id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL,
    order_request JSON NOT NULL,
    selected_venue TEXT NOT NULL,
    selected_market_id TEXT NOT NULL,
    rejected_alternatives JSON NOT NULL,
    scoring_breakdown JSON NOT NULL,
    routing_rationale TEXT NOT NULL,
    simulated_only BOOLEAN NOT NULL DEFAULT 1 CHECK(simulated_only = 1),
    cache_mode BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES equivalence_groups(group_id)
);
CREATE INDEX IF NOT EXISTS idx_decisions_group ON routing_decisions(group_id);
CREATE INDEX IF NOT EXISTS idx_decisions_created ON routing_decisions(created_at);
`

const migrationUsers = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('viewer', 'analyst', 'admin')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationSessions = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);
`

const migrationConfig = `
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_by TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCanonicalMarketSnapshots = `
CREATE TABLE IF NOT EXISTS canonical_market_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    market_id TEXT NOT NULL,
    venue TEXT NOT NULL,
    yes_price REAL,
    no_price REAL,
    spread REAL,
    liquidity REAL,
    snapshot_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_snapshots_market ON canonical_market_snapshots(market_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_time ON canonical_market_snapshots(snapshot_at);
`
