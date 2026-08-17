import { createHash, randomUUID } from "node:crypto";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { connectRedis, redisClient } from "../../src/redis/redis.client.js";
import { RedisUnavailableError } from "../../src/repositories/feature-flag.repository.js";
import {
  InvalidAPIKeyError,
  resolveTenantId,
} from "../../src/repositories/tenant-auth.repository.js";

// No integration test existed for this repository before -- the auth
// middleware (auth.middleware.ts) has no cookie/session logic at all, it
// only ever reads Authorization: Bearer and resolves through here, so
// feat/auth's session-cookie work on the dashboard/Go side never touches
// this path. Confirmed by reading auth.middleware.ts before writing these.

// Mirrors tenant-auth.repository.ts's own hashAPIKey (SHA-256) -- the same
// hashing the Go rollout-controller does when publishing
// tenant-auth:<hash> -> tenantId.
function hashAPIKey(apiKey: string): string {
  return createHash("sha256").update(apiKey).digest("hex");
}

const testApiKey = `tk_integration-test-${randomUUID()}`;
const testTenantId = `tenant-integration-test-${randomUUID()}`;
const testRedisKey = `tenant-auth:${hashAPIKey(testApiKey)}`;

describe("tenant-auth repository integration", () => {
  beforeAll(async () => {
    await connectRedis();
    await redisClient.set(testRedisKey, testTenantId);
  });

  afterAll(async () => {
    if (redisClient.isOpen) {
      await redisClient.del(testRedisKey);
      await redisClient.quit();
    }
  });

  it("resolves a tenant ID for a seeded key", async () => {
    const tenantId = await resolveTenantId(testApiKey);
    expect(tenantId).toBe(testTenantId);
  });

  it("throws InvalidAPIKeyError for a key with no matching redis entry", async () => {
    await expect(
      resolveTenantId(`tk_${randomUUID()}`),
    ).rejects.toBeInstanceOf(InvalidAPIKeyError);
  });

  it("falls back to the in-memory cache during a redis outage, for a key already resolved once", async () => {
    // Already resolved successfully in the first test, which populated the
    // cache -- resolve once more here to be explicit/order-independent.
    await resolveTenantId(testApiKey);

    await redisClient.disconnect();
    try {
      const tenantId = await resolveTenantId(testApiKey);
      expect(tenantId).toBe(testTenantId);
    } finally {
      await connectRedis();
    }
  });

  it("still throws RedisUnavailableError during an outage for a key never resolved before", async () => {
    const neverSeenKey = `tk_${randomUUID()}`;

    await redisClient.disconnect();
    try {
      await expect(resolveTenantId(neverSeenKey)).rejects.toBeInstanceOf(
        RedisUnavailableError,
      );
    } finally {
      await connectRedis();
    }
  });
});
