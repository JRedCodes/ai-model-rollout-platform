import type { FeatureFlag } from "@rollout-platform/contracts";

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
  if (
    flag.candidateModelVersionId === null ||
    flag.candidatePercentage === 0
  ) {
    return flag.stableModelVersionId;
  }

  const bucket = hashString(userId) % 100;

  return bucket < flag.candidatePercentage
    ? flag.candidateModelVersionId
    : flag.stableModelVersionId;
}