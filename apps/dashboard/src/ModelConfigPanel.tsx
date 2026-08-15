import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchModelConfigs, updateModelConfig, type ModelConfig } from "./api.ts";

function pct(rate: number) {
  return (rate * 100).toFixed(2);
}

interface RowState {
  failureRatePct: string;
  minLatencyMs: string;
  maxLatencyMs: string;
}

function toRowState(config: ModelConfig): RowState {
  return {
    failureRatePct: pct(config.failureRate),
    minLatencyMs: String(config.minLatencyMs),
    maxLatencyMs: String(config.maxLatencyMs),
  };
}

const inputClass =
  "rounded-md border border-slate-700 bg-slate-950 px-2 py-1 text-sm text-slate-200 tabular-nums focus:outline-none focus:ring-1 focus:ring-emerald-500";

function ModelConfigRow({ config }: { config: ModelConfig }) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<RowState>(() => toRowState(config));

  useEffect(() => {
    setDraft(toRowState(config));
  }, [config.modelVersionId, config.failureRate, config.minLatencyMs, config.maxLatencyMs]);

  const mutation = useMutation({
    mutationFn: () =>
      updateModelConfig(config.modelVersionId, {
        failureRate: Number(draft.failureRatePct) / 100,
        minLatencyMs: Number(draft.minLatencyMs),
        maxLatencyMs: Number(draft.maxLatencyMs),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["modelConfigs"] });
    },
  });

  const failureRateNum = Number(draft.failureRatePct);
  const minLatencyNum = Number(draft.minLatencyMs);
  const maxLatencyNum = Number(draft.maxLatencyMs);

  const isValid =
    Number.isFinite(failureRateNum) &&
    failureRateNum >= 0 &&
    failureRateNum <= 100 &&
    Number.isFinite(minLatencyNum) &&
    minLatencyNum > 0 &&
    Number.isFinite(maxLatencyNum) &&
    maxLatencyNum > 0 &&
    minLatencyNum <= maxLatencyNum;

  const isDirty =
    draft.failureRatePct !== pct(config.failureRate) ||
    draft.minLatencyMs !== String(config.minLatencyMs) ||
    draft.maxLatencyMs !== String(config.maxLatencyMs);

  return (
    <div className="grid grid-cols-[1fr_auto_auto_auto] gap-4 items-end border-t border-slate-800 pt-4 first:border-t-0 first:pt-0">
      <div>
        <p className="text-xs text-slate-500 mb-1">Model</p>
        <p className="font-mono text-sm text-slate-300">{config.modelVersionId}</p>
      </div>

      <label className="flex flex-col gap-1">
        <span className="text-xs text-slate-500">Failure rate %</span>
        <input
          type="number"
          min={0}
          max={100}
          step="0.01"
          value={draft.failureRatePct}
          onChange={(e) => setDraft((d) => ({ ...d, failureRatePct: e.target.value }))}
          className={`w-24 ${inputClass}`}
        />
      </label>

      <label className="flex flex-col gap-1">
        <span className="text-xs text-slate-500">Latency ms (min–max)</span>
        <div className="flex items-center gap-1">
          <input
            type="number"
            min={1}
            value={draft.minLatencyMs}
            onChange={(e) => setDraft((d) => ({ ...d, minLatencyMs: e.target.value }))}
            className={`w-16 ${inputClass}`}
          />
          <span className="text-slate-600">–</span>
          <input
            type="number"
            min={1}
            value={draft.maxLatencyMs}
            onChange={(e) => setDraft((d) => ({ ...d, maxLatencyMs: e.target.value }))}
            className={`w-16 ${inputClass}`}
          />
        </div>
      </label>

      <button
        onClick={() => mutation.mutate()}
        disabled={!isDirty || !isValid || mutation.isPending}
        className="text-xs font-medium rounded-md px-3 py-1.5 bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        {mutation.isPending ? "Saving…" : "Save"}
      </button>

      {mutation.isError && (
        <p className="col-span-4 text-xs text-red-400">
          Failed to save: {(mutation.error as Error).message}
        </p>
      )}
    </div>
  );
}

export function ModelConfigPanel() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["modelConfigs"],
    queryFn: fetchModelConfigs,
    refetchInterval: 5_000,
  });

  if (isLoading) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900 p-6 animate-pulse">
        <div className="h-4 bg-slate-800 rounded w-40 mb-3" />
        <div className="h-8 bg-slate-800 rounded w-full" />
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

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4">
      <p className="text-xs text-slate-500 uppercase tracking-wider">Model configuration</p>
      {data.map((config) => (
        <ModelConfigRow key={config.modelVersionId} config={config} />
      ))}
    </div>
  );
}
