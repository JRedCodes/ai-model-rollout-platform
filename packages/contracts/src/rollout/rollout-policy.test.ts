import { describe, expect, it } from "vitest";
import { rolloutPolicySchema } from "./rollout-policy.js";

const validPolicy = {
  policyId: "policy-1",
  policyVersion: 1,
  name: "default",
  guard: {
    recentWindow: { windowSize: 50, maximumErrorRate: 0.3 },
    absoluteWindow: { minimumRequests: 50, maximumErrorRate: 0.05 },
  },
  advancement: {
    minimumRequests: 100,
    maximumErrorRate: 0.02,
    maximumP95LatencyMs: 250,
    minimumStableDurationSeconds: 120,
  },
  cooldownSeconds: 0,
};

describe("rolloutPolicySchema", () => {
  it("accepts a valid policy", () => {
    expect(rolloutPolicySchema.safeParse(validPolicy).success).toBe(true);
  });

  it("rejects a maximumErrorRate above 1", () => {
    const result = rolloutPolicySchema.safeParse({
      ...validPolicy,
      guard: {
        ...validPolicy.guard,
        recentWindow: { ...validPolicy.guard.recentWindow, maximumErrorRate: 1.5 },
      },
    });
    expect(result.success).toBe(false);
  });

  it("rejects a non-positive windowSize", () => {
    const result = rolloutPolicySchema.safeParse({
      ...validPolicy,
      guard: {
        ...validPolicy.guard,
        recentWindow: { ...validPolicy.guard.recentWindow, windowSize: 0 },
      },
    });
    expect(result.success).toBe(false);
  });

  it("accepts a zero cooldownSeconds (nonnegative, not strictly positive)", () => {
    const result = rolloutPolicySchema.safeParse({ ...validPolicy, cooldownSeconds: 0 });
    expect(result.success).toBe(true);
  });

  it("rejects a negative cooldownSeconds", () => {
    const result = rolloutPolicySchema.safeParse({ ...validPolicy, cooldownSeconds: -1 });
    expect(result.success).toBe(false);
  });

  it("rejects a missing advancement block", () => {
    const { advancement: _advancement, ...withoutAdvancement } = validPolicy;
    const result = rolloutPolicySchema.safeParse(withoutAdvancement);
    expect(result.success).toBe(false);
  });
});
