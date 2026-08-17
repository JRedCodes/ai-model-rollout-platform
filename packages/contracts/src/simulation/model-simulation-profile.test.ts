import { describe, expect, it } from "vitest";
import { modelSimulationProfileSchema } from "./model-simulation-profile.js";

const validProfile = {
  modelVersionId: "model-v1",
  failureRate: 0.01,
  minLatencyMs: 50,
  maxLatencyMs: 150,
  updatedAt: new Date().toISOString(),
};

describe("modelSimulationProfileSchema", () => {
  it("accepts a valid profile", () => {
    expect(modelSimulationProfileSchema.safeParse(validProfile).success).toBe(true);
  });

  it("accepts minLatencyMs equal to maxLatencyMs", () => {
    const result = modelSimulationProfileSchema.safeParse({
      ...validProfile,
      minLatencyMs: 100,
      maxLatencyMs: 100,
    });
    expect(result.success).toBe(true);
  });

  it("rejects minLatencyMs greater than maxLatencyMs", () => {
    const result = modelSimulationProfileSchema.safeParse({
      ...validProfile,
      minLatencyMs: 200,
      maxLatencyMs: 100,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.path).toEqual(["minLatencyMs"]);
    }
  });

  it("rejects a failureRate above 1", () => {
    const result = modelSimulationProfileSchema.safeParse({
      ...validProfile,
      failureRate: 1.5,
    });
    expect(result.success).toBe(false);
  });

  it("rejects a non-positive maxLatencyMs", () => {
    const result = modelSimulationProfileSchema.safeParse({
      ...validProfile,
      minLatencyMs: 0,
      maxLatencyMs: 0,
    });
    expect(result.success).toBe(false);
  });
});
