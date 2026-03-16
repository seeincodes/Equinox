# Project Equinox — prd.md
> Cross-venue prediction market aggregation & routing infrastructure prototype  
> Version: 1.2 | Status: Draft | Classification: Confidential | Date: March 2026

---

## What This Is

Infrastructure prototype — **not a trading product**. Connects to Kalshi and Polymarket, normalizes markets into a single canonical model, detects equivalent markets across venues, and simulates routing decisions with logged rationale. Evaluation criteria: problem framing, decomposition, ambiguity handling, decision justification.

---

## Goals / Non-Goals

**In scope**
1. Ingest market metadata + live pricing from Kalshi and Polymarket APIs
2. Canonical Market Model (CMM) independent of any venue schema
3. Hybrid equivalence detection (rule-based + embedding similarity)
4. Routing simulation with human-readable decision log
5. Graceful degradation when one or both venue APIs are unavailable

**Out of scope (v1)**
- Real-money trading, wallet integration, live order execution
- Production UI or dashboard
- Regulatory compliance (KYC, AML, reporting)
- FIX 4.4 protocol
- More than two venues
- Historical data backfill

---

## Tech Stack

| Component | Choice | Why |
|---|---|---|
| Language | Go 1.22+ | Internal stack; goroutines for parallel polling; clean interfaces for adapters |
| HTTP | `net/http` + `resty` | resty adds retry/backoff; no heavy deps |
| Storage | SQLite (`mattn/go-sqlite3`) | Zero-dep persistence; portable; fine for prototype volumes |
| Embeddings | OpenAI `text-embedding-3-small` via HTTP | No mature local Go embedding lib; external call is documented dependency |
| String matching | Custom Levenshtein + Jaccard (or `go-fuzzy`) | Fast Stage 1 pre-filter before embedding calls |
| CLI | `spf13/cobra` | Standard for Go CLIs; subcommand structure |
| Config | `viper` + `.env` | Secrets separated from config; env var overrides for CI |
| Logging | `slog` (stdlib, Go 1.21+) | Structured JSON; routing decisions are first-class log outputs |
| Testing | stdlib `testing` + `testify` | Unit per layer; integration against Kalshi sandbox + Polymarket live |

---

## Architecture

Four strictly isolated layers. **Routing has zero imports from `adapters/`** — enforced by static analysis.

```
L1  Venue Adapters      → raw venue JSON + typed AdapterErrors
L2  Normalization       → []CanonicalMarket
L3  Equivalence         → []EquivalenceGroup
L4  Routing Engine      → RoutingDecision
```

### Data Flow

```
[poll every 60s]
Kalshi API  ──┐
              ├─► L1 Adapters ─► SQLite raw log ─► L2 Normalize ─► SQLite canonical
Polymarket ──┘

[on demand]
equinox match  ─► L3 Equivalence ─► SQLite groups
equinox route  ─► L4 Routing     ─► RoutingDecision JSON + slog
```

### Directory Structure

```
equinox/
├── cmd/                    # cobra CLI entry points
│   ├── ingest.go
│   ├── normalize.go
│   ├── match.go
│   ├── route.go
│   └── status.go
├── adapters/               # L1 — VenueAdapter interface + impls
│   ├── adapter.go          # interface definition
│   ├── kalshi/
│   └── polymarket/
├── models/                 # shared canonical structs
│   ├── market.go           # CanonicalMarket, Outcome, enums
│   ├── equivalence.go      # EquivalenceGroup, MatchFlag
│   └── routing.go          # RoutingDecision, OrderRequest, VenueScore
├── normalizer/             # L2 — venue → CMM transformers
├── equivalence/            # L3
│   ├── rules.go            # Stage 1 rule-based filters
│   └── embedding.go        # Stage 2 cosine similarity
├── routing/                # L4
│   ├── engine.go
│   └── scorer.go
├── store/                  # SQLite migrations + queries
├── config/                 # viper setup
└── main.go
```

---

## Data Models

### CanonicalMarket

```go
type CanonicalMarket struct {
    ID                  string           // "{venue}:{native_id}"
    Venue               Venue            // KALSHI | POLYMARKET
    Title               string           // as-returned by venue API
    NormalizedTitle     string           // lowercase, punctuation stripped, stemmed
    Description         string           // full resolution criteria
    Outcomes            []Outcome
    ResolutionTime      *time.Time       // nil if venue doesn't specify
    ResolutionTimeUTC   *time.Time       // derived; always UTC
    YesPrice            float64          // 0.0–1.0 implied probability
    NoPrice             float64          // should ≈ 1 - YesPrice
    Spread              float64          // YesAsk - YesBid
    Liquidity           float64          // USD equivalent
    Volume24h           *float64         // optional
    Status              MarketStatus     // OPEN | CLOSED | RESOLVED | SUSPENDED
    ContractType        ContractType     // BINARY | CATEGORICAL | SCALAR
    SettlementMechanism SettlementType   // CFTC_REGULATED | OPTIMISTIC_ORACLE | UNKNOWN
    SettlementNote      *string          // venue-specific resolution quirks
    RulesHash           string           // SHA-256 of normalized resolution criteria
    DataStalenessFlag   bool             // true if from cache/fallback due to API failure
    IngestedAt          time.Time
    RawPayload          json.RawMessage  // verbatim original response; never modified
}
```

> **Design rule:** CMM contains only venue-agnostic fields. Venue-specific fields live in the adapter layer. Optional fields are pointers.

### EquivalenceGroup

```go
type EquivalenceGroup struct {
    GroupID             string           // deterministic UUID from sorted member IDs
    Members             []CanonicalMarket
    ConfidenceScore     float64          // 0.0–1.0
    MatchMethod         MatchMethod      // RULE_BASED | EMBEDDING | HYBRID
    EmbeddingSimilarity *float64
    StringSimilarity    *float64
    ResolutionDelta     *time.Duration
    MatchRationale      string
    CreatedAt           time.Time
    Flags               []MatchFlag
}

// MatchFlag values
const (
    FlagResolutionTimeMismatch  MatchFlag = "RESOLUTION_TIME_MISMATCH"
    FlagLowConfidence           MatchFlag = "LOW_CONFIDENCE"
    FlagCategoricalMismatch     MatchFlag = "CATEGORICAL_MISMATCH"
    FlagSettlementDivergence    MatchFlag = "SETTLEMENT_DIVERGENCE"
    FlagStalePricingData        MatchFlag = "STALE_PRICING_DATA"
    FlagSingleVenueOnly         MatchFlag = "SINGLE_VENUE_ONLY"
    FlagEmbeddingUnavailable    MatchFlag = "EMBEDDING_UNAVAILABLE"
    FlagResolutionTimeMissing   MatchFlag = "RESOLUTION_TIME_MISSING"
)
```

### RoutingDecision

```go
type RoutingDecision struct {
    DecisionID          string
    OrderRequest        OrderRequest
    EquivalenceGroup    EquivalenceGroup
    SelectedVenue       Venue
    SelectedMarket      CanonicalMarket
    RejectedAlternatives []RejectedVenue
    ScoringBreakdown    map[Venue]VenueScore
    RoutingRationale    string           // human-readable narrative
    Timestamp           time.Time
    SimulatedOnly       bool             // ALWAYS true in prototype; type-enforced
    CacheMode           bool             // true if served from stale data
}
```

### VenueAdapter Interface

```go
type VenueAdapter interface {
    FetchMarkets(ctx context.Context) ([]RawMarket, error)
    FetchPricing(ctx context.Context, marketID string) (*RawPricing, error)
    VenueID() Venue
    HealthCheck(ctx context.Context) error
}
```

Adapters accept an optional `baseURL` override so `httptest.NewServer()` can be injected in tests. No live network calls in unit tests.

---

## Venue APIs

### Kalshi

| | |
|---|---|
| Base URL | `https://api.elections.kalshi.com/trade-api/v2` |
| Sandbox | `https://demo-api.kalshi.co/trade-api/v2` |
| Auth (reads) | None |
| Auth (trading) | RSA key-pair; sign each request; token auth expires every 30 min — use RSA |
| Pagination | Cursor-based; empty `cursor` field = last page |
| Rate limit | Tiered; ~100 req/min basic tier |

**Key endpoints:**

```
GET /markets?status=open&limit=100&cursor=        # list markets
GET /markets/{ticker}                             # single market
GET /markets/{ticker}/orderbook?depth=10          # order book
GET /events?status=open                           # event groups
GET /trades?ticker=KXFED-MARCH2026                # trade feed
GET /exchange/status                              # health check (use this for HealthCheck())
```

**Critical quirks:**
- `elections` subdomain serves ALL categories — not just elections
- Orderbook returns **bids only** — no asks. Compute: `YesAsk = 100 - best_no_bid`
- Prices are **cents (integer 1–99)**. Normalize: `YesPrice = (yes_bid + yes_ask) / 200.0`
- `status: "open"→OPEN`, `"closed"→CLOSED`, `"settled"→RESOLVED`

**CMM mapping:**
```
ticker          → ID suffix
title           → Title
title (normalized) → NormalizedTitle
close_time      → ResolutionTime (parse ISO 8601)
(yes_bid+yes_ask)/200 → YesPrice
yes_ask/100 - yes_bid/100 → Spread
volume          → Volume24h
status          → Status (map above)
CFTC_REGULATED  → SettlementMechanism (hardcoded)
```

---

### Polymarket

Two separate APIs with different schemas. Must join on `condition_id`.

| | Gamma API | CLOB API |
|---|---|---|
| Base URL | `https://gamma-api.polymarket.com` | `https://clob.polymarket.com` |
| Auth (reads) | None | None |
| Auth (trading) | — | EIP-712 wallet signing; derive via `py-clob-client` |
| Primary key | `id` | `condition_id` |

**Key endpoints:**

```
# Gamma — metadata
GET /markets?active=true&limit=100&closed=false

# CLOB — trading
GET /markets?next_cursor=                         # list with condition_id + token_id
GET /book?token_id={yes_token_id}                 # order book (both sides returned)
GET /price?token_id={yes_token_id}&side=BUY       # best price
GET /prices-history?market={token_id}&interval=1w&fidelity=60
```

**Critical quirks:**
- `outcomePrices` in Gamma response is a **JSON-encoded string**: `"[\"0.65\",\"0.35\"]"` — call `json.Unmarshal` on the string value before accessing
- Polymarket returns **both bids AND asks** in `/book` — no inference needed (unlike Kalshi)
- `condition_id` (CLOB) ≠ `id` (Gamma); build mapping layer to correlate
- `tokens[0].token_id` = YES outcome address; required for all `/book` and `/price` calls
- `/prices-history` returns empty for resolved markets at fidelity < 720 min; set `fidelity=720` for closed markets
- Polymarket US (`api.polymarket.us`) is a **completely separate system** with Ed25519 auth + KYC — out of scope for v1; do not confuse with global endpoints
- `neg_risk: true` in raw payload = this is a NegRisk sub-condition; **skip intra-NegRisk pairs** in equivalence Stage 1

**CMM mapping:**
```
condition_id            → ID suffix
question                → Title
question (normalized)   → NormalizedTitle
description             → Description
endDate                 → ResolutionTime
JSON.parse(outcomePrices)[0] → YesPrice
tokens[0] + tokens[1]  → Outcomes
(ask - bid) from CLOB   → Spread
liquidity               → Liquidity
active+funded=true      → OPEN; else evaluate closed/resolved flags
OPTIMISTIC_ORACLE       → SettlementMechanism (hardcoded)
```

> **Assumption A1:** USDC = USD 1:1 for all price normalization. Flag in every RoutingDecision that uses Polymarket prices.

---

## Equivalence Detection

### Definition

M1 ≡ M2 if **all three** hold:

| | Condition | Criterion |
|---|---|---|
| E1 | Semantic equivalence | Same real-world outcome (same event, same directional bet) |
| E2 | Temporal proximity | Resolution times within 48h, OR both unspecified |
| E3 | Contract compatibility | Same `ContractType` and same outcome count |

Pairs meeting E1+E3 but not E2: form group with `RESOLUTION_TIME_MISMATCH` flag — do not silently discard.

### Stage 1: Rule-Based Pre-Filter

Runs on all O(n²) pairs. Target: eliminate ~85% before embedding calls.

```
1. ContractType match         → exact; discard on mismatch
2. Outcome count match        → exact; discard on mismatch
3. Resolution window          → |delta| ≤ 48h; flag RESOLUTION_TIME_MISMATCH if outside, continue
4. Jaccard token overlap      → tokenize NormalizedTitle; threshold ≥ 0.25; discard below
5. Normalized Levenshtein     → threshold ≥ 0.40; discard below
6. NegRisk check              → if neg_risk=true in either market's RawPayload AND same parent group → skip (not cross-venue equivalent)
```

### Stage 2: Embedding Similarity

Runs on pairs passing Stage 1.

- Input: `NormalizedTitle + " " + Description[:200]`
- Model: `text-embedding-3-small`
- Metric: cosine similarity

| Score | Classification | Action |
|---|---|---|
| ≥ 0.92 | HIGH | Create group; `ConfidenceScore = sim*0.9 + jaccard*0.1` |
| 0.78–0.92 | MEDIUM | Create group with `LOW_CONFIDENCE` flag |
| < 0.78 | REJECT | Discard; log as rejected candidate |

> **Thresholds are initial values** — must be validated against a labeled test set before any production use.

**Cost management:**
- Cache embeddings by `NormalizedTitle` hash; recompute only on title change
- Batch up to 100 texts per API call
- If OpenAI unavailable: Stage 1 only → `LOW_CONFIDENCE` + `EMBEDDING_UNAVAILABLE` flag

### Settlement Divergence Flag

After group is formed: if members have different `SettlementMechanism` values, add `SETTLEMENT_DIVERGENCE` flag.

> **Why this matters:** In March 2025, a whale with 25% of UMA voting power manipulated a $7M Polymarket resolution. In 2024, Polymarket resolved a government shutdown contract YES while Kalshi resolved NO. Two "equivalent" markets can settle to different outcomes. The flag must surface prominently in routing output.

### Edge Cases

| Case | Handling |
|---|---|
| One venue missing resolution time | Waive E2; add `RESOLUTION_TIME_MISSING` flag |
| Market resolves early on one venue | Status diff on next ingest cycle; flag group `STALE` |
| YES on A = NO on B (negated equivalence) | **Out of scope v1**; documented gap |
| Multi-outcome categorical | Require outcome label fuzzy-match; stricter threshold 0.95 |
| Title updated post-match | `RulesHash` change → re-evaluate; archive old group |
| NegRisk sub-conditions | Skip intra-NegRisk pairs; detect via `neg_risk` field in `RawPayload` |
| Stale pricing | Apply 20% liquidity haircut in routing; add `STALE_PRICING_DATA` flag |

---

## Routing Engine

**Rules:**
- Zero imports from `adapters/` — enforced via static analysis
- `SimulatedOnly` is always `true`; type-enforced, not just documented
- Ties broken deterministically by `Venue` enum ordering

### Scoring Model

```
Score(v) = 0.40 × PriceQuality
         + 0.35 × Liquidity
         + 0.15 × SpreadQuality
         + 0.10 × MarketStatus
```

| Dimension | Formula | Notes |
|---|---|---|
| PriceQuality | `1 - │YesPrice - FairValue│`; FairValue = avg across group | Minimizes deviation from cross-venue consensus |
| Liquidity | `minmax_normalize(Liquidity)` across group | Proxy for slippage |
| SpreadQuality | `1 - minmax_normalize(Spread)` | Tighter spread = better |
| MarketStatus | `OPEN=1.0`, `SUSPENDED=0.3`, else `0.0` | Hard gate on non-open markets |

**Weights are configurable** in the routing engine's config struct.

> **Known gap — fees excluded:** Kalshi charges 0.7–3.5% variable per contract (higher-probability contracts cost more). Polymarket fees vary; consult current docs. The scoring model may route to a venue that is worse on net post-fee execution. Accepted limitation for v1; fee-inclusive routing is a v2 requirement.

> **Known gap — settlement risk not scored:** A fifth dimension `SettlementConfidence` should discount venues with contested recent resolutions. Not in v1, but `SettlementMechanism` in CMM is the foundation.

### Decision Output

Every `equinox route` call emits:

```
ROUTING DECISION [timestamp]
Order: BUY {size} {side} on "{market question}"
Equivalence Group: {group_id} ({method}, confidence: {score})
[WARNINGS: SETTLEMENT_DIVERGENCE | STALE_PRICING_DATA | LOW_CONFIDENCE if applicable]

SELECTED: {VENUE} — {market_id} (score: {score})
  Price Quality:  {score} (YesPrice {p}, FairValue {fv})
  Liquidity:      {score} (${amount})
  Spread Quality: {score} (spread: {spread})
  Market Status:  {score} ({status})

REJECTED: {VENUE} — {market_id} (score: {score})
  {primary rejection reason}

NOTE: USDC/USD assumed 1:1. Resolution delta: {delta}. SimulatedOnly=true.
```

---

## CLI

```bash
equinox ingest [--venue kalshi|polymarket]   # fetch + write to SQLite raw log
equinox normalize                            # raw → CanonicalMarket (idempotent)
equinox match [--dry-run]                    # run equivalence detector; --dry-run skips persist
equinox route --market <id> --side YES --size 100
equinox status                               # health check all adapters + ingestion stats
equinox explain --group <group_id>           # human-readable group breakdown
```

### Optional REST API (if time permits)

```
GET  /markets?venue=&status=
GET  /groups?min_confidence=
POST /route                    # body: OrderRequest → RoutingDecision
GET  /health
```

---

## API Failure Handling

Venue APIs will fail. The system must degrade gracefully and never produce output that is dishonest about its provenance.

### Failure Types

| Type | Trigger | Example |
|---|---|---|
| T1 | 5xx, timeout, DNS failure | API overloaded; network blip |
| T2 | HTTP 429 | Rate limit exceeded |
| T3 | HTTP 401/403 | Expired token; IP block |
| T4 | Schema change | Missing/renamed field; type mismatch on parse |
| T5 | Partial data | HTTP 200 but empty or truncated market list |
| T6 | Venue suspension | All requests fail; regulatory shutdown |
| T7 | Stale data served fresh | HTTP 200 with valid schema but prices unchanged for anomalous window |

### AdapterError Type

```go
type AdapterError struct {
    Venue      Venue
    Type       FailureType  // T1–T7
    Attempts   int
    LastError  error
    OccurredAt time.Time
}
```

Retry is owned entirely by the adapter layer. Normalization, equivalence, and routing receive either a valid result or a typed `AdapterError` — never partial data.

### Retry Policy

```
Eligible for retry:  T1, T2
NOT retried:         T3 (needs human), T4 (needs code change), T5 (treat as error)

Max attempts:        3 (1 initial + 2 retries)
Backoff:             exponential with ±20% jitter — ~1s, ~2s, ~4s
429 handling:        honour Retry-After header if present; else treat as T1
                     after 2× 429s: back off that venue's polling cadence for 5 min
After exhaustion:    return AdapterError; log at ERROR with structured fields
Context:             all HTTP calls accept context.Context; retries cancel on ctx.Done()
```

### Circuit Breaker

```
CLOSED   → normal operation
           ↓ 5 consecutive AdapterErrors
OPEN     → return cached error immediately; no HTTP calls
           ↓ after 60s cooldown
HALF-OPEN → single probe request
           ↓ success → CLOSED
           ↓ failure → OPEN
```

### Schema Validation (T4 detection)

On every response, validate required fields before passing upstream:

```go
// Example — Kalshi market
if market.Ticker == "" || (market.YesBid == 0 && market.YesAsk == 0) {
    return nil, &AdapterError{Venue: VenueKalshi, Type: T4,
        LastError: fmt.Errorf("required field missing: ticker=%q yes_bid=%d",
            market.Ticker, market.YesBid),
    }
}
```

On T4: log full raw response body at DEBUG; do not attempt partial normalization.

### Staleness Detection (T7 detection)

- If `YesPrice` unchanged across 5 consecutive ingest cycles → set `DataStalenessFlag=true`, log WARN
- If no new markets ingested from a venue for > 2× poll interval → log WARN: possible suspension
- Stale markets: eligible for routing with 20% liquidity haircut + `STALE_PRICING_DATA` flag in decision

### Venue Suspension Behavior

| Scenario | System behavior |
|---|---|
| One venue down | Ingest healthy venues normally. Failed venue markets get `DataStalenessFlag=true` after 2 missed cycles. Routing continues; affected groups get `SINGLE_VENUE_ONLY` flag. `equinox status` reports failure. |
| All venues down | Cache-only mode. `equinox route` returns decisions from last successful snapshot with `CacheMode=true` + staleness age. `equinox status` returns `ALL_VENUES_UNAVAILABLE`. |
| Venue resumes | Next ingest cycle clears `DataStalenessFlag`. Re-run `equinox match` to refresh groups. |
| Schema breaking change (T4) | Manual recovery only. No auto-heal. Log `SCHEMA_CHANGE_DETECTED` at ERROR. Continue serving last-known-good data from that venue. |

### Structured Failure Log Fields (required on all adapter failures)

```go
slog.Error("adapter failure",
    "venue",          venueID,
    "failure_type",   failureType,      // "T1"–"T7"
    "attempt",        attemptNumber,
    "status_code",    httpStatusCode,
    "endpoint",       requestURL,       // full URL, no credentials
    "error",          err.Error(),
    "elapsed_ms",     elapsed.Milliseconds(),
    "circuit_state",  circuitState,     // CLOSED | OPEN | HALF_OPEN
)
```

Log levels: T1=WARN (retries) / ERROR (final); T2=WARN; T3=ERROR; T4=ERROR; T5=WARN; T6=ERROR; T7=WARN

---

## Implementation Phases

| Phase | Name | Key Deliverables | Duration |
|---|---|---|---|
| P0 | Scaffolding | `go mod init`, cobra CLI, directory layout, viper config, SQLite schema + migrations | Day 1 |
| P1 | Venue Adapters | Kalshi adapter (sandbox), Polymarket adapter (read-only), `VenueAdapter` interface, retry/circuit breaker, raw ingest pipeline | Days 2–3 |
| P2 | Normalization | `CanonicalMarket` struct + validation, Kalshi→CMM transformer, Polymarket→CMM transformer, unit tests with fixture JSON | Days 3–4 |
| P3 | Equivalence | Stage 1 rule filters, Stage 2 OpenAI embedding, `EquivalenceGroup` output + SQLite storage, `equinox match` + `--dry-run` | Days 4–6 |
| P4 | Routing Engine | `VenueScore`, scoring model, `RoutingDecision` + narrative generator, `equinox route`, structured slog decision log | Days 6–7 |
| P5 | Hardening | Integration tests (sandbox + live), `equinox status`, README (≤15 min setup), API failure scenario tests R1–R8 | Day 8 |

---

## Assumptions

| # | Assumption | Risk if wrong |
|---|---|---|
| A1 | USDC = USD 1:1 | Stablecoin depeg distorts price comparison; flag in every routing decision |
| A2 | Venue resolution times refer to the same underlying deadline | Venues may use different reference events (announcement vs. certification); validate manually for high-stakes markets |
| A3 | Polymarket read-only endpoints remain public | API restrictions break ingest; fallback: The Graph subgraph |
| A4 | Kalshi sandbox is representative of production | Schema divergence requires separate adapter validation on live |
| A5 | `text-embedding-3-small` sufficient for market title similarity | Specialized financial embeddings may perform better; threshold calibration partially mitigates |
| A6 | 48h resolution window appropriate for temporal equivalence | Intraday markets may need tighter window; configurable param |
| A7 | Equivalent markets will resolve to the same outcome | Settlement mechanism divergence (UMA oracle vs CFTC) can cause different resolutions; surface `SETTLEMENT_DIVERGENCE` flag |
| A8 | Venue APIs will generally be available during ingest cycles | APIs fail; see Section 15 (API Failure Handling) |
| A9 | Kalshi sandbox accessible for development | If blocked, develop against Polymarket read-only only; switch to production with config change |

---

## Open Questions

| # | Question | Owner | Impact |
|---|---|---|---|
| Q1 | Do Kalshi/Polymarket impose IP-based rate limits affecting polling cadence? | Engineering | May require proxy rotation or reduced frequency |
| Q2 | Can an equivalence group contain >1 market per venue? | Product | Possible if venue lists same event under multiple tickers; current design assumes one-per-venue |
| Q3 | Acceptable routing latency in production? | Stakeholders | Drives whether real-time embedding calls are feasible vs. cached-only |
| Q4 | Kalshi lists event as BINARY, Polymarket as CATEGORICAL — policy? | Engineering | E3 excludes these; needs explicit decision |
| Q5 | Labeled test set for embedding threshold calibration? | Research | Thresholds are currently heuristic; empirical validation required |
| Q6 | Should routing penalize venues with contested resolution history? | Product | `SettlementMechanism` in CMM is the foundation; v2 scope |
| Q7 | Acceptable behavior when both venues simultaneously unavailable? | Engineering | Determines cache-only vs. fail-fast vs. queue strategy |
| Q8 | Fee sourcing — static config vs. fee endpoint vs. scraped docs? | Engineering | Static config safest for v1; Kalshi has documented schedule; Polymarket inconsistent |

---

## Acceptance Criteria

**Functional**

| # | Criterion | Verification |
|---|---|---|
| F1 | `equinox ingest` fetches from both venues within 30s | Integration test; Kalshi sandbox + Polymarket live |
| F2 | All ingested markets produce valid `CanonicalMarket` with no nil panics | Unit test; fixture JSON from both venues |
| F3 | `equinox match` identifies ≥5 known-equivalent pairs in live data | Manual validation |
| F4 | `equinox match` rejects ≥10 known-non-equivalent pairs without false positives | Manual validation |
| F5 | `equinox route` produces a `RoutingDecision` with human-readable rationale for any valid group | CLI test with `--dry-run` |
| F6 | Routing engine imports zero packages from `adapters/` | Static analysis / `go mod graph` |
| F7 | All routing decisions logged as structured JSON via slog | Log output inspection |
| F8 | `equinox status` reports health without crashing when one venue is unavailable | Adapter failover test |
| F9 | On 5xx, adapter retries with exponential backoff; marks markets `DataStalenessFlag=true` rather than panicking | Unit test: mock HTTP 503; verify retry behavior + flag |
| F10 | When both venues unavailable, `equinox route` returns typed error with clear message — not garbage data | Integration test: block both endpoints; verify error shape |

**Resilience tests (required before P5 sign-off)**

| Test | Scenario | Method |
|---|---|---|
| R1 | 503 × 2 then 200 | Mock server; verify successful ingest after retry; WARN not ERROR |
| R2 | 429 with `Retry-After: 2` | Mock; verify ≥2s wait before retry |
| R3 | 429 × 3 exhausts retries | Mock; verify polling cadence backed off 5 min |
| R4 | Response missing required field | Mock; verify T4 error + raw body logged; no partial normalization |
| R5 | Kalshi down, Polymarket healthy | Block Kalshi mock; verify Polymarket ingest succeeds; Kalshi gets `DataStalenessFlag`; routing has `SINGLE_VENUE_ONLY` |
| R6 | Both venues down; route called | Block both; verify `CacheMode=true` in decision; `equinox status` = `ALL_VENUES_UNAVAILABLE` |
| R7 | Circuit breaker opens at 5 failures | Mock 500 × 6; verify OPEN state after attempt 5; no HTTP calls after that |
| R8 | Stale price detection across 5 cycles | Mock identical prices × 5; verify `DataStalenessFlag=true`; liquidity haircut in routing |

**Quality**
- Each layer has ≥80% unit test line coverage
- README: new engineer running in ≤15 min
- No hard-coded credentials — all via env vars or `.env`
- All assumptions in this file appear as structured comments in code (`// ASSUMPTION A1: ...`)
- No adapter base URLs hard-coded — injectable for testing

---

## Future (v2+)

- Live order execution via Kalshi REST + Polymarket CLOB (wallet integration)
- Third venue: Gemini Prediction Markets (highest v2 priority — cleanest API schema)
- Real-time WebSocket-based pricing; sub-second equivalence updates
- Fee-inclusive routing: sixth scoring dimension `NetPriceAfterFees`
- Settlement confidence scoring: discount venues with contested resolutions in trailing 90 days
- NegRisk / combinatorial arbitrage detection: identify Polymarket sub-condition groups; detect YES(M1) + YES(M2) > $1.00
- API resilience: persistent retry queue, per-venue circuit breakers, auto cache-serve mode
- Historical backtesting of routing decisions against archived orderbook data
- GCP deployment: Cloud Run for ingest pipeline, Firestore for market storage
- Monitoring dashboard: live equivalence groups + routing decision history

---

## Venue API Quick Reference

### Kalshi endpoints

```bash
# List open markets
curl "https://api.elections.kalshi.com/trade-api/v2/markets?status=open&limit=10"

# Single market
curl "https://api.elections.kalshi.com/trade-api/v2/markets/KXBTC-25DEC-T100000"

# Order book (bids only — compute asks from opposite side)
curl "https://api.elections.kalshi.com/trade-api/v2/markets/KXBTC-25DEC-T100000/orderbook?depth=5"

# Health check
curl "https://api.elections.kalshi.com/trade-api/v2/exchange/status"
```

### Polymarket endpoints

```bash
# Gamma — market metadata
curl "https://gamma-api.polymarket.com/markets?active=true&limit=10"

# CLOB — get condition_id and token_id
curl "https://clob.polymarket.com/markets?next_cursor="

# CLOB — order book (both sides)
curl "https://clob.polymarket.com/book?token_id={yes_token_id}"

# CLOB — best price
curl "https://clob.polymarket.com/price?token_id={yes_token_id}&side=BUY"

# CLOB — price history
curl "https://clob.polymarket.com/prices-history?market={token_id}&interval=1w&fidelity=60"
```

---

*Project Equinox prd.md — v1.2 — March 2026 — CONFIDENTIAL*
