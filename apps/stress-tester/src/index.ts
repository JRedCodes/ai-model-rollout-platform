import { parseArgs } from "node:util";

import { Monitor } from "./monitor.js";
import { reset } from "./reset.js";
import { printReport } from "./report.js";
import { Runner } from "./runner.js";

const { values } = parseArgs({
  options: {
    mode: { type: "string", default: "steady" },
    reset: { type: "boolean", default: false },
    apiKey: { type: "string" },
  },
  strict: false,
});

const mode: "steady" | "burst" = values.mode === "burst" ? "burst" : "steady";

// Which tenant this run is paired with. Defaults to the seeded demo
// tenant's key so the existing scenarios keep working unmodified, but
// running two instances with different --apiKey values (one per tenant)
// exercises two rollouts' guard/controller pipelines independently.
const DEFAULT_DEMO_API_KEY = "tk_demo_2218a6e29efe8f4b3378390b46a0710d";
const apiKey =
  (typeof values.apiKey === "string" ? values.apiKey : undefined) ??
  process.env.TENANT_API_KEY ??
  DEFAULT_DEMO_API_KEY;

const SCENARIOS = {
  steady: {
    rps: 50,
    durationSecs: 600,
    label: "50 RPS × 10 min",
    description: "Exercises the full advance ladder to COMPLETE. No model config changes needed.",
    modelConfig: "model-v2 at 2% failure rate, 50–200ms latency (default — no changes needed)",
    expected:
      "Controller advances 10→25→50→75→100→COMPLETE over ~8 min (one step per 2-min window)",
  },
  burst: {
    rps: 200,
    durationSecs: 30,
    label: "200 RPS × 30 s",
    description: "Exercises guard fresh-window rollback. Requires a model config change.",
    modelConfig: [
      "model-v2 must be at 35% failure rate",
      "  Set:  curl -X PUT localhost:4003/models/model-v2 \\",
      '          -d \'{"failureRate":0.35,"minLatencyMs":50,"maxLatencyMs":200}\'',
      "        (or use the dashboard's Model configuration panel)",
      "        Takes effect immediately — no restart needed.",
      "",
      "  --reset also sets candidate_percentage to 100 so all traffic hits the candidate.",
    ].join("\n  "),
    expected: "Guard trips fresh window (>30% errors in last 50 req) within ~1s → rollback",
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
console.log(c.bold(c.cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")));
console.log(c.bold(c.cyan("  AI Model Rollout Platform — Stress Tester")));
console.log(c.bold(c.cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")));
console.log();
console.log(
  `  ${c.bold("Mode")}:      ${mode === "burst" ? c.yellow(mode) : mode}  ${c.dim("(" + scenario.label + ")")}`,
);
console.log(`  ${c.bold("Tenant key")}: ${c.dim(apiKey.slice(0, 12) + "…")}`);
console.log(`  ${c.bold("Profile")}:   ${scenario.description}`);
console.log();
console.log(`  ${c.bold("MODEL CONFIG")}`);
scenario.modelConfig.split("\n").forEach((line) => console.log(`  ${line}`));
console.log();
console.log(`  ${c.bold("EXPECTED")}:  ${scenario.expected}`);
console.log();
console.log(c.dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"));
console.log();

if (values.reset) {
  process.stdout.write("  Resetting rollout in Postgres...");
  await reset(mode, apiKey);
  console.log(" ✓");
  console.log();
  console.log(
    c.yellow("  ⚠  Restart the rollout controller: this reset an already-active rollout in"),
  );
  console.log(
    c.yellow("     place via SQL, which the supervisor's change detection doesn't pick up"),
  );
  console.log(
    c.yellow("     on its own (it only notices a rollout's ID appearing/changing/disappearing)."),
  );
  console.log(c.yellow("     Then run again without --reset to begin the load test."));
  console.log();
  process.exit(0);
}

const runner = new Runner(scenario.rps, scenario.durationSecs, apiKey);
const monitor = new Monitor(runner, apiKey);

monitor.start();
const finalStats = await runner.run();
monitor.stop();

printReport(finalStats, monitor.getTransitions(), scenario.durationSecs);
