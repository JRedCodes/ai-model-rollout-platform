import { useQuery } from "@tanstack/react-query";
import { fetchMetrics } from "./api.ts";

const ADVANCE_ERROR_THRESHOLD = 0.02;
const ADVANCE_P95_THRESHOLD = 250;
const ABSOLUTE_ERROR_THRESHOLD = 0.05;
const FRESH_ERROR_THRESHOLD = 0.3;

function pct(rate: number) {
  return (rate * 100).toFixed(2);
}

function errorColor(rate: number): string {
  if (rate >= FRESH_ERROR_THRESHOLD) return "text-red-400";
  if (rate >= ABSOLUTE_ERROR_THRESHOLD) return "text-red-400";
  if (rate > ADVANCE_ERROR_THRESHOLD) return "text-amber-400";
  return "text-emerald-400";
}

function latencyColor(p95: number): string {
  if (p95 > ADVANCE_P95_THRESHOLD) return "text-amber-400";
  return "text-emerald-400";
}

interface MetricCardProps {
  label: string;
  value: string;
  valueClass: string;
  sub?: string;
  threshold?: string;
}

function MetricCard({ label, value, valueClass, sub, threshold }: MetricCardProps) {
  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900 p-5">
      <p className="text-xs text-slate-500 uppercase tracking-wider mb-3">{label}</p>
      <p className={["text-3xl font-semibold tabular-nums", valueClass].join(" ")}>
        {value}
      </p>
      {sub && <p className="text-xs text-slate-500 mt-1">{sub}</p>}
      {threshold && (
        <p className="text-xs text-slate-600 mt-2 border-t border-slate-800 pt-2">
          {threshold}
        </p>
      )}
    </div>
  );
}

export function MetricsPanel() {
  const { data, isLoading } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 3_000,
  });

  if (isLoading || !data) {
    return (
      <div className="grid grid-cols-2 gap-4">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="rounded-lg border border-slate-800 bg-slate-900 p-5 animate-pulse">
            <div className="h-3 bg-slate-800 rounded w-24 mb-3" />
            <div className="h-8 bg-slate-800 rounded w-20" />
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-4">
      <MetricCard
        label="Window error rate"
        value={`${pct(data.windowErrorRate)}%`}
        valueClass={errorColor(data.windowErrorRate)}
        sub={`overall: ${pct(data.overallErrorRate)}%`}
        threshold={`advance: <${pct(ADVANCE_ERROR_THRESHOLD)}% · hold: >${pct(ABSOLUTE_ERROR_THRESHOLD)}% · rollback: >${pct(FRESH_ERROR_THRESHOLD)}%`}
      />
      <MetricCard
        label="Window P95 latency"
        value={`${data.windowP95LatencyMs}ms`}
        valueClass={latencyColor(data.windowP95LatencyMs)}
        threshold={`advance threshold: <${ADVANCE_P95_THRESHOLD}ms`}
      />
      <MetricCard
        label="Window requests"
        value={data.windowRequestCount.toLocaleString()}
        valueClass="text-slate-200"
        sub="last 2 minutes"
      />
      <MetricCard
        label="Total requests"
        value={data.totalRequests.toLocaleString()}
        valueClass="text-slate-200"
        sub="session lifetime"
      />
    </div>
  );
}
