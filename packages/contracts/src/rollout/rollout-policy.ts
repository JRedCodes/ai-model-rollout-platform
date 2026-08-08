import { z } from "zod";

export const rolloutPolicySchema = z.object({
  policyId: z.string().min(1),
  policyVersion: z.number().int().positive(),
  name: z.string().min(1),

  guard: z.object({
    recentWindow: z.object({
      windowSize: z.number().int().positive(),
      maximumErrorRate: z.number().min(0).max(1),
    }),

    absoluteWindow: z.object({
      minimumRequests: z.number().int().positive(),
      maximumErrorRate: z.number().min(0).max(1),
    }),
  }),

  advancement: z.object({
    minimumRequests: z.number().int().positive(),
    maximumErrorRate: z.number().min(0).max(1),
    maximumP95LatencyMs: z.number().int().positive(),
    minimumStableDurationSeconds: z.number().int().positive(),
  }),

  cooldownSeconds: z.number().int().nonnegative(),
});

export type RolloutPolicy = z.infer<
  typeof rolloutPolicySchema
>;