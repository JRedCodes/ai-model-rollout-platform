import type { FeatureFlag } from "../config/feature-flag.js";

function hashString(value: string): number {
  let hash = 0;

  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }

  return hash;
}

export function selectModel(
  userId: string,
  flag: FeatureFlag,
): string {
  const bucket = hashString(userId) % 100;

  return bucket < flag.candidatePercentage
    ? flag.candidateModelVersionId
    : flag.stableModelVersionId;
}