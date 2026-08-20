import { useQuery } from "@tanstack/react-query";
import { fetchDecisions } from "./api.ts";
import type { Decision } from "./api.ts";

const ACTION_STYLES: Record<string, { badge: string; dot: string }> = {
  ADVANCE: {
    badge: "bg-emerald-500/10 text-emerald-400 ring-emerald-500/20",
    dot: "bg-emerald-400",
  },
  HOLD: { badge: "bg-amber-500/10 text-amber-400 ring-amber-500/20", dot: "bg-amber-400" },
  ROLLBACK: { badge: "bg-red-500/10 text-red-400 ring-red-500/20", dot: "bg-red-400" },
  COMPLETE: { badge: "bg-sky-500/10 text-sky-400 ring-sky-500/20", dot: "bg-sky-400" },
  RESUME: { badge: "bg-cyan-500/10 text-cyan-400 ring-cyan-500/20", dot: "bg-cyan-400" },
};

function fallbackStyle() {
  return { badge: "bg-slate-500/10 text-slate-400 ring-slate-500/20", dot: "bg-slate-400" };
}

function ActionBadge({ action }: { action: string }) {
  const style = ACTION_STYLES[action] ?? fallbackStyle();
  return (
    <span
      className={[
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ring-1",
        style.badge,
      ].join(" ")}
    >
      <span className={["w-1.5 h-1.5 rounded-full shrink-0", style.dot].join(" ")} />
      {action}
    </span>
  );
}

function DecisionRow({ d }: { d: Decision }) {
  const ts = new Date(d.decidedAt);
  const time = ts.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

  return (
    <div className="flex items-start gap-3 py-3 border-b border-slate-800/60 last:border-0">
      <ActionBadge action={d.action} />
      <div className="flex-1 min-w-0">
        <p className="text-sm text-slate-300 truncate">{d.reason}</p>
        <p className="text-xs text-slate-600 mt-0.5">
          {d.source} · {time}
        </p>
      </div>
    </div>
  );
}

export function DecisionFeed() {
  const { data, isLoading } = useQuery({
    queryKey: ["decisions"],
    queryFn: fetchDecisions,
    refetchInterval: 10_000,
  });

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900 p-6">
      <p className="text-xs text-slate-500 uppercase tracking-wider mb-4">Decision log</p>

      {isLoading && (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-10 bg-slate-800 rounded animate-pulse" />
          ))}
        </div>
      )}

      {!isLoading && (!data || data.length === 0) && (
        <p className="text-sm text-slate-600">
          No decisions yet — waiting for controller evaluation.
        </p>
      )}

      {data && data.length > 0 && (
        <div>
          {data.map((d, i) => (
            <DecisionRow key={i} d={d} />
          ))}
        </div>
      )}
    </div>
  );
}
