// Curated content for the About page. Adapted (not copy-pasted verbatim)
// from README.md's Architecture/Services/Multi-tenancy sections, trimmed
// to what a visitor exploring the live demo needs -- not the developer-
// facing sections (Roadmap, Testing, env var tables) that belong in the
// README instead. Keep this in sync with README.md's Architecture section
// if the system's shape changes.

export const architectureDiagram = `
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
                                                                        │ users, sessions      │
                                                                        └──────────────────────┘
`.replace(/^\n/, "");

export interface FlowStep {
  title: string;
  description: string;
}

export const flowSteps: FlowStep[] = [
  {
    title: "Request flow",
    description:
      "Every simulated inference request hits the Edge Evaluator first. It reads the active feature flag from Redis, deterministically buckets the request into stable or candidate traffic, and forwards it to the Model Service -- which simulates a response (with a configurable failure rate and latency range) rather than calling a real model. The Edge Evaluator then publishes a telemetry event to a Redis Stream.",
  },
  {
    title: "Control flow",
    description:
      "The Rollout Controller (written in Go) continuously consumes that telemetry stream. Two loops watch it: a fast guard, every 5 seconds, that can trip an immediate rollback if error rates spike; and a controller, every 2 minutes, that decides whether to advance traffic to the candidate model, hold, or resume after a hold clears. Every decision is written to Postgres and pushed to Redis.",
  },
  {
    title: "Dashboard flow",
    description:
      "The dashboard is a real-time control panel, not a static report. It holds an SSE connection to the controller and re-fetches the moment any decision fires, so a guard rollback or a controller advance shows up within milliseconds -- no polling delay.",
  },
  {
    title: "Multi-tenancy & accounts",
    description:
      "Every account gets its own tenant: its own rollout, its own guard/controller pipeline, its own model catalog, fully isolated from every other visitor exploring this demo. Signing up creates that tenant and issues an API key (shown once) for driving traffic through the stress-tester CLI -- everything you see in the dashboard is scoped to your tenant alone.",
  },
];

export interface ServiceSummary {
  name: string;
  tech: string;
  location: string;
  summary: string;
}

export const services: ServiceSummary[] = [
  {
    name: "Edge Evaluator",
    tech: "TypeScript, port 4002",
    location: "apps/edge-evaluator",
    summary:
      "The entry point for simulated traffic. Authenticates the request, decides stable vs. candidate, forwards to the Model Service, and publishes telemetry.",
  },
  {
    name: "Model Service",
    tech: "TypeScript, port 4001",
    location: "apps/model-service",
    summary:
      "Simulates inference with a configurable failure rate and latency range per model version, so a rollout's health can be steered on demand -- no real model runs here.",
  },
  {
    name: "Rollout Controller",
    tech: "Go, port 4003",
    location: "apps/rollout-controller",
    summary:
      "The brains of the system: ingests telemetry, runs the guard and controller decision loops, owns every write to Postgres and Redis, and serves the management API and dashboard authentication.",
  },
  {
    name: "Dashboard",
    tech: "React + TypeScript, port 5173",
    location: "apps/dashboard",
    summary:
      "This app. A real-time control panel for monitoring and operating a live rollout, plus account and tenant management.",
  },
  {
    name: "Stress Tester",
    tech: "TypeScript CLI",
    location: "apps/stress-tester",
    summary:
      "A local load-generation tool that drives traffic through the Edge Evaluator and reports how the rollout progressed -- this is what actually makes a rollout advance.",
  },
];
