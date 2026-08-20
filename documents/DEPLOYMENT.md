# Deployment Plan — AWS

Target: an always-on, publicly reachable deployment of the AI Model Rollout Platform on AWS.

This plan originally added a temporary shared-secret gate on the mutating endpoints as a stopgap, since real auth wasn't built yet. That's no longer needed: `feat/auth` shipped real multi-tenant authentication first — bcrypt-hashed passwords, httpOnly-cookie sessions for the dashboard, and tenant API keys for everything else (`Authorization: Bearer`). `POST /tenants` (`X-Admin-Key`-gated, for ops/scripted tenant creation — e.g. the seeded demo tenant) and `POST /auth/signup` (self-serve, creates a tenant + the user that owns it together) now coexist as two independent ways to get a tenant; every mutating endpoint already requires one of the two auth paths to resolve a tenant before this plan even starts. See `documents/ROLLOUT_CONTROLLER.md`'s auth section for the design.

---

## Current-state findings that shape this plan

- **`rollout-controller` is a stateful singleton**, not a horizontally-scalable service. It holds in-memory atomic state (`held`, `rolledBack`, `pct`), owns the sole membership in its Redis Streams consumer group, and runs an in-process SSE hub. It must always run at **desired-count = 1**. A container that just runs continuously (ECS Fargate) is a better fit than a request-triggered, scale-to-zero model (Cloud Run) for this reason — its most important work (5s guard tick, 2-min controller tick, stream consumption) happens on background goroutines with no HTTP request involved at all.
- **`main.go` silently falls back to `localhost`** for `REDIS_URL`/`DATABASE_URL` if unset (`envOr(...)`). In a container this means a missing env var fails weirdly (tries to reach `localhost` inside the container) instead of failing fast at boot.
- **The dashboard's API calls are relative** (`const BASE = "/api"` in `api.ts`, `new EventSource("/api/events")` in `useSSE.ts`), and only resolve today because Vite's dev server proxies `/api` → `localhost:4003`. Once the dashboard is served from CloudFront and the API from an ALB on a different origin, there is no equivalent proxy — needs a build-time absolute API base URL.
- **CORS on the Go API is already origin-allowlisted and credentialed** (`ALLOWED_ORIGINS` env var, default `http://localhost:5173`, plus `Access-Control-Allow-Credentials: true` for the session cookie) rather than wide open — a wildcard origin can't be combined with credentialed requests at all, browsers refuse it outright, so this had to change before real auth could ship, not after. Still needs `ALLOWED_ORIGINS` to actually include the deployed dashboard's real origin once one exists (CloudFront domain / custom domain), and `COOKIE_SECURE=true` for the session cookie to survive the CloudFront/ALB cross-origin split (see `documents/ROLLOUT_CONTROLLER.md`'s Configuration table).
- **No Dockerfiles exist yet** for any of the four services — only Redis and Postgres have `docker-compose.yaml` entries today.
- **CI validates every push/PR** (`.github/workflows/ci.yml`: typecheck, build, unit tests, integration tests, and a full-stack Playwright E2E job, plus a separate scheduled load-test smoke workflow) but doesn't build or push any container image, and there's no deploy step — that gap is what Phase 5 below still needs to close.
- **Migrations self-run on boot** (`db.RunMigrations` in `main.go`) — convenient for deploys (no separate migration step), but means the ECS task's DB credentials need schema-modify privileges. The first tenant and rollout can now be created through the Management API (or the dashboard) after the first deploy — no `psql` step required, unlike when this plan was first written.

---

## Target architecture

| Piece | AWS service | Why |
|---|---|---|
| `rollout-controller` | ECS Fargate, desired count = 1 | Singleton by design — see above |
| `edge-evaluator` | ECS Fargate, desired count = 1 | Stateless today; could scale later, starting at 1 to match current behavior |
| `model-service` | ECS Fargate, desired count = 1 | Same as above |
| `dashboard` | S3 + CloudFront | Static Vite build — no reason to spend a container on it |
| Postgres | RDS, single `db.t4g.micro`, no Multi-AZ | Demo scale, not HA; replaces the local trust-auth instance |
| Redis | ElastiCache, single `cache.t4g.micro` | Same role as the current Docker Redis |
| Routing | Application Load Balancer, path-based rules | Dashboard talks to one API origin; ALB fans out to the three backend services |
| TLS / domain | ACM + Route53 | One cert for CloudFront (dashboard), one for the ALB (API) |
| Secrets | Secrets Manager → ECS task secrets | DB password and `ADMIN_API_KEY` (gates `POST /tenants`) never sit in plaintext in a task definition |
| Images | ECR, one repo per service | Standard ECS image source |
| IaC | Terraform | Reproducible; cheap to tear down/rebuild when not actively demoing |
| CI/CD | GitHub Actions → ECR → ECS | Extends the existing validate-on-every-PR workflow with an image build/push/deploy step, making a deploy a `git push` to `main`, not a manual `docker push` |

---

## Prerequisite code changes (before any AWS work)

1. **Fail fast on missing config.** In `main.go`, require `DATABASE_URL` and `REDIS_URL` — `log.Fatalf` if unset, rather than defaulting to `localhost`. Keep a `localhost` default only behind an explicit local-dev path if that's useful, but never as the production behavior.
2. **Add a build-time API base URL to the dashboard.** Introduce `VITE_API_URL`, defaulting to `/api` so local dev via the Vite proxy is unaffected. Update `api.ts`'s `BASE` and `useSSE.ts`'s `EventSource` URL to use it.
3. **Write a Dockerfile per service:**
   - `rollout-controller`: multi-stage — Go build stage, then a minimal alpine/distroless runtime image copying only the binary + `migrations/`.
   - `edge-evaluator`, `model-service`: multi-stage — `npm run build` (tsc), then a slim `node:20-alpine` runtime running `node dist/server.js`.
   - `dashboard`: build-only stage (`vite build`); the output is uploaded to S3, not run in a container.

---

## Phased rollout

### Phase 1 — Containerize locally
Write the four Dockerfiles, apply the two code changes above (fail-fast config, `VITE_API_URL`). Verify with `docker compose up` using all four services plus the existing Redis entry — confirm the whole stack behaves identically to `npm run dev`/`go run .` today (traffic flow, SSE updates, mid-rollout model-config change). No AWS involved yet.

### Phase 2 — IaC skeleton
Terraform for: VPC (public + private subnets), RDS instance, ElastiCache instance, ECR repos, an ALB with no listeners wired yet. Confirm the data plane is reachable from a bastion/local machine before anything depends on it.

### Phase 3 — Ship containers
Push images to ECR. Stand up the three Fargate services with task definitions pulling secrets from Secrets Manager. Wire ALB target groups + path-based routing. Create the first tenant and rollout through the Management API (or `curl`, per the README's "Create your first rollout") against the deployed ALB — no direct `psql` access to RDS needed for this anymore.

### Phase 4 — Dashboard + domain + TLS
S3 bucket + CloudFront distribution for the dashboard build. Route53 records and ACM certs for both the CloudFront and ALB origins. Set `ALLOWED_ORIGINS` to the real CloudFront/custom domain and `COOKIE_SECURE=true`, then confirm sign-up, sign-in, and the session cookie all survive the cross-origin split between the dashboard's origin and the ALB's — this is the first point in the whole plan where auth is genuinely cross-origin instead of same-origin-via-Vite-proxy, so it's the first real test of the `feat/auth` CORS/cookie config described in this doc's Current-state findings.

### Phase 5 — CI/CD
`.github/workflows/ci.yml` already validates every push/PR (typecheck, build, unit + integration tests, E2E). What's still missing: on push to `main`, build + push images to ECR and force a new ECS deployment. Extend the existing workflow with a `deploy` job (or add a new one gated on the existing jobs succeeding) rather than standing up a parallel pipeline.

### Phase 6 — Smoke test
Re-run the `stress-tester`'s `steady` scenario against the live AWS URL, the same way it was verified locally, to confirm guard/controller timing, SSE live updates, and a mid-rollout model-config change all behave the same in the deployed environment as they did locally. `.github/workflows/stress-smoke.yml`'s daily `burst`-mode run (see `documents/STRESS_TESTER.md`) is the closest existing precedent for automating this against a live URL, though today it only ever targets `localhost`.

---

## Open items / explicitly deferred

- **Management API** — this now exists (`POST /rollouts`, `PUT /models/{id}`, etc. — see `documents/ROLLOUT_CONTROLLER.md`), so Phase 3's `psql` seed step is no longer strictly needed; a rollout can be created via the API (or the dashboard's create-rollout form) once a tenant exists. Left as a documented fallback rather than removed from Phase 3, since it's still the simplest way to seed one from a script before the dashboard is reachable.
- **Multi-AZ / HA** — intentionally skipped for a demo-scale deployment; RDS and ElastiCache are both single-node. This is now a genuinely multi-tenant platform, not single-tenant as originally scoped here — the single-node sizing is still the right call for demo traffic, but "single-tenant" was never an accurate reason for it and shouldn't be cited as one.
- **Horizontal scaling of `edge-evaluator`/`model-service`** — both are stateless and could go beyond desired-count=1 later, but there's no load reason to do that yet.
