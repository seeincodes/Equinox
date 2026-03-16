# REST API Improvements — Equinox

Three new endpoints added to improve usability for traders, researchers, and exchange operators.

## New Endpoints

### 1. POST `/route/batch` — Batch Routing

Route multiple orders in a single request. Useful for algorithms and traders managing portfolios.

**Request:**
```json
[
  {"market_id": "KALSHI:KXBTC-100K", "side": "YES", "size": 100},
  {"market_id": "POLYMARKET:0xf5a1...", "side": "NO", "size": 250}
]
```

**Response:**
```json
{
  "decisions": [
    {
      "decision_id": "rd-...",
      "selected_venue": "POLYMARKET",
      "selected_market": "POLYMARKET:0xf5a1...",
      "score": 0.845,
      "routing_rationale": "..."
    }
  ],
  "successful_count": 2,
  "errors": [],
  "error_count": 0
}
```

**Use case:** Submit 100 orders to the routing engine, get back ranked decisions + errors for any failed routes.

---

### 2. GET `/decisions` — Audit Trail with Filtering

Query historical routing decisions with optional filters by venue and timestamp.

**Query Parameters:**
- `venue` (optional): Filter by selected venue (e.g., `KALSHI`, `POLYMARKET`)
- `after` (optional): Only decisions created after this date (ISO 8601 format: `2026-03-13` or `2026-03-13T15:30:00Z`)

**Examples:**
```bash
# All routing decisions
curl http://localhost:8080/decisions

# Decisions routed to Polymarket after March 13
curl http://localhost:8080/decisions?venue=POLYMARKET&after=2026-03-13

# Decisions in last 24 hours (Kalshi only)
curl http://localhost:8080/decisions?venue=KALSHI&after=2026-03-14T00:00:00Z
```

**Response:**
```json
{
  "decisions": [
    {
      "decision_id": "rd-1234567890",
      "group_id": "grp-abc123",
      "order_request": {"market_id": "KALSHI:...", "side": "YES", "size": 100},
      "selected_venue": "POLYMARKET",
      "selected_market_id": "POLYMARKET:0xf5a1...",
      "scoring_breakdown": {...},
      "routing_rationale": "Polymarket has 0.06 tighter spread and $600K more liquidity",
      "cache_mode": false,
      "created_at": "2026-03-14T15:32:10Z"
    }
  ],
  "count": 47
}
```

**Use case:**
- Compliance: audit all routing decisions in a time window
- Backtesting: analyze what venues were selected and why
- Performance analysis: compare routing decisions across venues

---

### 3. GET `/groups/:id/history` — Pricing Time-Series

Get historical pricing data for all markets in an equivalence group.

**Example:**
```bash
curl http://localhost:8080/groups/grp-abc123/history
```

**Response:**
```json
{
  "group_id": "grp-abc123",
  "history": [
    {
      "timestamp": "2026-03-14T15:30:00Z",
      "members": {
        "KALSHI": {
          "raw": {
            "ticker": "KXBTC-100K",
            "yes_bid": 65,
            "yes_ask": 68,
            "yes_price": 0.665,
            "spread": 0.03,
            "liquidity": 234500
          }
        },
        "POLYMARKET": {
          "raw": {
            "condition_id": "0xf5a1...",
            "yes_price": 0.62,
            "spread": 0.04,
            "liquidity": 847000
          }
        }
      }
    }
  ],
  "count": 47
}
```

**Use case:**
- Arbitrage detection: spot pricing divergences between venues
- Liquidity tracking: see how liquidity evolved on each venue
- Research: analyze market structure and venue competitiveness

---

## Persistence

All routing decisions (from both `/route` and `/route/batch`) are automatically persisted to the `routing_decisions` SQLite table with:
- `decision_id` — unique identifier
- `group_id` — equivalence group link
- `order_request` — original order
- `selected_venue` — chosen venue
- `scoring_breakdown` — per-venue scores
- `created_at` — timestamp

This enables the audit trail (`/decisions`) endpoint to work without additional configuration.

---

## Testing

All new endpoints are covered by unit tests:
```bash
CGO_ENABLED=1 go test ./api -v
```

Includes:
- ✅ Batch routing with partial failures
- ✅ Audit trail filtering by venue
- ✅ Audit trail filtering by timestamp
- ✅ Group history retrieval
- ✅ Error handling for invalid inputs

---

## Integration Example

```python
import requests
import json

BASE = "http://localhost:8080"

# 1. Route multiple orders
orders = [
    {"market_id": "KALSHI:...", "side": "YES", "size": 100},
    {"market_id": "POLYMARKET:...", "side": "YES", "size": 100}
]
resp = requests.post(f"{BASE}/route/batch", json=orders)
decisions = resp.json()["decisions"]

# 2. Analyze routing decisions
for dec in decisions:
    print(f"Routed to {dec['selected_venue']} with score {dec['score']:.3f}")

# 3. Audit historical decisions
audit = requests.get(f"{BASE}/decisions?venue=POLYMARKET&after=2026-03-13").json()
print(f"Routed {audit['count']} orders to Polymarket since March 13")

# 4. Detect arbitrage opportunities
group = audit["decisions"][0]["group_id"]
history = requests.get(f"{BASE}/groups/{group}/history").json()

for snapshot in history["history"]:
    ts = snapshot["timestamp"]
    kalshi = snapshot["members"].get("KALSHI", {}).get("raw", {})
    poly = snapshot["members"].get("POLYMARKET", {}).get("raw", {})

    if kalshi and poly:
        price_diff = abs(kalshi["yes_price"] - poly["yes_price"])
        if price_diff > 0.05:
            print(f"{ts}: {price_diff:.2%} price divergence detected")
```

---

## Implementation Details

- **Batch routing:** Persists each decision independently; partial failures don't block successful routes
- **Audit trail:** Queryable by venue and date range; defaults to last 1000 decisions
- **Group history:** Returns raw market payloads from all ingestion cycles for the venues in that group
- **Performance:** All endpoints use indexed queries (venue, created_at, group_id) for O(log n) lookup

---

## Future Enhancements

- `GET /decisions/:id` — retrieve a single decision with full details
- `POST /decisions/search` — complex filtering (confidence ranges, settlement mechanism, flags)
- `GET /groups/:id/opportunities` — compute arbitrage spreads in real-time
- WebSocket `/decisions/stream` — live routing decision feed
