import { Router } from "express";

import {
  edgeInferenceRequestSchema,
  type EdgeInferenceResponse,
} from "@rollout-platform/contracts";

import { requestModelInference } from "../clients/model-service.client.js";
import {
  FeatureFlagNotFoundError,
  InvalidFeatureFlagError,
  getActiveFeatureFlag,
} from "../repositories/feature-flag.repository.js";
import { selectModel } from "../services/traffic-assignment.service.js";
import { publishInferenceEvent } from "../services/telemetry.service.js";

export const inferenceRouter = Router();

inferenceRouter.post("/infer", async (request, response) => {
  const parsedRequest = edgeInferenceRequestSchema.safeParse(
    request.body,
  );

  if (!parsedRequest.success) {
    response.status(400).json({
      error: "Invalid request body",
      details: parsedRequest.error.issues,
    });

    return;
  }

  try {
    const featureFlag = await getActiveFeatureFlag();

    const selectedModelVersionId = selectModel(
      parsedRequest.data.userId,
      featureFlag,
    );

    const modelResponse = await requestModelInference(
      selectedModelVersionId,
      {
        requestId: parsedRequest.data.requestId,
        input: parsedRequest.data.input,
      },
    );

    publishInferenceEvent(
      parsedRequest.data.requestId,
      parsedRequest.data.userId,
      featureFlag,
      selectedModelVersionId,
      modelResponse,
    );

    const edgeResponse: EdgeInferenceResponse =
      modelResponse.success
        ? {
            requestId: modelResponse.requestId,
            success: true,
            result: {
              classification:
                modelResponse.output.classification,
            },
          }
        : {
            requestId: modelResponse.requestId,
            success: false,
            errorType: modelResponse.errorType,
          };

    response.status(200).json(edgeResponse);
  } catch (error: unknown) {
    console.error("Edge inference failed:", error);

    if (error instanceof FeatureFlagNotFoundError) {
      const edgeResponse: EdgeInferenceResponse = {
        requestId: parsedRequest.data.requestId,
        success: false,
        errorType: "ROUTING_CONFIGURATION_UNAVAILABLE",
      };

      response.status(503).json(edgeResponse);
      return;
    }

    if (error instanceof InvalidFeatureFlagError) {
      const edgeResponse: EdgeInferenceResponse = {
        requestId: parsedRequest.data.requestId,
        success: false,
        errorType: "INVALID_ROUTING_CONFIGURATION",
      };

      response.status(500).json(edgeResponse);
      return;
    }

    const edgeResponse: EdgeInferenceResponse = {
      requestId: parsedRequest.data.requestId,
      success: false,
      errorType: "MODEL_SERVICE_UNAVAILABLE",
    };

    response.status(502).json(edgeResponse);
  }
});