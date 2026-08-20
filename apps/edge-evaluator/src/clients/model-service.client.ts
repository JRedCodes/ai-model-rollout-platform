// clients/model-service.client.ts
import {
  modelInferenceResponseSchema,
  type ModelInferenceRequest,
  type ModelInferenceResponse,
} from "@rollout-platform/contracts";

import { env } from "../config/env.js";

export async function requestModelInference(
  modelVersionId: string,
  request: ModelInferenceRequest,
): Promise<ModelInferenceResponse> {
  const encodedModelVersionId = encodeURIComponent(modelVersionId);

  const response = await fetch(
    `${env.MODEL_SERVICE_URL}/v1/models/${encodedModelVersionId}/infer`,
    {
      method: "POST",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(request),
      signal: AbortSignal.timeout(2_000),
    },
  );

  if (!response.ok) {
    throw new Error(`Model service returned HTTP ${response.status}`);
  }

  const body: unknown = await response.json();

  return modelInferenceResponseSchema.parse(body);
}
