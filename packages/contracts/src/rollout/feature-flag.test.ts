import { describe, expect, it } from "vitest";
import { featureFlagSchema } from "./feature-flag.js";

const validFlag = {
  flagKey: "model-routing",
  rolloutId: "rollout-1",
  rolloutPhaseId: "phase-1",
  stableModelVersionId: "model-v1",
  candidateModelVersionId: "model-v2",
  candidatePercentage: 10,
  configurationVersion: 1,
};

describe("featureFlagSchema", () => {
  it("accepts a valid, active flag", () => {
    expect(featureFlagSchema.safeParse(validFlag).success).toBe(true);
  });

  it("accepts null rolloutId/rolloutPhaseId/candidateModelVersionId (no active rollout)", () => {
    const result = featureFlagSchema.safeParse({
      ...validFlag,
      rolloutId: null,
      rolloutPhaseId: null,
      candidateModelVersionId: null,
      candidatePercentage: 0,
    });
    expect(result.success).toBe(true);
  });

  it("rejects candidatePercentage above 100", () => {
    const result = featureFlagSchema.safeParse({
      ...validFlag,
      candidatePercentage: 101,
    });
    expect(result.success).toBe(false);
  });

  it("rejects candidatePercentage below 0", () => {
    const result = featureFlagSchema.safeParse({
      ...validFlag,
      candidatePercentage: -1,
    });
    expect(result.success).toBe(false);
  });

  it("rejects a non-positive configurationVersion", () => {
    const result = featureFlagSchema.safeParse({
      ...validFlag,
      configurationVersion: 0,
    });
    expect(result.success).toBe(false);
  });
});
