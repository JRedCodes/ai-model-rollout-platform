import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../redis/redis.client.js", () => ({
  redisClient: { get: vi.fn() },
}));

vi.mock("../config/env.js", () => ({
  env: { FEATURE_FLAG_KEY: "feature-flag:model-routing:test" },
}));

import { redisClient } from "../redis/redis.client.js";
import {
  FeatureFlagNotFoundError,
  InvalidFeatureFlagError,
  RedisUnavailableError,
  getFeatureFlag,
} from "./feature-flag.repository.js";

const mockGet = vi.mocked(redisClient.get);

const validFlag = JSON.stringify({
  flagKey: "model-routing",
  rolloutId: "rollout-001",
  rolloutPhaseId: "phase-1",
  stableModelVersionId: "model-v1",
  candidateModelVersionId: "model-v2",
  candidatePercentage: 10,
  configurationVersion: 1,
});

describe("feature flag repository", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("returns a parsed flag on success", async () => {
    mockGet.mockResolvedValue(validFlag);

    const flag = await getFeatureFlag("any-key");

    expect(flag.stableModelVersionId).toBe("model-v1");
  });

  it("throws FeatureFlagNotFoundError when key is missing", async () => {
    mockGet.mockResolvedValue(null);

    await expect(getFeatureFlag("any-key")).rejects.toBeInstanceOf(FeatureFlagNotFoundError);
  });

  it("throws InvalidFeatureFlagError when value is not valid JSON", async () => {
    mockGet.mockResolvedValue("not-json{{{");

    await expect(getFeatureFlag("any-key")).rejects.toBeInstanceOf(InvalidFeatureFlagError);
  });

  it("throws InvalidFeatureFlagError when JSON does not match schema", async () => {
    mockGet.mockResolvedValue(JSON.stringify({ unexpected: true }));

    await expect(getFeatureFlag("any-key")).rejects.toBeInstanceOf(InvalidFeatureFlagError);
  });

  it("throws RedisUnavailableError when the redis client throws", async () => {
    mockGet.mockRejectedValue(new Error("connect ECONNREFUSED"));

    await expect(getFeatureFlag("any-key")).rejects.toBeInstanceOf(RedisUnavailableError);
  });
});
