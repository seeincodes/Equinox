# Project Equinox

Cross-venue prediction market aggregation and intelligent order routing.

Equinox connects to Kalshi and Polymarket, normalizes markets into a canonical model, detects equivalent markets across venues using hybrid rule-based + embedding similarity, and simulates routing decisions with full scoring breakdowns.

## Repository Structure

```
backend/     Go API server + CLI
frontend/    React dashboard (Vite + TypeScript + Tailwind)
docs/        PRD, architecture docs, calculations reference
```

## Quick Start

### Prerequisites

- Go 1.22+ (with CGO enabled for SQLite)
- Node.js 20.19+
- OpenAI API key (for embedding-based matching)

### Backend

```bash
cd backend
cp .env.example .env   # Add your API keys
go build -o equinox .

# Seed a dashboard user
./equinox user add --email admin@example.com --password yourpassword --role admin

# Ingest, normalize, and match markets
./equinox ingest
./equinox normalize
./equinox match

# Start the API server
./equinox serve --addr :8080
```

### Frontend

```bash
cd frontend
npm install
npm run dev    # Opens on http://localhost:5173
```

The Vite dev server proxies `/api/*` requests to the Go backend at `:8080`.

### CLI Commands

| Command | Description |
|---------|-------------|
| `equinox ingest` | Fetch markets from Kalshi and Polymarket |
| `equinox normalize` | Transform raw data into canonical markets |
| `equinox match` | Detect equivalent markets across venues |
| `equinox route --market <id> --side YES --size 100` | Simulate a routing decision |
| `equinox serve` | Start the REST API server |
| `equinox status` | Health check all venue adapters |
| `equinox user add --email --password --role` | Add a dashboard user |
| `equinox user list` | List all dashboard users |

### Dashboard

The web dashboard provides:

- **Dashboard** — Live equivalence group cards with venue prices, confidence badges, and flags
- **Group Detail** — Side-by-side venue comparison, price history chart, routing simulation
- **Decisions** — Paginated audit log of all routing decisions with scoring breakdowns
- **Settings** (admin) — Configurable routing weights and confidence thresholds

### Roles

| Role | Browse | Route | Configure |
|------|--------|-------|-----------|
| Viewer | Yes | No | No |
| Analyst | Yes | Yes | No |
| Admin | Yes | Yes | Yes |

## API Endpoints

All endpoints are prefixed with `/api/`. Authentication required except `/api/health` and `/api/auth/login`.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/login` | Login with email/password |
| POST | `/api/auth/logout` | Destroy session |
| GET | `/api/auth/me` | Current user info |
| GET | `/api/markets` | List canonical markets |
| GET | `/api/groups` | List equivalence groups with embedded members |
| GET | `/api/groups/{id}/history` | Price history time-series |
| POST | `/api/route` | Simulate routing decision |
| POST | `/api/route/batch` | Batch routing simulation |
| GET | `/api/decisions` | Paginated audit trail |
| GET | `/api/config` | Current config (admin) |
| PUT | `/api/config` | Update config (admin) |
| GET | `/api/health` | System health check |

## Architecture

```
L1: Venue Adapters     → Fetch raw data from Kalshi + Polymarket APIs
L2: Normalization      → Transform to canonical market model
L3: Equivalence        → Detect matching markets (Jaccard + embeddings)
L4: Routing Engine     → Score venues and select optimal route
```

See [docs/PRD.md](docs/PRD.md) for the full product requirements document and [docs/CALCULATIONS.md](docs/CALCULATIONS.md) for scoring formulas.
