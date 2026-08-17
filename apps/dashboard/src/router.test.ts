import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { navigate, useRoute } from "./router.ts";

function setPath(path: string) {
  window.history.pushState(null, "", path);
}

describe("navigate", () => {
  beforeEach(() => {
    setPath("/");
  });

  it("pushes the new path onto history", () => {
    navigate("/signin");
    expect(window.location.pathname).toBe("/signin");
  });

  it("does nothing when already on the target route", () => {
    setPath("/signup");
    const pushStateSpy = vi.spyOn(window.history, "pushState");

    navigate("/signup");

    expect(pushStateSpy).not.toHaveBeenCalled();
    pushStateSpy.mockRestore();
  });
});

describe("useRoute", () => {
  beforeEach(() => {
    setPath("/");
  });

  afterEach(() => {
    setPath("/");
  });

  it("returns the current path when it's a known route", () => {
    setPath("/signin");
    const { result } = renderHook(() => useRoute());
    expect(result.current).toBe("/signin");
  });

  it("defaults to / for an unrecognized path", () => {
    setPath("/does-not-exist");
    const { result } = renderHook(() => useRoute());
    expect(result.current).toBe("/");
  });

  it("updates when navigate() is called", () => {
    const { result } = renderHook(() => useRoute());
    expect(result.current).toBe("/");

    act(() => {
      navigate("/account");
    });

    expect(result.current).toBe("/account");
  });
});
