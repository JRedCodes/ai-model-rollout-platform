import { z } from "zod";

export const inferenceCompletedEventSchema = z.object({
  schemaVersion: z.literal(1),
  eventId: z.string().min(1),

  requestId: z.string().min(1),
  userId: z.string().min(1),

  rolloutId: z.string().min(1).nullable(),
  rolloutPhaseId: z.string().min(1).nullable(),

  modelVersionId: z.string().min(1),
  assignment: z.enum(["stable", "candidate"]),

  success: z.boolean(),
  errorType: z
    .enum([
      "SIMULATED_FAILURE",
      "MODEL_TIMEOUT",
      "INVALID_RESPONSE",
    ])
    .nullable(),

  latencyMs: z.number().int().nonnegative(),
  occurredAt: z.string().datetime(),
})
.superRefine((event, context) => {
    if (event.success && event.errorType !== null) {
      context.addIssue({
        code: "custom",
        path: ["errorType"],
        message: "Successful events cannot contain an error type",
      });
    }

    if (!event.success && event.errorType === null) {
      context.addIssue({
        code: "custom",
        path: ["errorType"],
        message: "Failed events must contain an error type",
      });
    }
  });

export type InferenceCompletedEvent = z.infer<
  typeof inferenceCompletedEventSchema
>;