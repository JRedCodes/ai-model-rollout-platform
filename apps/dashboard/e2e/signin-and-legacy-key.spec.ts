import { expect, test } from "@playwright/test";
import { PASSWORD, uniqueEmail } from "./helpers.ts";

// The seeded demo tenant from the migrations -- documented in the README
// as dev-only, on purpose, for exactly this kind of no-signup smoke path.
const DEMO_TENANT_API_KEY = "tk_demo_2218a6e29efe8f4b3378390b46a0710d";

test("sign in with an existing account", async ({ page, request }) => {
  const email = uniqueEmail("signin");
  // Creates the account via the API directly (a disconnected request
  // context, not page.context().request) so its session cookie is
  // discarded -- this test starts from a genuinely signed-out browser and
  // drives the real sign-in form, rather than the signup flow again.
  const signUpRes = await request.post("/api/auth/signup", {
    data: { email, password: PASSWORD },
  });
  expect(signUpRes.ok()).toBe(true);

  await page.goto("/signin");
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByText("Model configuration")).toBeVisible();
});

test("sign in with the wrong password shows an error and stays on the sign-in form", async ({
  page,
  request,
}) => {
  const email = uniqueEmail("signin-wrong-password");
  await request.post("/api/auth/signup", { data: { email, password: PASSWORD } });

  await page.goto("/signin");
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("Password").fill("the-wrong-password");
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByText("Invalid email or password.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
});

test("legacy API key path logs in with the seeded demo tenant key", async ({ page }) => {
  await page.goto("/signin");
  await page.getByRole("button", { name: "Use it directly" }).click();

  await page.getByPlaceholder("tk_...").fill(DEMO_TENANT_API_KEY);
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page.getByText("Model configuration")).toBeVisible();

  // A legacy-key session has no real user account behind it -- /account
  // should silently fall back to the dashboard (App.tsx) rather than
  // showing an AccountPanel with nothing to display.
  await page.getByRole("button", { name: "Account" }).click();
  await expect(page.getByText("Model configuration")).toBeVisible();
  await expect(page.getByText("Signed in as", { exact: false })).toHaveCount(0);
});
