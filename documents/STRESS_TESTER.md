# Stress Tester

`apps/stress-tester` — TypeScript, local CLI (`tsx src/index.ts`, no build step, no HTTP server of its own).

Local load-generation tool. Not a demo convenience — it's the only thing in this platform that actually makes a rollout advance: without real request volume flowing through the Edge Evaluator, the guard and controller never have enough data to evaluate, and a rollout just sits at its starting percentage indefinitely.

---

## Role in the system

Drives HTTP traffic against the Edge Evaluator as a real tenant (via `--apiKey`, a `Bearer` token exactly like any other caller — this tool has no special access), polls the Rollout Controller's `GET /rollout` to narrate state transitions live, and prints a final report. It's the human-in-the-loop way to exercise the two load-driven scenarios this platform is built around: a rollout climbing its full advance ladder to `COMPLETE`, and the guard tripping a rollback under sustained failures.

---

## Low-level design

### `index.ts` — entry point and scenario definitions

Parses `--mode` (`steady` | `burst`, default `steady`), `--reset` (boolean), `--apiKey` (falls back to `TENANT_API_KEY` env, then the seeded demo tenant's key `tk_demo_2218a6e29efe8f4b3378390b46a0710d` — so the documented scenarios keep working unmodified against a fresh clone with zero setup). Two fixed scenarios:

| Mode     | RPS | Duration | Requires                                                                         |
| -------- | --- | -------- | -------------------------------------------------------------------------------- |
| `steady` | 50  | 10 min   | Nothing — default seeded model configs already sit inside the advance thresholds |
| `burst`  | 200 | 30s      | `model-v2` manually set to 35% failure rate first (`PUT /models/model-v2`)       |

If `--reset` is passed, calls `reset()` (below) and exits _before_ starting any load — `--reset` and an actual run are always two separate invocations, never combined. Otherwise constructs a `Runner`, wires a `Monitor` to it, runs both concurrently, then calls `printReport` with the runner's final stats and every transition the monitor observed.

### `runner.ts` — `Runner`

Generates load on a fixed-interval dispatch loop (`setInterval` at `1000/rps` ms), not a tight loop — `maxInFlight = rps * 3` caps concurrent in-flight requests so a slow response doesn't cause unbounded request pileup. Each dispatched request picks a random user ID from a fixed pool of 200 (`stress-user-0` … `stress-user-199`) — a bounded pool, not a fresh random ID per request, so the Edge Evaluator's deterministic per-user hash bucketing (see `documents/EDGE_EVALUATOR.md`) actually gets exercised the way it would with a real, semi-stable user base, rather than every single request rolling an independent coin flip. `sendRequest` POSTs `/v1/infer` with a 5-second timeout; a non-2xx or a body without `success: true` both count as a failure — this file makes no attempt to distinguish _why_ a request failed, only whether it did, since that distinction belongs to the guard/controller's own metrics on the receiving end, not this client.

### `monitor.ts` — `Monitor`

Two independent timers: a 1-second stats line (`RPS`, error %, a rolling-window P95 over the last 200 recorded latencies, and the live `<pct>% RUNNING|HELD` state) and a 2-second poll of `GET /rollout`. The poll is where scenario narration comes from — it diffs the current response against the last one it saw and prints a transition line the moment it detects: percentage dropping to 0 (distinguishes `ROLLBACK`, held=true, from `COMPLETE`, held=false — both drop candidate traffic to zero, only the guard's held flag tells them apart), a fresh hold (`!prev.held && current.held`), or an advance (`current.candidatePercentage > prev.candidatePercentage` while not held). A transient poll failure is silently ignored (`catch {}`) — a single missed poll shouldn't abort a multi-minute run.

A known minor cosmetic issue, not fixed here (out of scope for a docs branch): the live status line briefly showed `undefined% RUNNING` immediately after a `HOLD` resolved back to `RUNNING` during a real verification run in `test/e2e-playwright` — `candidatePercentage` came back transiently unset in one polled response at exactly that transition. Doesn't affect the final report or transition log, only that one live status line.

### `report.ts` — `printReport`

Pure formatting over the `Runner`'s final stats and the `Monitor`'s collected transition list — total/success/failure counts, achieved RPS, P50/P95/P99 latency, and the full ordered transition log (colored by type: red rollback, yellow hold, green everything else). No I/O of its own.

### `reset.ts` — `reset(mode, apiKey)`

A separate, deliberately narrow tool: resolves the tenant's current active rollout via `GET /rollout` (fails loudly if none exists — `--reset` only ever operates on an already-active rollout, it never creates one), then updates that row directly against `DATABASE_URL` with raw SQL — `UPDATE rollouts SET status='RUNNING', candidate_percentage=$1, configuration_version=1` (100% for `burst`, 10% for `steady`), plus `DELETE`s that rollout's `rollout_decisions` and `inference_events` so a fresh run starts from a clean slate rather than accumulating decisions/events across repeated test runs against the same rollout.

**The one gotcha every user of `--reset` needs to know, and the tool prints a warning about it:** this SQL update happens completely outside the Go Rollout Controller's supervisor loop, which only notices a tenant's active rollout when its _ID_ appears, changes, or disappears (see `documents/ROLLOUT_CONTROLLER.md`) — an in-place field update on the same row is invisible to it. If the controller already has a pipeline running for that rollout, its in-memory state (the percentage the writer goroutine thinks is current, the guard/controller's metrics windows) doesn't know anything changed. **The controller has to be restarted after a `--reset` on an already-active rollout, before the next run, for the reset to actually take effect** — confirmed by running into this directly while verifying `.github/workflows/stress-smoke.yml`'s setup sequence (see below), which sidesteps the whole problem by never using `--reset` in the first place.

---

## Tests

No unit or integration test suite exists for this CLI — its only automated coverage is exercising it as a real process, in two places:

- **`.github/workflows/stress-smoke.yml`** — a scheduled (`cron`, daily) + `workflow_dispatch`-triggered GitHub Actions workflow, deliberately never on `push`/`pull_request` (see `documents/DASHBOARD.md`'s and `documents/ROLLOUT_CONTROLLER.md`'s CI notes for why a full load run has no business blocking a PR). Boots the full stack via `docker compose` + built binaries, then runs the `burst` scenario against the seeded demo tenant. It sidesteps the `--reset`-then-restart gotcha above entirely: rather than creating the demo tenant's rollout at the normal 10% default and then `--reset`-ing it to burst's 100% target, it creates the rollout directly at `candidatePercentage: 100` via `POST /rollouts` and sets `model-v2`'s failure rate via `PUT /models/model-v2` — no SQL-level reset ever happens, so there's nothing for the controller to miss, and no restart step needed on a fresh CI run. This exact sequence (not just the CI mechanics around it) was verified for real against the live local stack before being committed: created a rollout, set the failure rate, ran `npm run burst`, and watched the guard correctly fire a `HOLD` at 100% candidate within seconds — 5,419 requests, 11.6% observed failure rate.
- **`apps/dashboard/e2e/`** (Playwright) exercises the same Edge Evaluator/Model Service/Rollout Controller path this tool drives traffic through, from the opposite direction (a browser driving the dashboard) — see `documents/DASHBOARD.md`.

Run manually: `npm run steady --workspace @rollout-platform/stress-tester` or `npm run burst --workspace @rollout-platform/stress-tester` (add `-- --apiKey=tk_...` for a non-demo tenant, `-- --reset` to reset an already-active rollout — remembering the controller-restart caveat above).

---

## Configuration

| Variable                 | Default                                      | Description                                |
| ------------------------ | -------------------------------------------- | ------------------------------------------ |
| `EDGE_EVALUATOR_URL`     | `http://localhost:4002`                      | Where load is sent                         |
| `ROLLOUT_CONTROLLER_URL` | `http://localhost:4003`                      | Polled for live state narration            |
| `DATABASE_URL`           | `postgres://localhost:5432/rollout_platform` | Only used by `--reset`, direct SQL         |
| `TENANT_API_KEY`         | seeded demo tenant's key                     | Overridable per-run via `--apiKey` instead |
