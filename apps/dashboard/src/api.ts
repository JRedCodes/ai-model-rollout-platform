const BASE = "/api";

export interface RolloutState {
  rolloutId: string;
  rolloutPhaseId: string;
  stableModelVersionId: string;
  candidateModelVersionId: string;
  candidatePercentage: number;
  held: boolean;
}

export interface RolloutMetrics {
  totalRequests: number;
  overallErrorRate: number;
  windowRequestCount: number;
  windowErrorRate: number;
  windowP95LatencyMs: number;
}

export interface Decision {
  action: string;
  reason: string;
  source: string;
  decidedAt: string;
}

export interface ModelConfig {
  modelVersionId: string;
  failureRate: number;
  minLatencyMs: number;
  maxLatencyMs: number;
}

export interface ModelConfigUpdate {
  failureRate: number;
  minLatencyMs: number;
  maxLatencyMs: number;
}

export async function fetchRollout(): Promise<RolloutState> {
  const res = await fetch(`${BASE}/rollout`);
  if (!res.ok) throw new Error(`GET /rollout: ${res.status}`);
  return res.json() as Promise<RolloutState>;
}

export async function fetchMetrics(): Promise<RolloutMetrics> {
  const res = await fetch(`${BASE}/rollout/metrics`);
  if (!res.ok) throw new Error(`GET /rollout/metrics: ${res.status}`);
  return res.json() as Promise<RolloutMetrics>;
}

export async function fetchDecisions(): Promise<Decision[]> {
  const res = await fetch(`${BASE}/rollout/decisions`);
  if (!res.ok) throw new Error(`GET /rollout/decisions: ${res.status}`);
  return res.json() as Promise<Decision[]>;
}

export async function postRollback(): Promise<void> {
  const res = await fetch(`${BASE}/rollout/rollback`, { method: "POST" });
  if (!res.ok) throw new Error(`POST /rollout/rollback: ${res.status}`);
}

export async function fetchModelConfigs(): Promise<ModelConfig[]> {
  const res = await fetch(`${BASE}/models`);
  if (!res.ok) throw new Error(`GET /models: ${res.status}`);
  return res.json() as Promise<ModelConfig[]>;
}

export async function updateModelConfig(
  modelVersionId: string,
  update: ModelConfigUpdate,
): Promise<ModelConfig> {
  const res = await fetch(`${BASE}/models/${modelVersionId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(update),
  });
  if (!res.ok) throw new Error(`PUT /models/${modelVersionId}: ${res.status}`);
  return res.json() as Promise<ModelConfig>;
}
