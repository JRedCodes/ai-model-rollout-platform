# Dashboard

`apps/dashboard` — React 19 + TypeScript, Vite 6, Tailwind CSS 3, TanStack Query. Port 5173.

Real-time control panel for monitoring and operating a live rollout, plus the account/tenant management flow that gets a visitor from "landed on the page" to "has a tenant and an API key."

---

## Role in the system

The only service in this platform meant for a human, not another service. Everything it shows is fetched from the Rollout Controller's Management API and kept live via a per-tenant SSE connection (`GET /events`) — the controller pushes on every decision, the dashboard invalidates and refetches within milliseconds, no polling lag on state that matters. A few panels (metrics window count, request totals) still poll on an interval since they're not decision-driven.

---

## Low-level design

### Entry and auth gating — `App.tsx`

`App.tsx` is the router: it doesn't render a fixed tree, it picks one of five views (`SignIn`, `SignUp`, `AccountPanel`, `AboutPage`, `Dashboard`) based on two independent pieces of state:

- **`authenticated`** — `hasLegacyKey || session.status === "signed-in"`. Two ways in, checked in this order everywhere in the app: a legacy API key stored in `localStorage` (`apiKey.ts`), or a real signed-in session (`useSession.ts`, backed by an httpOnly cookie the app can't read directly — see below).
- **`route`** — from `router.ts`, reflecting `window.location.pathname`.

Routing logic, in order: `/about` always wins (reachable pre- and post-auth, doesn't wait on the session check) → unauthenticated shows `SignUp` on `/signup`, `SignIn` everywhere else → authenticated + `/account` **and** a real session (not just a legacy key) shows `AccountPanel` → everything else shows `Dashboard`. That middle branch matters: a legacy-key-only visitor has no user account behind them (the seeded demo tenant, for instance, has no `users` row at all), so `/account` silently falls back to the Dashboard for them instead of showing a panel with nothing to display.

A brief loading guard (blank screen, not a flash of the sign-in form) covers the window while `GET /auth/me` is in flight on first load, for a legacy-key-less visitor.

### `router.ts` — no dependency

`useRoute()` + `navigate()` built directly on `history.pushState`/`popstate`, mirroring `apiKey.ts`'s own window-event pattern for cross-component state. Five views didn't justify pulling in `react-router-dom`. `VALID_ROUTES` is a fixed list (`/`, `/signin`, `/signup`, `/account`, `/about`); anything else falls back to `/`.

### Auth surface — `apiKey.ts`, `useSession.ts`, `SignUp.tsx`, `SignIn.tsx`, `ApiKeyGate.tsx`, `AccountPanel.tsx`

- **`apiKey.ts`** — the legacy path. `localStorage` get/set/clear plus a custom `window` event so other components (notably `App.tsx`) react to a key appearing or disappearing without prop drilling. Predates real auth; kept because it's still how the seeded demo tenant's key gets used, and it's a zero-friction option for visitors who don't want to create an account.
- **`useSession.ts`** — the real path. httpOnly cookies aren't readable from JS by design, so "am I signed in" can only be answered by asking the server: `GET /auth/me` with `credentials: "include"`. Returns `[state, refresh]` — `state` is `{status: "loading" | "signed-out"} | {status: "signed-in", session}`, `refresh()` lets `SignUp`/`SignIn` force an immediate re-check right after establishing a session instead of waiting for a natural re-render.
- **`SignUp.tsx`** — posts `/auth/signup`, then shows the returned plaintext API key exactly once with a copy-to-clipboard step. "Continue to dashboard" stays disabled until the key's actually been copied (tracked via a local `copied` flag set by the copy button's own `onClick`, not by polling the clipboard) — it's not possible to click through without ever having a chance to see the key.
- **`SignIn.tsx`** — posts `/auth/signin`. Toggles inline (no route change) to `ApiKeyGate` via a "use an existing API key" link, and links to `/signup` and `/about`.
- **`ApiKeyGate.tsx`** — the original, now-demoted entry point. Takes an optional `onBack` so `SignIn` can toggle back to itself.
- **`AccountPanel.tsx`** — only ever reached by a real signed-in session (see routing logic above). Shows the signed-in email and a masked key placeholder (`tk_••••••••` — the real key is never retrievable after creation, by design, see `documents/ROLLOUT_CONTROLLER.md`'s auth section). "Regenerate key" hits `POST /auth/regenerate-key` and shows the new key once, same copy-gate pattern as `SignUp`.

### `api.ts` — the request layer

`apiFetch` wraps `fetch`: prefixes `/api` (proxied to the Rollout Controller — see [Local dev vs. production](#local-dev-vs-production) below), always sends `credentials: "include"` so the session cookie rides along, and additionally attaches a stored legacy key as `Authorization: Bearer` when present (the backend's `authMiddleware` tries Bearer first regardless, so a stored key always wins if both a key and a session exist). Any `401` clears the stored legacy key (a no-op if there wasn't one) and throws a typed `UnauthorizedError`, which — via `apiKey.ts`'s change event — bounces `App` back to the sign-in view automatically. Every typed fetch helper (`fetchRollout`, `fetchMetrics`, `fetchDecisions`, `postRollback`, `createRollout`, `fetchModelConfigs`, `updateModelConfig`, `regenerateApiKey`, `signOut`) goes through this one function.

### `useSSE.ts` — live updates

One `EventSource` for the whole app, opened in `Dashboard`. A stored legacy key goes as an `?api_key=` query param (the one route the server accepts that from — `EventSource` can't send custom headers at all); without one, `withCredentials: true` carries the session cookie instead. On any message, invalidates all four query keys the dashboard's panels read (`rollout`, `metrics`, `decisions`, `modelConfigs`) — the server doesn't say *what* changed, so the client just refetches everything cheap enough to refetch.

### Panels — `StatusPanel.tsx`, `MetricsPanel.tsx`, `ModelConfigPanel.tsx`, `DecisionFeed.tsx`, `CreateRolloutPanel.tsx`

Each panel owns its own `useQuery`/`useMutation`, no shared state beyond TanStack Query's cache. `StatusPanel` renders `CreateRolloutPanel` in place of the active-rollout view when `!data.active` — this is why `Force rollback`'s button and `CreateRolloutPanel`'s form are never on screen at the same time, and why a just-created rollout can take a few seconds to actually replace the create-rollout form: `GET /rollout` lags Postgres by up to one supervisor reconcile tick (~5s) on the Go side, not a dashboard bug — see `documents/ROLLOUT_CONTROLLER.md`.

`ModelConfigPanel` renders one `ModelConfigRow` per model version; both `StatusPanel` and `ModelConfigPanel` can show the same model ID on screen simultaneously (status view + config table), which is why both carry `data-testid` attributes (`status-stable-model`, `status-candidate-model`, `model-config-failure-rate-<id>`, etc.) — added specifically so Playwright's strict-mode text matching has something unambiguous to grab, not decorative.

`StatusPanel`'s "Force rollback" button goes through a native `confirm()` dialog before calling `postRollback` — the one place in the app that blocks on a browser-native prompt rather than an in-app one.

### `AboutPage.tsx` + `content/architecture.ts`

The visitor-facing "how this works" page — architecture diagram, data/control-flow explanations, component list, an explicit "this is a simulated demo, no real inference happens" banner, and a numbered sign-up → copy key → run stress-tester walkthrough. `content/architecture.ts` holds the actual copy as a data module (diagram string + typed arrays of flow steps and service summaries) so `AboutPage.tsx` stays purely presentational — adapted from, not copy-pasted from, the README's own Architecture section; the README also carries developer-facing content (Roadmap, env var tables) that doesn't belong in front of a demo visitor.

### Local dev vs. production

`vite.config.ts`'s dev server proxies `/api` → `http://localhost:4003`, which is *why* everything above can use relative `/api/...` paths and same-origin cookies with zero CORS complexity locally. There's no equivalent in a built+`vite preview`/production deployment (`preview` doesn't inherit `server.proxy`) — `documents/DEPLOYMENT.md` covers the build-time `VITE_API_URL` override this needs before a real deploy.

---

## Tests

### Unit (`npm test`, vitest + `@testing-library/react` + jsdom)

| File | Covers |
|---|---|
| `apiKey.test.ts` | get/set/clear round-tripping, the change-event subscription including unsubscribe actually stopping notifications — pure logic, no React |
| `router.test.ts` | `navigate()` pushing history and no-op'ing on the current route, `useRoute()` reflecting the path/defaulting unknown paths/updating on `navigate()` (via `renderHook`/`act`) |
| `useSession.test.ts` | loading → signed-in/signed-out transitions off a mocked `fetch` (ok, non-ok, rejected), `refresh()` triggering a second request |

`vitest.config.ts` scopes `test.include` to `src/**/*.test.{ts,tsx}` explicitly — added defensively after a stale `dist/` (from the plain-`tsc`-build packages) was found silently double-running every test in `contracts`/`model-service`; `vite build` doesn't actually leak test files into `dist/` the way those do, but the explicit scope costs nothing and matches the other packages.

### End-to-end (`npm run test:e2e`, Playwright, needs the full stack running)

`playwright.config.ts` assumes redis/postgres/model-service/edge-evaluator/rollout-controller/dashboard are all already running (locally via each service's own dev command, in CI via `docker compose` + built binaries — see `.github/workflows/ci.yml`'s `e2e` job) — no `webServer` auto-start, since Playwright manages one process and this suite needs the whole stack.

| File | Covers |
|---|---|
| `signup-flow.spec.ts` | Full UI sign-up → API-key copy-gate (clipboard read-back, not just a click) → dashboard → first rollout creation, explicitly picking a stable model since a brand-new tenant has no completed rollout to default one from |
| `signin-and-legacy-key.spec.ts` | Sign-in with a fixture account, wrong password shows an error and stays on the form, the legacy demo-tenant-key path, and `/account`'s dashboard-fallback behavior for a legacy-key session |
| `model-config-and-rollback.spec.ts` | Editing a model's config and confirming it persisted server-side (reload, don't trust optimistic state); force rollback, including the native `confirm()` dialog |

`e2e/helpers.ts` provides `apiSignUp(context, email)` — signs up via `context.request` (not the top-level `request` fixture) specifically because it shares its cookie jar with the test's pages, so a subsequent `page.goto()` is already authenticated without re-driving the sign-up form when signup itself isn't what's under test.

Two things every E2E author on this codebase should know going in, both discovered writing these specs, both about the same underlying cause: `GET /rollout`'s up-to-5s lag behind Postgres (see `documents/ROLLOUT_CONTROLLER.md`) means any assertion about a rollout just having been created or rolled back needs `{ timeout: 15_000 }`, not Playwright's 5s default — and Playwright's role/text locators are fuzzy-matching by default (`getByRole("button", { name: "Copy" })` matched *both* "Copy" and "Copy your key to continue" until `exact: true` was added).

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `VITE_API_URL` *(planned, not yet wired — see `documents/DEPLOYMENT.md`)* | `/api` | Build-time API base; the `/api` default only resolves via Vite's dev proxy today |
