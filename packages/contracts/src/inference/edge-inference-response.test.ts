import { describe, expect, it } from "vitest";
import { edgeInferenceResponseSchema } from "./edge-inference-response.js";

describe("edgeInferenceResponseSchema", () => {
  it("accepts a successful response", () => {
    const result = edgeInferenceResponseSchema.safeParse({
      requestId: "req-1",
      success: true,
      result: { classification: "ACCOUNT_ACCESS" },
    });
    expect(result.success).toBe(true);
  });

  it("accepts a failed response", () => {
    const result = edgeInferenceResponseSchema.safeParse({
      requestId: "req-1",
      success: false,
      errorType: "SIMULATED_FAILURE",
    });
    expect(result.success).toBe(true);
  });

  it("rejects a successful response missing result", () => {
    const result = edgeInferenceResponseSchema.safeParse({
      requestId: "req-1",
      success: true,
    });
    expect(result.success).toBe(false);
  });

  it("rejects a failed response carrying a result instead of errorType", () => {
    const result = edgeInferenceResponseSchema.safeParse({
      requestId: "req-1",
      success: false,
      result: { classification: "ACCOUNT_ACCESS" },
    });
    expect(result.success).toBe(false);
  });

  it("rejects an unrecognized success value", () => {
    const result = edgeInferenceResponseSchema.safeParse({
      requestId: "req-1",
      success: "yes",
    });
    expect(result.success).toBe(false);
  });
});
