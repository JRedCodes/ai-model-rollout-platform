import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../repositories/model-config.repository.js", () => ({
  getModelConfig: vi.fn(),
  UnsupportedModelVersionError: class UnsupportedModelVersionError extends Error {
    constructor(modelVersionId: string) {
      super(`Unsupported model version: ${modelVersionId}`);
      this.name = "UnsupportedModelVersionError";
    }
  },
}));

import {
  getModelConfig,
  UnsupportedModelVersionError,
} from "../repositories/model-config.repository.js";
import { simulateInference } from "./model-simulator.service.js";

const mockGetModelConfig = vi.mocked(getModelConfig);

const baseRequest = {
  requestId: "req-1",
  tenantId: "tenant-1",
  input: { message: "hello" },
};

describe("simulateInference", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("computes latency within [minLatencyMs, maxLatencyMs] from Math.random()", async () => {
    mockGetModelConfig.mockResolvedValue({
      modelVersionId: "model-v1",
      failureRate: 0,
      minLatencyMs: 50,
      maxLatencyMs: 150,
      updatedAt: new Date().toISOString(),
    });
    // First Math.random() call is the latency roll; 0.5 -> floor(0.5*101)+50 = 100.
    vi.spyOn(Math, "random").mockReturnValueOnce(0.5).mockReturnValueOnce(0);

    const resultPromise = simulateInference("model-v1", baseRequest);
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.latencyMs).toBe(100);
  });

  it("clamps to minLatencyMs when Math.random() returns 0", async () => {
    mockGetModelConfig.mockResolvedValue({
      modelVersionId: "model-v1",
      failureRate: 0,
      minLatencyMs: 50,
      maxLatencyMs: 150,
      updatedAt: new Date().toISOString(),
    });
    vi.spyOn(Math, "random").mockReturnValueOnce(0).mockReturnValueOnce(0);

    const resultPromise = simulateInference("model-v1", baseRequest);
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.latencyMs).toBe(50);
  });

  it("reaches maxLatencyMs as Math.random() approaches 1", async () => {
    mockGetModelConfig.mockResolvedValue({
      modelVersionId: "model-v1",
      failureRate: 0,
      minLatencyMs: 50,
      maxLatencyMs: 150,
      updatedAt: new Date().toISOString(),
    });
    vi.spyOn(Math, "random").mockReturnValueOnce(0.999999).mockReturnValueOnce(0);

    const resultPromise = simulateInference("model-v1", baseRequest);
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.latencyMs).toBe(150);
  });

  it("returns a successful response when the failure roll doesn't trigger", async () => {
    mockGetModelConfig.mockResolvedValue({
      modelVersionId: "model-v1",
      failureRate: 0,
      minLatencyMs: 10,
      maxLatencyMs: 10,
      updatedAt: new Date().toISOString(),
    });
    // Second Math.random() call is the failure roll: 0 < failureRate(0) is false.
    vi.spyOn(Math, "random").mockReturnValueOnce(0).mockReturnValueOnce(0);

    const resultPromise = simulateInference("model-v1", baseRequest);
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(true);
    expect(result.requestId).toBe("req-1");
    if (result.success) {
      expect(result.output).toEqual({ classification: "ACCOUNT_ACCESS" });
    }
  });

  it("returns a failed response when the failure roll triggers", async () => {
    mockGetModelConfig.mockResolvedValue({
      modelVersionId: "model-v1",
      failureRate: 1,
      minLatencyMs: 10,
      maxLatencyMs: 10,
      updatedAt: new Date().toISOString(),
    });
    // Second Math.random() call: 0.5 < failureRate(1) is true.
    vi.spyOn(Math, "random").mockReturnValueOnce(0).mockReturnValueOnce(0.5);

    const resultPromise = simulateInference("model-v1", baseRequest);
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.errorType).toBe("SIMULATED_FAILURE");
    }
  });

  it("propagates UnsupportedModelVersionError from the config lookup", async () => {
    mockGetModelConfig.mockRejectedValue(new UnsupportedModelVersionError("model-vX"));

    await expect(simulateInference("model-vX", baseRequest)).rejects.toBeInstanceOf(
      UnsupportedModelVersionError,
    );
  });
});
