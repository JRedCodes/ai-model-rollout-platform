import { defineConfig } from "vitest/config";

// Scoped to src/ so a stale dist/ (tsc output, now excluded from actually
// containing *.test.ts -- see tsconfig.json) can never get picked up
// alongside the real source tests and double-run everything.
export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
  },
});
