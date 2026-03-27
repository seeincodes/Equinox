# Project Equinox — User Flow

## Primary Flow

```
┌──────────────────────┐
│  1. equinox ingest   │  ~15-30s
│  [--venue kalshi|    │
│   polymarket]        │
│                      │
│  Fetches raw market  │
│  data from venue APIs│
│  → SQLite raw log    │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  2. equinox normalize│  ~2-5s
│                      │
│  Transforms raw venue│
│  JSON → CanonicalMkt │
│  Idempotent operation│
│  → SQLite canonical  │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  3. equinox match    │  ~10-30s (with embeddings)
│  [--dry-run]         │  ~2-5s  (rules only)
│                      │
│  Stage 1: Rule filter│
│  Stage 2: Embeddings │
│  → EquivalenceGroups │
│  → SQLite groups     │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  4. equinox route    │  ~1-2s
│  --market <id>       │
│  --side YES|NO       │
│  --size 100          │
│                      │
│  Scores venues →     │
│  RoutingDecision     │
│  + structured log    │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  5. Review output    │
│                      │
│  Human-readable      │
│  routing narrative   │
│  with scoring        │
│  breakdown + flags   │
└──────────────────────┘

Auxiliary commands:

┌──────────────────────┐     ┌──────────────────────┐
│  equinox status      │     │  equinox explain     │
│                      │     │  --group <group_id>  │
│  Health check all    │     │                      │
│  adapters + stats    │     │  Group breakdown     │
│  (anytime)           │     │  with match details  │
└──────────────────────┘     └──────────────────────┘
```

## API Endpoints (Optional REST — if time permits)

### GET /markets

List canonical markets with optional filters.

```
Request:  GET /markets?venue=KALSHI&status=OPEN
Response: 200 OK
{
  "markets": [
    {
      "id": "KALSHI:KXFED-MARCH2026",
      "venue": "KALSHI",
      "title": "Fed rate cut March 2026",
      "normalized_title": "fed rate cut march 2026",
      "yes_price": 0.65,
      "no_price": 0.35,
      "spread": 0.02,
      "liquidity": 125000.00,
      "status": "OPEN",
      "contract_type": "BINARY",
      "settlement_mechanism": "CFTC_REGULATED",
      "resolution_time": "2026-03-19T18:00:00Z",
      "data_staleness_flag": false
    }
  ],
  "count": 1
}
```

### GET /groups

List equivalence groups with optional minimum confidence filter.

```
Request:  GET /groups?min_confidence=0.80
Response: 200 OK
{
  "groups": [
    {
      "group_id": "a1b2c3d4-...",
      "members": ["KALSHI:KXFED-MARCH2026", "POLYMARKET:0xabc..."],
      "confidence_score": 0.94,
      "match_method": "HYBRID",
      "flags": ["SETTLEMENT_DIVERGENCE"],
      "match_rationale": "Titles match (Jaccard: 0.87, embedding: 0.96). Both resolve March 2026. Settlement mechanisms differ: CFTC_REGULATED vs OPTIMISTIC_ORACLE."
    }
  ],
  "count": 1
}
```

### POST /route

Submit an order request and receive a routing decision.

```
Request:  POST /route
{
  "market_id": "KALSHI:KXFED-MARCH2026",
  "side": "YES",
  "size": 100
}

Response: 200 OK
{
  "decision_id": "d5e6f7...",
  "selected_venue": "POLYMARKET",
  "selected_market_id": "POLYMARKET:0xabc...",
  "scoring_breakdown": {
    "KALSHI": {"price_quality": 0.85, "liquidity": 0.60, "spread_quality": 0.70, "market_status": 1.0, "total": 0.76},
    "POLYMARKET": {"price_quality": 0.90, "liquidity": 0.85, "spread_quality": 0.80, "market_status": 1.0, "total": 0.87}
  },
  "routing_rationale": "POLYMARKET selected. Higher liquidity ($250K vs $125K) and tighter spread (0.01 vs 0.02). NOTE: USDC/USD assumed 1:1. Settlement mechanisms differ (CFTC_REGULATED vs OPTIMISTIC_ORACLE). SimulatedOnly=true.",
  "simulated_only": true,
  "cache_mode": false
}
```

### GET /health

System health check.

```
Request:  GET /health
Response: 200 OK
{
  "status": "DEGRADED",
  "venues": {
    "KALSHI": {"status": "HEALTHY", "last_ingest": "2026-03-12T10:00:00Z", "markets_count": 245},
    "POLYMARKET": {"status": "UNHEALTHY", "last_ingest": "2026-03-12T09:45:00Z", "error": "circuit_breaker_open", "markets_count": 312}
  },
  "database": {"status": "HEALTHY", "canonical_markets": 557, "equivalence_groups": 42},
  "timestamp": "2026-03-12T10:05:00Z"
}
```

## Example Queries

| Query | Expected Result | Expected Answer |
|---|---|---|
| `equinox ingest` | Fetches ~200-500 markets from each venue | Stored in SQLite with timestamps |
| `equinox ingest --venue kalshi` | Fetches Kalshi markets only | Polymarket untouched |
| `equinox normalize` | Converts all raw markets to CMM | Idempotent; re-running updates existing |
| `equinox match` | Finds ~5-20 cross-venue equivalence groups | Groups with confidence scores + flags |
| `equinox match --dry-run` | Same analysis but no persistence | Outputs groups to stdout only |
| `equinox route --market KALSHI:KXFED-MARCH2026 --side YES --size 100` | Scores all venues in the equivalence group | Routing decision with rationale + SimulatedOnly=true |
| `equinox status` | Checks venue API health + DB stats | "KALSHI: HEALTHY, POLYMARKET: HEALTHY, DB: 557 markets, 42 groups" |
| `equinox explain --group a1b2c3d4` | Details one equivalence group | Member markets, match method, confidence, flags, rationale |
| `equinox route` when Kalshi is down | Routes using Polymarket only | Decision includes `SINGLE_VENUE_ONLY` flag |
| `equinox route` when both venues down | Returns cached decision | `CacheMode=true` with staleness age |
