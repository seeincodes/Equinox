# Project Equinox — Architecture Memo

## Project Summary

Equinox is a cross-venue prediction market aggregation and routing infrastructure prototype. It connects to Kalshi (CFTC-regulated) and Polymarket (crypto-based, optimistic oracle) to normalize markets into a single canonical model, detect equivalent markets across venues using hybrid rule-based and embedding similarity, and simulate routing decisions with scored rationale. This is an infrastructure prototype — no real money, no live orders, evaluation focuses on problem decomposition and decision justification.

## Key Architecture Decisions

### 1. Go over Python/TypeScript

Go was chosen because it's the internal stack at Peak6. Goroutines provide natural parallelism for polling multiple venue APIs concurrently without callback complexity. Clean interface types enable the adapter pattern without generics overhead. The tradeoff: Go's embedding/ML ecosystem is weaker than Python's, which is why embeddings are an external API call (OpenAI) rather than a local model. This is an acceptable dependency for a prototype.

### 2. SQLite over PostgreSQL/Redis

SQLite was chosen over PostgreSQL because the prototype is single-machine, single-process. There's no concurrent write contention (ingest is sequential per-venue, normalize is idempotent). SQLite is zero-config, portable, and the entire database travels with the repo. PostgreSQL would add operational overhead (Docker, connection management, migrations tooling) with no benefit at prototype scale (~1,000 markets). Redis was rejected because persistence is a requirement — routing decisions and equivalence groups must survive restarts.

### 3. Four-Layer Architecture with Strict Import Boundaries

The system is divided into Adapters (L1) → Normalization (L2) → Equivalence (L3) → Routing (L4), with the critical constraint that **routing has zero imports from adapters**. This isn't just convention — it's enforced via static analysis. The reason: routing must reason over venue-agnostic canonical data. If routing could reach into adapter types, it would inevitably accumulate venue-specific logic, breaking the ability to add a third venue without modifying the routing engine. The Canonical Market Model (CMM) is the contract boundary.

### 4. Hybrid Equivalence (Rules + Embeddings) over Pure ML

Pure embedding similarity would be simpler but has two problems: (a) cost — calling OpenAI for every O(n²) pair at 500 markets = 125,000 pairs is expensive and slow, and (b) false positives — embeddings alone can't distinguish markets with identical titles but different resolution dates or contract types. Stage 1 (rules) eliminates ~85% of pairs cheaply using structural features (contract type, outcome count, resolution window, token overlap). Stage 2 (embeddings) handles the semantic similarity that rules can't capture ("Fed cuts rates" vs "FOMC rate decision"). Pure rule-based was rejected because market titles vary too much across venues for string matching alone.

### 5. OpenAI text-embedding-3-small over Local Models

No mature Go-native embedding library exists that handles financial/prediction market language well. Running a Python sidecar (e.g., sentence-transformers) was considered but adds deployment complexity and a cross-language boundary. OpenAI's API is a single HTTP call with excellent Go support via `net/http`. The tradeoff is an external dependency — mitigated by caching embeddings by title hash and graceful degradation (Stage 1 only with `EMBEDDING_UNAVAILABLE` flag when OpenAI is down).

### 6. Explicit Failure Taxonomy (T1–T7) over Generic Error Handling

Rather than treating all API errors as retryable, the system classifies failures into 7 types with distinct handling. T1 (5xx) and T2 (429) are retried. T3 (auth) and T4 (schema change) require human intervention. T5 (partial data) is treated as error, not silently accepted. T7 (stale data served as fresh) is a novel category — HTTP 200 with valid schema but unchanged prices indicates a venue issue. This taxonomy prevents the common failure mode of retrying non-transient errors and of silently accepting degraded data.

### 7. Settlement Divergence as a First-Class Flag

In March 2025, a whale with 25% of UMA voting power manipulated a $7M Polymarket resolution. In 2024, Kalshi and Polymarket resolved the same government shutdown event to opposite outcomes. "Equivalent" markets on different venues can settle differently because the settlement mechanisms are fundamentally different (CFTC regulatory process vs. optimistic oracle with token-holder voting). Rather than hiding this risk in the equivalence layer, `SETTLEMENT_DIVERGENCE` is surfaced as a prominent flag in every routing decision where it applies.

## Processing Strategy

1. **Ingest** (every 60s): Adapters poll Kalshi and Polymarket APIs in parallel via goroutines. Raw JSON responses are logged verbatim to SQLite. Adapter layer owns all retry/circuit-breaker logic — downstream layers never see partial or ambiguous data.

2. **Normalize**: Each venue's raw data is transformed through venue-specific transformers into the Canonical Market Model. Title normalization (lowercase, strip punctuation, stem) and rules hash (SHA-256 of resolution criteria) are computed. Idempotent — safe to re-run.

3. **Match**: All cross-venue pairs (O(n²)) go through Stage 1 rule filtering (contract type, outcome count, resolution window, Jaccard ≥ 0.25, Levenshtein ≥ 0.40). Survivors go through Stage 2 embedding similarity (cosine via OpenAI). Groups are formed with confidence scores and flags.

4. **Route**: Given an order request, the engine finds the equivalence group, scores each venue member on price quality (40%), liquidity (35%), spread (15%), and market status (10%), and selects the best venue. Every decision is logged as structured JSON with human-readable narrative. `SimulatedOnly` is always true — type-enforced, not just documented.

## Known Failure Modes

| Failure Mode | Mitigation |
|---|---|
| Kalshi sandbox differs from production schema | Validate against production once; injectable base URL allows switching |
| Polymarket `outcomePrices` JSON-in-JSON parsing | Dedicated parse step with explicit `json.Unmarshal` on string value |
| OpenAI API unavailable | Graceful degradation to Stage 1 rules only + `EMBEDDING_UNAVAILABLE` flag |
| USDC depeg invalidates price comparison | Assumption A1 flagged in every Polymarket routing decision |
| Equivalent markets resolve to different outcomes | `SETTLEMENT_DIVERGENCE` flag surfaced prominently |
| Rate limiting from aggressive polling | Exponential backoff, Retry-After header support, 5-min venue cooldown after repeated 429s |
| Stale data served as fresh (T7) | Staleness detection after 5 unchanged cycles; 20% liquidity haircut in routing |
| Circuit breaker stuck open | 60s auto-cooldown with single probe request; `equinox status` reports state |
| NegRisk sub-conditions falsely matched | Explicit `neg_risk` check in Stage 1 pre-filter; skip intra-NegRisk pairs |
