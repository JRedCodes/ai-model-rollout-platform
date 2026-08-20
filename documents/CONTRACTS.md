# Contracts

`packages/contracts` — TypeScript, Zod. No runtime service, no port; a library every TypeScript service in the monorepo depends on.

---

## Role in the system

The shared schema layer for every service boundary in the TypeScript half of the platform (Edge Evaluator, Model Service, Dashboard all depend on `@rollout-platform/contracts`; the Go Rollout Controller has its own equivalent types, hand-kept in sync). The point of centralizing these here rather than letting each service define its own request/response types is that the _same_ Zod schema validates a payload on both sides of a call — the Edge Evaluator's outgoing request to the Model Service and the Model Service's incoming validation of that request are checked against one definition, not two independently-maintained ones that could drift apart. It's also the one place a "valid `InferenceCompletedEvent`" or "valid `FeatureFlag`" is actually defined, rather than being an implicit contract enforced by convention.

---

## Low-level design

### `inference/` — the request/response pairs that cross HTTP boundaries

Four schemas, two request/response pairs:

- **`edge-inference-request.ts`** / **`model-inference-request.ts`** — near-identical shapes (`requestId`, a caller identifier — `userId` for the edge-facing one, `tenantId` for the one the Model Service receives — and `input.message`, trimmed and required non-empty). Kept as two separate schemas rather than one shared one because they're validated at two different trust boundaries with two different identity concepts; collapsing them would blur that distinction for a savings of a few lines.
- **`edge-inference-response.ts`** / **`model-inference-response.ts`** — both use `z.discriminatedUnion("success", [...])` rather than a single object with optional success/failure fields. The same pattern solves the same problem twice: a response is _either_ a success variant (with a `result`/`output` payload) _or_ a failure variant (with an `errorType`), never a struct where both are optional and a caller has to remember which fields are meaningful together. The discriminant (`success: true` vs `success: false`) is what lets Zod — and TypeScript, via `z.infer` — narrow the type automatically once a caller checks `.success`.

### `rollout/` — only one of four schemas is actually reachable from outside this package

`feature-flag.ts` (`FeatureFlag` — the Edge Evaluator's per-tenant routing config: rollout/phase IDs nullable when no rollout is active, `candidatePercentage` clamped 0–100, `configurationVersion` a positive int) is re-exported from `index.ts` and is genuinely used across service boundaries (Edge Evaluator reads and validates it from Redis).

`rollout-decision.ts`, `rollout-policy.ts`, and `rollout-state.ts` also live in this directory and are fully written, valid Zod schemas with their own test coverage — but **`index.ts` does not re-export any of them**, and `package.json`'s `"exports"` field only declares a single `"."` entry pointing at `./dist/index.js`. Node's `exports` field, once present, restricts a package to _only_ the subpaths it explicitly lists — there's no `"./rollout/rollout-decision"` entry, so even a deep import from outside this package (`@rollout-platform/contracts/rollout/rollout-decision`) would fail to resolve for a real consumer, not just an inconvenient-but-possible one. In practice, nothing in `apps/*` imports these three today (confirmed by their absence from every other service's source), so they're reachable only from within `packages/contracts` itself — their own test files import them via relative paths, which sidesteps the package boundary entirely. This is a real, pre-existing gap in the barrel export, documented here as an observation, not something this docs branch is fixing.

### `simulation/model-simulation-profile.ts` — a cross-field invariant, not just field types

`ModelSimulationProfile` (`failureRate` 0–1, `minLatencyMs`/`maxLatencyMs` positive ints, `updatedAt` an ISO datetime string) adds a `.refine()` on top of the base object: `minLatencyMs <= maxLatencyMs`, reporting its error against `path: ["minLatencyMs"]` specifically so a caller's per-field error handling points at the field that's actually wrong relative to the other, not a generic top-level failure. This is the Model Service's and Rollout Controller's shared definition of what a valid per-tenant, per-model simulation profile looks like.

### `telemetry/inference-completed-event.ts` — the same idea, enforced with `.superRefine()`

`InferenceCompletedEvent` is what the Edge Evaluator publishes to the Redis Stream and the Rollout Controller's ingestion consumer reads back. Beyond per-field types (`schemaVersion` pinned to the literal `1`, `rolloutId`/`rolloutPhaseId` nullable, `errorType` a closed enum, nullable), a `.superRefine()` enforces that `success` and `errorType` can't disagree: a successful event must carry a `null` errorType, a failed one must carry a non-null one — either violation raises an issue on `path: ["errorType"]` with a message naming which side of the rule broke.

Both refinements above exist because the alternative — trusting every caller to remember "don't set both `minLatencyMs` past `maxLatencyMs`" or "don't set an `errorType` on a success" — is exactly the kind of invariant that erodes quietly over time across a service boundary. Encoding it in the schema makes the invalid state fail validation the moment it's constructed, on either side of the boundary, rather than surfacing later as a confusing downstream symptom.

---

## Tests

Added in `test/ts-unit-coverage` — this package had zero test coverage before (build/typecheck scripts only, no test framework installed). One file per schema, 58 cases total: valid-input acceptance, the schema's specific invalid-input rejections (wrong enum value, out-of-range number, empty required string, etc.), and — for the two schemas with cross-field refinements — an explicit assertion that the refinement's _specific_ error path is what gets reported, not just that validation failed.

| File                                          | Covers                                                                                                                                                                                                                                                  |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `inference/edge-inference-request.test.ts`    | Valid request, empty `requestId`, whitespace-only `message` (rejected), message trimming, missing `input`                                                                                                                                               |
| `inference/edge-inference-response.test.ts`   | Success/failure variants, a success missing `result`, a failure carrying a `result` instead of `errorType`, an unrecognized `success` value                                                                                                             |
| `inference/model-inference-request.test.ts`   | Valid request, empty `tenantId`, whitespace-only `message`                                                                                                                                                                                              |
| `inference/model-inference-response.test.ts`  | Success/failure variants, unrecognized `errorType`, negative/non-integer `latencyMs`                                                                                                                                                                    |
| `rollout/feature-flag.test.ts`                | Valid active flag, all-null "no active rollout" shape, `candidatePercentage` out of 0–100, non-positive `configurationVersion`                                                                                                                          |
| `rollout/rollout-decision.test.ts`            | Valid proposal, all five `action` enum values, an unrecognized action, negative `expectedStateVersion`, non-ISO `proposedAt`                                                                                                                            |
| `rollout/rollout-policy.test.ts`              | Valid policy, out-of-range error rate, non-positive window size, zero vs. negative `cooldownSeconds`, missing `advancement` block                                                                                                                       |
| `rollout/rollout-state.test.ts`               | All six status enum values, an unrecognized status, a lowercase variant                                                                                                                                                                                 |
| `simulation/model-simulation-profile.test.ts` | Valid profile, `minLatencyMs == maxLatencyMs` (allowed), `minLatencyMs > maxLatencyMs` (rejected, asserts the error path is `["minLatencyMs"]`), out-of-range `failureRate`, non-positive latency                                                       |
| `telemetry/inference-completed-event.test.ts` | Valid success/failure events, success carrying an `errorType` (rejected), failure with a `null` errorType (rejected) — both asserting the error path is `["errorType"]` — null rollout fields allowed, wrong `schemaVersion`, unrecognized `assignment` |

Run: `npm test --workspace @rollout-platform/contracts` (or `npm test` from `packages/contracts`).

`vitest.config.ts` scopes `test.include` to `src/**/*.test.ts`, and `tsconfig.json` excludes `src/**/*.test.ts` from the build — added together (`test/ts-unit-coverage`) after a stale `dist/` was found silently double-running every test in this package: with no scoping, plain `vitest run` picked up both the `src/` source tests and `tsc`'s compiled `dist/*.test.js` copies of the same tests, reporting 116 "tests" instead of the real 58.
