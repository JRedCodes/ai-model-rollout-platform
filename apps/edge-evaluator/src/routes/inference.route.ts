import { Router } from "express";

import {
  edgeInferenceRequestSchema,
  type EdgeInferenceResponse,
} from "@rollout-platform/contracts";

import { requestModelInference } from "../clients/model-service.client.js";
import { featureFlag } from "../config/feature-flag.js";
import { selectModel } from "../services/traffic-assignment.service.js";

export const inferenceRouter = Router();

inferenceRouter.post("/infer", async (request, response) => {
  const parsedRequest = edgeInferenceRequestSchema.safeParse(request.body);

  if (!parsedRequest.success) {
    response.status(400).json({
      error: "Invalid request body",
      details: parsedRequest.error.flatten(),
    });

    return;
  }

  const selectedModelVersionId = selectModel(
    parsedRequest.data.userId,
    featureFlag,
  );

  try {
    const modelResponse = await requestModelInference(
      selectedModelVersionId,
      {
        requestId: parsedRequest.data.requestId,
        input: parsedRequest.data.input,
      },
    );

    const edgeResponse: EdgeInferenceResponse = modelResponse.success
      ? {
          requestId: modelResponse.requestId,
          success: true,
          result: {
            classification: modelResponse.output.classification,
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

    const edgeResponse: EdgeInferenceResponse = {
      requestId: parsedRequest.data.requestId,
      success: false,
      errorType: "MODEL_SERVICE_UNAVAILABLE",
    };

    response.status(502).json(edgeResponse);
  }
});