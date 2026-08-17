import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clearApiKey, getApiKey, onApiKeyChange, setApiKey } from "./apiKey.ts";

describe("apiKey", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("returns null when no key is stored", () => {
    expect(getApiKey()).toBeNull();
  });

  it("persists a key across get calls", () => {
    setApiKey("tk_abc123");
    expect(getApiKey()).toBe("tk_abc123");
  });

  it("removes the key on clear", () => {
    setApiKey("tk_abc123");
    clearApiKey();
    expect(getApiKey()).toBeNull();
  });

  it("notifies subscribers when the key is set", () => {
    const callback = vi.fn();
    const unsubscribe = onApiKeyChange(callback);

    setApiKey("tk_abc123");

    expect(callback).toHaveBeenCalledTimes(1);
    unsubscribe();
  });

  it("notifies subscribers when the key is cleared", () => {
    setApiKey("tk_abc123");
    const callback = vi.fn();
    const unsubscribe = onApiKeyChange(callback);

    clearApiKey();

    expect(callback).toHaveBeenCalledTimes(1);
    unsubscribe();
  });

  it("stops notifying after unsubscribe", () => {
    const callback = vi.fn();
    const unsubscribe = onApiKeyChange(callback);
    unsubscribe();

    setApiKey("tk_abc123");

    expect(callback).not.toHaveBeenCalled();
  });
});
