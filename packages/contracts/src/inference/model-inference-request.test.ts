import { describe, expect, it } from "vitest";
import { modelInferenceRequestSchema } from "./model-inference-request.js";

describe("modelInferenceRequestSchema", () => {
  it("accepts a valid request", () => {
    const result = modelInferenceRequestSchema.safeParse({
      requestId: "req-1",
      tenantId: "tenant-1",
      input: { message: "hello" },
    });
    expect(result.success).toBe(true);
  });

  it("rejects an empty tenantId", () => {
    const result = modelInferenceRequestSchema.safeParse({
      requestId: "req-1",
      tenantId: "",
      input: { message: "hello" },
    });
    expect(result.success).toBe(false);
  });

  it("rejects a whitespace-only message", () => {
    const result = modelInferenceRequestSchema.safeParse({
      requestId: "req-1",
      tenantId: "tenant-1",
      input: { message: "   " },
    });
    expect(result.success).toBe(false);
  });
});
