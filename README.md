# AI Model Rollout Platform

A distributed backend platform for safely deploying new AI model versions through progressive traffic rollouts. The system shifts traffic incrementally from a stable model to a candidate model, publishes telemetry after every inference, and makes autonomous decisions to advance, hold, or roll back a rollout based on live error rates and latency.

Built as an incremental engineering project — each subsystem developed and verified in isolation before integrating into the whole.

---

## Architecture

```
                    ┌─────────────────┐
                    │  Stress Tester  │
                    └────────┬────────┘
                             │ POST /v1/infer
                             ▼
                    ┌─────────────────┐
                    │  Edge Evaluator │  :4002
                    │  (TypeScript)   │
                    └──┬──────────┬───┘
                       │          │
              fetch    │          │ xAdd
              to model │          ▼
                       │   ┌─────────────────────┐
                       │   │  Redis Streams       │
                       │   │  telemetry:inference │
                       │   └──────────┬──────────┘
                       │              │ XReadGroup
                       ▼              ▼
              ┌──────────────┐  ┌──────────────────────┐
              │ Model Service│  │  Rollout Controller  │  :4003
              │ (TypeScript) │  │  (Go)                │
              │  :4001       │  │                      │
              └──────────────┘  │  ┌────────────────┐  │
                                │  │ ingestion      │  │
                                │  │ batchlogger    │  │
                                │  │ guard (5s)     │  │
                                │  │ controller(2m) │  │
                                │  │ writer         │  │
                                │  │ api server     │  │
                                │  └───────┬────────┘  │
                                └──────────┼───────────┘
                                           │
                          ┌────────────────┴───────────────┐
                          │                                │
                          ▼                                ▼
                   ┌─────────────┐                ┌──────────────┐
                   │    Redis    │                │  PostgreSQL  │
                   │ feature flag│                │  rollouts    │
                   │ (read by    │                │  inference_  │
                   │  edge eval) │                │  events      │
                   └─────────────┘                │  rollout_    │
                                                  │  decisions   │
                                                  └──────────────┘
```

**Request flow:** Every inference request hits the Edge Evaluator, which reads the active feature flag from Redis, deterministically assigns the user to stable or candidate model traffic, forwards to the Model Service, and publishes an `InferenceCompletedEvent` to a Redis Stream.

**Control flow:** The Rollout Controller consumes the stream, evaluates metrics every 5 seconds (guard) and every 2 minutes (controller), and writes decisions back to both Postgres and Redis.

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
- `model-v1`: 1% failure rate, 50–150ms latency
- `model-v2` (steady scenario): 2% failure rate, 50–200ms latency — stays inside controller advance thresholds so the rollout can climb the ladder
- `model-v2` (burst/rollback scenario): temporarily set `failureRate: 0.35` in `apps/model-service/src/config/models.ts` and restart the service to exercise the guard's rollback path

### Stress Tester — `apps/stress-tester` (TypeScript, CLI)

Local CLI tool for end-to-end load testing. Generates HTTP traffic against the Edge Evaluator, monitors rollout state changes via the Rollout Controller API, and prints a final report.

```bash
# From apps/stress-tester
npm start -- --mode=steady   # 50 RPS × 5 min — exercises advance → hold
npm start -- --mode=burst    # 200 RPS × 30 s — exercises guard rollback

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
| `writer` | event-driven + 60s heartbeat | Sole owner of Redis and Postgres writes; re-seeds the Redis feature flag every 60s |
| `api` | — | HTTP read API + manual rollback |

### Contracts — `packages/contracts` (TypeScript)

Shared Zod schemas for all service boundaries, organised into subdirectories: `inference/`, `rollout/`, `simulation/`, `telemetry/`.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language (services) | TypeScript 7, Go 1.24 |
| Runtime | Node.js 20+, Go toolchain |
| HTTP | Express 5 |
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
| `POST` | `/rollout/rollback` | Force an immediate rollback, bypassing guard/controller |

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

### In-progress rollouts are immutable

Config cannot be changed while a rollout is `RUNNING` or `HELD`. The only action available on a live rollout is **Force Rollback** (`POST /rollout/rollback`). To reconfigure, let the rollout complete or roll back, then insert a new row into `rollouts`.

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
- [ ] React dashboard with rollout control panel
- [ ] Management API (create / configure rollouts via HTTP)
- [ ] Multi-tenant rollouts (per-user model pairs)
- [ ] Authentication
