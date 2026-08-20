import { z } from "zod";

export const featureFlagSchema = z.object({
  flagKey: z.string().min(1),

  rolloutId: z.string().min(1).nullable(),
  rolloutPhaseId: z.string().min(1).nullable(),

  stableModelVersionId: z.string().min(1),
  candidateModelVersionId: z.string().min(1).nullable(),
  candidatePercentage: z.number().min(0).max(100),

  configurationVersion: z.number().int().positive(),
});

export type FeatureFlag = z.infer<typeof featureFlagSchema>;
