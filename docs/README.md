# Equinox

Cross-venue prediction market aggregation and routing infrastructure prototype.

Connects to Kalshi and Polymarket, normalizes markets into a canonical model, detects equivalent markets across venues, and simulates routing decisions with logged rationale.

## Quick Start

### Prerequisites

- Go 1.22+ with CGO enabled
- SQLite3 (bundled via `mattn/go-sqlite3`)
- OpenAI API key (optional — equivalence detection falls back to rule-based matching without it)

### Setup

```bash
git clone <repo-url> && cd equinox
cp .env.example .env   # edit with your API keys
go mod download
CGO_ENABLED=1 go build -o equinox .
```

### Configure

Edit `.env` with your credentials:

```
KALSHI_BASE_URL=https://demo-api.kalshi.co/trade-api/v2
KALSHI_API_KEY=your-key-here
POLYMARKET_GAMMA_URL=https://gamma-api.polymarket.com
POLYMARKET_CLOB_URL=https://clob.polymarket.com
OPENAI_API_KEY=sk-...
SQLITE_DB_PATH=./equinox.db
```

### Run

```bash
# 1. Ingest raw market data from both venues
./equinox ingest

# 2. Normalize raw data into canonical form
./equinox normalize

# 3. Detect equivalent markets across venues
./equinox match --dry-run    # preview without persisting
./equinox match              # persist to database

# 4. Simulate a routing decision
./equinox route --market "KALSHI:KXBTC-100K" --side YES --size 100

# 5. Check system health
./equinox status

# 6. Explain an equivalence group
./equinox explain --group <group_id>

# 7. Start the REST API server
./equinox serve --addr :8080
```

### REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/markets?venue=&status=` | List canonical markets, with optional venue/status filters |
| `GET` | `/groups?min_confidence=` | List equivalence groups, filtered by minimum confidence |
| `POST` | `/route` | Route an order (body: `{"market_id":"...","side":"YES","size":100}`) |
| `GET` | `/health` | System health: venue connectivity, database counts |

### Test

```bash
CGO_ENABLED=1 go test ./... -count=1
```

## Architecture

Four strictly isolated layers. The routing engine has zero imports from `adapters/`.

```
L1  Venue Adapters      → raw venue JSON + typed AdapterErrors
L2  Normalization       → []CanonicalMarket
L3  Equivalence         → []EquivalenceGroup
L4  Routing Engine      → RoutingDecision
```

### Directory Structure

```
equinox/
├── api/              # Optional REST API server
├── cmd/              # Cobra CLI commands (ingest, normalize, match, route, status, explain, serve)
├── adapters/         # L1 — VenueAdapter interface, retry, circuit breaker
│   ├── kalshi/       # Kalshi exchange adapter
│   └── polymarket/   # Polymarket adapter (Gamma + CLOB APIs)
├── normalizer/       # L2 — venue → CanonicalMarket transformers
├── equivalence/      # L3 — rule-based + embedding similarity matching
├── routing/          # L4 — scoring model, venue ranking, decision output
├── models/           # Shared canonical structs and enums
├── store/            # SQLite migrations, queries, staleness detection
├── config/           # Viper config from .env
└── main.go
```

## Key Design Decisions

- **SimulatedOnly is type-enforced** — the `simulatedOnlyTrue` type always marshals to `true`
- **Retry with circuit breaker** — exponential backoff (3 attempts, ±20% jitter), circuit opens after 5 consecutive failures
- **Graceful degradation** — if OpenAI is unavailable, equivalence detection falls back to Stage 1 rule-based matching with `LOW_CONFIDENCE` + `EMBEDDING_UNAVAILABLE` flags
- **Settlement divergence flagging** — cross-venue groups with different settlement mechanisms get `SETTLEMENT_DIVERGENCE` warnings
- **USDC/USD assumed 1:1** (Assumption A1) — flagged in every routing decision involving Polymarket
