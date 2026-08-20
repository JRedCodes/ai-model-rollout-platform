# AI Model Rollout Platform

A distributed backend platform for safely deploying new AI model versions through progressive traffic rollouts. The system shifts traffic incrementally from a stable model to a candidate model, publishes telemetry after every inference, and makes autonomous decisions to advance, hold, resume, or roll back a rollout based on live error rates and latency.

Built as an incremental engineering project — each subsystem developed and verified in isolation before integrating into the whole.

---

## Architecture

```
        ┌───────────────┐                         ┌───────────┐
        │ Stress Tester │                         │ Dashboard │ :5173
        └───────────────┘                         └───────────┘
                │ POST /v1/infer                        │ GET /api/rollout
                │                                       │ GET /api/models
                │                                       │ PUT /api/models/:id
                │                                       │ GET /api/events (SSE)
                │                                       │ POST /api/rollouts
                │                                       │
                ▼                                       ▼
        ┌────────────────┐                        ┌────────────────────┐
        │ Edge Evaluator │ :4002                  │ Rollout Controller │ :4003
        │ (TypeScript)   │                        │ (Go)               │
        └──┬──────────┬──┘                        │                    │
     fetch │          │ xAdd                      │ ingestion          │
           │          ▼                           │ batchlogger        │
           │        ┌─────────────────┐ XReadGroup│ guard (5s)         │
           │        │ Redis Streams   │──────────►│ controller (2m)    │
           │        │ telemetry:infer │           │ writer             │
           │        └─────────────────┘           │ api server         │
           │                                      └────────────────────┘
           ▼                                                 │
        ┌───────────────┐ reads model-config          ┌──────┴──────────────────────┐
        │ Model Service │──────────────►              ▼                             ▼
        │ (TypeScript)  │               ┌───────────────────────────┐   ┌──────────────────────┐
        │ :4001         │               │ Redis                     │   │ PostgreSQL           │
        └───────────────┘               │ feature flag  (edge eval) │   │ rollouts             │
                                        │ model config  (model svc) │   │ inference_events     │
                                        └───────────────────────────┘   │ rollout_decisions    │
                                                                        │ model_configurations │
                                                                        └──────────────────────┘
```

**Request flow:** Every inference request hits the Edge Evaluator, which reads the active feature flag from Redis, deterministically assigns the user to stable or candidate model traffic, and forwards to the Model Service. The Model Service reads that model version's simulation config (failure rate, latency range) from Redis before responding, then the Edge Evaluator publishes an `InferenceCompletedEvent` to a Redis Stream.

**Control flow:** The Rollout Controller consumes the stream, evaluates metrics every 5 seconds (guard) and every 2 minutes (controller), and writes decisions back to both Postgres and Redis.

**Dashboard flow:** The Dashboard connects to the controller's SSE stream (`GET /events`) and receives a push event the moment any decision fires. All other data is fetched via REST and invalidated on each SSE push.

**Multi-tenancy:** Every tenant is fully independent — its own rollout, guard/controller pipeline, model catalog, and SSE stream. The tenant's API key is what identifies it: every request (except `POST /tenants` and `GET /health`) carries `Authorization: Bearer <tenant-api-key>`, and the Rollout Controller and Edge Evaluator both resolve the tenant from that key rather than trusting anything the caller claims. A `tenants` table (Postgres, source of truth) and a `tenant-auth:<key-hash> → tenantId` cache (Redis, published by the Rollout Controller so the Edge Evaluator can authenticate without direct Postgres access) sit alongside the diagram above.

---

## Services

### Edge Evaluator — `apps/edge-evaluator` (TypeScript, port 4002)

**Deep dive:** [`documents/EDGE_EVALUATOR.md`](documents/EDGE_EVALUATOR.md)

- Requires `Authorization: Bearer <tenant-api-key>` on every request — resolved to a tenant ID via a Redis lookup (`tenant-auth:<key-hash>`), with an in-memory cache of already-resolved keys so a tenant that's already serving traffic keeps working through a brief Redis outage
- Validates incoming requests with Zod
- Reads the authenticated tenant's rollout configuration from Redis (`feature-flag:model-routing:<tenantId>`)
- Deterministic hash-based traffic assignment (`hashString(userId) % 100`)
- Forwards to Model Service (with the tenant ID) and normalises the response
- Publishes `InferenceCompletedEvent` (tagged with the tenant ID) to Redis Streams (fire-and-forget)
- Falls back to the stable model when Redis is unreachable (after auth already succeeded)

### Model Service — `apps/model-service` (TypeScript, port 4001)

**Deep dive:** [`documents/MODEL_SERVICE.md`](documents/MODEL_SERVICE.md)

- Simulates model inference with configurable latency and failure rates
- Reads per-tenant, per-model config (`failureRate`, `minLatencyMs`, `maxLatencyMs`) from Redis (`model-config:<tenantId>:<modelVersionId>`), published by the Rollout Controller from Postgres — no redeploy needed to change simulated behavior, and one tenant's model catalog is fully independent of another's
- Falls back to a built-in default profile (1% failure, 50–150ms) if Redis is unreachable or returns unparseable data; a genuinely unconfigured model version still 404s
- Every new tenant is seeded with `model-v1` (1% failure rate, 50–150ms) and `model-v2` (2% failure rate, 50–200ms — stays inside controller advance thresholds so a rollout can climb the full ladder) at creation time
- To exercise the guard's rollback path mid-rollout, `PUT` a higher failure rate to `model-v2` (e.g. `0.35`) via the dashboard's Model configuration panel or `curl -X PUT localhost:4003/models/model-v2 -H "Authorization: Bearer <key>" -d '{"failureRate":0.35,"minLatencyMs":50,"maxLatencyMs":200}'`

### Dashboard — `apps/dashboard` (React + TypeScript, port 5173)

**Deep dive:** [`documents/DASHBOARD.md`](documents/DASHBOARD.md)

Real-time control panel for monitoring and operating a live rollout.

- **Sign-up / sign-in gate** — the app renders nothing until you're authenticated: create an account (issues a tenant + a one-time-shown API key) or sign in, backed by an httpOnly session cookie. A legacy "use an existing API key" path still exists for scripted/demo use (e.g. the seeded demo tenant below) — that key is persisted to `localStorage` and attached as a Bearer token, and a `401` anywhere clears it and falls back to the gate automatically
- **Status panel** — rollout ID, RUNNING/HELD badge, stable and candidate model versions, current candidate traffic percentage, 5-step advancement ladder
- **Metrics panel** — 2-minute window error rate (color-coded against advance/hold/rollback thresholds), P95 latency, window request count, total lifetime requests
- **Decision feed** — last 50 decisions from Postgres with ADVANCE/HOLD/RESUME/ROLLBACK/COMPLETE badges, reason, source, and timestamp
- **Model configuration panel** — edit failure rate and latency range per model version; saves via `PUT /api/models/:id`, takes effect on the next inference request
- **Force rollback** — immediately clears candidate traffic, bypassing the guard and controller
- **Live updates via SSE** — the controller pushes an event on every state transition; the dashboard invalidates and refetches within milliseconds, no polling lag

```bash
npm run dev --workspace @rollout-platform/dashboard
# → http://localhost:5173
```

### Stress Tester — `apps/stress-tester` (TypeScript, CLI)

**Deep dive:** [`documents/STRESS_TESTER.md`](documents/STRESS_TESTER.md)

Local CLI tool for end-to-end load testing. Generates HTTP traffic against the Edge Evaluator, monitors rollout state changes via the Rollout Controller API, and prints a final report.

```bash
# From apps/stress-tester
npm start -- --mode=steady   # 50 RPS × 10 min — full advance ladder to COMPLETE
npm start -- --mode=burst    # 200 RPS × 30 s  — exercises guard rollback

# --apiKey pairs this run with a specific tenant; defaults to the seeded
# demo tenant's key. Run two instances with different --apiKey values to
# exercise two tenants' rollouts independently.
npm start -- --mode=steady --apiKey=tk_...

# --reset: truncates inference_events + rollout_decisions and resets that
# tenant's active rollout to RUNNING at the appropriate starting percentage
# (resolved via GET /rollout with --apiKey, not a fixed ID).
# Always restart the rollout controller after --reset -- it modifies an
# already-active rollout's row in place via SQL, which the supervisor's
# change detection (rollout ID appearing/changing/disappearing) doesn't see.
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
| `TENANT_API_KEY` | seeded demo tenant's key (same as `--apiKey`) |

### Rollout Controller — `apps/rollout-controller` (Go, port 4003)

**Deep dive:** [`documents/ROLLOUT_CONTROLLER.md`](documents/ROLLOUT_CONTROLLER.md)

Single binary. Four goroutines run for the process's whole life, independent of which tenants (if any) currently have an active rollout:

| Goroutine | Interval | Responsibility |
|-----------|----------|----------------|
| `api` | — | HTTP read/write API, per-tenant SSE hubs, manual rollback |
| `batchlogger` | 10s | Bulk-flushes inference events to `inference_events` via `COPY` (shared across all tenants) |
| `modelConfigSeeder` | heartbeat | Re-publishes every tenant's model configurations from Postgres to Redis |
| `tenantSeeder` | heartbeat | Re-publishes every tenant's `tenant-auth:<key-hash> → tenantId` entry to Redis |

A **supervisor loop** in `main.go` is a reconciliation loop, not a single-rollout cycle: every ~5s it diffs Postgres's current set of active `(tenant, rollout)` pairs against which tenants actually have a pipeline running, tears down pipelines whose tenant went idle or moved to a different rollout, and starts pipelines for anything newly active. Multiple tenants' pipelines run concurrently, each with four goroutines of its own:

| Goroutine | Interval | Responsibility |
|-----------|----------|----------------|
| `ingestion` | continuous | Consumes the shared Redis Stream via a **per-tenant** consumer group, discarding any event whose `rolloutId` isn't this tenant's (every group independently sees the entire stream — Redis Streams consumer groups have no server-side content filtering) |
| `guard` | 5s | Early rollback detection — fresh window (last 50 reqs) and absolute window |
| `controller` | 2 min | Evaluates error rate + P95 latency; advances, holds, or resumes the rollout |
| `writer` | event-driven + 60s heartbeat | Sole owner of this tenant's Redis and Postgres writes; re-seeds its feature flag every 60s; broadcasts SSE events to its own tenant's connected dashboard clients only |

When a tenant has no active rollout, its pipeline simply doesn't exist — `GET /rollout` and `GET /rollout/metrics` respond with `{"active": false}` for that tenant until it creates one via `POST /rollouts`. Which tenant a request is asking about is always resolved from its `Authorization` header, never from anything in the URL or body.

### Contracts — `packages/contracts` (TypeScript)

**Deep dive:** [`documents/CONTRACTS.md`](documents/CONTRACTS.md)

Shared Zod schemas for all service boundaries, organised into subdirectories: `inference/`, `rollout/`, `simulation/`, `telemetry/`.

---

## Documentation

The summaries above cover what each service does; the `documents/` directory covers how, in depth — low-level design per module/file, the reasoning behind non-obvious decisions, and exactly what's test-covered and what isn't:

- [`documents/EDGE_EVALUATOR.md`](documents/EDGE_EVALUATOR.md)
- [`documents/MODEL_SERVICE.md`](documents/MODEL_SERVICE.md)
- [`documents/ROLLOUT_CONTROLLER.md`](documents/ROLLOUT_CONTROLLER.md)
- [`documents/DASHBOARD.md`](documents/DASHBOARD.md)
- [`documents/STRESS_TESTER.md`](documents/STRESS_TESTER.md)
- [`documents/CONTRACTS.md`](documents/CONTRACTS.md)
- [`documents/DEPLOYMENT.md`](documents/DEPLOYMENT.md) — the AWS deployment plan; not yet executed, see its own Open Items

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

### 4. Start the services

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

The rollout controller no longer requires a pre-existing `rollouts` row — it starts up idle and waits for one. A migration also seeds one **demo tenant** (id `tenant-demo`, API key `tk_demo_2218a6e29efe8f4b3378390b46a0710d` — dev-only, documented here on purpose) so you can skip straight to step 6 without bootstrapping anything.

### 5. Create a tenant (optional — skip to use the seeded demo tenant)

```bash
curl -X POST localhost:4003/tenants \
  -H "X-Admin-Key: dev-admin-key" \
  -d '{"name": "Acme Corp"}'
# → {"id": "...", "name": "Acme Corp", "apiKey": "tk_..."}
```

`X-Admin-Key` is a single shared secret (`ADMIN_API_KEY`, defaults to `dev-admin-key`) — distinct from the per-tenant key this returns. The `apiKey` in the response is shown exactly once; only its hash is ever stored. Two default model configs (`model-v1`, `model-v2`) are seeded for the new tenant automatically.

### 6. Create your first rollout

```bash
curl -X POST localhost:4003/rollouts \
  -H "Authorization: Bearer tk_demo_2218a6e29efe8f4b3378390b46a0710d" \
  -d '{
    "rolloutPhaseId": "phase-1",
    "stableModelVersionId": "model-v1",
    "candidateModelVersionId": "model-v2",
    "candidatePercentage": 10
  }'
```

Swap in your own tenant's key from step 5 if you created one. Or use the dashboard's create-rollout form (after entering that key at its API key gate), shown automatically whenever no rollout is active. The controller picks it up within ~5 seconds — no restart needed.

---

## Environment Variables

### Edge Evaluator (`apps/edge-evaluator/.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4002` | HTTP port |
| `MODEL_SERVICE_URL` | `http://localhost:4001` | Model Service base URL |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL |
| `FEATURE_FLAG_KEY_PREFIX` | `feature-flag:model-routing:` | Prefix + tenant ID = the Redis key read for that tenant's rollout config |
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
| `FEATURE_FLAG_KEY_PREFIX` | `feature-flag:model-routing:` | Prefix used when computing a new rollout's feature flag key — must match the Edge Evaluator's `FEATURE_FLAG_KEY_PREFIX` |
| `ADMIN_API_KEY` | `dev-admin-key` | Shared secret gating `POST /tenants` — distinct from any per-tenant key |

---

## Example Request

```bash
curl -X POST http://localhost:4002/v1/infer \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer tk_demo_2218a6e29efe8f4b3378390b46a0710d" \
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

Every route below is scoped to whichever tenant `Authorization: Bearer <key>` resolves to — never to anything a caller specifies separately. Two exceptions have their own auth: `POST /tenants` is gated by `X-Admin-Key` instead, and `GET /events` (browsers' `EventSource` can't send custom headers) also accepts the key as an `?api_key=` query param.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — unauthenticated |
| `POST` | `/tenants` | Create a tenant — `X-Admin-Key`, not a tenant key. Returns the plaintext API key once; seeds two default model configs |
| `GET` | `/rollout` | Current rollout state (model versions, percentage, held status), or `{"active": false}` if none is active |
| `GET` | `/rollout/metrics` | Live metrics snapshot (error rates, P95 latency, window counts), or `{"active": false}` |
| `GET` | `/rollout/decisions` | Last 50 decisions for the active rollout (`[]` if none) |
| `POST` | `/rollout/rollback` | Force an immediate rollback, bypassing guard/controller — `409` if no rollout is active |
| `GET` | `/rollouts` | List all of this tenant's rollouts, newest first |
| `GET` | `/rollouts/{id}` | Get a single rollout by ID — `404` if it belongs to another tenant |
| `POST` | `/rollouts` | Create a rollout — `409` if this tenant already has one `RUNNING`/`HELD`. `stableModelVersionId` is optional; omitted, it defaults to the most recently `COMPLETED` rollout's candidate |
| `GET` | `/models` | List this tenant's model configurations (failure rate, latency range) |
| `GET` | `/models/{id}` | Get a single model's configuration |
| `PUT` | `/models/{id}` | Update a model's failure rate and latency range — persists to Postgres, publishes to Redis, broadcasts an SSE event |
| `GET` | `/events` | SSE stream — pushes `advance`, `hold`, `resume`, `rollback`, `complete` events to this tenant's connected clients only |

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

**Guard always wins.** When the guard issues a rollback, the held flag blocks any advance. Recovery requires creating a new rollout via `POST /rollouts`.

### Completion promotes the winner

Reaching 100% and staying healthy fires `COMPLETE`. The rollout's candidate is promoted to stable in-memory before the feature flag is cleared, so traffic keeps flowing to the model that won — completing a rollout locks in the candidate, it doesn't revert to the pre-rollout model. The controller then goes idle (see below) until the next rollout is created.

### No active rollout is a valid state

The controller doesn't require a rollout to be running. On boot, or any time none is `RUNNING`/`HELD` (e.g. right after a `COMPLETE`), it idles: `GET /rollout` and `GET /rollout/metrics` return `{"active": false}`, and a supervisor loop polls Postgres every ~5s for a newly created rollout. Creating one via `POST /rollouts` — with `stableModelVersionId` left unset to default to the last completed rollout's candidate — is how a completed rollout's "next slot" gets filled; the controller picks it up within one poll cycle, no restart required.

### HOLD recovers automatically; ROLLBACK doesn't

A `HOLD` isn't sticky — the controller keeps evaluating metrics through it instead of freezing. If a full 2-minute window comes back clean (error rate ≤ 2%, P95 ≤ 250ms), it fires `RESUME`: the hold clears and the rollout returns to `RUNNING` at its *current* percentage, without also advancing in that same cycle. A second consecutive clean window is what actually advances to the next step — one window proves it's safe again, a second earns the next step. This applies whether the guard or the controller raised the hold. The guard's 5s checks keep running independently throughout, so a resume that turns out to be premature gets caught and re-held within seconds rather than sitting on a false "recovered" status for a full 2-minute cycle.

`ROLLBACK` is different and stays fully manual: once `ROLLED_BACK`, fixing the underlying cause does not bring a rollout back on its own. Recovery requires a new rollout row — a rollback is treated as a hard stop that deserves a human decision, not something metrics alone should walk back.

---

## Redis Resilience

Redis is the control plane between the rollout controller and the Edge Evaluator — now including authentication, not just routing config.

- **Startup seeding** — on boot, the rollout controller writes every tenant's feature flag, model configs, and auth entry from Postgres to Redis immediately.
- **60-second heartbeats** — `writer` re-publishes each tenant's feature flag, and `modelConfigSeeder`/`tenantSeeder` re-publish model configs and auth entries, so Redis recovers automatically within one heartbeat cycle after a restart or flush.
- **Edge Evaluator auth cache** — since authentication is now a Redis read, a resolved `key → tenantId` mapping is cached in memory after first success. A tenant already serving traffic keeps working through a *transient* Redis outage; a brand-new tenant's very first request during one still fails, since there's nothing cached yet to fall back to.
- **Edge Evaluator model-routing fallback** — if Redis is unreachable for the feature-flag lookup specifically (auth having already succeeded, from cache or otherwise), the Edge Evaluator routes that tenant's traffic to `STABLE_MODEL_FALLBACK_ID` and publishes telemetry with null rollout fields. Clients receive a valid inference response rather than an error.

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

### `tenants`
`id`, `name`, `api_key_hash` (SHA-256 of the plaintext key — never stored), `created_at`/`updated_at`. The plaintext key is generated once, returned in `POST /tenants`'s response, and never persisted or retrievable again.

### `rollouts`
Stores the rollout configuration and current lifecycle status, scoped by `tenant_id`. Policy fields (thresholds, window sizes) are inlined per row, so each rollout could in principle carry its own policy — though today's Management API doesn't expose overriding them yet. Only one rollout per tenant may be `RUNNING`/`HELD` at a time, enforced by a partial unique index on `tenant_id` (`rollouts_single_active_per_tenant_idx`); the `status` column drives what the supervisor loop's reconciler loads.

### `inference_events`
One row per inference, written by the batch logger in 10-second bulk flushes. Indexed on `(rollout_id, occurred_at)` for the time-window queries the guard and controller issue. Shared across all tenants — isolation is by `rollout_id`, not a separate `tenant_id` column, since every rollout already belongs to exactly one tenant.

### `rollout_decisions`
Append-only audit log of every decision the guard, controller, or manual API makes. Fields: `action` (HOLD / RESUME / ROLLBACK / ADVANCE / COMPLETE), `reason`, `source` (guard / controller / manual), `decided_at`.

### `model_configurations`
Per-tenant, per-model-version simulation params (`failure_rate`, `min_latency_ms`, `max_latency_ms`), edited via the Rollout Controller's `/models/{id}` API. Primary key is `(tenant_id, model_version_id)` — two tenants can each have their own independent "model-v1" with entirely different simulated behavior. Published to Redis on every write so Model Service picks up changes without a restart.

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
- [x] Management API — create and configure rollouts via HTTP (no psql required)
- [x] Promotion loop — on COMPLETE, promote candidate to stable and prepare next rollout slot
- [x] Multi-tenant rollouts (per-user model pairs)
- [x] Authentication
