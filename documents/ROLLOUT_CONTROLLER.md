# Rollout Controller

`apps/rollout-controller` — Go, port 4003.

The brains of the platform: a single binary that ingests inference telemetry, runs the guard and controller decision loops, owns every write to Postgres and Redis, and serves the management API, dashboard authentication, and SSE. Everything the Edge Evaluator and Model Service do is instrumented traffic generation; this is where that traffic turns into decisions.

---

## Role in the system

Every tenant that has an active rollout gets its own independent **pipeline** — four goroutines (ingestion, guard, controller, writer) reading a shared Redis Stream and writing to that tenant's slice of Postgres/Redis. A **supervisor** loop in `main.go` reconciles which pipelines should exist by polling Postgres every ~5 seconds, tearing down pipelines whose rollout ended and starting new ones. Four more goroutines run for the whole process's life regardless of which tenants are active: the HTTP API server, a shared batch logger, and two Redis re-seeders (model configs, tenant auth).

This process is a **stateful singleton**, not horizontally scalable — see `documents/DEPLOYMENT.md`. It holds in-memory atomic state (`held`, `rolledBack`, `pct` per tenant) and owns the sole membership in the Redis Streams consumer group namespace it creates. Desired count must always be 1.

---

## Low-level design

### `main.go`

Boot sequence: run migrations (`db.RunMigrations`) → connect Postgres (`db.NewPool`) → connect Redis (`redisc.NewClient`) → seed Redis immediately from Postgres (model configs, tenant auth) so dependent services have valid data before the first request → construct `api.Server` → start the four whole-process goroutines → block in `runSupervisor`.

`runSupervisor` is a reconciliation loop, not a single-rollout cycle. Every `pipelinePollInterval` (5s) it:

1. Calls `repo.ListActiveRollouts` — every tenant's current RUNNING/HELD rollout.
2. Tears down any running pipeline whose tenant is no longer in that set, or whose rollout ID changed underneath it (completed/rolled back and a new one created).
3. Starts a pipeline (`runPipeline`) for anything newly active.

`runPipeline` builds a fresh `metrics.Store`, `writer.Writer`, `ingestion` consumer, `guard.Guard`, and `controller.Controller` for that one tenant/rollout, seeds Redis immediately (`w.SeedRedis`), registers itself in the `api.PipelineRegistry`, runs all four goroutines until its context is cancelled, then deregisters.

`envOr(key, fallback)` reads env vars with a fallback — see [Configuration](#configuration) for what actually gets read this way and what the fallbacks are.

### `internal/metrics` — the shared sliding window

`Store` is a thread-safe, time-ordered slice of `EventRecord{Timestamp, Success, LatencyMs}`. `Record` prunes anything older than 10 minutes but always retains at least 100 entries so the guard's fresh window is always satisfiable even right after a burst of old-event pruning. `LastN(n)` and `Since(t)` return copies, never the live slice. Two free functions, `ErrorRate` and `P95Latency`, operate on any `[]EventRecord` — both guard and controller call these on different windows of the same store.

### `internal/ingestion` — consuming the telemetry stream

Consumes the shared Redis Stream (`telemetry:inference-completed`) via a **per-tenant** consumer group (`rollout-controller-<tenantID>`). Every tenant's consumer group independently sees the _entire_ stream — Redis Streams consumer groups have no server-side content filtering — so `ingestion.Run` discards any event whose `rolloutId` doesn't match this tenant's, feeding only its own events into the shared `metrics.Store`.

### `internal/guard` — fast rollback detection (5s tick)

`Guard.evaluate()` runs two checks, fresh-window first:

1. **Fresh window** (last `FreshWindowSize` requests, default 50): if error rate exceeds `FreshWindowMaxErrorRate` (30%), fires `CmdRollback` immediately and returns — the absolute-window check never runs in the same tick.
2. **Absolute window** (every retained event): if error rate exceeds `AbsoluteMaxErrorRate` (5%), fires `CmdHold`.

Both checks are skipped entirely if total request count is below `MinRequestsBeforeGuard` (50) — a rollout can't be judged on too little data.

`Guard` doesn't hold a `*writer.Writer` — it takes `commands chan<- writer.Command` directly in its constructor, decoupling it from the writer's other responsibilities. `Controller` (below) originally held the whole `*writer.Writer` until `test/go-unit-coverage` extracted a `StateReader` interface for the same reason; `Guard` had the narrower dependency from the start.

### `internal/controller` — the 2-minute decision loop

`Controller.evaluate()`:

1. If `state.IsRolledBack()`, skip entirely — a rollback is a hard stop, only a brand-new rollout row un-does it.
2. Pull the last `ControllerIntervalSecs` (120s) of events via `store.Since`. Skip if fewer than `AdvanceMinRequests` (100) — not enough data yet.
3. Compute error rate and P95 latency for that window.
4. If currently held: a clean window (`errorRate <= AdvanceMaxErrorRate && p95 <= AdvanceMaxP95LatencyMs`) fires `CmdResume` — returns to RUNNING at the _current_ percentage, does not also advance in the same tick. An unhealthy window while held does nothing (stays held; the guard's 5s checks keep running independently in the meantime).
5. If not held: error rate or P95 over threshold fires `CmdHold`; otherwise fires `CmdAdvance`.

`StateReader` (`IsHeld() bool`, `IsRolledBack() bool`) is a small interface `*writer.Writer` satisfies for free, extracted specifically so `evaluate()`'s branching could be unit-tested against a fake instead of a real Redis/Postgres-backed `Writer` — see `controller_test.go`.

### `internal/writer` — the sole state mutator

`Writer` is the only thing in a tenant's pipeline allowed to write to that tenant's Redis feature flag or Postgres rows. `Run` reads from `Commands chan Command` (buffered, fed by guard/controller/the API's manual-rollback handler) and dispatches to `handle`, which for each `CommandType` (`HOLD`/`ROLLBACK`/`ADVANCE`/`COMPLETE`/`RESUME`) updates in-memory atomics (`held`, `rolledBack`, `pct` — all `atomic.Bool`/`atomic.Int32`, safe to read from any goroutine via `IsHeld`/`IsRolledBack`/`CurrentPercentage`), persists the new state to Postgres, re-writes the Redis feature flag, records a decision row, and broadcasts an SSE event.

`ADVANCE` walks a fixed ladder (`percentageSteps = []int{10, 25, 50, 75, 100}`); reaching 100% and staying healthy is what actually fires `COMPLETE` — which promotes the candidate to stable _in-memory_ before clearing the feature flag, so traffic keeps flowing to the model that won.

A 60-second heartbeat inside `Run` re-seeds the Redis feature flag (unless currently held) — the same resilience pattern described in the README's Redis Resilience section, scoped per-tenant here.

### `internal/db` — Postgres access

`db.go`: `NewPool` (pgxpool, pings on connect) and `RunMigrations` (golang-migrate, `file://` source, no-ops if already current).

`rollout_repository.go` (`RolloutRepository`) is the only thing touching the `rollouts`, `rollout_decisions` tables directly. Notable methods:

- `LoadActiveForTenant` — the config+policy a pipeline boots with; returns `ErrNoActiveRollout` (not found, expected) if none.
- `CreateRollout` — returns `ErrActiveRolloutExists` on the unique-constraint violation from `rollouts_single_active_per_tenant_idx` (Postgres error code `23505`), rather than a generic error.
- `GetRollout(ctx, tenantID, id)` — always scoped by tenant; a rollout belonging to another tenant is indistinguishable from a nonexistent one (`*RolloutNotFoundError`), not a 403 — avoids confirming another tenant's rollout IDs exist.
- `LatestCompletedCandidate` — what a new rollout's `stableModelVersionId` defaults to when omitted; `ErrNoCompletedRollout` if the tenant has never completed one (the Management API surfaces this as "stableModelVersionId is required").

### `internal/tenant` — tenant identity and API keys

`Repository.Create` generates a `tk_`-prefixed random API key (`internal/token.Generate`, 128-bit, hex), hashes it (`internal/token.Hash`, SHA-256) for storage, and inserts the tenant row plus two default `model_configurations` rows, all in one transaction. The plaintext key is returned exactly once — only its hash is ever persisted, and there is deliberately no way to retrieve it again later (see [Auth package](#internal-auth--users-sessions-passwords) for how a _lost_ key is handled instead).

`CreateTx` is the same logic run inside a _caller-supplied_ transaction — added so `auth.UserRepository.SignUp` could create a tenant and its owning user atomically without duplicating the seeding logic.

`RegenerateAPIKey` replaces a tenant's key outright — the old hash is overwritten, so the old key stops matching immediately, no separate revocation step.

`Seeder` (`seeder.go`) republishes every tenant's `tenant-auth:<key-hash> → tenantId` mapping to Redis on a heartbeat, so the Edge Evaluator (no direct Postgres access) can authenticate without asking this service per-request.

### `internal/auth` — users, sessions, passwords

Added in the `feat/auth` branch to back real sign-up/sign-in on the dashboard, on top of (not replacing) tenant API keys.

- **`password.go`** — thin bcrypt wrapper (`HashPassword`, `VerifyPassword`).
- **`user_repository.go`** — `users` wraps `tenants` 1:1 (`tenant_id` FK, `UNIQUE`). `SignUp` creates both rows in one transaction via `tenant.Repository.CreateTx`, normalizes email (lowercase, trimmed) before storing or looking up, and maps both "email already taken" (Postgres `23505` on `users.email`) and "wrong password" to the caller as `ErrEmailTaken` / `ErrInvalidCredentials` respectively — `SignIn` deliberately returns the _same_ `ErrInvalidCredentials` for "no such user" and "wrong password" so a failed attempt can't be used to enumerate registered emails.
- **`session_repository.go`** — opaque, `internal/token`-generated bearer tokens, SHA-256-hashed before storage in a `sessions` table (`token_hash` PK, `user_id`, `expires_at`), `SessionDuration` = 30 days. `UserIDForToken`'s query filters on `expires_at > NOW()` directly in SQL.

**Why sessions are opaque tokens, not JWT:** this service already treats Postgres as the source of truth for every request; JWT's main selling point (stateless verification) buys nothing here, while its main cost (no cheap revocation without a blocklist) actively hurts — both sign-out and API-key regeneration need instant invalidation. An opaque, hashed, DB-backed token is simpler and strictly better for this app's shape.

**Why the plaintext API key is never made retrievable, even to a logged-in user:** the alternative (storing it retrievably) would weaken a real security property — a compromised DB/session store currently leaks nothing usable. Instead `POST /auth/regenerate-key` invalidates the current key and mints a new one, shown once, same UX as sign-up. The dashboard warns that the old key stops working immediately.

### `internal/token` — shared token primitives

Extracted from `tenant.Repository` during `feat/auth` so session tokens didn't duplicate the same random-generate + SHA-256-hash pattern tenant API keys already used. `Generate()` → 128-bit random, hex-encoded. `Hash()` → SHA-256, hex-encoded. Callers add their own prefix convention on top (tenant keys get `tk_`; sessions don't need one, they're only ever read from a cookie).

### `internal/api` — HTTP, auth middleware, SSE

`server.go` wires two `http.ServeMux`s: an outer one with unauthenticated routes (`GET /health`, `POST /tenants` — gated by `X-Admin-Key` instead, `POST /auth/signup|signin|signout`, `GET /events` — see below), and an inner `authed` mux (everything else) wrapped by `authMiddleware`.

`authMiddleware` resolves a tenant (and, from a session, a user) two ways, tried in order:

1. `Authorization: Bearer <tenant-api-key>` → `tenantRepo.GetIDByAPIKey`. Used by the stress-tester, curl, any scripted caller. No user ID attached.
2. The session cookie, if no bearer header is present → `sessionRepo.UserIDForToken` → `userRepo.GetByID` for that session's tenant. Used by the dashboard once signed in. Both a tenant ID _and_ a user ID land in the request context (`tenantIDFromContext`, `userIDFromContext`), the latter only ever set on this path — handlers that require a real account (`GET /auth/me`, `POST /auth/regenerate-key`) check for it explicitly and 401 if absent, even if a bearer key would otherwise have resolved a tenant.

`GET /events` (SSE) is the one exception: browsers' `EventSource` can't send custom headers at all, so it accepts the key as an `?api_key=` query param instead of going through `authMiddleware`.

CORS (`newCORSMiddleware`) echoes back an allowlisted `Origin` (`ALLOWED_ORIGINS` env, default `http://localhost:5173`) with `Access-Control-Allow-Credentials: true`, rather than `*` — a wildcard origin can't be combined with credentialed (cookie-carrying) requests at all, browsers refuse it outright.

`hub.go` (`SSEHubRegistry`/`SSEHub`) is one broadcast hub per tenant, so a decision fired for one tenant's pipeline never reaches another tenant's connected dashboard. `pipeline.go` (`PipelineRegistry`) is the supervisor's map of currently-running tenant pipelines, read by every "current state" handler (`GET /rollout`, `GET /rollout/metrics`) — a tenant with no entry here means `{"active": false}`, _regardless of what Postgres says_, which is why there's an unavoidable lag (up to one `pipelinePollInterval`, 5s) between a rollout being created/rolled back in Postgres and the API reflecting it. (Found and worked around explicitly in `test/e2e-playwright`'s Playwright specs, which needed longer-than-default assertion timeouts because of exactly this.)

`auth_handlers.go` has the sign-up/in/out/me/regenerate-key handlers; see [Auth package](#internal-auth--users-sessions-passwords) above for the design decisions behind them, and the README's "Rollout Controller API" table for the full route list.

### `internal/modelconfig` — per-model simulation params

`Repository` is per-tenant, per-model-version CRUD (`model_configurations` table, composite PK `(tenant_id, model_version_id)`). `Seeder` republishes every tenant's configs to Redis on a heartbeat, same pattern as `tenant.Seeder` — the Model Service reads these from Redis, never Postgres directly.

### `internal/batchlogger` and `internal/redis`

`batchlogger.BatchLogger` is shared across _all_ tenants' pipelines (constructed once in `main.go`, passed into every `runPipeline` call) — it bulk-flushes inference events to `inference_events` via Postgres `COPY` every 10 seconds, batching writes regardless of which tenant they belong to since the table itself isn't partitioned by tenant (isolation is by `rollout_id`, which already implies a tenant). `internal/redis` is a thin `NewClient(url)` wrapper around `github.com/redis/go-redis/v9`.

---

## Tests

### Unit (`go test ./...`, no external services)

| Package      | File                 | Covers                                                                                                        |
| ------------ | -------------------- | ------------------------------------------------------------------------------------------------------------- |
| `guard`      | `guard_test.go`      | Fresh/absolute window thresholds, priority when both would breach, below-minimum-requests skip                |
| `controller` | `controller_test.go` | Advance/hold/resume decision branches, rolled-back short-circuit, held-stays-held vs. held-recovers-to-resume |
| `metrics`    | `store_test.go`      | `ErrorRate`/`P95Latency` edge cases (empty, single sample, small-N index behavior)                            |
| `tenant`     | `repository_test.go` | `generateAPIKey`'s `tk_` prefix convention (the one pure, DB-free piece of this package)                      |
| `auth`       | `password_test.go`   | bcrypt hash/verify round-tripping, salting, malformed-hash rejection                                          |
| `token`      | `token_test.go`      | `Generate`/`Hash` primitives — uniqueness, determinism, output format                                         |

`controller`'s tests needed the `StateReader` interface extraction described above to be possible at all without a real Writer. `tenant` and (partially) `auth`'s DB-bound methods (`Create`, `GetIDByAPIKey`, `SignUp`, `SignIn`, session CRUD) are deliberately **not** unit-tested with mocks — see integration tests below instead.

Run with coverage/race: `go test ./... -race -cover` (wired into CI's `go` job).

### Integration (`go test -tags=integration ./...`, needs a real Postgres)

All gated behind `//go:build integration`, so `go test ./...` (no tag) never touches them. `internal/dbtest.Pool(t)` is the shared setup: connects via `DATABASE_URL` (no default — deliberately never falls back to a local dev database), runs migrations, skips (not fails) if unreachable.

| Package                                                                                       | File                                     | Covers                                                                                                                                                                                                                             |
| --------------------------------------------------------------------------------------------- | ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `tenant`                                                                                      | `repository_integration_test.go`         | `Create`+`GetIDByAPIKey` round trip, invalid key, default model config seeding, `RegenerateAPIKey` invalidating the old key immediately, `ListAuth`                                                                                |
| `auth`                                                                                        | `user_repository_integration_test.go`    | `SignUp` (incl. duplicate email, email normalization on both signup and signin), `SignIn` (all three failure/success paths going through `ErrInvalidCredentials`), `GetByID`                                                       |
| `auth`                                                                                        | `session_repository_integration_test.go` | `Create`+`UserIDForToken` round trip, invalid token, `Delete` invalidating a session, a directly-inserted already-expired session being rejected                                                                                   |
| `db` (package `db_test` — avoids an import cycle through `dbtest`, which itself imports `db`) | `rollout_repository_integration_test.go` | `CreateRollout`/`GetRollout`/`ListRollouts`, tenant-scoped not-found, single-active-rollout conflict, status/percentage updates, decisions ordering+limit, `ListActiveRollouts`, `LoadActiveForTenant`, `LatestCompletedCandidate` |

Run locally against a docker-composed Postgres: `DATABASE_URL=postgres://jakeredding@localhost:5432/rollout_platform go test -tags=integration ./... -race` (see `docker-compose.yaml`'s `postgres` service). CI's `go-integration` job does exactly this.

### End-to-end

Covered indirectly — `apps/dashboard/e2e/` (Playwright) and the stress-tester CLI both exercise this service as a real running process rather than in isolation. See `documents/DASHBOARD.md` and `documents/STRESS_TESTER.md`.

---

## Configuration

All read via `envOr(key, fallback)` in `main.go`, plus a few more in `internal/api`. None currently fail fast on a missing value — flagged as a pre-deployment gap in `documents/DEPLOYMENT.md`.

| Variable                  | Default                                                  | Used for                                                                                                                 |
| ------------------------- | -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `REDIS_URL`               | `redis://localhost:6379`                                 | Redis connection                                                                                                         |
| `DATABASE_URL`            | `postgres://jakeredding@localhost:5432/rollout_platform` | Postgres connection                                                                                                      |
| `MIGRATIONS_PATH`         | `./migrations`                                           | SQL migration files                                                                                                      |
| `FEATURE_FLAG_KEY_PREFIX` | `feature-flag:model-routing:`                            | Must match the Edge Evaluator's own copy of this prefix                                                                  |
| `ADMIN_API_KEY`           | `dev-admin-key`                                          | Gates `POST /tenants` (`X-Admin-Key` header)                                                                             |
| `ALLOWED_ORIGINS`         | `http://localhost:5173`                                  | Comma-separated CORS allowlist                                                                                           |
| `COOKIE_SECURE`           | `false`                                                  | `true` → session cookie gets `Secure` + `SameSite=None` (cross-origin deployment); `false` → no `Secure`, `SameSite=Lax` |
