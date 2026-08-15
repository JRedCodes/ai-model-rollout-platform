import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2_000,
      retry: 2,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <div className="min-h-screen bg-slate-950 text-slate-50 font-sans">
        <header className="border-b border-slate-800 px-6 py-4 flex items-center gap-3">
          <div className="w-2 h-2 rounded-full bg-emerald-500" />
          <span className="text-sm font-semibold tracking-widest uppercase text-slate-400">
            Rollout Platform
          </span>
        </header>
        <main className="max-w-5xl mx-auto p-6">
          <p className="text-slate-500 text-sm">Connecting to rollout controller...</p>
        </main>
      </div>
    </QueryClientProvider>
  );
}
