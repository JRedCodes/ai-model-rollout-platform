import { describe, expect, it } from "vitest";

import type { FeatureFlag } from "@rollout-platform/contracts";
import { selectModel } from "./traffic-assignment.service.js";

const baseConfig: FeatureFlag = {
  flagKey: "model-routing",
  rolloutId: "rollout-101",
  rolloutPhaseId: "phase-1",
  stableModelVersionId: "model-v1",
  candidateModelVersionId: "model-v2",
  candidatePercentage: 0,
  configurationVersion: 1,
};

describe("selectModel", () => {
  it("selects the stable model at 0% candidate traffic", () => {
    const result = selectModel("user-1", {
      ...baseConfig,
      candidatePercentage: 0,
    });

    expect(result).toBe("model-v1");
  });

  it("selects the candidate model at 100% candidate traffic", () => {
    const result = selectModel("user-1", {
      ...baseConfig,
      candidatePercentage: 100,
    });

    expect(result).toBe("model-v2");
  });

  it("assigns the same user consistently", () => {
    const config = {
      ...baseConfig,
      candidatePercentage: 20,
    };

    const first = selectModel("user-42", config);
    const second = selectModel("user-42", config);

    expect(second).toBe(first);
  });

  it("selects the stable model when no candidate is active", () => {
    const result = selectModel("user-1", {
      ...baseConfig,
      rolloutId: null,
      rolloutPhaseId: null,
      candidateModelVersionId: null,
      candidatePercentage: 0,
      configurationVersion: 2,
    });

    expect(result).toBe("model-v1");
  });
});