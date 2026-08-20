import { z } from "zod";

export const rolloutStatusSchema = z.enum([
  "PENDING",
  "RUNNING",
  "HELD",
  "ROLLING_BACK",
  "ROLLED_BACK",
  "COMPLETED",
]);

export type RolloutStatus = z.infer<typeof rolloutStatusSchema>;
