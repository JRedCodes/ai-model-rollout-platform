import { randomUUID } from "crypto";

import type {
  FeatureFlag,
  ModelInferenceResponse,
} from "@rollout-platform/contracts";

import { env } from "../config/env.js";
import { redisClient } from "../redis/redis.client.js";

export function publishInferenceEvent(
  requestId: string,
  userId: string,
  tenantId: string,
  featureFlag: FeatureFlag | null,
  selectedModelVersionId: string,
  modelResponse: ModelInferenceResponse,
): void {
  const event = {
    schemaVersion: 1,
    eventId: randomUUID(),
    requestId,
    userId,
    tenantId,
    rolloutId: featureFlag?.rolloutId ?? null,
    rolloutPhaseId: featureFlag?.rolloutPhaseId ?? null,
    modelVersionId: modelResponse.modelVersionId,
    assignment:
      featureFlag === null ||
      selectedModelVersionId === featureFlag.stableModelVersionId
        ? "stable"
        : "candidate",
    success: modelResponse.success,
    errorType: modelResponse.success ? null : modelResponse.errorType,
    latencyMs: modelResponse.latencyMs,
    occurredAt: new Date().toISOString(),
  };

  redisClient
    .xAdd(env.TELEMETRY_STREAM_KEY, "*", { event: JSON.stringify(event) })
    .catch((error) => {
      console.error("Failed to publish inference event to stream:", error);
    });
}
