# Project Equinox — Error & Fix Log

## Template

```
### [CATEGORY-NNN] Short description
- **Date:** YYYY-MM-DD
- **Error:** What happened (exact error message or behavior)
- **Context:** What you were doing when it occurred
- **Root Cause:** Why it happened
- **Fix:** What resolved it (include code/commands if helpful)
- **Prevention:** How to avoid recurrence
```

## Log

*No errors logged yet.*

## Common Issues to Watch For

### Go / Build

- **CGO required for SQLite:** `mattn/go-sqlite3` requires CGO_ENABLED=1. If builds fail with "binary-only package" errors, ensure `CGO_ENABLED=1` and a C compiler is available.
- **Go module replace directives:** If using local package replacements during development, remember to remove `replace` directives before committing.
- **Context cancellation in goroutines:** Parallel adapter polling must respect `ctx.Done()` — leaked goroutines will cause test hangs and resource leaks.

### Kalshi API

- **Orderbook returns bids only:** Do not assume asks are present. Compute: `YesAsk = 100 - best_no_bid`. Missing this will produce incorrect spread calculations.
- **Prices are cents (1–99), not decimals:** Normalize with `/ 100.0`. Forgetting this produces prices >1.0, which breaks the scoring model.
- **`elections` subdomain serves ALL categories:** Don't filter by subdomain name — it's misleading.
- **Cursor pagination:** Empty `cursor` field (not absent, but empty string) means last page. Infinite loops if not handled.
- **Sandbox vs production URL mismatch:** Sandbox is `demo-api.kalshi.co`, production is `api.elections.kalshi.com`. Note the different TLDs (`.co` vs `.com`).

### Polymarket API

- **`outcomePrices` is a JSON-encoded string:** The value looks like `"[\"0.65\",\"0.35\"]"` — it's a string containing JSON, not a JSON array. Must `json.Unmarshal` the string value first.
- **`condition_id` (CLOB) ≠ `id` (Gamma):** These are different fields across the two APIs. Build an explicit mapping layer.
- **`tokens[0].token_id` required for orderbook calls:** The CLOB `/book` endpoint needs the YES token ID, not the condition ID.
- **NegRisk sub-conditions:** `neg_risk: true` in raw payload means this is a sub-condition of a larger event. Skip intra-NegRisk pairs in equivalence to avoid false matches.
- **Polymarket US is a separate system:** `api.polymarket.us` uses Ed25519 auth + KYC. Do not confuse with `gamma-api.polymarket.com` / `clob.polymarket.com`.

### OpenAI Embeddings

- **Rate limits on embedding API:** Batch up to 100 texts per call. Single-text calls will hit rate limits quickly at scale.
- **Cache invalidation:** Embeddings are cached by NormalizedTitle hash. If normalization logic changes, the cache must be cleared.
- **API unavailability:** Must gracefully degrade to Stage 1 rules only — never block the entire matching pipeline on embedding availability.

### SQLite

- **WAL mode recommended:** Enable `PRAGMA journal_mode=WAL` for better concurrent read performance during ingest + query.
- **UNIQUE constraint on raw_markets:** The (venue, native_id, ingested_at) constraint prevents duplicate ingestion but requires timestamp precision. Use millisecond timestamps.
- **JSON column queries:** Use SQLite JSON functions (`json_extract`, `json_each`) for querying JSON columns. Don't parse in application code for simple filters.

### Equivalence & Routing

- **Threshold sensitivity:** Embedding similarity thresholds (0.92 HIGH, 0.78 LOW) are heuristic. Monitor false positive/negative rates and adjust based on labeled test data.
- **Division by zero in minmax normalization:** If all venues in a group have identical liquidity or spread, minmax returns NaN. Handle the single-value case explicitly.
- **Deterministic tie-breaking:** When scores are equal, break ties by Venue enum ordering. Without this, routing results are non-reproducible.
