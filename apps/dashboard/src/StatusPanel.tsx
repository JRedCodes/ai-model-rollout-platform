import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchRollout, postRollback } from "./api.ts";
import { CreateRolloutPanel } from "./CreateRolloutPanel.tsx";

const LADDER = [10, 25, 50, 75, 100];

function StatusBadge({ held }: { held: boolean }) {
  return held ? (
    <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-xs font-medium text-amber-400 ring-1 ring-amber-500/20">
      <span className="w-1.5 h-1.5 rounded-full bg-amber-400" />
      HELD
    </span>
  ) : (
    <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-xs font-medium text-emerald-400 ring-1 ring-emerald-500/20">
      <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
      RUNNING
    </span>
  );
}

function AdvancementLadder({ pct }: { pct: number }) {
  return (
    <div className="mt-4">
      <p className="text-xs text-slate-500 mb-2 uppercase tracking-wider">Traffic ladder</p>
      <div className="flex items-center gap-1">
        <span className="text-xs text-slate-500 w-10 shrink-0">0%</span>
        <div className="flex-1 flex items-center gap-1">
          {LADDER.map((step, i) => {
            const reached = pct >= step;
            const active = pct > 0 && ((i === 0 && pct < step) || (pct >= (LADDER[i - 1] ?? 0) && pct < step) || pct === step);
            return (
              <div key={step} className="flex-1 flex flex-col items-center gap-1">
                <div
                  className={[
                    "h-2 w-full rounded-full transition-colors",
                    reached ? "bg-emerald-500" : active ? "bg-emerald-500/40" : "bg-slate-700",
                  ].join(" ")}
                />
                <span
                  className={[
                    "text-xs tabular-nums",
                    reached ? "text-emerald-400" : "text-slate-600",
                  ].join(" ")}
                >
                  {step}%
                </span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export function StatusPanel() {
  const qc = useQueryClient();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["rollout"],
    queryFn: fetchRollout,
    refetchInterval: 5_000,
  });

  const rollback = useMutation({
    mutationFn: postRollback,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["rollout"] });
    },
  });

  if (isLoading) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900 p-6 animate-pulse">
        <div className="h-4 bg-slate-800 rounded w-32 mb-3" />
        <div className="h-8 bg-slate-800 rounded w-16" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div className="rounded-lg border border-red-900/50 bg-red-950/20 p-6 text-red-400 text-sm">
        Could not reach rollout controller. Is it running on :4003?
      </div>
    );
  }

  if (!data.active) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-slate-800 bg-slate-900 p-6">
          <p className="text-xs text-slate-500 uppercase tracking-wider mb-1">Rollout</p>
          <p className="text-sm text-slate-400">
            No active rollout — create one below to start shifting traffic.
          </p>
        </div>
        <CreateRolloutPanel />
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs text-slate-500 uppercase tracking-wider mb-1">Rollout</p>
          <p className="font-mono text-sm text-slate-300">{data.rolloutId}</p>
        </div>
        <StatusBadge held={data.held} />
      </div>

      <div className="grid grid-cols-2 gap-4 text-sm">
        <div>
          <p className="text-xs text-slate-500 mb-1">Stable</p>
          <p className="font-mono text-slate-300">{data.stableModelVersionId}</p>
        </div>
        <div>
          <p className="text-xs text-slate-500 mb-1">Candidate</p>
          <p className="font-mono text-slate-300">{data.candidateModelVersionId || "—"}</p>
        </div>
      </div>

      <div>
        <p className="text-xs text-slate-500 mb-1">Candidate traffic</p>
        <p className="text-4xl font-semibold tabular-nums text-slate-100">
          {data.candidatePercentage}
          <span className="text-xl text-slate-500">%</span>
        </p>
      </div>

      <AdvancementLadder pct={data.candidatePercentage} />

      <div className="pt-2 border-t border-slate-800">
        <button
          onClick={() => {
            if (confirm("Force rollback? This will immediately clear candidate traffic.")) {
              rollback.mutate();
            }
          }}
          disabled={rollback.isPending}
          className="text-xs text-red-400 hover:text-red-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {rollback.isPending ? "Rolling back…" : "Force rollback"}
        </button>
      </div>
    </div>
  );
}
