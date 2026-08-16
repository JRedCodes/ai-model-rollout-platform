import {
  featureFlagSchema,
  type FeatureFlag,
} from "@rollout-platform/contracts";

import { env } from "../config/env.js";
import { redisClient } from "../redis/redis.client.js";

export class FeatureFlagNotFoundError extends Error {
  constructor(featureFlagKey: string) {
    super(`Feature flag not found: ${featureFlagKey}`);
    this.name = "FeatureFlagNotFoundError";
  }
}

export class InvalidFeatureFlagError extends Error {
  constructor(featureFlagKey: string) {
    super(`Feature flag is invalid: ${featureFlagKey}`);
    this.name = "InvalidFeatureFlagError";
  }
}

export class RedisUnavailableError extends Error {
  constructor(cause: unknown) {
    super("Redis is unavailable");
    this.name = "RedisUnavailableError";
    this.cause = cause;
  }
}

export async function getFeatureFlag(
  featureFlagKey: string,
): Promise<FeatureFlag> {
  let serializedFlag: string | null;

  try {
    serializedFlag = await redisClient.get(featureFlagKey);
  } catch (cause) {
    throw new RedisUnavailableError(cause);
  }

  if (serializedFlag === null) {
    throw new FeatureFlagNotFoundError(featureFlagKey);
  }

  let parsedValue: unknown;

  try {
    parsedValue = JSON.parse(serializedFlag);
  } catch {
    throw new InvalidFeatureFlagError(featureFlagKey);
  }

  const parsedFlag = featureFlagSchema.safeParse(parsedValue);

  if (!parsedFlag.success) {
    throw new InvalidFeatureFlagError(featureFlagKey);
  }

  return parsedFlag.data;
}

// Each tenant publishes its feature flag under its own key -- there's no
// single fixed flag to read anymore, so the caller's authenticated tenant
// ID is required.
export function getActiveFeatureFlag(tenantId: string): Promise<FeatureFlag> {
  return getFeatureFlag(`${env.FEATURE_FLAG_KEY_PREFIX}${tenantId}`);
}