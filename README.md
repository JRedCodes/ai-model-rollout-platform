# AI Model Rollout Platform

A distributed backend platform for safely deploying new AI model versions through progressive rollouts.

The project simulates how production AI infrastructure incrementally shifts traffic from a stable model to a candidate model while validating requests, routing traffic, publishing telemetry, and automatically reacting to rollout health. It is being built incrementally as a learning project with each subsystem developed as an isolated engineering milestone before integrating into the complete platform.

---

## Current Status

### Completed

- TypeScript monorepo using npm workspaces
- Shared contracts package using Zod
- Model Service
  - Simulated model inference
  - Request validation
  - Configurable latency
  - Configurable failure rates
- Edge Evaluator
  - Request validation
  - Deterministic traffic assignment
  - Service-to-service communication
  - Redis-backed feature flag retrieval
- Redis infrastructure using Docker Compose
- Unit and integration tests with Vitest

### In Progress

- Telemetry event publishing
- Background worker
- Rollout controller
- Real-time dashboard
- CLI regression runner

---

## Architecture

```
                +----------------+
                | Stress Tester  |
                +--------+-------+
                         |
                         |
                         v
                +----------------+
                | Edge Evaluator |
                +--------+-------+
                         |
                         |
                         v
                +----------------+
                | Model Service  |
                +--------+-------+
                         |
                         |
                         v
                  Model Response

               Redis Feature Flags
                       ^
                       |
                +--------------+
                | Redis Cache  |
                +--------------+
```

The current system uses Redis as the runtime configuration source for feature flags. The Edge Evaluator retrieves the active rollout configuration from Redis before selecting which model receives each inference request.

---

## Technologies

- TypeScript
- Node.js
- Express
- Redis
- Docker Compose
- Zod
- Vitest

---

## Repository Structure

```
apps/
    edge-evaluator/
    model-service/

packages/
    contracts/
```

### Edge Evaluator

Responsible for:

- validating incoming requests
- retrieving routing configuration
- selecting a model version
- forwarding inference requests
- returning normalized responses

### Model Service

Responsible for:

- simulating model inference
- configurable latency
- configurable failure rates
- returning standardized inference responses

### Contracts

Shared request and response schemas used by all services.

---

## Running the Project

### Install dependencies

```bash
npm install
```

### Start Redis

```bash
docker compose up -d redis
```

### Start the Model Service

```bash
npm run dev --workspace @rollout-platform/model-service
```

### Start the Edge Evaluator

```bash
npm run dev --workspace @rollout-platform/edge-evaluator
```

---

## Example Request

```bash
curl -X POST \
http://localhost:4002/v1/infer \
-H "Content-Type: application/json" \
-d '{
  "requestId": "request-1",
  "userId": "user-42",
  "input": {
    "message": "Hello from the Edge Evaluator"
  }
}'
```

---

## Testing

Run unit tests

```bash
npm test --workspace @rollout-platform/edge-evaluator
```

Run integration tests

```bash
npm run test:integration --workspace @rollout-platform/edge-evaluator
```

---

## Roadmap

- [x] Model Service
- [x] Edge Evaluator
- [x] Redis-backed feature flags
- [ ] Telemetry event publishing
- [ ] Background telemetry worker
- [ ] Rollout controller
- [ ] PostgreSQL persistence
- [ ] React dashboard
- [ ] Live metrics
- [ ] CLI regression runner
- [ ] Authentication

---

## Purpose

This project is designed to explore backend engineering concepts commonly found in modern AI infrastructure, including:

- distributed systems
- service-to-service communication
- runtime configuration management
- feature flags
- event-driven architecture
- resilient API design
- testing strategies
- scalable backend architecture