import type { RunStats } from "./runner.js";
import type { Transition } from "./monitor.js";

const c = {
  bold: (s: string) => `\x1b[1m${s}\x1b[0m`,
  dim: (s: string) => `\x1b[2m${s}\x1b[0m`,
  cyan: (s: string) => `\x1b[36m${s}\x1b[0m`,
  green: (s: string) => `\x1b[32m${s}\x1b[0m`,
  red: (s: string) => `\x1b[31m${s}\x1b[0m`,
  yellow: (s: string) => `\x1b[33m${s}\x1b[0m`,
};

export function printReport(
  stats: RunStats,
  transitions: Transition[],
  durationSecs: number,
): void {
  const { totalRequests, successCount, errorCount, latencies } = stats;

  const errorRate =
    totalRequests > 0
      ? ((errorCount / totalRequests) * 100).toFixed(1)
      : "0.0";
  const successRate = (100 - parseFloat(errorRate)).toFixed(1);
  const achievedRps = (totalRequests / durationSecs).toFixed(1);
  const p50 = calcPercentile(latencies, 0.5);
  const p95 = calcPercentile(latencies, 0.95);
  const p99 = calcPercentile(latencies, 0.99);

  console.log();
  console.log(
    c.bold(
      c.cyan(
        "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
      ),
    ),
  );
  console.log(c.bold(c.cyan("  FINAL REPORT")));
  console.log(
    c.bold(
      c.cyan(
        "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
      ),
    ),
  );
  console.log();
  console.log(`  ${c.bold("Requests")}    ${totalRequests.toLocaleString()} total`);
  console.log(
    `              ${c.green(`${successCount.toLocaleString()} succeeded`)}  (${successRate}%)`,
  );
  console.log(
    `              ${errorCount > 0 ? c.red(`${errorCount.toLocaleString()} failed`) : `${errorCount} failed`}  (${errorRate}%)`,
  );
  console.log();
  console.log(`  ${c.bold("Throughput")}  ${achievedRps} RPS`);
  console.log();
  console.log(`  ${c.bold("Latency")}     P50  ${p50}ms`);
  console.log(`               P95  ${p95}ms`);
  console.log(`               P99  ${p99}ms`);
  console.log();

  if (transitions.length > 0) {
    console.log(`  ${c.bold("Decisions")}`);
    for (const t of transitions) {
      const colorFn = t.description.startsWith("ROLLBACK")
        ? c.red
        : t.description.startsWith("HOLD")
          ? c.yellow
          : c.green;
      console.log(`    ${c.dim(`[${t.elapsed}]`)}  ${colorFn(t.description)}`);
    }
  } else {
    console.log(`  ${c.dim("No rollout decisions observed during run")}`);
  }

  console.log();
  console.log(
    c.bold(
      c.cyan(
        "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
      ),
    ),
  );
  console.log();
}

function calcPercentile(latencies: number[], p: number): number {
  if (latencies.length === 0) return 0;
  const sorted = [...latencies].sort((a, b) => a - b);
  const idx = Math.min(
    Math.floor(sorted.length * p),
    sorted.length - 1,
  );
  return sorted[idx] ?? 0;
}
