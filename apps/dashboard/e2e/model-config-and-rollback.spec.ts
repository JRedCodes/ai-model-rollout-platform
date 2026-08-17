import { expect, test } from "@playwright/test";
import { apiSignUp, uniqueEmail } from "./helpers.ts";

test("editing a model's config persists", async ({ page, context }) => {
  await apiSignUp(context, uniqueEmail("modelconfig"));
  await page.goto("/");

  const failureRateInput = page.getByTestId("model-config-failure-rate-model-v1");
  const minLatencyInput = page.getByTestId("model-config-min-latency-model-v1");
  const maxLatencyInput = page.getByTestId("model-config-max-latency-model-v1");
  const saveButton = page.getByTestId("model-config-save-model-v1");

  await expect(failureRateInput).toBeVisible();
  await expect(saveButton).toBeDisabled(); // nothing edited yet

  await failureRateInput.fill("5");
  await minLatencyInput.fill("60");
  await maxLatencyInput.fill("200");
  await expect(saveButton).toBeEnabled();
  await saveButton.click();

  await expect(saveButton).toBeDisabled(); // saved -> no longer dirty

  // Reload to confirm the new values actually persisted server-side,
  // rather than only reflecting optimistic local state.
  await page.reload();
  await expect(page.getByTestId("model-config-failure-rate-model-v1")).toHaveValue("5.00");
  await expect(page.getByTestId("model-config-min-latency-model-v1")).toHaveValue("60");
  await expect(page.getByTestId("model-config-max-latency-model-v1")).toHaveValue("200");
});

test("force rollback clears an active rollout", async ({ page, context }) => {
  const { apiKey } = await apiSignUp(context, uniqueEmail("rollback"));

  // Creates the rollout via the API -- the create-rollout UI flow is
  // already covered by the signup-flow spec, so this test's setup stays
  // focused on getting to "a rollout is active" as fast as possible.
  const createRes = await context.request.post("/api/rollouts", {
    headers: { Authorization: `Bearer ${apiKey}` },
    data: {
      rolloutPhaseId: "phase-1",
      candidateModelVersionId: "model-v2",
      stableModelVersionId: "model-v1",
      candidatePercentage: 10,
    },
  });
  expect(createRes.ok()).toBe(true);

  await page.goto("/");
  // Same supervisor-reconcile latency noted in signup-flow.spec.ts: this
  // rollout was just created via the API, and GET /rollout won't reflect
  // it as active until the supervisor's next ~5s tick picks it up.
  await expect(page.getByTestId("status-stable-model")).toHaveText("model-v1", {
    timeout: 15_000,
  });

  page.once("dialog", (dialog) => void dialog.accept());
  await page.getByRole("button", { name: "Force rollback" }).click();

  // The rollback command is handled asynchronously by the writer goroutine,
  // and the pipeline only actually tears down on the supervisor's next
  // ~5s reconcile tick -- give this more room than Playwright's default
  // assertion timeout.
  await expect(page.getByText("No active rollout", { exact: false })).toBeVisible({
    timeout: 15_000,
  });
});
