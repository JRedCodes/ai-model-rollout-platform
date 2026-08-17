import { defineConfig } from "vitest/config";

// Unit tests only -- colocated with source, no external services required.
// Integration tests (tests/integration/**) have their own config and their
// own npm script, since they need a running Redis (and model-service).
export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
  },
});
