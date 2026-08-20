import { z } from "zod";

export const modelInferenceRequestSchema = z.object({
  requestId: z.string().min(1),
  tenantId: z.string().min(1),
  input: z.object({
    message: z.string().trim().min(1, "Message is required"),
  }),
});

export type ModelInferenceRequest = z.infer<typeof modelInferenceRequestSchema>;
