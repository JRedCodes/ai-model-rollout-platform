import { getApiKey, clearApiKey } from "./apiKey.ts";
import { API_BASE } from "./apiBase.ts";

const BASE = API_BASE;

export class UnauthorizedError extends Error {
  constructor() {
    super("Unauthorized — API key missing or invalid");
    this.name = "UnauthorizedError";
  }
}

// Wraps fetch: prefers a legacy stored API key (Bearer header) when
// present, otherwise relies on the session cookie (credentials: include)
// -- authMiddleware tries Bearer first either way, so a stored key always
// wins if both exist. On a 401, clears any stored key (so the App root
// falls back to the sign-in view if it was relying on one) and throws a
// typed error instead of letting callers see an opaque failed response.
async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const apiKey = getApiKey();
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
      ...init.headers,
    },
  });

  if (res.status === 401) {
    clearApiKey();
    throw new UnauthorizedError();
  }

  return res;
}

export type RolloutState =
  | { active: false }
  | {
      active: true;
      rolloutId: string;
      rolloutPhaseId: string;
      stableModelVersionId: string;
      candidateModelVersionId: string;
      candidatePercentage: number;
      held: boolean;
    };

export type RolloutMetrics =
  | { active: false }
  | {
      active: true;
      totalRequests: number;
      overallErrorRate: number;
      windowRequestCount: number;
      windowErrorRate: number;
      windowP95LatencyMs: number;
    };

export interface Rollout {
  id: string;
  rolloutPhaseId: string;
  stableModelVersionId: string;
  candidateModelVersionId: string | null;
  candidatePercentage: number;
  status: string;
  createdAt: string;
}

export interface CreateRolloutInput {
  rolloutPhaseId: string;
  stableModelVersionId?: string;
  candidateModelVersionId: string;
  candidatePercentage?: number;
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
  const res = await apiFetch(`/rollout`);
  if (!res.ok) throw new Error(`GET /rollout: ${res.status}`);
  return res.json() as Promise<RolloutState>;
}

export async function fetchMetrics(): Promise<RolloutMetrics> {
  const res = await apiFetch(`/rollout/metrics`);
  if (!res.ok) throw new Error(`GET /rollout/metrics: ${res.status}`);
  return res.json() as Promise<RolloutMetrics>;
}

export async function fetchDecisions(): Promise<Decision[]> {
  const res = await apiFetch(`/rollout/decisions`);
  if (!res.ok) throw new Error(`GET /rollout/decisions: ${res.status}`);
  return res.json() as Promise<Decision[]>;
}

export async function postRollback(): Promise<void> {
  const res = await apiFetch(`/rollout/rollback`, { method: "POST" });
  if (!res.ok) throw new Error(`POST /rollout/rollback: ${res.status}`);
}

export async function createRollout(input: CreateRolloutInput): Promise<Rollout> {
  const res = await apiFetch(`/rollouts`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const message = await res.text();
    throw new Error(message || `POST /rollouts: ${res.status}`);
  }
  return res.json() as Promise<Rollout>;
}

export async function regenerateApiKey(): Promise<{ apiKey: string }> {
  const res = await apiFetch(`/auth/regenerate-key`, { method: "POST" });
  if (!res.ok) throw new Error(`POST /auth/regenerate-key: ${res.status}`);
  return res.json() as Promise<{ apiKey: string }>;
}

export async function signOut(): Promise<void> {
  const res = await apiFetch(`/auth/signout`, { method: "POST" });
  if (!res.ok) throw new Error(`POST /auth/signout: ${res.status}`);
}

export async function fetchModelConfigs(): Promise<ModelConfig[]> {
  const res = await apiFetch(`/models`);
  if (!res.ok) throw new Error(`GET /models: ${res.status}`);
  return res.json() as Promise<ModelConfig[]>;
}

export async function updateModelConfig(
  modelVersionId: string,
  update: ModelConfigUpdate,
): Promise<ModelConfig> {
  const res = await apiFetch(`/models/${modelVersionId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(update),
  });
  if (!res.ok) throw new Error(`PUT /models/${modelVersionId}: ${res.status}`);
  return res.json() as Promise<ModelConfig>;
}
