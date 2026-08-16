import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createRollout, fetchModelConfigs } from "./api.ts";

const inputClass =
  "rounded-md border border-slate-700 bg-slate-950 px-2 py-1 text-sm text-slate-200 tabular-nums focus:outline-none focus:ring-1 focus:ring-emerald-500";

const AUTO_STABLE = "";

export function CreateRolloutPanel() {
  const qc = useQueryClient();

  const { data: models } = useQuery({
    queryKey: ["modelConfigs"],
    queryFn: fetchModelConfigs,
  });

  const [rolloutPhaseId, setRolloutPhaseId] = useState("");
  const [candidateModelVersionId, setCandidateModelVersionId] = useState("");
  const [stableModelVersionId, setStableModelVersionId] = useState(AUTO_STABLE);
  const [candidatePercentage, setCandidatePercentage] = useState("10");

  const mutation = useMutation({
    mutationFn: () =>
      createRollout({
        rolloutPhaseId,
        candidateModelVersionId,
        candidatePercentage: Number(candidatePercentage),
        ...(stableModelVersionId ? { stableModelVersionId } : {}),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["rollout"] });
      void qc.invalidateQueries({ queryKey: ["metrics"] });
      void qc.invalidateQueries({ queryKey: ["decisions"] });
      setRolloutPhaseId("");
      setCandidateModelVersionId("");
      setStableModelVersionId(AUTO_STABLE);
      setCandidatePercentage("10");
    },
  });

  const pctNum = Number(candidatePercentage);
  const isValid =
    rolloutPhaseId.trim() !== "" &&
    candidateModelVersionId !== "" &&
    Number.isFinite(pctNum) &&
    pctNum >= 0 &&
    pctNum <= 100;

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900 p-6 space-y-4">
      <p className="text-xs text-slate-500 uppercase tracking-wider">Create rollout</p>

      <div className="grid grid-cols-2 gap-4">
        <label className="flex flex-col gap-1">
          <span className="text-xs text-slate-500">Rollout phase ID</span>
          <input
            type="text"
            placeholder="e.g. phase-2"
            value={rolloutPhaseId}
            onChange={(e) => setRolloutPhaseId(e.target.value)}
            className={inputClass}
          />
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-xs text-slate-500">Candidate traffic %</span>
          <input
            type="number"
            min={0}
            max={100}
            value={candidatePercentage}
            onChange={(e) => setCandidatePercentage(e.target.value)}
            className={inputClass}
          />
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-xs text-slate-500">Candidate model</span>
          <select
            value={candidateModelVersionId}
            onChange={(e) => setCandidateModelVersionId(e.target.value)}
            className={inputClass}
          >
            <option value="">Select a model…</option>
            {models?.map((m) => (
              <option key={m.modelVersionId} value={m.modelVersionId}>
                {m.modelVersionId}
              </option>
            ))}
          </select>
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-xs text-slate-500">Stable model</span>
          <select
            value={stableModelVersionId}
            onChange={(e) => setStableModelVersionId(e.target.value)}
            className={inputClass}
          >
            <option value={AUTO_STABLE}>Auto — last completed rollout's candidate</option>
            {models?.map((m) => (
              <option key={m.modelVersionId} value={m.modelVersionId}>
                {m.modelVersionId}
              </option>
            ))}
          </select>
        </label>
      </div>

      <button
        onClick={() => mutation.mutate()}
        disabled={!isValid || mutation.isPending}
        className="text-xs font-medium rounded-md px-3 py-1.5 bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        {mutation.isPending ? "Creating…" : "Create rollout"}
      </button>

      {mutation.isError && (
        <p className="text-xs text-red-400">{(mutation.error as Error).message}</p>
      )}

      {mutation.isSuccess && (
        <p className="text-xs text-emerald-400">
          Created {mutation.data.id} — {mutation.data.stableModelVersionId} → {mutation.data.candidateModelVersionId}
        </p>
      )}
    </div>
  );
}
