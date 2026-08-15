import pg from "pg";

const { Pool } = pg;

const DATABASE_URL =
  process.env.DATABASE_URL ?? "postgres://localhost:5432/rollout_platform";
const ROLLOUT_ID = process.env.ROLLOUT_ID ?? "rollout-001";

export async function reset(mode: "steady" | "burst"): Promise<void> {
  // burst starts at 100% so every request hits the candidate, guaranteeing
  // that the guard's fresh window sees the model-v2 failure rate directly.
  const candidatePercentage = mode === "burst" ? 100 : 10;

  const pool = new Pool({ connectionString: DATABASE_URL });

  try {
    const updated = await pool.query(
      `UPDATE rollouts
       SET status = 'RUNNING',
           candidate_percentage = $1,
           configuration_version = 1
       WHERE id = $2
       RETURNING id`,
      [candidatePercentage, ROLLOUT_ID],
    );

    if (updated.rowCount === 0) {
      throw new Error(
        `No rollout found with id "${ROLLOUT_ID}". Set ROLLOUT_ID env var if using a different id.`,
      );
    }

    await pool.query(
      "DELETE FROM rollout_decisions WHERE rollout_id = $1",
      [ROLLOUT_ID],
    );

    await pool.query(
      "DELETE FROM inference_events WHERE rollout_id = $1",
      [ROLLOUT_ID],
    );
  } finally {
    await pool.end();
  }
}
