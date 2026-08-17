import { expect, test } from "@playwright/test";
import { PASSWORD, uniqueEmail } from "./helpers.ts";

test.use({ permissions: ["clipboard-read", "clipboard-write"] });

test("sign up, see the API key once, and create a rollout", async ({ page }) => {
  const email = uniqueEmail("signup");

  await page.goto("/signup");

  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("Password (min. 8 characters)").fill(PASSWORD);
  await page.getByRole("button", { name: "Create account" }).click();

  // The key is shown exactly once -- "Continue" stays disabled until it's
  // been copied (SignUp.tsx), so this exercises that gate rather than
  // bypassing it.
  const keyDisplay = page.locator("code").filter({ hasText: /^tk_/ });
  await expect(keyDisplay).toBeVisible();
  const apiKey = await keyDisplay.textContent();
  expect(apiKey).toMatch(/^tk_[0-9a-f]+$/);

  const continueButton = page.getByRole("button", {
    name: "Copy your key to continue",
  });
  await expect(continueButton).toBeDisabled();

  await page.getByRole("button", { name: "Copy", exact: true }).click();

  const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
  expect(clipboardText).toBe(apiKey);

  await page.getByRole("button", { name: "Continue to dashboard" }).click();

  // Now on the dashboard, signed in.
  await expect(page.getByText("No active rollout", { exact: false })).toBeVisible();
  await expect(page.getByText("Model configuration")).toBeVisible();

  // A brand-new tenant has no completed rollout, so the backend requires
  // an explicit stable model -- "Auto" isn't selectable for a first rollout.
  await page.getByLabel("Rollout phase ID").fill("phase-1");
  await page.getByLabel("Candidate model").selectOption("model-v2");
  await page.getByLabel("Stable model").selectOption("model-v1");
  await page.getByRole("button", { name: "Create rollout" }).click();

  await expect(page.getByText(/^Created /)).toBeVisible();

  // GET /rollout reads an in-memory pipeline registry that the
  // supervisor's reconcile loop only populates on its next ~5s tick after
  // a rollout is created in Postgres -- until then the dashboard still
  // (correctly) shows "no active rollout" even though the rollout exists.
  // Found by this test initially asserting too fast against the default
  // 5s timeout.
  await expect(page.getByText("RUNNING")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("status-stable-model")).toHaveText("model-v1");
  await expect(page.getByTestId("status-candidate-model")).toHaveText("model-v2");
});
