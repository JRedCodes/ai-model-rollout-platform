import { useEffect, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StatusPanel } from "./StatusPanel.tsx";
import { MetricsPanel } from "./MetricsPanel.tsx";
import { DecisionFeed } from "./DecisionFeed.tsx";
import { ModelConfigPanel } from "./ModelConfigPanel.tsx";
import { SignIn } from "./SignIn.tsx";
import { SignUp } from "./SignUp.tsx";
import { AccountPanel } from "./AccountPanel.tsx";
import { AboutPage } from "./AboutPage.tsx";
import { useSSE } from "./useSSE.ts";
import { getApiKey, onApiKeyChange } from "./apiKey.ts";
import { useSession } from "./useSession.ts";
import { navigate, useRoute } from "./router.ts";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2_000,
      retry: 2,
    },
  },
});

function Dashboard() {
  useSSE();

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans">
      <header className="border-b border-slate-800 px-6 py-4 flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="w-2 h-2 rounded-full bg-emerald-500" />
          <span className="text-sm font-semibold tracking-widest uppercase text-slate-400">
            Rollout Platform
          </span>
        </div>
        <div className="flex items-center gap-4">
          <button
            type="button"
            onClick={() => navigate("/about")}
            className="text-xs text-slate-500 hover:text-slate-300 underline underline-offset-2"
          >
            About
          </button>
          <button
            type="button"
            onClick={() => navigate("/account")}
            className="text-xs text-slate-500 hover:text-slate-300 underline underline-offset-2"
          >
            Account
          </button>
        </div>
      </header>
      <main className="max-w-5xl mx-auto p-6 space-y-4">
        <StatusPanel />
        <MetricsPanel />
        <ModelConfigPanel />
        <DecisionFeed />
      </main>
    </div>
  );
}

function useLegacyKeyPresent(): boolean {
  const [hasKey, setHasKey] = useState(() => getApiKey() !== null);

  useEffect(() => onApiKeyChange(() => setHasKey(getApiKey() !== null)), []);

  return hasKey;
}

export default function App() {
  const hasLegacyKey = useLegacyKeyPresent();
  const [session, refreshSession] = useSession();
  const route = useRoute();

  const authenticated = hasLegacyKey || session.status === "signed-in";

  function handleAuthenticated() {
    refreshSession();
    navigate("/");
  }

  function handleSignedOut() {
    refreshSession();
    navigate("/signin");
  }

  let view: React.ReactNode;
  if (route === "/about") {
    // Reachable regardless of auth state, and doesn't wait on the session
    // check -- a visitor should be able to read this before ever signing in.
    view = <AboutPage />;
  } else if (!hasLegacyKey && session.status === "loading") {
    // Avoids flashing the sign-in form for an already-signed-in user while
    // GET /auth/me is still in flight on first load.
    view = <div className="min-h-screen bg-slate-950" />;
  } else if (!authenticated) {
    view =
      route === "/signup" ? (
        <SignUp onAuthenticated={handleAuthenticated} />
      ) : (
        <SignIn onAuthenticated={handleAuthenticated} />
      );
  } else if (route === "/account" && session.status === "signed-in") {
    // Only a real (signed-up) account has anything to manage here -- a
    // visitor using a raw legacy API key with no session falls through to
    // the dashboard instead.
    view = <AccountPanel onSignedOut={handleSignedOut} />;
  } else {
    view = <Dashboard />;
  }

  return <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>;
}
