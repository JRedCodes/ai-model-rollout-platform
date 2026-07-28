import { z } from "zod";

export const modelInferenceRequestSchema = z.object({
    requestId: z.string(),
    input: z.object({
        message: z.string().trim().min(1, "Message is required")
    })
});

export type ModelInferenceRequest = z.infer<
  typeof modelInferenceRequestSchema
>;