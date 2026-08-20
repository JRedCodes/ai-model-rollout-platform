import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { regenerateApiKey, signOut } from "./api.ts";
import { useSession } from "./useSession.ts";
import { navigate } from "./router.ts";

export function AccountPanel({ onSignedOut }: { onSignedOut: () => void }) {
  const [session] = useSession();
  const [newKey, setNewKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const regenerate = useMutation({
    mutationFn: regenerateApiKey,
    onSuccess: (body) => {
      setNewKey(body.apiKey);
      setCopied(false);
    },
  });

  const signOutMutation = useMutation({
    mutationFn: signOut,
    onSuccess: onSignedOut,
  });

  if (session.status !== "signed-in") {
    return null;
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans flex items-center justify-center p-6">
      <div className="w-full max-w-sm rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4">
        <div>
          <p className="text-sm font-semibold tracking-widest uppercase text-slate-400">Account</p>
          <p className="text-xs text-slate-500 mt-1">{session.session.email}</p>
        </div>

        {newKey ? (
          <div className="space-y-2">
            <p className="text-xs text-slate-500">
              Your new key -- paste it into the stress-tester CLI (
              <code className="text-slate-600">--apiKey</code>). It's shown only once, and your old
              key stopped working immediately.
            </p>
            <div className="flex gap-2">
              <code className="flex-1 min-w-0 truncate rounded-md border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-emerald-400 font-mono">
                {newKey}
              </code>
              <button
                type="button"
                onClick={() => {
                  void navigator.clipboard.writeText(newKey);
                  setCopied(true);
                }}
                className="shrink-0 text-sm font-medium rounded-md px-3 py-2 bg-slate-800 text-slate-200 hover:bg-slate-700 transition-colors"
              >
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-between gap-2">
            <code className="text-sm text-slate-500 font-mono">tk_••••••••</code>
            <button
              type="button"
              onClick={() => regenerate.mutate()}
              disabled={regenerate.isPending}
              className="text-sm font-medium rounded-md px-3 py-2 bg-slate-800 text-slate-200 hover:bg-slate-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {regenerate.isPending ? "Regenerating..." : "Regenerate key"}
            </button>
          </div>
        )}

        {regenerate.isError && <p className="text-xs text-red-400">Failed to regenerate key.</p>}

        <div className="pt-2 border-t border-slate-800 space-y-2">
          <button
            type="button"
            onClick={() => navigate("/")}
            className="w-full text-sm font-medium rounded-md px-3 py-2 bg-slate-800 text-slate-200 hover:bg-slate-700 transition-colors"
          >
            Back to dashboard
          </button>
          <button
            type="button"
            onClick={() => signOutMutation.mutate()}
            disabled={signOutMutation.isPending}
            className="w-full text-xs text-slate-500 hover:text-slate-300 disabled:opacity-40 underline underline-offset-2"
          >
            {signOutMutation.isPending ? "Signing out..." : "Sign out"}
          </button>
        </div>
      </div>
    </div>
  );
}
