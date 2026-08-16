import {
  modelSimulationProfileSchema,
  type ModelSimulationProfile,
} from "@rollout-platform/contracts";

import { redisClient } from "../redis/redis.client.js";

const REDIS_KEY_PREFIX = "model-config:";

// Used only when Redis is unreachable or returns unusable data for a model
// version that otherwise has a valid config seeded — keeps inference
// serving through a transient Redis outage instead of failing every
// request. A genuinely missing config (valid Redis connection, no key)
// still surfaces as UnsupportedModelVersionError below.
const DEFAULT_FAILURE_RATE = 0.01;
const DEFAULT_MIN_LATENCY_MS = 50;
const DEFAULT_MAX_LATENCY_MS = 150;

export class UnsupportedModelVersionError extends Error {
  constructor(modelVersionId: string) {
    super(`Unsupported model version: ${modelVersionId}`);
    this.name = "UnsupportedModelVersionError";
  }
}

// Tenant-scoped: each tenant has its own independent model catalog, so the
// model version ID alone isn't a unique cache key -- two tenants can each
// have their own "model-v1" with different simulated behavior.
function redisKey(tenantId: string, modelVersionId: string): string {
  return `${REDIS_KEY_PREFIX}${tenantId}:${modelVersionId}`;
}

function buildDefaultProfile(
  modelVersionId: string,
): ModelSimulationProfile {
  return {
    modelVersionId,
    failureRate: DEFAULT_FAILURE_RATE,
    minLatencyMs: DEFAULT_MIN_LATENCY_MS,
    maxLatencyMs: DEFAULT_MAX_LATENCY_MS,
    updatedAt: new Date().toISOString(),
  };
}

export async function getModelConfig(
  tenantId: string,
  modelVersionId: string,
): Promise<ModelSimulationProfile> {
  let serializedProfile: string | null;

  try {
    serializedProfile = await redisClient.get(redisKey(tenantId, modelVersionId));
  } catch (cause) {
    console.warn(
      `model-config: redis unavailable, falling back to default profile for ${modelVersionId}`,
      cause,
    );
    return buildDefaultProfile(modelVersionId);
  }

  if (serializedProfile === null) {
    throw new UnsupportedModelVersionError(modelVersionId);
  }

  let parsedValue: unknown;

  try {
    parsedValue = JSON.parse(serializedProfile);
  } catch {
    console.warn(
      `model-config: invalid JSON for ${modelVersionId}, falling back to default profile`,
    );
    return buildDefaultProfile(modelVersionId);
  }

  const parsedProfile = modelSimulationProfileSchema.safeParse(parsedValue);

  if (!parsedProfile.success) {
    console.warn(
      `model-config: invalid profile for ${modelVersionId}, falling back to default profile`,
    );
    return buildDefaultProfile(modelVersionId);
  }

  return parsedProfile.data;
}
