import { createHash } from "node:crypto";

import { redisClient } from "../redis/redis.client.js";
import { RedisUnavailableError } from "./feature-flag.repository.js";

const REDIS_KEY_PREFIX = "tenant-auth:";

export class InvalidAPIKeyError extends Error {
  constructor() {
    super("Invalid API key");
    this.name = "InvalidAPIKeyError";
  }
}

function hashAPIKey(apiKey: string): string {
  return createHash("sha256").update(apiKey).digest("hex");
}

// In-memory cache of api-key-hash -> tenantId for keys that have already
// authenticated successfully at least once. The Rollout Controller is the
// source of truth (Postgres) and Redis is the fast-read cache it publishes
// to, same as the feature flag and model config; this is one layer further
// out, so a tenant that's already been serving traffic keeps working
// through a *transient* Redis outage instead of every request suddenly
// 401ing. A tenant's very first request during an outage still fails --
// there's nothing to fall back to for a key never seen before.
const resolvedTenantCache = new Map<string, string>();

export async function resolveTenantId(apiKey: string): Promise<string> {
  const hash = hashAPIKey(apiKey);

  let tenantId: string | null;
  try {
    tenantId = await redisClient.get(`${REDIS_KEY_PREFIX}${hash}`);
  } catch (cause) {
    const cached = resolvedTenantCache.get(hash);
    if (cached !== undefined) {
      return cached;
    }
    throw new RedisUnavailableError(cause);
  }

  if (tenantId === null) {
    throw new InvalidAPIKeyError();
  }

  resolvedTenantCache.set(hash, tenantId);
  return tenantId;
}
