# AI Model Rollout Platform

A distributed backend platform for safely deploying new AI model versions through progressive traffic rollouts. The system shifts traffic incrementally from a stable model to a candidate model, publishes telemetry after every inference, and makes autonomous decisions to advance, hold, or roll back a rollout based on live error rates and latency.

Built as an incremental engineering project — each subsystem developed and verified in isolation before integrating into the whole.

---

## Architecture

```
  ┌─────────────────┐       ┌─────────────────┐
  │  Stress Tester  │       │    Dashboard    │  :5173
  └────────┬────────┘       └────────┬────────┘
           │ POST /v1/infer          │ GET /api/rollout
           │                         │ GET /api/events (SSE)
           │                         │ PUT /api/models/:id
           ▼                         ▼
  ┌─────────────────┐       ┌──────────────────────┐
  │  Edge Evaluator │  :4002│  Rollout Controller  │  :4003
  │  (TypeScript)   │       │  (Go)                │
  └──┬──────────┬───┘       │                      │
     │          │           │  ┌────────────────┐  │
fetch│          │ xAdd      │  │ ingestion      │  │
     │          ▼           │  │ batchlogger    │  │
     │   ┌──────────────────┐  │  │ guard (5s)     │  │
     │   │  Redis Streams   │  │  │ controller(2m) │  │
     │   │  telemetry:infer │  │  │ writer         │  │
     │   └──────────┬───────┘  │  │ api server     │  │
     │              │ XReadGroup│  └───────┬────────┘  │
     ▼              ▼           └──────────┼───────────┘
┌──────────────┐                          │
│ Model Service│            ┌─────────────┴──────────────┐
│ (TypeScript) │            │                            │
│  :4001       │            ▼                            ▼
└──────────────┘     ┌─────────────┐             ┌──────────────┐
                     │    Redis    │             │  PostgreSQL  │
                     │ feature flag│             │  rollouts    │
                     │ (read by    │             │  inference_  │
                     │  edge eval) │             │  events      │
                     └─────────────┘             │  rollout_    │
                                                 │  decisions   │
                                                 └──────────────┘
```

**Request flow:** Every inference request hits the Edge Evaluator, which reads the active feature flag from Redis, deterministically assigns the user to stable or candidate model traffic, forwards to the Model Service, and publishes an `InferenceCompletedEvent` to a Redis Stream.

**Control flow:** The Rollout Controller consumes the stream, evaluates metrics every 5 seconds (guard) and every 2 minutes (controller), and writes decisions back to both Postgres and Redis.

**Dashboard flow:** The Dashboard connects to the controller's SSE stream (`GET /events`) and receives a push event the moment any decision fires. All other data is fetched via REST and invalidated on each SSE push.

---

## Services

### Edge Evaluator — `apps/edge-evaluator` (TypeScript, port 4002)

- Validates incoming requests with Zod
- Reads the active rollout configuration from Redis
- Deterministic hash-based traffic assignment (`hashString(userId) % 100`)
- Forwards to Model Service and normalises the response
- Publishes `InferenceCompletedEvent` to Redis Streams (fire-and-forget)
- Falls back to the stable model when Redis is unreachable

### Model Service — `apps/model-service` (TypeScript, port 4001)

- Simulates model inference with configurable latency and failure rates
- Reads per-model config (`failureRate`, `minLatencyMs`, `maxLatencyMs`) from Redis, published by the Rollout Controller from Postgres — no redeploy needed to change simulated behavior
- Falls back to a built-in default profile (1% failure, 50–150ms) if Redis is unreachable or returns unparseable data; a genuinely unconfigured model version still 404s
- `model-v1` seeds at 1% failure rate, 50–150ms latency
- `model-v2` seeds at 2% failure rate, 50–200ms latency (steady scenario) — stays inside controller advance thresholds so the rollout can climb the full ladder
- To exercise the guard's rollback path mid-rollout, `PUT` a higher failure rate to `model-v2` (e.g. `0.35`) via the dashboard's Model configuration panel or `curl -X PUT localhost:4003/models/model-v2 -d '{"failureRate":0.35,"minLatencyMs":50,"maxLatencyMs":200}'`

### Dashboard — `apps/dashboard` (React + TypeScript, port 5173)

Real-time control panel for monitoring and operating a live rollout.

- **Status panel** — rollout ID, RUNNING/HELD badge, stable and candidate model versions, current candidate traffic percentage, 5-step advancement ladder
- **Metrics panel** — 2-minute window error rate (color-coded against advance/hold/rollback thresholds), P95 latency, window request count, total lifetime requests
- **Decision feed** — last 50 decisions from Postgres with ADVANCE/HOLD/ROLLBACK/COMPLETE badges, reason, source, and timestamp
- **Model configuration panel** — edit failure rate and latency range per model version; saves via `PUT /api/models/:id`, takes effect on the next inference request
- **Force rollback** — immediately clears candidate traffic, bypassing the guard and controller
- **Live updates via SSE** — the controller pushes an event on every state transition; the dashboard invalidates and refetches within milliseconds, no polling lag

```bash
npm run dev --workspace @rollout-platform/dashboard
# → http://localhost:5173
```

### Stress Tester — `apps/stress-tester` (TypeScript, CLI)

Local CLI tool for end-to-end load testing. Generates HTTP traffic against the Edge Evaluator, monitors rollout state changes via the Rollout Controller API, and prints a final report.

```bash
# From apps/stress-tester
npm start -- --mode=steady   # 50 RPS × 10 min — full advance ladder to COMPLETE
npm start -- --mode=burst    # 200 RPS × 30 s  — exercises guard rollback

# --reset: truncates inference_events + rollout_decisions and resets the
# rollout row to RUNNING at the appropriate starting percentage.
# Always restart the rollout controller after --reset to reload in-memory state.
npm start -- --mode=steady --reset
```

| Mode | RPS | Duration | Candidate % start | Expected outcome |
|------|-----|----------|-------------------|-----------------|
| `steady` | 50 | 10 min | 10% | Controller advances 10→25→50→75→100→COMPLETE, one step per 2-min window |
| `burst` | 200 | 30 s | 100% | Guard trips absolute window (>5% errors) within ~5 s → hold; fresh window (>30%) → rollback |

**Env vars** (all have sane defaults):

| Variable | Default |
|----------|---------|
| `EDGE_EVALUATOR_URL` | `http://localhost:4002` |
| `ROLLOUT_CONTROLLER_URL` | `http://localhost:4003` |
| `DATABASE_URL` | `postgres://localhost:5432/rollout_platform` |
| `ROLLOUT_ID` | `rollout-001` |

### Rollout Controller — `apps/rollout-controller` (Go, port 4003)

Single binary with six goroutines:

| Goroutine | Interval | Responsibility |
|-----------|----------|----------------|
| `ingestion` | continuous | Consumes Redis Stream via consumer group, writes to metrics store + batch logger buffer |
| `batchlogger` | 10s | Bulk-flushes inference events to `inference_events` via `COPY` |
| `guard` | 5s | Early rollback detection — fresh window (last 50 reqs) and absolute window |
| `controller` | 2 min | Evaluates error rate + P95 latency; advances or holds the rollout |
| `writer` | event-driven + 60s heartbeat | Sole owner of Redis and Postgres writes; re-seeds the Redis feature flag every 60s; broadcasts SSE events to connected dashboard clients |
| `api` | — | HTTP read/write API, SSE hub, manual rollback |

### Contracts — `packages/contracts` (TypeScript)

Shared Zod schemas for all service boundaries, organised into subdirectories: `inference/`, `rollout/`, `simulation/`, `telemetry/`.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language (services) | TypeScript 7, Go 1.24 |
| Runtime | Node.js 20+, Go toolchain |
| HTTP | Express 5, Go `net/http` |
| Frontend | React 19, Vite 6, Tailwind CSS 3, TanStack Query |
| Validation | Zod 4 |
| Caching / messaging | Redis 7 (Docker), Redis Streams |
| Database | PostgreSQL 18 |
| DB driver (Go) | `pgx/v5` with `pgxpool` |
| Migrations | `golang-migrate` (SQL files) |
| Testing | Vitest 4 |
| Monorepo | npm workspaces |
| Infrastructure | Docker Compose |

---

## Prerequisites

- **Node.js 20+** — Vitest 4 requires it (`node --version`)
- **Go 1.24+** — (`go version`)
- **Docker Desktop** — for Redis
- **PostgreSQL 18** — Postgres.app or any local install on port 5432

---

## Getting Started

### 1. Install dependencies

```bash
npm install
```

### 2. Start Redis

```bash
docker compose up -d redis
```

### 3. Create the database

In Postgres.app (or psql), create the database once:

```sql
CREATE DATABASE rollout_platform;
```

Migrations run automatically when the rollout controller starts.

### 4. Seed your first rollout

The rollout controller requires at least one `RUNNING` row in the `rollouts` table. Insert one before starting the controller:

```bash
psql postgresql://<your-mac-username>@localhost:5432/rollout_platform -c "
INSERT INTO rollouts (
    id, rollout_phase_id,
    stable_model_version_id, candidate_model_version_id,
    candidate_percentage, configuration_version,
    status, feature_flag_key
) VALUES (
    'rollout-001', 'phase-1',
    'model-v1', 'model-v2',
    10, 1,
    'RUNNING', 'feature-flag:model-routing:development'
);
"
```

### 5. Start the services

Each in its own terminal:

```bash
# Model Service
npm run dev --workspace @rollout-platform/model-service

# Edge Evaluator
npm run dev --workspace @rollout-platform/edge-evaluator

# Rollout Controller (run from its directory)
cd apps/rollout-controller && go run .

# Dashboard (optional — open http://localhost:5173)
npm run dev --workspace @rollout-platform/dashboard
```

---

## Environment Variables

### Edge Evaluator (`apps/edge-evaluator/.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4002` | HTTP port |
| `MODEL_SERVICE_URL` | `http://localhost:4001` | Model Service base URL |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL |
| `FEATURE_FLAG_KEY` | `feature-flag:model-routing:development` | Redis key for active rollout config |
| `TELEMETRY_STREAM_KEY` | `telemetry:inference-completed` | Redis Stream for telemetry events |
| `STABLE_MODEL_FALLBACK_ID` | `model-v1` | Model used when Redis is unreachable |

### Model Service (`apps/model-service/.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4001` | HTTP port |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL — source of per-model simulation config |

### Rollout Controller (environment or shell)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL |
| `DATABASE_URL` | `postgres://jakeredding@localhost:5432/rollout_platform` | Postgres connection URL |
| `MIGRATIONS_PATH` | `./migrations` | Path to SQL migration files |

---

## Example Request

```bash
curl -X POST http://localhost:4002/v1/infer \
  -H "Content-Type: application/json" \
  -d '{
    "requestId": "req-1",
    "userId": "user-42",
    "input": { "message": "Hello" }
  }'
```

**Success response:**
```json
{
  "requestId": "req-1",
  "success": true,
  "result": { "classification": "ACCOUNT_ACCESS" }
}
```

---

## Rollout Controller API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/rollout` | Current rollout state (model versions, percentage, held status) |
| `GET` | `/rollout/metrics` | Live metrics snapshot (error rates, P95 latency, window counts) |
| `GET` | `/rollout/decisions` | Last 50 decisions for the active rollout |
| `POST` | `/rollout/rollback` | Force an immediate rollback, bypassing guard/controller |
| `GET` | `/models` | List model configurations (failure rate, latency range) |
| `GET` | `/models/{id}` | Get a single model's configuration |
| `PUT` | `/models/{id}` | Update a model's failure rate and latency range — persists to Postgres, publishes to Redis, broadcasts an SSE event |
| `GET` | `/events` | SSE stream — pushes `advance`, `hold`, `rollback`, `complete` events to connected clients |

---

## Rollout Lifecycle

### Traffic advancement ladder

`10% → 25% → 50% → 75% → 100% → COMPLETE`

The controller advances one step per successful 2-minute evaluation window.

### Guard thresholds (evaluated every 5s, minimum 50 requests)

| Window | Metric | Default threshold | Action on breach |
|--------|--------|-------------------|-----------------|
| Fresh (last 50 reqs) | Error rate | > 30% | Immediate rollback |
| Absolute (all retained) | Error rate | > 5% | Hold (block advancement) |

### Controller thresholds (evaluated every 2 minutes)

| Metric | Default threshold | Action on breach |
|--------|-------------------|-----------------|
| Error rate | > 2% | Hold |
| P95 latency | > 250ms | Hold |

**Guard always wins.** When the guard issues a rollback and the controller would advance, the hold flag blocks the advance. Only a manual rollback clears a guard-triggered hold.

### Holds require manual intervention

A hold does not automatically clear when metrics recover. A held rollout can only be unstuck via `POST /rollout/rollback` (which rolls back to the stable model). Restarting from a good baseline requires a new rollout row.

---

## Redis Resilience

Redis is the control plane between the rollout controller and the Edge Evaluator.

- **Startup seeding** — on boot, the rollout controller writes the current feature flag from Postgres to Redis immediately.
- **60-second heartbeat** — the writer goroutine re-publishes the feature flag every 60 seconds, so Redis recovers automatically within one heartbeat cycle after a restart.
- **Edge Evaluator fallback** — if Redis is completely unreachable (connection failure, not just a missing key), the Edge Evaluator routes all traffic to `STABLE_MODEL_FALLBACK_ID` and publishes telemetry with null rollout fields. Clients receive a valid inference response rather than an error.

---

## Testing

```bash
# Unit tests
npm test --workspace @rollout-platform/edge-evaluator

# Integration tests (requires Redis)
npm run test:integration --workspace @rollout-platform/edge-evaluator

# Go build + vet
cd apps/rollout-controller && go build ./... && go vet ./...
```

> **Note:** Vitest 4 requires Node.js 20+. Run `node --version` to confirm.

---

## Database Schema

### `rollouts`
Stores the rollout configuration and current lifecycle status. Policy fields (thresholds, window sizes) are inlined per row — multiple concurrent rollouts can carry independent policies. The `status` column drives what the controller evaluates: only `RUNNING` and `HELD` rows are loaded.

### `inference_events`
One row per inference, written by the batch logger in 10-second bulk flushes. Indexed on `(rollout_id, occurred_at)` for the time-window queries the guard and controller issue.

### `rollout_decisions`
Append-only audit log of every decision the guard, controller, or manual API makes. Fields: `action` (HOLD / ROLLBACK / ADVANCE / COMPLETE), `reason`, `source` (guard / controller / manual), `decided_at`.

### `model_configurations`
Per-model-version simulation params (`failure_rate`, `min_latency_ms`, `max_latency_ms`), edited via the Rollout Controller's `/models/{id}` API. Published to Redis on every write so Model Service picks up changes without a restart.

---

## Roadmap

- [x] TypeScript monorepo with npm workspaces
- [x] Shared contracts package with Zod schemas
- [x] Model Service with simulated inference
- [x] Edge Evaluator — request validation, traffic assignment, Redis routing
- [x] Telemetry event publishing to Redis Streams
- [x] Go rollout controller — ingestion, guard, controller, writer, API
- [x] PostgreSQL persistence — migrations, batch logger, decision audit log
- [x] Redis resilience — startup seeding, heartbeat, Edge Evaluator fallback
- [x] Stress tester — steady advance and burst guard-rollback scenarios, live monitor, final report
- [x] React dashboard — live status panel, metrics panel, decision feed, SSE-driven updates, force rollback
- [x] Dynamic model configuration — Postgres-backed model params, mid-rollout config changes via dashboard
- [ ] Management API — create and configure rollouts via HTTP (no psql required)
- [ ] Promotion loop — on COMPLETE, promote candidate to stable and prepare next rollout slot
- [ ] Multi-tenant rollouts (per-user model pairs)
- [ ] Authentication
