# Project Equinox — Task List

## Phase 1: MVP

### 1.1 Project Scaffolding
- [x] `go mod init equinox`
- [x] Set up Cobra CLI with root command and subcommands (`ingest`, `normalize`, `match`, `route`, `status`, `explain`)
- [x] Create directory structure: `cmd/`, `adapters/`, `models/`, `normalizer/`, `equivalence/`, `routing/`, `store/`, `config/`
- [x] Configure Viper for `.env` + env var overrides
- [x] Set up `slog` with structured JSON output
- [x] Create SQLite database + initial migration (raw_markets, canonical_markets, equivalence_groups, routing_decisions)

### 1.2 Data Models
- [x] Implement `CanonicalMarket` struct with all fields (ID, Venue, Title, NormalizedTitle, Outcomes, YesPrice, NoPrice, Spread, Liquidity, etc.)
- [x] Implement `EquivalenceGroup` struct with ConfidenceScore, MatchMethod, Flags
- [x] Implement `RoutingDecision` struct with ScoringBreakdown, RoutingRationale, SimulatedOnly (type-enforced `true`)
- [x] Implement `VenueScore` and `OrderRequest` structs
- [x] Implement enums: `Venue`, `MarketStatus`, `ContractType`, `SettlementType`, `MatchMethod`, `MatchFlag`, `FailureType`
- [x] Implement `AdapterError` struct with Venue, Type (T1–T7), Attempts, LastError

### 1.3 Venue Adapters — Kalshi
- [x] Implement `VenueAdapter` interface (`FetchMarkets`, `FetchPricing`, `VenueID`, `HealthCheck`)
- [x] Kalshi adapter: fetch markets from `GET /markets?status=open&limit=100&cursor=`
- [x] Kalshi adapter: handle cursor-based pagination
- [x] Kalshi adapter: fetch orderbook from `GET /markets/{ticker}/orderbook?depth=10`
- [x] Kalshi adapter: compute YesAsk from `100 - best_no_bid`
- [x] Kalshi adapter: normalize prices from cents (1–99) to 0.0–1.0
- [x] Kalshi adapter: map status strings (`open`→OPEN, `closed`→CLOSED, `settled`→RESOLVED)
- [x] Kalshi adapter: health check via `GET /exchange/status`
- [x] Kalshi adapter: injectable `baseURL` for testing
- [x] Kalshi adapter: schema validation (T4 detection) — require `ticker` + valid pricing fields

### 1.4 Venue Adapters — Polymarket
- [x] Polymarket adapter: fetch metadata from Gamma API `GET /markets?active=true&limit=100&closed=false`
- [x] Polymarket adapter: fetch CLOB data from `GET /markets?next_cursor=`
- [x] Polymarket adapter: build `condition_id` ↔ Gamma `id` mapping layer
- [x] Polymarket adapter: parse `outcomePrices` (JSON-encoded string: `"[\"0.65\",\"0.35\"]"`)
- [x] Polymarket adapter: fetch orderbook from `GET /book?token_id={yes_token_id}` (both sides)
- [x] Polymarket adapter: map status from `active`+`funded` flags
- [x] Polymarket adapter: hardcode `OPTIMISTIC_ORACLE` as SettlementMechanism
- [x] Polymarket adapter: injectable `baseURL` for testing
- [x] Polymarket adapter: detect and preserve `neg_risk` field in RawPayload

### 1.5 Retry & Circuit Breaker
- [x] Implement exponential backoff retry (max 3 attempts, ±20% jitter, ~1s/2s/4s)
- [x] Retry only T1 (5xx/timeout) and T2 (429); fail immediately on T3/T4/T5
- [x] Honour `Retry-After` header on 429 responses
- [x] Back off venue polling cadence for 5 min after 2× consecutive 429s
- [x] Implement circuit breaker: CLOSED → OPEN (after 5 consecutive errors) → HALF-OPEN (60s cooldown) → probe
- [x] All HTTP calls accept `context.Context`; retries cancel on `ctx.Done()`

### 1.6 Raw Ingest Pipeline
- [x] `equinox ingest` command: fetch from both venues (or `--venue` filter)
- [x] Write raw API responses to SQLite `raw_markets` table
- [x] Log structured adapter failures per Section 15 format
- [x] Handle partial venue failure (one succeeds, one fails)

### 1.7 Normalization (L2)
- [x] Kalshi → CMM transformer: map all fields per CMM mapping spec
- [x] Polymarket → CMM transformer: map all fields per CMM mapping spec
- [x] Title normalization: lowercase, strip punctuation, stem
- [x] Compute `RulesHash` as SHA-256 of normalized resolution criteria
- [x] `equinox normalize` command: idempotent raw → canonical conversion
- [x] Write canonical markets to SQLite `canonical_markets` table
- [x] Unit tests with fixture JSON from both venues

### 1.8 Equivalence Detection (L3)
- [x] Stage 1 rule-based pre-filter: ContractType match, outcome count, resolution window (48h), Jaccard ≥ 0.25, Levenshtein ≥ 0.40, NegRisk skip
- [x] Stage 2 embedding similarity: call OpenAI `text-embedding-3-small`, cache by NormalizedTitle hash, batch up to 100 texts
- [x] Classification: HIGH (≥0.92), MEDIUM (0.78–0.92 + LOW_CONFIDENCE flag), REJECT (<0.78)
- [x] Confidence formula: `sim*0.9 + jaccard*0.1`
- [x] Settlement divergence flag: add `SETTLEMENT_DIVERGENCE` if members have different SettlementMechanism
- [x] Fallback: if OpenAI unavailable, Stage 1 only + `LOW_CONFIDENCE` + `EMBEDDING_UNAVAILABLE` flags
- [x] `equinox match` command with `--dry-run` option
- [x] Write equivalence groups to SQLite `equivalence_groups` table

### 1.9 Routing Engine (L4)
- [x] Implement scoring model: `0.40×PriceQuality + 0.35×Liquidity + 0.15×SpreadQuality + 0.10×MarketStatus`
- [x] PriceQuality: `1 - |YesPrice - FairValue|` where FairValue = avg across group
- [x] Liquidity: `minmax_normalize(Liquidity)` across group
- [x] SpreadQuality: `1 - minmax_normalize(Spread)`
- [x] MarketStatus: OPEN=1.0, SUSPENDED=0.3, else 0.0
- [x] Configurable weights via config struct
- [x] Deterministic tie-breaking by Venue enum ordering
- [x] `SimulatedOnly` always true (type-enforced)
- [x] Stale data: apply 20% liquidity haircut + `STALE_PRICING_DATA` flag
- [x] Human-readable routing decision output (per PRD format)
- [x] `equinox route --market <id> --side YES --size 100` command
- [x] Static analysis enforcement: zero imports from `adapters/`

### 1.10 Status & Explain Commands
- [x] `equinox status`: health check all adapters + ingestion stats
- [x] `equinox explain --group <group_id>`: human-readable group breakdown

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
