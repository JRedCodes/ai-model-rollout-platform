# Edge Evaluator

`apps/edge-evaluator` — TypeScript, Express, port 4002.

The entry point for every simulated inference request. Authenticates the caller, decides stable vs. candidate traffic, forwards to the Model Service, and publishes telemetry — the first four things that happen to any request in this system happen here.

---

## Role in the system

Everything downstream depends on what this service decides. It never runs a model itself; it resolves _which_ model version a request should hit (via the tenant's feature flag in Redis) and hands off to the Model Service to actually simulate a response. It has no direct Postgres access at all — both its authentication and its routing configuration are Redis reads, published there by the Rollout Controller, which is the only thing with a database connection in this system. That indirection is deliberate: this service can authenticate and route traffic even if the Rollout Controller's database is briefly unreachable, as long as Redis itself is up.

---

## Low-level design

### `app.ts` + `server.ts` — wiring

`app.ts` is two lines of routing: `GET /health` (unauthenticated), and `POST /v1/infer` behind `requireTenantAuth` (`app.use("/v1", requireTenantAuth, inferenceRouter)` — the middleware runs for the whole `/v1` prefix, not just this one route, so any future route added under `/v1` inherits the auth gate for free). `server.ts` connects Redis before binding the HTTP port at all (`await connectRedis()` ahead of `app.listen`) and exits the process on failure — this service refuses to come up without Redis, unlike the request-time resilience described below, which only covers Redis going away _after_ a successful boot.

### `config/env.ts` — configuration

All env vars with inline defaults, read once at import time (see [Configuration](#configuration)).

### `middleware/auth.middleware.ts` — `requireTenantAuth`

Reads `Authorization: Bearer <key>` and nothing else — no cookie parsing, no session awareness of any kind. This matters beyond this service: when `feat/auth` added real sign-up/sign-in with session cookies to the dashboard and Rollout Controller, this middleware was the thing that proved that work couldn't leak into the traffic-serving path — confirmed by reading this file before writing `tenant-auth.repository.integration.test.ts` (below), and it's still true. A missing bearer token is a `401 MISSING_BEARER_TOKEN` before `resolveTenantId` is even called; an invalid one is `401 INVALID_API_KEY` from inside the `try`; a Redis outage during resolution is `503 AUTH_UNAVAILABLE`, distinguishing "you're not allowed in" from "we can't tell right now." On success, `req.tenantId` is set (a global `Express.Request` interface augmentation right in this file) for `inference.route.ts` to read.

### `repositories/tenant-auth.repository.ts` — `resolveTenantId`

Hashes the presented key (SHA-256, matching the Go side's `internal/token.Hash`) and reads `tenant-auth:<hash>` from Redis — populated by the Rollout Controller's `tenant.Seeder`, republished on a heartbeat (see `documents/ROLLOUT_CONTROLLER.md`). The one piece of state this service keeps beyond a single request: `resolvedTenantCache`, a plain in-memory `Map<hash, tenantId>` that remembers every key that has _ever_ resolved successfully in this process's lifetime. If a later Redis read throws, a cache hit is returned instead of propagating the error — a tenant already serving traffic keeps working through a transient Redis outage. A key that has never been seen before during an outage has nothing to fall back to and still fails with `RedisUnavailableError`. This is the same behavior the README's Redis Resilience section describes; this file is where it's actually implemented.

### `repositories/feature-flag.repository.ts` — `getActiveFeatureFlag`

Reads `feature-flag:model-routing:<tenantId>` (prefix from `env.FEATURE_FLAG_KEY_PREFIX`, must match the Rollout Controller's own copy of that prefix), parses it against `featureFlagSchema` from `@rollout-platform/contracts`, and throws one of three typed errors depending on exactly what went wrong: `RedisUnavailableError` (the read itself failed), `FeatureFlagNotFoundError` (key doesn't exist — no rollout ever created for this tenant, or one just hasn't been seeded yet), `InvalidFeatureFlagError` (the stored value doesn't parse as JSON, or doesn't match the schema). `inference.route.ts` handles all three differently — see below.

### `services/traffic-assignment.service.ts` — `selectModel`

Pure function, no I/O: `hashString(userId) % 100` bucketed against `flag.candidatePercentage`. Deterministic on purpose — the same user always lands in the same bucket for a given flag, so a rollout's traffic split doesn't flicker per-request for one person. Returns the stable model outright if there's no candidate configured or the candidate percentage is 0, without needing to hash at all.

### `clients/model-service.client.ts` — `requestModelInference`

A `fetch` to the Model Service's `/v1/models/:id/infer`, 2-second timeout (`AbortSignal.timeout`), response validated against `modelInferenceResponseSchema`. Any non-2xx or schema mismatch throws — `inference.route.ts` treats every failure mode here identically (`MODEL_SERVICE_UNAVAILABLE`), it doesn't distinguish a timeout from a 500 from a malformed body.

### `services/telemetry.service.ts` — `publishInferenceEvent`

Builds an `InferenceCompletedEvent` (schema versioned, `schemaVersion: 1`) and `XADD`s it to the shared Redis Stream (`env.TELEMETRY_STREAM_KEY`) — fire-and-forget, a rejected promise is only logged, never propagated to the caller. A client's inference response is never held up by, or failed because of, telemetry publishing. `assignment` (`"stable" | "candidate"`) is derived here by comparing the model that was actually used against the feature flag's stable ID, not passed in — so it stays correct even on the stable-fallback path below, where there effectively is no "candidate" for this request.

### `routes/inference.route.ts` — the actual handler

`POST /v1/infer` in order: validate the body against `edgeInferenceRequestSchema` → read the tenant's feature flag → `selectModel` → forward to the Model Service → publish telemetry → shape the response. The `catch` block is where the interesting behavior lives, and it's structured around one specific case getting different treatment from every other:

- **`RedisUnavailableError`** (from the feature-flag read specifically — auth already succeeded by this point, via cache or a fresh lookup) — routes to `env.STABLE_MODEL_FALLBACK_ID` instead of failing the request, and publishes telemetry with a `null` feature flag (so `rolloutId`/`rolloutPhaseId` land as `null` and `assignment` comes out `"stable"`). The client gets a normal 200 response. This is the one fallback path in the whole handler; every other error type below still fails the request, just with a specific status/error code rather than a generic 500.
- **`FeatureFlagNotFoundError`** → `503 ROUTING_CONFIGURATION_UNAVAILABLE` (transient — no rollout seeded yet for this tenant).
- **`InvalidFeatureFlagError`** → `500 INVALID_ROUTING_CONFIGURATION` (not transient — something wrote bad data).
- Anything else (Model Service unreachable/timed out/malformed) → `502 MODEL_SERVICE_UNAVAILABLE`.

---

## Tests

### Unit (`npm test`, vitest, no external services)

`vitest.config.ts` scopes `test.include` to `src/**/*.test.ts` specifically — without it, `vitest run`'s default include also picks up `tests/integration/**`, silently requiring a live Redis for a plain `npm test`. Added in `ci/foundation` after finding exactly that.

| File                                           | Covers                                                                                                                                                          |
| ---------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `services/traffic-assignment.service.test.ts`  | `selectModel` at 0%/100% candidate traffic, same-user consistency across calls, no-candidate-configured fallback to stable — pure function, no mocking needed   |
| `repositories/feature-flag.repository.test.ts` | `getFeatureFlag`'s four outcomes (valid flag, missing key, invalid JSON, schema mismatch) plus the Redis-throws case, via `vi.mock("../redis/redis.client.js")` |

### Integration (`npm run test:integration`, needs a running Redis; one spec also needs a running Model Service)

`vitest.integration.config.ts` scopes to `tests/integration/**/*.test.ts` — a separate config file, not just a different glob passed on the CLI, specifically so the two suites can't accidentally merge back together. Local: `docker compose up -d redis` (`docker-compose.yaml`), then `npm run test:integration`. CI's `node` job does the same.

| File                                          | Covers                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `feature-flag.repository.integration.test.ts` | Seeds a real feature flag key in Redis, reads it back through `getFeatureFlag`, confirms it round-trips through the schema unchanged                                                                                                                                                                                                                                                                                                                                                                 |
| `model-service.client.integration.test.ts`    | Seeds the one `model-config:<tenantId>:<modelVersionId>` key `requestModelInference` needs (self-contained rather than depending on the Go service also running to seed it), then makes a real HTTP call to a running Model Service                                                                                                                                                                                                                                                                  |
| `tenant-auth.repository.integration.test.ts`  | Added in `test/integration-coverage` — there was **no** coverage at all for `tenant-auth.repository.ts` before this, despite it being the piece that actually matters for confirming session-cookie auth doesn't leak into this service. Seeded-key resolution, an unseeded key, and — the actual point of the in-memory cache — a _real_ simulated Redis outage (`redisClient.disconnect()`, not a mock) proving a previously-resolved key still works while a never-seen key still correctly fails |

### End-to-end

Exercised as a real running process, not tested in isolation — by `apps/dashboard/e2e/` (every Playwright spec's traffic ultimately flows through here) and by the stress-tester CLI's load scenarios. See `documents/DASHBOARD.md` and `documents/STRESS_TESTER.md`.

---

## Configuration

| Variable                   | Default                         | Description                                                                                                        |
| -------------------------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `PORT`                     | `4002`                          | HTTP port                                                                                                          |
| `MODEL_SERVICE_URL`        | `http://localhost:4001`         | Model Service base URL                                                                                             |
| `REDIS_URL`                | `redis://localhost:6379`        | Redis connection URL                                                                                               |
| `FEATURE_FLAG_KEY_PREFIX`  | `feature-flag:model-routing:`   | Prefix + tenant ID = the Redis key read for that tenant's rollout config; must match the Rollout Controller's copy |
| `TELEMETRY_STREAM_KEY`     | `telemetry:inference-completed` | Redis Stream telemetry events are published to                                                                     |
| `STABLE_MODEL_FALLBACK_ID` | `model-v1`                      | Model used when Redis is unreachable for the feature-flag lookup                                                   |
