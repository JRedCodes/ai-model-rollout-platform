export type ModelSimulationConfig = {
  modelVersionId: string;
  failureRate: number;
  minLatencyMs: number;
  maxLatencyMs: number;
};

export const modelConfigurations: Record<
  string,
  ModelSimulationConfig
> = {
  "model-v1": {
    modelVersionId: "model-v1",
    failureRate: 0.01,
    minLatencyMs: 50,
    maxLatencyMs: 150,
  },

  "model-v2": {
    modelVersionId: "model-v2",
    failureRate: 0.02,
    minLatencyMs: 50,
    maxLatencyMs: 200,
  },
};