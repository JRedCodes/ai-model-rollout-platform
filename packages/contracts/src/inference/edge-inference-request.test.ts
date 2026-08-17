import { describe, expect, it } from "vitest";
import { edgeInferenceRequestSchema } from "./edge-inference-request.js";

describe("edgeInferenceRequestSchema", () => {
  it("accepts a valid request", () => {
    const result = edgeInferenceRequestSchema.safeParse({
      requestId: "req-1",
      userId: "user-1",
      input: { message: "hello" },
    });
    expect(result.success).toBe(true);
  });

  it("rejects an empty requestId", () => {
    const result = edgeInferenceRequestSchema.safeParse({
      requestId: "",
      userId: "user-1",
      input: { message: "hello" },
    });
    expect(result.success).toBe(false);
  });

  it("rejects a whitespace-only message", () => {
    const result = edgeInferenceRequestSchema.safeParse({
      requestId: "req-1",
      userId: "user-1",
      input: { message: "   " },
    });
    expect(result.success).toBe(false);
  });

  it("trims the message", () => {
    const result = edgeInferenceRequestSchema.safeParse({
      requestId: "req-1",
      userId: "user-1",
      input: { message: "  hello  " },
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.input.message).toBe("hello");
    }
  });

  it("rejects a missing input object", () => {
    const result = edgeInferenceRequestSchema.safeParse({
      requestId: "req-1",
      userId: "user-1",
    });
    expect(result.success).toBe(false);
  });
});
