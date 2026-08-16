import { useEffect, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StatusPanel } from "./StatusPanel.tsx";
import { MetricsPanel } from "./MetricsPanel.tsx";
import { DecisionFeed } from "./DecisionFeed.tsx";
import { ModelConfigPanel } from "./ModelConfigPanel.tsx";
import { ApiKeyGate } from "./ApiKeyGate.tsx";
import { useSSE } from "./useSSE.ts";
import { getApiKey, onApiKeyChange } from "./apiKey.ts";

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
      <header className="border-b border-slate-800 px-6 py-4 flex items-center gap-3">
        <div className="w-2 h-2 rounded-full bg-emerald-500" />
        <span className="text-sm font-semibold tracking-widest uppercase text-slate-400">
          Rollout Platform
        </span>
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

function useApiKeyPresent(): boolean {
  const [hasKey, setHasKey] = useState(() => getApiKey() !== null);

  useEffect(() => onApiKeyChange(() => setHasKey(getApiKey() !== null)), []);

  return hasKey;
}

export default function App() {
  const hasKey = useApiKeyPresent();

  return (
    <QueryClientProvider client={queryClient}>
      {hasKey ? <Dashboard /> : <ApiKeyGate />}
    </QueryClientProvider>
  );
}
