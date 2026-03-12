# Project Equinox — Agent Skill

## Context

Cross-venue prediction market aggregation and routing infrastructure prototype connecting Kalshi and Polymarket, built in Go.

## Codebase

- **Target:** CLI tool (`equinox`) with optional REST API
- **Size:** Medium (~20-30 source files across 8 packages)
- **Structure:** Four-layer architecture (Adapters → Normalization → Equivalence → Routing) with strict import boundaries

## Stack

- Go 1.22+
- Cobra (CLI)
- Viper (config) + `.env`
- SQLite via `mattn/go-sqlite3`
- `go-resty/resty` (HTTP client with retry)
- OpenAI `text-embedding-3-small` (embeddings for equivalence Stage 2)
- Custom Levenshtein + Jaccard (equivalence Stage 1)
- `log/slog` (structured JSON logging)
- `testing` + `testify` (tests)

## Key Files

| File | Purpose |
|---|---|
| `PRD.md` | Full product requirements, API specs, data models, acceptance criteria |
| `TASK_LIST.md` | Phased implementation checklist (MVP → Polish → Final) |
| `TECH_STACK.md` | Architecture diagram, stack decisions, DB schema, env vars, cost estimates |
| `USER_FLOW.md` | CLI user journey, API endpoint specs, example queries |
| `MEMO.md` | Architecture decisions with rationale (why Go, why SQLite, why hybrid equivalence, etc.) |
| `ERROR_FIX_LOG.md` | Error tracking template + common issues by technology area |
| `.env` | Environment variable template (all empty) |
| `.cursor/rules/tech-stack-lock.mdc` | Locked technology decisions — do not deviate |
| `.cursor/rules/env-files-read-only.mdc` | Environment file protection rules |
| `.cursor/rules/error-resolution-log.mdc` | When and how to log errors |

## Processing Strategy

1. **Ingest:** Parallel-poll Kalshi + Polymarket APIs → raw JSON to SQLite
2. **Normalize:** Venue-specific transformers → Canonical Market Model (CMM)
3. **Match:** Stage 1 rule pre-filter (85% elimination) → Stage 2 embedding similarity → EquivalenceGroups
4. **Route:** Score venues (40% price + 35% liquidity + 15% spread + 10% status) → RoutingDecision with narrative

## Known Patterns

- **Adapter pattern:** All venue APIs implement `VenueAdapter` interface; injectable base URLs for testing
- **Error taxonomy:** T1–T7 failure types with distinct retry/escalation behavior
- **Circuit breaker:** CLOSED → OPEN (5 errors) → HALF-OPEN (60s cooldown) per venue
- **Graceful degradation:** Single venue failure → continue with flags; both down → cache mode
- **SimulatedOnly enforcement:** `RoutingDecision.SimulatedOnly` is type-enforced `true`, not just documented
- **Assumption annotations:** All assumptions tagged in code as `// ASSUMPTION A1: ...` through `A9`
- **Settlement divergence:** First-class flag — CFTC vs optimistic oracle can resolve same event differently
- **USDC = USD 1:1:** Assumption A1, flagged in every Polymarket routing decision
