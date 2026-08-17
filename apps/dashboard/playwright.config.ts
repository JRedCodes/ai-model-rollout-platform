import { defineConfig, devices } from "@playwright/test";

// Assumes the full stack (redis, postgres, model-service, edge-evaluator,
// rollout-controller, and this dashboard) is already running -- locally via
// each service's own dev command, or in CI via docker compose + built
// binaries (see .github/workflows/ci.yml's e2e job). No webServer
// auto-start here since Playwright can only manage one process, and this
// suite needs the whole stack, not just the dashboard.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: "list",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
