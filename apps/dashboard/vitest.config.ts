import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    // vite build doesn't leak test files into dist/ the way the other
    // packages' plain tsc builds can, but scope explicitly anyway for
    // consistency and defense-in-depth (see model-service/contracts'
    // vitest.config.ts, added after a stale dist/ caused every test in
    // those packages to silently run twice).
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
