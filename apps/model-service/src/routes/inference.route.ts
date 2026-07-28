import { Router } from "express";
import { modelInferenceRequestSchema } from "@rollout-platform/contracts";
import {
  simulateInference,
  UnsupportedModelVersionError,
} from "../services/model-simulator.service.js";

export const inferenceRouter = Router();

inferenceRouter.post("/:modelVersionId/infer", async (req, res) => {
  const parsed = modelInferenceRequestSchema.safeParse(req.body);

  if (!parsed.success) {
    return res.status(400).json({
      error: "INVALID_INFERENCE_REQUEST",
      details: parsed.error.issues,
    });
  }

  const { modelVersionId } = req.params;

  if (!modelVersionId) {
    return res.status(400).json({
      error: "MISSING_MODEL_VERSION_ID",
    });
  }

  try {
    const inferenceResult = await simulateInference(
      modelVersionId,
      parsed.data,
    );

    return res.status(200).json(inferenceResult);
  } catch (error) {
    if (error instanceof UnsupportedModelVersionError) {
      return res.status(404).json({
        error: "MODEL_VERSION_NOT_FOUND",
        modelVersionId,
      });
    }

    console.error(error);

    return res.status(500).json({
      error: "MODEL_SERVICE_INTERNAL_ERROR",
    });
  }
});