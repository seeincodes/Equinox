# Project Equinox — Tech Stack

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI (Cobra)                          │
│   ingest │ normalize │ match │ route │ status │ explain     │
└────┬─────────┬──────────┬──────────┬──────────┬─────────────┘
     │         │          │          │          │
     ▼         ▼          │          │          │
┌─────────────────┐       │          │          │
│  L1: Adapters   │       │          │          │
│  ┌───────────┐  │       │          │          │
│  │  Kalshi   │  │       │          │          │
│  │  Adapter  │──┼───┐   │          │          │
│  └───────────┘  │   │   │          │          │
│  ┌───────────┐  │   │   │          │          │
│  │Polymarket │  │   │   │          │          │
│  │  Adapter  │──┼───┤   │          │          │
│  └───────────┘  │   │   │          │          │
│  [retry+CB]     │   │   │          │          │
└─────────────────┘   │   │          │          │
                      ▼   ▼          │          │
               ┌──────────────┐      │          │
               │   SQLite DB  │◄─────┼──────────┤
               │  raw_markets │      │          │
               │  canonical   │      │          │
               │  groups      │      │          │
               │  decisions   │      │          │
               └──────┬───────┘      │          │
                      │              │          │
                      ▼              │          │
               ┌──────────────┐      │          │
               │L2: Normalizer│      │          │
               │ venue → CMM  │──────┤          │
               └──────────────┘      │          │
                                     ▼          │
               ┌───────────────────────────┐    │
               │    L3: Equivalence        │    │
               │  Stage 1: Rules (Jaccard, │    │
               │    Levenshtein, type)     │    │
               │  Stage 2: Embeddings      │────┤
               │    (OpenAI API)           │    │
               └───────────────────────────┘    │
                                                ▼
               ┌───────────────────────────────────┐
               │       L4: Routing Engine          │
               │  Score = 0.40×Price + 0.35×Liq    │
               │        + 0.15×Spread + 0.10×Status│
               │  → RoutingDecision + slog output  │
               │  [ZERO imports from adapters/]     │
               └───────────────────────────────────┘
```

## Stack Decisions

| Layer | Technology | Version | Rationale |
|---|---|---|---|
| Language | Go | 1.22+ | Internal Peak6 stack; goroutines for parallel polling; clean interfaces for adapter pattern |
| HTTP Client | `net/http` + `go-resty/resty` | stdlib + v2 | resty adds retry/backoff out of the box; minimal dependency surface |
| Storage | SQLite via `mattn/go-sqlite3` | latest | Zero-dep persistence; portable single-file DB; sufficient for prototype volumes |
| Embeddings | OpenAI `text-embedding-3-small` | API v1 | No mature local Go embedding library; external call is a documented dependency |
| String Matching | Custom Levenshtein + Jaccard | N/A | Fast Stage 1 pre-filter before embedding API calls; avoids unnecessary external calls |
| CLI Framework | `spf13/cobra` | v1 | Standard for Go CLI apps; subcommand structure maps cleanly to pipeline stages |
| Configuration | `spf13/viper` + `.env` | v1 | Secrets separated from config; env var overrides for CI |
| Logging | `log/slog` (stdlib) | Go 1.21+ | Structured JSON logging; routing decisions are first-class log outputs |
| Testing | stdlib `testing` + `stretchr/testify` | latest | Unit tests per layer; integration tests against Kalshi sandbox + Polymarket live |

## Key Dependencies

### Backend (Go)

```
github.com/spf13/cobra         # CLI framework
github.com/spf13/viper         # Configuration management
github.com/go-resty/resty/v2   # HTTP client with retry
github.com/mattn/go-sqlite3    # SQLite driver (CGO)
github.com/stretchr/testify    # Test assertions and mocks
github.com/joho/godotenv       # .env file loading (optional, viper handles)
```

### External APIs

```
OpenAI API (text-embedding-3-small)  # Embedding generation for equivalence Stage 2
Kalshi Trade API v2                  # Prediction market data (sandbox + production)
Polymarket Gamma API                 # Market metadata
Polymarket CLOB API                  # Order book and pricing data
```

## Environment Variables

```bash
# Kalshi
KALSHI_BASE_URL=              # https://api.elections.kalshi.com/trade-api/v2 (prod) or https://demo-api.kalshi.co/trade-api/v2 (sandbox)
KALSHI_API_KEY=               # RSA private key path or key content (for authenticated endpoints)

# Polymarket
POLYMARKET_GAMMA_URL=         # https://gamma-api.polymarket.com
POLYMARKET_CLOB_URL=          # https://clob.polymarket.com

# OpenAI (for embeddings)
OPENAI_API_KEY=               # API key for text-embedding-3-small

# Database
SQLITE_DB_PATH=               # Path to SQLite database file (default: ./equinox.db)

# Polling
POLL_INTERVAL_SECONDS=        # Market polling interval (default: 60)

# Equivalence
EMBEDDING_SIMILARITY_HIGH=    # High confidence threshold (default: 0.92)
EMBEDDING_SIMILARITY_LOW=     # Low confidence threshold (default: 0.78)
JACCARD_THRESHOLD=            # Stage 1 Jaccard token overlap threshold (default: 0.25)
LEVENSHTEIN_THRESHOLD=        # Stage 1 normalized Levenshtein threshold (default: 0.40)
RESOLUTION_WINDOW_HOURS=      # Temporal equivalence window (default: 48)

# Routing
WEIGHT_PRICE_QUALITY=         # Scoring weight (default: 0.40)
WEIGHT_LIQUIDITY=             # Scoring weight (default: 0.35)
WEIGHT_SPREAD_QUALITY=        # Scoring weight (default: 0.15)
WEIGHT_MARKET_STATUS=         # Scoring weight (default: 0.10)
STALENESS_LIQUIDITY_HAIRCUT=  # Liquidity discount for stale data (default: 0.20)

# Logging
LOG_LEVEL=                    # slog level: DEBUG, INFO, WARN, ERROR (default: INFO)
LOG_FORMAT=                   # json or text (default: json)
```

## Database Schema

```sql
-- Raw market data as received from venue APIs
CREATE TABLE raw_markets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    venue TEXT NOT NULL CHECK(venue IN ('KALSHI', 'POLYMARKET')),
    native_id TEXT NOT NULL,
    raw_payload JSON NOT NULL,
    ingested_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(venue, native_id, ingested_at)
);

CREATE INDEX idx_raw_markets_venue ON raw_markets(venue);
CREATE INDEX idx_raw_markets_ingested ON raw_markets(ingested_at);

-- Normalized canonical market model
CREATE TABLE canonical_markets (
    id TEXT PRIMARY KEY,                    -- "{venue}:{native_id}"
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

CREATE INDEX idx_canonical_venue ON canonical_markets(venue);
CREATE INDEX idx_canonical_status ON canonical_markets(status);
CREATE INDEX idx_canonical_normalized_title ON canonical_markets(normalized_title);

-- Equivalence groups linking cross-venue markets
CREATE TABLE equivalence_groups (
    group_id TEXT PRIMARY KEY,
    member_ids JSON NOT NULL,               -- sorted array of canonical market IDs
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

CREATE INDEX idx_groups_confidence ON equivalence_groups(confidence_score);

-- Embedding cache for OpenAI API cost management
CREATE TABLE embedding_cache (
    title_hash TEXT PRIMARY KEY,            -- SHA-256 of NormalizedTitle
    embedding BLOB NOT NULL,                -- float32 array serialized
    model TEXT NOT NULL,                    -- "text-embedding-3-small"
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Routing decisions log
CREATE TABLE routing_decisions (
    decision_id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES equivalence_groups(group_id),
    order_request JSON NOT NULL,
    selected_venue TEXT NOT NULL,
    selected_market_id TEXT NOT NULL,
    rejected_alternatives JSON NOT NULL,
    scoring_breakdown JSON NOT NULL,
    routing_rationale TEXT NOT NULL,
    simulated_only BOOLEAN NOT NULL DEFAULT 1 CHECK(simulated_only = 1),
    cache_mode BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_decisions_group ON routing_decisions(group_id);
CREATE INDEX idx_decisions_created ON routing_decisions(created_at);
```

## Cost Estimates

| Scale Tier | Markets | Embedding Calls/day | OpenAI Cost/mo | Infra Cost/mo | Total/mo |
|---|---|---|---|---|---|
| Development | ~100 | ~50 | ~$0.50 | $0 (local) | ~$0.50 |
| Prototype (target) | ~500 | ~200 | ~$2 | $0 (local) | ~$2 |
| Light Production | ~2,000 | ~1,000 | ~$10 | ~$20 (Cloud Run) | ~$30 |
| Full Production | ~10,000 | ~5,000 | ~$50 | ~$100 (Cloud Run + Firestore) | ~$150 |

> Embedding costs based on `text-embedding-3-small` at $0.02/1M tokens. Market titles average ~20 tokens. Cached embeddings are not re-computed.
