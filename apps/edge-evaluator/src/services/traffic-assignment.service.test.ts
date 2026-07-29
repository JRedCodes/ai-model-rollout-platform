import { describe, expect, it } from "vitest";
import { selectModel } from "./traffic-assignment.service.js";

const baseConfig = {
  stableModelVersionId: "model-v1",
  candidateModelVersionId: "model-v2",
  candidatePercentage: 0,
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
  const first = selectModel("user-42", baseConfig);
  const second = selectModel("user-42", baseConfig);

  expect(second).toBe(first);
});
});