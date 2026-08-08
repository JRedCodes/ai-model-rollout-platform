import {
  afterAll,
  beforeAll,
  describe,
  expect,
  it,
} from "vitest";

import {
  connectRedis,
  redisClient,
} from "../../src/redis/redis.client.js";

import {
  getFeatureFlag,
} from "../../src/repositories/feature-flag.repository.js";

const testFeatureFlagKey =
  "feature-flag:model-routing:test";

const testFeatureFlag = {
  flagKey: "model-routing",
  rolloutId: "rollout-101",
  rolloutPhaseId: "phase-1",
  stableModelVersionId: "model-v1",
  candidateModelVersionId: "model-v2",
  candidatePercentage: 20,
  configurationVersion: 1,
};

describe("feature flag repository integration", () => {
  beforeAll(async () => {
    await connectRedis();

    await redisClient.set(
      testFeatureFlagKey,
      JSON.stringify(testFeatureFlag),
    );
  });

  afterAll(async () => {
    if (redisClient.isOpen) {
      await redisClient.del(testFeatureFlagKey);
      await redisClient.quit();
    }
  });

  it("reads and validates a stored feature flag", async () => {
    const flag = await getFeatureFlag(testFeatureFlagKey);

    expect(flag).toEqual(testFeatureFlag);
  });
});