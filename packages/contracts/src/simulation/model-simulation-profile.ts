import { z } from "zod";

export const modelSimulationProfileSchema = z
  .object({
    modelVersionId: z.string().min(1),
    failureRate: z.number().min(0).max(1),
    minLatencyMs: z.number().int().positive(),
    maxLatencyMs: z.number().int().positive(),
    updatedAt: z.string().datetime(),
  })
  .refine((profile) => profile.minLatencyMs <= profile.maxLatencyMs, {
    message: "minLatencyMs must be <= maxLatencyMs",
    path: ["minLatencyMs"],
  });

export type ModelSimulationProfile = z.infer<
  typeof modelSimulationProfileSchema
>;
