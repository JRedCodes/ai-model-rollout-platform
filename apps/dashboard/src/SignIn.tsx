import { useState } from "react";
import { navigate } from "./router.ts";
import { ApiKeyGate } from "./ApiKeyGate.tsx";

export function SignIn({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [useLegacyKey, setUseLegacyKey] = useState(false);

  if (useLegacyKey) {
    return <ApiKeyGate onBack={() => setUseLegacyKey(false)} />;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const res = await fetch("/api/auth/signin", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      if (!res.ok) {
        setError("Invalid email or password.");
        return;
      }

      onAuthenticated();
    } finally {
      setSubmitting(false);
    }
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
          <p className="text-xs text-slate-500 mt-1">Sign in to continue.</p>
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
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-200 font-mono focus:outline-none focus:ring-1 focus:ring-emerald-500"
        />

        {error && <p className="text-xs text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={submitting || !email.trim() || !password}
          className="w-full text-sm font-medium rounded-md px-3 py-2 bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {submitting ? "Signing in..." : "Sign in"}
        </button>

        <p className="text-xs text-slate-600">
          Don't have an account?{" "}
          <button
            type="button"
            onClick={() => navigate("/signup")}
            className="text-slate-400 hover:text-slate-200 underline underline-offset-2"
          >
            Sign up
          </button>
        </p>
        <p className="text-xs text-slate-600">
          Already have a tenant API key?{" "}
          <button
            type="button"
            onClick={() => setUseLegacyKey(true)}
            className="text-slate-400 hover:text-slate-200 underline underline-offset-2"
          >
            Use it directly
          </button>
        </p>
      </form>
    </div>
  );
}
