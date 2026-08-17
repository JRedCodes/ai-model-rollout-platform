import { describe, expect, it } from "vitest";
import { rolloutStatusSchema } from "./rollout-state.js";

describe("rolloutStatusSchema", () => {
  it.each(["PENDING", "RUNNING", "HELD", "ROLLING_BACK", "ROLLED_BACK", "COMPLETED"])(
    "accepts %s",
    (status) => {
      expect(rolloutStatusSchema.safeParse(status).success).toBe(true);
    },
  );

  it("rejects an unrecognized status", () => {
    expect(rolloutStatusSchema.safeParse("PAUSED").success).toBe(false);
  });

  it("rejects a lowercase variant", () => {
    expect(rolloutStatusSchema.safeParse("running").success).toBe(false);
  });
});
