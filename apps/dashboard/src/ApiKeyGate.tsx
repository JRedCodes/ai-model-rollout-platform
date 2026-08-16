import { useState } from "react";
import { setApiKey } from "./apiKey.ts";

export function ApiKeyGate() {
  const [draft, setDraft] = useState("");

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans flex items-center justify-center p-6">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (draft.trim()) setApiKey(draft.trim());
        }}
        className="w-full max-w-sm rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4"
      >
        <div>
          <p className="text-sm font-semibold tracking-widest uppercase text-slate-400">
            Rollout Platform
          </p>
          <p className="text-xs text-slate-500 mt-1">
            Enter your tenant API key to continue.
          </p>
        </div>

        <input
          type="password"
          autoFocus
          placeholder="tk_..."
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          className="w-full rounded-md border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-200 font-mono focus:outline-none focus:ring-1 focus:ring-emerald-500"
        />

        <button
          type="submit"
          disabled={!draft.trim()}
          className="w-full text-sm font-medium rounded-md px-3 py-2 bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Continue
        </button>

        <p className="text-xs text-slate-600">
          Keys are issued via <code className="text-slate-500">POST /tenants</code>{" "}
          (admin-gated) and stored only in this browser.
        </p>
      </form>
    </div>
  );
}
