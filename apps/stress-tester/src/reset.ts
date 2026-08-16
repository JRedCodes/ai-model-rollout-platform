import pg from "pg";

const { Pool } = pg;

const DATABASE_URL =
  process.env.DATABASE_URL ?? "postgres://localhost:5432/rollout_platform";
const ROLLOUT_CONTROLLER_URL =
  process.env.ROLLOUT_CONTROLLER_URL ?? "http://localhost:4003";

// Rollouts are now created via POST /rollouts with a server-generated ID
// (no more fixed "rollout-001") -- so the rollout to reset has to be
// resolved through the tenant's own API key, not assumed.
async function resolveActiveRolloutId(apiKey: string): Promise<string> {
  const res = await fetch(`${ROLLOUT_CONTROLLER_URL}/rollout`, {
    headers: { Authorization: `Bearer ${apiKey}` },
  });

  if (!res.ok) {
    throw new Error(`Failed to resolve active rollout: GET /rollout ${res.status}`);
  }

  const body = (await res.json()) as { active: boolean; rolloutId?: string };

  if (!body.active || !body.rolloutId) {
    throw new Error(
      "No active rollout for this tenant. Create one first: POST /rollouts (see README).",
    );
  }

  return body.rolloutId;
}

export async function reset(mode: "steady" | "burst", apiKey: string): Promise<void> {
  // burst starts at 100% so every request hits the candidate, guaranteeing
  // that the guard's fresh window sees the model-v2 failure rate directly.
  const candidatePercentage = mode === "burst" ? 100 : 10;

  const rolloutId = await resolveActiveRolloutId(apiKey);

  const pool = new Pool({ connectionString: DATABASE_URL });

  try {
    const updated = await pool.query(
      `UPDATE rollouts
       SET status = 'RUNNING',
           candidate_percentage = $1,
           configuration_version = 1
       WHERE id = $2
       RETURNING id`,
      [candidatePercentage, rolloutId],
    );

    if (updated.rowCount === 0) {
      throw new Error(`No rollout found with id "${rolloutId}".`);
    }

    await pool.query(
      "DELETE FROM rollout_decisions WHERE rollout_id = $1",
      [rolloutId],
    );

    await pool.query(
      "DELETE FROM inference_events WHERE rollout_id = $1",
      [rolloutId],
    );
  } finally {
    await pool.end();
  }
}
