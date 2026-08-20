import type { BrowserContext } from "@playwright/test";

// Real, disposable password used across every e2e fixture account -- these
// are throwaway accounts created fresh per test run, not real users.
export const PASSWORD = "correct-horse-battery-staple";

export function uniqueEmail(label: string): string {
  return `e2e-${label}-${Date.now()}-${Math.random().toString(36).slice(2)}@example.com`;
}

interface SignUpResult {
  id: string;
  email: string;
  apiKey: string;
}

// Signs up a fresh account via the API instead of the UI, for tests where
// the signup form itself isn't what's under test -- faster, and keeps the
// spec's asserts focused. Uses context.request (not the top-level request
// fixture) specifically because it shares its cookie jar with the pages in
// this context: the session cookie POST /auth/signup sets becomes usable by
// a subsequent page.goto() in the same test, without a separate sign-in
// step.
export async function apiSignUp(context: BrowserContext, email: string): Promise<SignUpResult> {
  const res = await context.request.post("/api/auth/signup", {
    data: { email, password: PASSWORD },
  });
  if (!res.ok()) {
    throw new Error(`signup failed: ${res.status()} ${await res.text()}`);
  }
  return res.json() as Promise<SignUpResult>;
}
