import { describe, expect, it } from "vitest";
import { modelInferenceResponseSchema } from "./model-inference-response.js";

describe("modelInferenceResponseSchema", () => {
  it("accepts a successful response", () => {
    const result = modelInferenceResponseSchema.safeParse({
      requestId: "req-1",
      modelVersionId: "model-v1",
      success: true,
      output: { classification: "ACCOUNT_ACCESS" },
      latencyMs: 100,
    });
    expect(result.success).toBe(true);
  });

  it("accepts a failed response with a recognized errorType", () => {
    const result = modelInferenceResponseSchema.safeParse({
      requestId: "req-1",
      modelVersionId: "model-v1",
      success: false,
      errorType: "MODEL_TIMEOUT",
      latencyMs: 2000,
    });
    expect(result.success).toBe(true);
  });

  it("rejects a failed response with an unrecognized errorType", () => {
    const result = modelInferenceResponseSchema.safeParse({
      requestId: "req-1",
      modelVersionId: "model-v1",
      success: false,
      errorType: "SOMETHING_ELSE",
      latencyMs: 100,
    });
    expect(result.success).toBe(false);
  });

  it("rejects a negative latencyMs", () => {
    const result = modelInferenceResponseSchema.safeParse({
      requestId: "req-1",
      modelVersionId: "model-v1",
      success: true,
      output: { classification: "ACCOUNT_ACCESS" },
      latencyMs: -1,
    });
    expect(result.success).toBe(false);
  });

  it("rejects a non-integer latencyMs", () => {
    const result = modelInferenceResponseSchema.safeParse({
      requestId: "req-1",
      modelVersionId: "model-v1",
      success: true,
      output: { classification: "ACCOUNT_ACCESS" },
      latencyMs: 100.5,
    });
    expect(result.success).toBe(false);
  });
});
