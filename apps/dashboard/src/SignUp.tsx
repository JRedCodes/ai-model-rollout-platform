import { useState } from "react";
import { navigate } from "./router.ts";

interface SignUpResponse {
  id: string;
  email: string;
  apiKey: string;
}

export function SignUp({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [issuedKey, setIssuedKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const res = await fetch("/api/auth/signup", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      if (!res.ok) {
        setError(
          res.status === 409
            ? "That email is already registered."
            : (await res.text()) || "Sign up failed.",
        );
        return;
      }

      const body = (await res.json()) as SignUpResponse;
      setIssuedKey(body.apiKey);
    } finally {
      setSubmitting(false);
    }
  }

  if (issuedKey) {
    return (
      <div className="min-h-screen bg-slate-950 text-slate-50 font-sans flex items-center justify-center p-6">
        <div className="w-full max-w-sm rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4">
          <div>
            <p className="text-sm font-semibold tracking-widest uppercase text-slate-400">
              Account created
            </p>
            <p className="text-xs text-slate-500 mt-1">
              This is your tenant API key -- paste it into the stress-tester CLI
              (<code className="text-slate-500">--apiKey</code>) to run the
              simulation. It's shown only once.
            </p>
          </div>

          <div className="flex gap-2">
            <code className="flex-1 min-w-0 truncate rounded-md border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-emerald-400 font-mono">
              {issuedKey}
            </code>
            <button
              type="button"
              onClick={() => {
                void navigator.clipboard.writeText(issuedKey);
                setCopied(true);
              }}
              className="shrink-0 text-sm font-medium rounded-md px-3 py-2 bg-slate-800 text-slate-200 hover:bg-slate-700 transition-colors"
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>

          <button
            type="button"
            disabled={!copied}
            onClick={onAuthenticated}
            className="w-full text-sm font-medium rounded-md px-3 py-2 bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {copied ? "Continue to dashboard" : "Copy your key to continue"}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans flex items-center justify-center p-6">
      <form
        onSubmit={(e) => void handleSubmit(e)}
        className="w-full max-w-sm rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4"
      >
        <div>
          <p className="text-sm font-semibold tracking-widest uppercase text-slate-400">
            Rollout Platform
          </p>
          <p className="text-xs text-slate-500 mt-1">
            Create an account to get a tenant and an API key.{" "}
            <button
              type="button"
              onClick={() => navigate("/about")}
              className="underline underline-offset-2 hover:text-slate-300"
            >
              What is this?
            </button>
          </p>
        </div>

        <input
          type="email"
          autoFocus
          required
          placeholder="you@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-md border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-emerald-500"
        />

        <input
          type="password"
          required
          minLength={8}
          placeholder="Password (min. 8 characters)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-200 font-mono focus:outline-none focus:ring-1 focus:ring-emerald-500"
        />

        {error && <p className="text-xs text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={submitting || !email.trim() || password.length < 8}
          className="w-full text-sm font-medium rounded-md px-3 py-2 bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {submitting ? "Creating account..." : "Create account"}
        </button>

        <p className="text-xs text-slate-600">
          Already have an account?{" "}
          <button
            type="button"
            onClick={() => navigate("/signin")}
            className="text-slate-400 hover:text-slate-200 underline underline-offset-2"
          >
            Sign in
          </button>
        </p>
      </form>
    </div>
  );
}
