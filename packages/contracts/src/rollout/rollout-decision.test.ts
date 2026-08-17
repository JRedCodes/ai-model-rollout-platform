import { describe, expect, it } from "vitest";
import { rolloutDecisionProposalSchema } from "./rollout-decision.js";

const validProposal = {
  decisionId: "decision-1",
  rolloutId: "rollout-1",
  rolloutPhaseId: "phase-1",
  expectedStateVersion: 1,
  action: "ADVANCE",
  reasonCode: "HEALTHY_WINDOW",
  proposedAt: new Date().toISOString(),
};

describe("rolloutDecisionProposalSchema", () => {
  it("accepts a valid proposal", () => {
    expect(rolloutDecisionProposalSchema.safeParse(validProposal).success).toBe(true);
  });

  it.each(["ADVANCE", "HOLD", "ROLLBACK", "COMPLETE", "RESUME"])(
    "accepts action %s",
    (action) => {
      const result = rolloutDecisionProposalSchema.safeParse({
        ...validProposal,
        action,
      });
      expect(result.success).toBe(true);
    },
  );

  it("rejects an unrecognized action", () => {
    const result = rolloutDecisionProposalSchema.safeParse({
      ...validProposal,
      action: "PAUSE",
    });
    expect(result.success).toBe(false);
  });

  it("rejects a negative expectedStateVersion", () => {
    const result = rolloutDecisionProposalSchema.safeParse({
      ...validProposal,
      expectedStateVersion: -1,
    });
    expect(result.success).toBe(false);
  });

  it("rejects a non-ISO proposedAt", () => {
    const result = rolloutDecisionProposalSchema.safeParse({
      ...validProposal,
      proposedAt: "not-a-date",
    });
    expect(result.success).toBe(false);
  });
});
