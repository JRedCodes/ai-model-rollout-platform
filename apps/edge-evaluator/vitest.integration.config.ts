import { defineConfig } from "vitest/config";

// Requires a running Redis (REDIS_URL) and, for
// model-service.client.integration.test.ts, a running model-service
// (MODEL_SERVICE_URL). See docker-compose.yaml at the repo root.
export default defineConfig({
  test: {
    include: ["tests/integration/**/*.test.ts"],
  },
});
