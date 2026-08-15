import type { Runner } from "./runner.js";

const CONTROLLER_URL =
  process.env.ROLLOUT_CONTROLLER_URL ?? "http://localhost:4003";

export interface Transition {
  elapsed: string;
  description: string;
}

interface RolloutState {
  candidatePercentage: number;
  held: boolean;
}

const c = {
  dim: (s: string) => `\x1b[2m${s}\x1b[0m`,
  green: (s: string) => `\x1b[32m${s}\x1b[0m`,
  yellow: (s: string) => `\x1b[33m${s}\x1b[0m`,
  red: (s: string) => `\x1b[31m${s}\x1b[0m`,
};

export class Monitor {
  private transitions: Transition[] = [];
  private lastState: RolloutState | null = null;
  private startMs = 0;
  private statsInterval: ReturnType<typeof setInterval> | null = null;
  private pollInterval: ReturnType<typeof setInterval> | null = null;

  constructor(private readonly runner: Runner) {}

  start(): void {
    this.startMs = Date.now();

    this.statsInterval = setInterval(() => this.printLine(), 1000);
    this.pollInterval = setInterval(() => void this.pollState(), 2000);
  }

  stop(): void {
    if (this.statsInterval) clearInterval(this.statsInterval);
    if (this.pollInterval) clearInterval(this.pollInterval);
  }

  getTransitions(): Transition[] {
    return this.transitions;
  }

  getLastState(): RolloutState | null {
    return this.lastState;
  }

  private elapsed(): string {
    const secs = Math.floor((Date.now() - this.startMs) / 1000);
    const m = String(Math.floor(secs / 60)).padStart(2, "0");
    const s = String(secs % 60).padStart(2, "0");
    return `${m}:${s}`;
  }

  private printLine(): void {
    const { totalRequests, errorCount, windowLatencies } =
      this.runner.stats;

    const elapsed = (Date.now() - this.startMs) / 1000 || 1;
    const rps = Math.round(totalRequests / elapsed);
    const errorPct =
      totalRequests > 0
        ? ((errorCount / totalRequests) * 100).toFixed(1)
        : "0.0";
    const p95 = percentile(windowLatencies, 0.95);

    const stateStr = this.lastState
      ? `${this.lastState.candidatePercentage}% ${this.lastState.held ? "HELD" : "RUNNING"}`
      : "connecting...";

    const stateColored = this.lastState?.held
      ? c.yellow(stateStr)
      : c.green(stateStr);

    process.stdout.write(
      `${c.dim(`[${this.elapsed()}]`)} RPS: ${String(rps).padStart(3)} | Errors: ${errorPct.padStart(4)}% | P95: ${String(p95).padStart(4)}ms | ${stateColored}\n`,
    );
  }

  private async pollState(): Promise<void> {
    try {
      const resp = await fetch(`${CONTROLLER_URL}/rollout`, {
        signal: AbortSignal.timeout(2000),
      });
      if (!resp.ok) return;

      const body = (await resp.json()) as {
        candidatePercentage: number;
        held: boolean;
      };
      const current: RolloutState = {
        candidatePercentage: body.candidatePercentage,
        held: body.held,
      };

      if (this.lastState !== null) {
        const prev = this.lastState;
        const t = this.elapsed();

        if (prev.candidatePercentage > 0 && current.candidatePercentage === 0) {
          // pct dropped to 0 — rollback (held=true) or complete (held=false)
          const isRollback = current.held;
          const transition: Transition = {
            elapsed: t,
            description: isRollback
              ? `ROLLBACK — candidate cleared, all traffic → stable`
              : `COMPLETE — rollout fully promoted`,
          };
          this.transitions.push(transition);
          process.stdout.write(
            isRollback
              ? `${c.red(`  ↙ [${t}] ${transition.description}`)}\n`
              : `${c.green(`  ✓ [${t}] ${transition.description}`)}\n`,
          );
        } else if (!prev.held && current.held) {
          const transition: Transition = {
            elapsed: t,
            description: `HOLD — advancing paused at ${current.candidatePercentage}%`,
          };
          this.transitions.push(transition);
          process.stdout.write(
            `${c.yellow(`  ⚠ [${t}] ${transition.description}`)}\n`,
          );
        } else if (
          !current.held &&
          current.candidatePercentage > prev.candidatePercentage
        ) {
          const transition: Transition = {
            elapsed: t,
            description: `ADVANCE — ${prev.candidatePercentage}% → ${current.candidatePercentage}%`,
          };
          this.transitions.push(transition);
          process.stdout.write(
            `${c.green(`  ↑ [${t}] ${transition.description}`)}\n`,
          );
        }
      }

      this.lastState = current;
    } catch {
      // ignore transient poll errors
    }
  }
}

function percentile(latencies: number[], p: number): number {
  if (latencies.length === 0) return 0;
  const sorted = [...latencies].sort((a, b) => a - b);
  const idx = Math.min(Math.floor(sorted.length * p), sorted.length - 1);
  return sorted[idx] ?? 0;
}
