import { Router } from "express";

import {
  edgeInferenceRequestSchema,
  type EdgeInferenceResponse,
} from "@rollout-platform/contracts";

import { requestModelInference } from "../clients/model-service.client.js";
import { env } from "../config/env.js";
import {
  FeatureFlagNotFoundError,
  InvalidFeatureFlagError,
  RedisUnavailableError,
  getActiveFeatureFlag,
} from "../repositories/feature-flag.repository.js";
import { publishInferenceEvent } from "../services/telemetry.service.js";
import { selectModel } from "../services/traffic-assignment.service.js";

export const inferenceRouter = Router();

inferenceRouter.post("/infer", async (request, response) => {
  const parsedRequest = edgeInferenceRequestSchema.safeParse(request.body);

  if (!parsedRequest.success) {
    response.status(400).json({
      error: "Invalid request body",
      details: parsedRequest.error.issues,
    });
    return;
  }

  // Guaranteed set by requireTenantAuth, which runs before this router and
  // rejects the request before it gets here otherwise.
  const tenantId = request.tenantId as string;

  try {
    const featureFlag = await getActiveFeatureFlag(tenantId);

    const selectedModelVersionId = selectModel(parsedRequest.data.userId, featureFlag);

    const modelResponse = await requestModelInference(selectedModelVersionId, {
      requestId: parsedRequest.data.requestId,
      tenantId,
      input: parsedRequest.data.input,
    });

    publishInferenceEvent(
      parsedRequest.data.requestId,
      parsedRequest.data.userId,
      tenantId,
      featureFlag,
      selectedModelVersionId,
      modelResponse,
    );

    const edgeResponse: EdgeInferenceResponse = modelResponse.success
      ? {
          requestId: modelResponse.requestId,
          success: true,
          result: { classification: modelResponse.output.classification },
        }
      : {
          requestId: modelResponse.requestId,
          success: false,
          errorType: modelResponse.errorType,
        };

    response.status(200).json(edgeResponse);
  } catch (error: unknown) {
    if (error instanceof RedisUnavailableError) {
      console.warn("Redis unavailable — routing to stable model fallback");

      try {
        const modelResponse = await requestModelInference(env.STABLE_MODEL_FALLBACK_ID, {
          requestId: parsedRequest.data.requestId,
          tenantId,
          input: parsedRequest.data.input,
        });

        publishInferenceEvent(
          parsedRequest.data.requestId,
          parsedRequest.data.userId,
          tenantId,
          null,
          env.STABLE_MODEL_FALLBACK_ID,
          modelResponse,
        );

        const edgeResponse: EdgeInferenceResponse = modelResponse.success
          ? {
              requestId: modelResponse.requestId,
              success: true,
              result: { classification: modelResponse.output.classification },
            }
          : {
              requestId: modelResponse.requestId,
              success: false,
              errorType: modelResponse.errorType,
            };

        response.status(200).json(edgeResponse);
      } catch {
        response.status(502).json({
          requestId: parsedRequest.data.requestId,
          success: false,
          errorType: "MODEL_SERVICE_UNAVAILABLE",
        } satisfies EdgeInferenceResponse);
      }

      return;
    }

    console.error("Edge inference failed:", error);

    if (error instanceof FeatureFlagNotFoundError) {
      response.status(503).json({
        requestId: parsedRequest.data.requestId,
        success: false,
        errorType: "ROUTING_CONFIGURATION_UNAVAILABLE",
      } satisfies EdgeInferenceResponse);
      return;
    }

    if (error instanceof InvalidFeatureFlagError) {
      response.status(500).json({
        requestId: parsedRequest.data.requestId,
        success: false,
        errorType: "INVALID_ROUTING_CONFIGURATION",
      } satisfies EdgeInferenceResponse);
      return;
    }

    response.status(502).json({
      requestId: parsedRequest.data.requestId,
      success: false,
      errorType: "MODEL_SERVICE_UNAVAILABLE",
    } satisfies EdgeInferenceResponse);
  }
});
