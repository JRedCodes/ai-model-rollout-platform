# Deployment Plan — AWS

Target: an always-on, publicly reachable deployment of the AI Model Rollout Platform on AWS.

This plan originally added a temporary shared-secret gate on the mutating endpoints as a stopgap, since real auth wasn't built yet. That's no longer needed: `feat/auth` shipped real multi-tenant authentication first — bcrypt-hashed passwords, httpOnly-cookie sessions for the dashboard, and tenant API keys for everything else (`Authorization: Bearer`). `POST /tenants` (`X-Admin-Key`-gated, for ops/scripted tenant creation — e.g. the seeded demo tenant) and `POST /auth/signup` (self-serve, creates a tenant + the user that owns it together) now coexist as two independent ways to get a tenant; every mutating endpoint already requires one of the two auth paths to resolve a tenant before this plan even starts. See `documents/ROLLOUT_CONTROLLER.md`'s auth section for the design.

---

## Current-state findings that shape this plan

- **`rollout-controller` is a stateful singleton**, not a horizontally-scalable service. It holds in-memory atomic state (`held`, `rolledBack`, `pct`), owns the sole membership in its Redis Streams consumer group, and runs an in-process SSE hub. It must always run at **desired-count = 1**. A container that just runs continuously (ECS Fargate) is a better fit than a request-triggered, scale-to-zero model (Cloud Run) for this reason — its most important work (5s guard tick, 2-min controller tick, stream consumption) happens on background goroutines with no HTTP request involved at all.
- ~~**`main.go` silently falls back to `localhost`** for `REDIS_URL`/`DATABASE_URL` if unset~~ — **done.** Both are now required (`requireEnv`), `log.Fatalf` at boot if either is unset. Local dev now exports them explicitly (see README's Getting Started) rather than relying on a default.
- ~~**The dashboard's API calls are relative**~~ — **done.** `VITE_API_URL` (default `/api`, unchanged local-dev behavior) is now a build-time env var, wired through a shared `apiBase.ts` used by both `api.ts` and `useSSE.ts`. Still needs to actually be set to the real ALB origin at deploy time (Phase 4).
- **CORS on the Go API is already origin-allowlisted and credentialed** (`ALLOWED_ORIGINS` env var, default `http://localhost:5173`, plus `Access-Control-Allow-Credentials: true` for the session cookie) rather than wide open — a wildcard origin can't be combined with credentialed requests at all, browsers refuse it outright, so this had to change before real auth could ship, not after. Still needs `ALLOWED_ORIGINS` to actually include the deployed dashboard's real origin once one exists (CloudFront domain / custom domain), and `COOKIE_SECURE=true` for the session cookie to survive the CloudFront/ALB cross-origin split (see `documents/ROLLOUT_CONTROLLER.md`'s Configuration table).
- ~~**No Dockerfiles exist yet**~~ — **done.** All four services have one (`apps/*/Dockerfile`), and `docker-compose.yaml` builds and runs all four alongside Redis and Postgres — verified end-to-end (sign-up, rollout creation, real inference traffic, SSE, the dashboard's nginx proxy) against an isolated copy of the exact compose config, not just written and assumed correct.
- **CI validates every push/PR** (`.github/workflows/ci.yml`: typecheck, build, unit tests, integration tests, and a full-stack Playwright E2E job, plus a separate scheduled load-test smoke workflow) but doesn't build or push any container image, and there's no deploy step — that gap is what Phase 5 below still needs to close.
- **Migrations self-run on boot** (`db.RunMigrations` in `main.go`) — convenient for deploys (no separate migration step), but means the ECS task's DB credentials need schema-modify privileges. The first tenant and rollout can now be created through the Management API (or the dashboard) after the first deploy — no `psql` step required, unlike when this plan was first written.

---

## Target architecture

| Piece                | AWS service                                 | Why                                                                                                                                                         |
| -------------------- | ------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rollout-controller` | ECS Fargate, desired count = 1              | Singleton by design — see above                                                                                                                             |
| `edge-evaluator`     | ECS Fargate, desired count = 1              | Stateless today; could scale later, starting at 1 to match current behavior                                                                                 |
| `model-service`      | ECS Fargate, desired count = 1              | Same as above                                                                                                                                               |
| `dashboard`          | S3 + CloudFront                             | Static Vite build — no reason to spend a container on it                                                                                                    |
| Postgres             | RDS, single `db.t4g.micro`, no Multi-AZ     | Demo scale, not HA; replaces the local trust-auth instance                                                                                                  |
| Redis                | ElastiCache, single `cache.t4g.micro`       | Same role as the current Docker Redis                                                                                                                       |
| Routing              | Application Load Balancer, path-based rules | Dashboard talks to one API origin; ALB fans out to the three backend services                                                                               |
| TLS / domain         | ACM + Route53                               | One cert for CloudFront (dashboard), one for the ALB (API)                                                                                                  |
| Secrets              | Secrets Manager → ECS task secrets          | DB password and `ADMIN_API_KEY` (gates `POST /tenants`) never sit in plaintext in a task definition                                                         |
| Images               | ECR, one repo per service                   | Standard ECS image source                                                                                                                                   |
| IaC                  | Terraform                                   | Reproducible; cheap to tear down/rebuild when not actively demoing                                                                                          |
| CI/CD                | GitHub Actions → ECR → ECS                  | Extends the existing validate-on-every-PR workflow with an image build/push/deploy step, making a deploy a `git push` to `main`, not a manual `docker push` |

---

## Prerequisite code changes (before any AWS work)

**All three complete** as of `chore/production-hardening` — this section is kept as a record of what shaped Phase 1, not a remaining TODO.

1. ~~**Fail fast on missing config.**~~ `main.go` now requires `DATABASE_URL` and `REDIS_URL` via `requireEnv`, `log.Fatalf` if either is unset — no `localhost` fallback in any environment, local dev included.
2. ~~**Add a build-time API base URL to the dashboard.**~~ `VITE_API_URL` (default `/api`) is wired through `apiBase.ts`, used by both `api.ts` and `useSSE.ts`.
3. ~~**Write a Dockerfile per service.**~~ All four exist (`apps/*/Dockerfile`) and match the shapes originally specced here — `rollout-controller` and the two Node services multi-stage as planned; `dashboard` ended up with a runtime stage too (nginx serving the static build) so `docker-compose up` could actually run and curl it like the other three for local parity testing, not because it needs one in the real AWS deploy (that's still S3 + CloudFront, per Target architecture above).

---

## Phased rollout

### Phase 1 — Containerize locally — **done**

The four Dockerfiles and both prerequisite code changes are in (see above). `docker compose up` (all four app services + Postgres + Redis) verified against a fully isolated copy of the compose config: sign-up, rollout creation, real inference traffic through edge-evaluator → model-service, SSE, and the dashboard's nginx-proxied API access all confirmed working, including the two eventual-consistency lags (~5s supervisor reconcile, 60s model-config heartbeat) that are expected baseline behavior, not containerization bugs — see `documents/ROLLOUT_CONTROLLER.md`. No AWS involved yet; Phase 2 is the next actual step.

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
