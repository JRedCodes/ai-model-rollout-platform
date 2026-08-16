import { describe, expect, it } from "vitest";

import { requestModelInference } from "../../src/clients/model-service.client.js";

describe("model-service client integration", () => {
  it("calls the Model Service and returns a valid response", async () => {
    const response = await requestModelInference("model-v1", {
      requestId: "integration-request-1",
      tenantId: "tenant-demo",
      input: {
        message: "Hello from the Edge Evaluator"
      }
    });

    expect(response.requestId).toBe("integration-request-1");
    expect(response.success).toBeTypeOf("boolean");
  });
});