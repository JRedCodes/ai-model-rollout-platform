import { parseArgs } from "node:util";

import { Monitor } from "./monitor.js";
import { reset } from "./reset.js";
import { printReport } from "./report.js";
import { Runner } from "./runner.js";

const { values } = parseArgs({
  options: {
    mode: { type: "string", default: "steady" },
    reset: { type: "boolean", default: false },
  },
  strict: false,
});

const mode: "steady" | "burst" =
  values.mode === "burst" ? "burst" : "steady";

const SCENARIOS = {
  steady: {
    rps: 50,
    durationSecs: 300,
    label: "50 RPS × 5 min",
    description:
      "Exercises controller advance → hold path. No model config changes needed.",
    modelConfig: "model-v2 at 8% failure rate (default — no changes needed)",
    expected:
      "Controller advances 10% → 25% at ~2 min, then holds when error rate exceeds 2%",
  },
  burst: {
    rps: 200,
    durationSecs: 30,
    label: "200 RPS × 30 s",
    description:
      "Exercises guard fresh-window rollback. Requires a model config change.",
    modelConfig: [
      "model-v2 must be at 35% failure rate",
      "  Edit:    apps/model-service/src/config/models.ts",
      "  Change:  failureRate: 0.08  →  failureRate: 0.35",
      "  Restart: npm run dev --workspace @rollout-platform/model-service",
      "",
      "  --reset also sets candidate_percentage to 100 so all traffic hits the candidate.",
    ].join("\n  "),
    expected:
      "Guard trips fresh window (>30% errors in last 50 req) within ~1s → rollback",
  },
} as const;

const scenario = SCENARIOS[mode];

const c = {
  bold: (s: string) => `\x1b[1m${s}\x1b[0m`,
  dim: (s: string) => `\x1b[2m${s}\x1b[0m`,
  cyan: (s: string) => `\x1b[36m${s}\x1b[0m`,
  yellow: (s: string) => `\x1b[33m${s}\x1b[0m`,
};

console.log();
console.log(
  c.bold(
    c.cyan(
      "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
    ),
  ),
);
console.log(c.bold(c.cyan("  AI Model Rollout Platform — Stress Tester")));
console.log(
  c.bold(
    c.cyan(
      "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
    ),
  ),
);
console.log();
console.log(
  `  ${c.bold("Mode")}:      ${mode === "burst" ? c.yellow(mode) : mode}  ${c.dim("(" + scenario.label + ")")}`,
);
console.log(`  ${c.bold("Profile")}:   ${scenario.description}`);
console.log();
console.log(`  ${c.bold("MODEL CONFIG")}`);
scenario.modelConfig
  .split("\n")
  .forEach((line) => console.log(`  ${line}`));
console.log();
console.log(`  ${c.bold("EXPECTED")}:  ${scenario.expected}`);
console.log();
console.log(
  c.dim(
    "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
  ),
);
console.log();

if (values.reset) {
  process.stdout.write("  Resetting rollout in Postgres...");
  await reset(mode);
  console.log(" ✓");
  console.log();
  console.log(
    c.yellow(
      "  ⚠  Restart the rollout controller so it reloads config from DB and re-seeds Redis.",
    ),
  );
  console.log(
    c.yellow(
      "     Then run again without --reset to begin the load test.",
    ),
  );
  console.log();
  process.exit(0);
}

const runner = new Runner(scenario.rps, scenario.durationSecs);
const monitor = new Monitor(runner);

monitor.start();
const finalStats = await runner.run();
monitor.stop();

printReport(finalStats, monitor.getTransitions(), scenario.durationSecs);
