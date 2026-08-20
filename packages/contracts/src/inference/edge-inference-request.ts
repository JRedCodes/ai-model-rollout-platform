import { z } from "zod";

export const edgeInferenceRequestSchema = z.object({
  requestId: z.string().min(1),
  userId: z.string().min(1),
  input: z.object({
    message: z.string().trim().min(1, "Message is required"),
  }),
});

export type EdgeInferenceRequest = z.infer<typeof edgeInferenceRequestSchema>;
