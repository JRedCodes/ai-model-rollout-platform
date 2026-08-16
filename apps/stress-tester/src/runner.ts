import { randomUUID } from "node:crypto";

const USER_IDS = Array.from({ length: 200 }, (_, i) => `stress-user-${i}`);
const EDGE_URL = process.env.EDGE_EVALUATOR_URL ?? "http://localhost:4002";

export interface RunStats {
  totalRequests: number;
  successCount: number;
  errorCount: number;
  latencies: number[];
  windowLatencies: number[];
}

export class Runner {
  readonly stats: RunStats = {
    totalRequests: 0,
    successCount: 0,
    errorCount: 0,
    latencies: [],
    windowLatencies: [],
  };

  constructor(
    private readonly rps: number,
    private readonly durationSecs: number,
    private readonly apiKey: string,
  ) {}

  async run(): Promise<RunStats> {
    return new Promise((resolve) => {
      const intervalMs = Math.max(1, Math.round(1000 / this.rps));
      let inFlight = 0;
      const maxInFlight = this.rps * 3;
      let stopped = false;

      const doneTimer = setTimeout(() => {
        stopped = true;
        clearInterval(dispatchInterval);
        resolve(this.stats);
      }, this.durationSecs * 1000);
      doneTimer.unref();

      const dispatchInterval = setInterval(() => {
        if (stopped || inFlight >= maxInFlight) return;

        inFlight++;
        const userId = USER_IDS[Math.floor(Math.random() * USER_IDS.length)]!;
        const start = Date.now();

        sendRequest(userId, this.apiKey)
          .then((success) => {
            const latencyMs = Date.now() - start;
            this.stats.totalRequests++;
            if (success) this.stats.successCount++;
            else this.stats.errorCount++;
            this.recordLatency(latencyMs);
          })
          .catch(() => {
            this.stats.totalRequests++;
            this.stats.errorCount++;
          })
          .finally(() => {
            inFlight--;
          });
      }, intervalMs);
    });
  }

  private recordLatency(ms: number): void {
    this.stats.latencies.push(ms);
    this.stats.windowLatencies.push(ms);
    if (this.stats.windowLatencies.length > 200) {
      this.stats.windowLatencies.shift();
    }
  }
}

async function sendRequest(userId: string, apiKey: string): Promise<boolean> {
  const resp = await fetch(`${EDGE_URL}/v1/infer`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${apiKey}`,
    },
    body: JSON.stringify({
      requestId: randomUUID(),
      userId,
      input: { message: "stress-test" },
    }),
    signal: AbortSignal.timeout(5000),
  });

  if (!resp.ok) return false;
  const body = (await resp.json()) as { success: boolean };
  return body.success === true;
}
