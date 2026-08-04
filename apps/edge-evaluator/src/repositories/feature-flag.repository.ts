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

export async function getFeatureFlag(
  featureFlagKey: string,
): Promise<FeatureFlag> {
  const serializedFlag = await redisClient.get(featureFlagKey);

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

export function getActiveFeatureFlag(): Promise<FeatureFlag> {
  return getFeatureFlag(env.FEATURE_FLAG_KEY);
}