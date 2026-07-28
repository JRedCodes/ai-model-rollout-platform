import { z } from "zod";

export const edgeInferenceResponseSchema = z.discriminatedUnion("success", [
  z.object({
    requestId: z.string(),
    success: z.literal(true),
    result: z.object({
      classification: z.string(),
    }),
  }),

  z.object({
    requestId: z.string(),
    success: z.literal(false),
    errorType: z.string(),
  }),
]);

export type EdgeInferenceResponse = z.infer<
  typeof edgeInferenceResponseSchema
>;