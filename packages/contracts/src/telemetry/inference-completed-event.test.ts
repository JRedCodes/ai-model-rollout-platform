import { describe, expect, it } from "vitest";
import { inferenceCompletedEventSchema } from "./inference-completed-event.js";

const validSuccessEvent = {
  schemaVersion: 1,
  eventId: "event-1",
  requestId: "req-1",
  userId: "user-1",
  tenantId: "tenant-1",
  rolloutId: "rollout-1",
  rolloutPhaseId: "phase-1",
  modelVersionId: "model-v1",
  assignment: "stable",
  success: true,
  errorType: null,
  latencyMs: 100,
  occurredAt: new Date().toISOString(),
};

describe("inferenceCompletedEventSchema", () => {
  it("accepts a valid successful event with errorType null", () => {
    expect(inferenceCompletedEventSchema.safeParse(validSuccessEvent).success).toBe(true);
  });

  it("accepts a valid failed event with a non-null errorType", () => {
    const result = inferenceCompletedEventSchema.safeParse({
      ...validSuccessEvent,
      success: false,
      errorType: "SIMULATED_FAILURE",
    });
    expect(result.success).toBe(true);
  });

  it("rejects a successful event that carries an errorType", () => {
    const result = inferenceCompletedEventSchema.safeParse({
      ...validSuccessEvent,
      success: true,
      errorType: "SIMULATED_FAILURE",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.path).toEqual(["errorType"]);
    }
  });

  it("rejects a failed event with a null errorType", () => {
    const result = inferenceCompletedEventSchema.safeParse({
      ...validSuccessEvent,
      success: false,
      errorType: null,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.path).toEqual(["errorType"]);
    }
  });

  it("accepts null rolloutId/rolloutPhaseId (no active rollout at the time)", () => {
    const result = inferenceCompletedEventSchema.safeParse({
      ...validSuccessEvent,
      rolloutId: null,
      rolloutPhaseId: null,
    });
    expect(result.success).toBe(true);
  });

  it("rejects a schemaVersion other than 1", () => {
    const result = inferenceCompletedEventSchema.safeParse({
      ...validSuccessEvent,
      schemaVersion: 2,
    });
    expect(result.success).toBe(false);
  });

  it("rejects an unrecognized assignment value", () => {
    const result = inferenceCompletedEventSchema.safeParse({
      ...validSuccessEvent,
      assignment: "control",
    });
    expect(result.success).toBe(false);
  });
});
