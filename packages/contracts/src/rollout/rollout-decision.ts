import { z } from "zod";

export const rolloutDecisionActionSchema = z.enum([
  "ADVANCE",
  "HOLD",
  "ROLLBACK",
  "COMPLETE",
]);

export const rolloutDecisionProposalSchema = z.object({
  decisionId: z.string().min(1),
  rolloutId: z.string().min(1),
  rolloutPhaseId: z.string().min(1),

  expectedStateVersion: z.number().int().nonnegative(),
  action: rolloutDecisionActionSchema,

  reasonCode: z.string().min(1),
  proposedAt: z.string().datetime(),
});

export type RolloutDecisionProposal = z.infer<
  typeof rolloutDecisionProposalSchema
>;