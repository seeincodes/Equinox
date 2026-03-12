# Project Equinox — Task List

## Phase 1: MVP

### 1.1 Project Scaffolding
- [ ] `go mod init equinox`
- [ ] Set up Cobra CLI with root command and subcommands (`ingest`, `normalize`, `match`, `route`, `status`, `explain`)
- [ ] Create directory structure: `cmd/`, `adapters/`, `models/`, `normalizer/`, `equivalence/`, `routing/`, `store/`, `config/`
- [ ] Configure Viper for `.env` + env var overrides
- [ ] Set up `slog` with structured JSON output
- [ ] Create SQLite database + initial migration (raw_markets, canonical_markets, equivalence_groups, routing_decisions)

### 1.2 Data Models
- [ ] Implement `CanonicalMarket` struct with all fields (ID, Venue, Title, NormalizedTitle, Outcomes, YesPrice, NoPrice, Spread, Liquidity, etc.)
- [ ] Implement `EquivalenceGroup` struct with ConfidenceScore, MatchMethod, Flags
- [ ] Implement `RoutingDecision` struct with ScoringBreakdown, RoutingRationale, SimulatedOnly (type-enforced `true`)
- [ ] Implement `VenueScore` and `OrderRequest` structs
- [ ] Implement enums: `Venue`, `MarketStatus`, `ContractType`, `SettlementType`, `MatchMethod`, `MatchFlag`, `FailureType`
- [ ] Implement `AdapterError` struct with Venue, Type (T1–T7), Attempts, LastError

### 1.3 Venue Adapters — Kalshi
- [ ] Implement `VenueAdapter` interface (`FetchMarkets`, `FetchPricing`, `VenueID`, `HealthCheck`)
- [ ] Kalshi adapter: fetch markets from `GET /markets?status=open&limit=100&cursor=`
- [ ] Kalshi adapter: handle cursor-based pagination
- [ ] Kalshi adapter: fetch orderbook from `GET /markets/{ticker}/orderbook?depth=10`
- [ ] Kalshi adapter: compute YesAsk from `100 - best_no_bid`
- [ ] Kalshi adapter: normalize prices from cents (1–99) to 0.0–1.0
- [ ] Kalshi adapter: map status strings (`open`→OPEN, `closed`→CLOSED, `settled`→RESOLVED)
- [ ] Kalshi adapter: health check via `GET /exchange/status`
- [ ] Kalshi adapter: injectable `baseURL` for testing
- [ ] Kalshi adapter: schema validation (T4 detection) — require `ticker` + valid pricing fields

### 1.4 Venue Adapters — Polymarket
- [ ] Polymarket adapter: fetch metadata from Gamma API `GET /markets?active=true&limit=100&closed=false`
- [ ] Polymarket adapter: fetch CLOB data from `GET /markets?next_cursor=`
- [ ] Polymarket adapter: build `condition_id` ↔ Gamma `id` mapping layer
- [ ] Polymarket adapter: parse `outcomePrices` (JSON-encoded string: `"[\"0.65\",\"0.35\"]"`)
- [ ] Polymarket adapter: fetch orderbook from `GET /book?token_id={yes_token_id}` (both sides)
- [ ] Polymarket adapter: map status from `active`+`funded` flags
- [ ] Polymarket adapter: hardcode `OPTIMISTIC_ORACLE` as SettlementMechanism
- [ ] Polymarket adapter: injectable `baseURL` for testing
- [ ] Polymarket adapter: detect and preserve `neg_risk` field in RawPayload

### 1.5 Retry & Circuit Breaker
- [ ] Implement exponential backoff retry (max 3 attempts, ±20% jitter, ~1s/2s/4s)
- [ ] Retry only T1 (5xx/timeout) and T2 (429); fail immediately on T3/T4/T5
- [ ] Honour `Retry-After` header on 429 responses
- [ ] Back off venue polling cadence for 5 min after 2× consecutive 429s
- [ ] Implement circuit breaker: CLOSED → OPEN (after 5 consecutive errors) → HALF-OPEN (60s cooldown) → probe
- [ ] All HTTP calls accept `context.Context`; retries cancel on `ctx.Done()`

### 1.6 Raw Ingest Pipeline
- [ ] `equinox ingest` command: fetch from both venues (or `--venue` filter)
- [ ] Write raw API responses to SQLite `raw_markets` table
- [ ] Log structured adapter failures per Section 15 format
- [ ] Handle partial venue failure (one succeeds, one fails)

### 1.7 Normalization (L2)
- [ ] Kalshi → CMM transformer: map all fields per CMM mapping spec
- [ ] Polymarket → CMM transformer: map all fields per CMM mapping spec
- [ ] Title normalization: lowercase, strip punctuation, stem
- [ ] Compute `RulesHash` as SHA-256 of normalized resolution criteria
- [ ] `equinox normalize` command: idempotent raw → canonical conversion
- [ ] Write canonical markets to SQLite `canonical_markets` table
- [ ] Unit tests with fixture JSON from both venues

### 1.8 Equivalence Detection (L3)
- [ ] Stage 1 rule-based pre-filter: ContractType match, outcome count, resolution window (48h), Jaccard ≥ 0.25, Levenshtein ≥ 0.40, NegRisk skip
- [ ] Stage 2 embedding similarity: call OpenAI `text-embedding-3-small`, cache by NormalizedTitle hash, batch up to 100 texts
- [ ] Classification: HIGH (≥0.92), MEDIUM (0.78–0.92 + LOW_CONFIDENCE flag), REJECT (<0.78)
- [ ] Confidence formula: `sim*0.9 + jaccard*0.1`
- [ ] Settlement divergence flag: add `SETTLEMENT_DIVERGENCE` if members have different SettlementMechanism
- [ ] Fallback: if OpenAI unavailable, Stage 1 only + `LOW_CONFIDENCE` + `EMBEDDING_UNAVAILABLE` flags
- [ ] `equinox match` command with `--dry-run` option
- [ ] Write equivalence groups to SQLite `equivalence_groups` table

### 1.9 Routing Engine (L4)
- [ ] Implement scoring model: `0.40×PriceQuality + 0.35×Liquidity + 0.15×SpreadQuality + 0.10×MarketStatus`
- [ ] PriceQuality: `1 - |YesPrice - FairValue|` where FairValue = avg across group
- [ ] Liquidity: `minmax_normalize(Liquidity)` across group
- [ ] SpreadQuality: `1 - minmax_normalize(Spread)`
- [ ] MarketStatus: OPEN=1.0, SUSPENDED=0.3, else 0.0
- [ ] Configurable weights via config struct
- [ ] Deterministic tie-breaking by Venue enum ordering
- [ ] `SimulatedOnly` always true (type-enforced)
- [ ] Stale data: apply 20% liquidity haircut + `STALE_PRICING_DATA` flag
- [ ] Human-readable routing decision output (per PRD format)
- [ ] `equinox route --market <id> --side YES --size 100` command
- [ ] Static analysis enforcement: zero imports from `adapters/`

### 1.10 Status & Explain Commands
- [ ] `equinox status`: health check all adapters + ingestion stats
- [ ] `equinox explain --group <group_id>`: human-readable group breakdown

## Phase 2: Polish

### 2.1 Staleness Detection
- [ ] Detect unchanged `YesPrice` across 5 consecutive ingest cycles → set `DataStalenessFlag=true`
- [ ] Detect no new markets from a venue for > 2× poll interval → log WARN
- [ ] Venue suspension behavior: `SINGLE_VENUE_ONLY` flag when one venue down
- [ ] Cache-only mode: `CacheMode=true` when all venues unavailable

### 2.2 Optional REST API
- [ ] `GET /markets?venue=&status=`
- [ ] `GET /groups?min_confidence=`
- [ ] `POST /route` (body: OrderRequest → RoutingDecision)
- [ ] `GET /health`

### 2.3 Edge Case Handling
- [ ] Handle missing resolution time (waive E2, add `RESOLUTION_TIME_MISSING` flag)
- [ ] Handle market resolving early on one venue
- [ ] Handle `RulesHash` change → re-evaluate group; archive old
- [ ] Multi-outcome categorical: stricter threshold 0.95

## Phase 3: Final

### 3.1 Resilience Tests (R1–R8)
- [ ] R1: 503 × 2 then 200 — verify successful ingest after retry
- [ ] R2: 429 with `Retry-After: 2` — verify ≥2s wait
- [ ] R3: 429 × 3 exhausts retries — verify 5 min polling backoff
- [ ] R4: Response missing required field — verify T4 error, no partial normalization
- [ ] R5: Kalshi down, Polymarket healthy — verify partial ingest + flags
- [ ] R6: Both venues down, route called — verify `CacheMode=true`
- [ ] R7: Circuit breaker opens at 5 failures — verify OPEN state, no HTTP after
- [ ] R8: Stale price detection across 5 cycles — verify flag + liquidity haircut

### 3.2 Acceptance Criteria Validation
- [ ] F1: `equinox ingest` fetches from both venues within 30s
- [ ] F2: All ingested markets produce valid `CanonicalMarket` with no nil panics
- [ ] F3: `equinox match` identifies ≥5 known-equivalent pairs in live data
- [ ] F4: `equinox match` rejects ≥10 known-non-equivalent pairs without false positives
- [ ] F5: `equinox route` produces `RoutingDecision` with human-readable rationale
- [ ] F6: Routing engine imports zero packages from `adapters/`
- [ ] F7: All routing decisions logged as structured JSON via slog
- [ ] F8: `equinox status` reports health when one venue unavailable
- [ ] F9: On 5xx, adapter retries with backoff; marks `DataStalenessFlag=true`
- [ ] F10: Both venues unavailable → typed error, not garbage data

### 3.3 Quality & Documentation
- [ ] ≥80% unit test line coverage per layer
- [ ] README: new engineer running in ≤15 min
- [ ] No hard-coded credentials — all via env vars / `.env`
- [ ] All assumptions (A1–A9) as structured comments in code (`// ASSUMPTION A1: ...`)
- [ ] No adapter base URLs hard-coded — injectable for testing
