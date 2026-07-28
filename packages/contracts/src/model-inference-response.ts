import { z } from "zod";

export const modelInferenceResponseSchema = z.discriminatedUnion("success", [
  z.object({
    requestId: z.string(),
    modelVersionId: z.string(),
    success: z.literal(true),
    output: z.object({
      classification: z.string(),
    }),
    latencyMs: z.number().nonnegative(),
  }),

  z.object({
    requestId: z.string(),
    modelVersionId: z.string(),
    success: z.literal(false),
    errorType: z.enum([
      "SIMULATED_FAILURE",
      "MODEL_TIMEOUT",
      "INVALID_RESPONSE",
    ]),
    latencyMs: z.number().nonnegative(),
  }),
]);

export type ModelInferenceResponse = z.infer<
  typeof modelInferenceResponseSchema
>;