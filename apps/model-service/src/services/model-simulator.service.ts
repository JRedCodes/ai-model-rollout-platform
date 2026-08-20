import type { ModelInferenceRequest, ModelInferenceResponse } from "@rollout-platform/contracts";

import {
  getModelConfig,
  UnsupportedModelVersionError,
} from "../repositories/model-config.repository.js";

export { UnsupportedModelVersionError };

function randomInteger(min: number, max: number): number {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, milliseconds);
  });
}

export async function simulateInference(
  modelVersionId: string,
  request: ModelInferenceRequest,
): Promise<ModelInferenceResponse> {
  const configuration = await getModelConfig(request.tenantId, modelVersionId);

  const latencyMs = randomInteger(configuration.minLatencyMs, configuration.maxLatencyMs);

  await wait(latencyMs);

  const shouldFail = Math.random() < configuration.failureRate;

  if (shouldFail) {
    return {
      requestId: request.requestId,
      modelVersionId,
      success: false,
      errorType: "SIMULATED_FAILURE",
      latencyMs,
    };
  }

  return {
    requestId: request.requestId,
    modelVersionId,
    success: true,
    output: {
      classification: "ACCOUNT_ACCESS",
    },
    latencyMs,
  };
}
