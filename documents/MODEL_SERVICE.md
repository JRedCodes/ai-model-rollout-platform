# Model Service

`apps/model-service` — TypeScript, Express, port 4001.

Simulates inference. No real model runs anywhere in this platform — this service exists specifically so a rollout's health can be steered on demand (via a config change, not a redeploy) to exercise every guard/controller decision path.

---

## Role in the system

The last hop in the request path: the Edge Evaluator decides _which_ model version a request should hit and forwards here; this service decides whether that particular call succeeds or fails and how long it takes, based on a per-tenant, per-model simulation profile it reads from Redis. It never talks to Postgres, and it never talks to the Edge Evaluator either — only the reverse. Like the Edge Evaluator, its Redis reads are resilient to the Rollout Controller (the only thing with a database connection) being briefly unreachable, since the config it needs was already published to Redis ahead of time.

---

## Low-level design

### `app.ts` + `server.ts` — wiring

`app.ts`: `GET /health` (unauthenticated — this service has no auth of its own; the Edge Evaluator is the only caller, already authenticated at that layer, and nothing external is meant to reach this service directly), and `POST /v1/models/:modelVersionId/infer` behind no middleware at all. `server.ts` connects Redis before binding the port and exits on failure, the same fail-fast-at-boot pattern the Edge Evaluator uses (see `documents/EDGE_EVALUATOR.md`).

### `repositories/model-config.repository.ts` — `getModelConfig`

Reads `model-config:<tenantId>:<modelVersionId>` from Redis, published by the Rollout Controller's `modelconfig.Seeder` from Postgres. Three distinct outcomes, and the distinction is deliberate:

- **Redis unreachable** — falls back to a built-in default profile (`DEFAULT_FAILURE_RATE = 0.01`, 50–150ms), logging a warning. Keeps inference serving through a transient outage instead of failing every request.
- **Key missing** (Redis reachable, nothing stored for this tenant/model pair) — throws `UnsupportedModelVersionError`. A genuinely unconfigured model version is a real 404, not something to paper over with a default.
- **Value present but invalid** (bad JSON, or valid JSON that fails `modelSimulationProfileSchema`) — also falls back to the default profile rather than failing, on the theory that a model version which _was_ configured shouldn't suddenly 404 just because its stored config got corrupted; better to serve degraded-but-working traffic and let the bad data get investigated separately.

Every tenant is seeded with `model-v1` (1% failure, 50–150ms) and `model-v2` (2% failure, 50–200ms — deliberately still inside the controller's advance thresholds, so a fresh rollout can climb the whole advance ladder without any config change) at tenant-creation time. That seeding logic lives in the Go Rollout Controller's `tenant.Repository`, not here — see `documents/ROLLOUT_CONTROLLER.md`.

### `services/model-simulator.service.ts` — `simulateInference`

Three steps, in order: roll a random latency within `[minLatencyMs, maxLatencyMs]` (`randomInteger`, uniform), actually `wait()` that long (a real `setTimeout`, not just a reported number — the caller genuinely experiences the simulated latency), then roll `Math.random() < failureRate` to decide success or failure. A failure response carries `errorType: "SIMULATED_FAILURE"` and the same rolled latency; a success carries a fixed `output.classification: "ACCOUNT_ACCESS"` — the actual classification value is a hardcoded placeholder, since what the simulated model "decided" was never the point.

### `routes/inference.route.ts` — the handler

Validates the body against `modelInferenceRequestSchema`, checks `modelVersionId` is present in the path, calls `simulateInference`, and maps its one typed error (`UnsupportedModelVersionError` → `404 MODEL_VERSION_NOT_FOUND`) distinctly from everything else (`500 MODEL_SERVICE_INTERNAL_ERROR`, logged). A schema validation failure short-circuits before any of that with `400 INVALID_INFERENCE_REQUEST` and the Zod issues attached.

---

## Tests

### Unit (`npm test`, vitest, no external services)

Added in `test/ts-unit-coverage` — this package had zero test coverage before (build/typecheck scripts only). `vitest.config.ts` scopes `test.include` to `src/**/*.test.ts`, and `tsconfig.json` excludes `src/**/*.test.ts` from the build — both added together after a stale `dist/` was found silently double-running every test here too: without the scoping, plain `vitest run`'s default include picked up both the real source test and `tsc`'s compiled `dist/*.test.js` copy of the same file, running it twice per invocation.

| File                                       | Covers                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `services/model-simulator.service.test.ts` | Mocks the config repository (`vi.mock`) and `Math.random` (`vi.spyOn`) with fake timers (`vi.useFakeTimers`/`vi.runAllTimersAsync`) to deterministically drive: latency landing exactly at the min/mid/max boundary of `[minLatencyMs, maxLatencyMs]` as `Math.random()` sweeps 0→~1, a success response when the failure roll doesn't trigger, a failure response when it does, and `UnsupportedModelVersionError` propagating unchanged from the config lookup |

Worth knowing if you touch this test file: vitest 4's fake timers hang indefinitely under Node 18 — the test never resolves and the run dies on the default 10-second hook timeout, with no assertion failure to point at, just a confusing timeout error. They work correctly on Node 20+. This is exactly the README's already-documented "Vitest 4 requires Node.js 20+" prerequisite, rediscovered the hard way while writing this file — confirmed by switching Node versions with `nvm` mid-debugging and re-running the exact same test.

Run: `npm test --workspace @rollout-platform/model-service`.

### End-to-end

Exercised as a real running process, not tested in isolation — every inference the Edge Evaluator forwards ultimately lands here, whether driven by `apps/dashboard/e2e/`'s Playwright specs or the stress-tester CLI's load scenarios. See `documents/DASHBOARD.md` and `documents/STRESS_TESTER.md`.

---

## Configuration

| Variable    | Default                  | Description                                                              |
| ----------- | ------------------------ | ------------------------------------------------------------------------ |
| `PORT`      | `4001`                   | HTTP port                                                                |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL — source of per-tenant, per-model simulation config |
