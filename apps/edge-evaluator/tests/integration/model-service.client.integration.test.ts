import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { requestModelInference } from "../../src/clients/model-service.client.js";
import { connectRedis, redisClient } from "../../src/redis/redis.client.js";

// model-service reads its per-tenant model config from Redis
// (model-config:<tenantId>:<modelVersionId>); in real dev/prod that's
// seeded by the rollout-controller, which isn't running for this test, so
// this seeds the one key it needs directly -- keeps the test self-contained
// against a bare Redis instead of depending on the Go service too.
const testTenantId = "integration-test-tenant";
const testModelVersionId = "model-v1";
const testModelConfigKey = `model-config:${testTenantId}:${testModelVersionId}`;

const testModelConfig = {
  modelVersionId: testModelVersionId,
  failureRate: 0,
  minLatencyMs: 1,
  maxLatencyMs: 5,
  updatedAt: new Date().toISOString(),
};

describe("model-service client integration", () => {
  beforeAll(async () => {
    await connectRedis();
    await redisClient.set(testModelConfigKey, JSON.stringify(testModelConfig));
  });

  afterAll(async () => {
    if (redisClient.isOpen) {
      await redisClient.del(testModelConfigKey);
      await redisClient.quit();
    }
  });

  it("calls the Model Service and returns a valid response", async () => {
    const response = await requestModelInference(testModelVersionId, {
      requestId: "integration-request-1",
      tenantId: testTenantId,
      input: {
        message: "Hello from the Edge Evaluator",
      },
    });

    expect(response.requestId).toBe("integration-request-1");
    expect(response.success).toBeTypeOf("boolean");
  });
});
