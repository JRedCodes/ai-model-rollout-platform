# Deployment Plan — AWS

Target: an always-on, publicly reachable deployment of the AI Model Rollout Platform on AWS.

Note on sequencing: the README roadmap already lists **Authentication** as a future item, after the Management API and Promotion Loop. This plan doesn't jump that queue — it adds a minimal, temporary shared-secret gate on the two mutating endpoints (`PUT /models/{id}`, `POST /rollout/rollback`) purely because they'd otherwise be open to the entire internet the moment this goes live. That stopgap is meant to be deleted once real auth lands.

---

## Current-state findings that shape this plan

- **`rollout-controller` is a stateful singleton**, not a horizontally-scalable service. It holds in-memory atomic state (`held`, `rolledBack`, `pct`), owns the sole membership in its Redis Streams consumer group, and runs an in-process SSE hub. It must always run at **desired-count = 1**. A container that just runs continuously (ECS Fargate) is a better fit than a request-triggered, scale-to-zero model (Cloud Run) for this reason — its most important work (5s guard tick, 2-min controller tick, stream consumption) happens on background goroutines with no HTTP request involved at all.
- **`main.go` silently falls back to `localhost`** for `REDIS_URL`/`DATABASE_URL` if unset (`envOr(...)`). In a container this means a missing env var fails weirdly (tries to reach `localhost` inside the container) instead of failing fast at boot.
- **The dashboard's API calls are relative** (`const BASE = "/api"` in `api.ts`, `new EventSource("/api/events")` in `useSSE.ts`), and only resolve today because Vite's dev server proxies `/api` → `localhost:4003`. Once the dashboard is served from CloudFront and the API from an ALB on a different origin, there is no equivalent proxy — needs a build-time absolute API base URL.
- **CORS on the Go API is wide open** (`Access-Control-Allow-Origin: *`), which is actually convenient for a cross-origin dashboard/API split, but combined with no auth means anyone can currently call the mutating endpoints.
- **No Dockerfiles exist yet** for any of the four services — only Redis has a `docker-compose.yaml` entry today.
- **No CI** exists — nothing currently validates a build/typecheck/`go vet` before it reaches a deploy step.
- **Migrations self-run on boot** (`db.RunMigrations` in `main.go`) — convenient for deploys (no separate migration step), but means the ECS task's DB credentials need schema-modify privileges, and the first `rollouts` row still has to be seeded by hand via `psql` (Management API isn't built yet, per roadmap) after the first deploy.

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
| Secrets | Secrets Manager → ECS task secrets | DB password and the shared-secret auth token never sit in plaintext in a task definition |
| Images | ECR, one repo per service | Standard ECS image source |
| IaC | Terraform | Reproducible; cheap to tear down/rebuild when not actively demoing |
| CI/CD | GitHub Actions → ECR → ECS | Closes the "no CI" gap and makes deploys a `git push`, not a manual `docker push` |

---

## Prerequisite code changes (before any AWS work)

1. **Fail fast on missing config.** In `main.go`, require `DATABASE_URL` and `REDIS_URL` — `log.Fatalf` if unset, rather than defaulting to `localhost`. Keep a `localhost` default only behind an explicit local-dev path if that's useful, but never as the production behavior.
2. **Add a build-time API base URL to the dashboard.** Introduce `VITE_API_URL`, defaulting to `/api` so local dev via the Vite proxy is unaffected. Update `api.ts`'s `BASE` and `useSSE.ts`'s `EventSource` URL to use it.
3. **Add a shared-secret gate on mutating endpoints.** `PUT /models/{id}` and `POST /rollout/rollback` should require a header (e.g. `X-Deploy-Token`) checked against a value from Secrets Manager. Temporary, deleted once the roadmap's real Authentication item ships.
4. **Write a Dockerfile per service:**
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
Push images to ECR. Stand up the three Fargate services with task definitions pulling secrets from Secrets Manager. Wire ALB target groups + path-based routing. Seed the first `rollouts` row via `psql` against RDS (same manual step as local setup today, per the README's "Seed your first rollout").

### Phase 4 — Dashboard + domain + TLS
S3 bucket + CloudFront distribution for the dashboard build. Route53 records and ACM certs for both the CloudFront and ALB origins. Confirm the shared-secret header flows from dashboard → API for the mutating endpoints.

### Phase 5 — CI/CD
GitHub Actions workflow: on push to `main`, run `npm run typecheck`/`npm run build` and `go build ./... && go vet ./...` (this alone closes a real gap — nothing validates a build today); on success, build + push images to ECR and force a new ECS deployment.

### Phase 6 — Smoke test
Re-run the `stress-tester`'s `steady` scenario against the live AWS URL, the same way it was verified locally, to confirm guard/controller timing, SSE live updates, and a mid-rollout model-config change all behave the same in the deployed environment as they did locally.

---

## Open items / explicitly deferred

- **Real authentication** — tracked on the README roadmap already; this plan's shared-secret header is a placeholder, not a substitute.
- **Management API** — still no HTTP way to create a rollout; Phase 3's manual `psql` seed step stays until that roadmap item ships.
- **Multi-AZ / HA** — intentionally skipped for a single-tenant portfolio deployment; RDS and ElastiCache are both single-node.
- **Horizontal scaling of `edge-evaluator`/`model-service`** — both are stateless and could go beyond desired-count=1 later, but there's no load reason to do that yet.
