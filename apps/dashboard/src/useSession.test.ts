import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useSession } from "./useSession.ts";

const mockFetch = vi.fn();

beforeEach(() => {
  vi.stubGlobal("fetch", mockFetch);
});

afterEach(() => {
  vi.unstubAllGlobals();
  mockFetch.mockReset();
});

describe("useSession", () => {
  it("starts in the loading state", () => {
    mockFetch.mockReturnValue(new Promise(() => {})); // never resolves
    const { result } = renderHook(() => useSession());
    expect(result.current[0]).toEqual({ status: "loading" });
  });

  it("resolves to signed-in with the session on a successful GET /auth/me", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ id: "user-1", email: "user@example.com" }),
    });

    const { result } = renderHook(() => useSession());

    await waitFor(() => {
      expect(result.current[0].status).toBe("signed-in");
    });
    expect(result.current[0]).toEqual({
      status: "signed-in",
      session: { id: "user-1", email: "user@example.com" },
    });
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/auth/me",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("resolves to signed-out on a non-ok response", async () => {
    mockFetch.mockResolvedValue({ ok: false, json: async () => ({}) });

    const { result } = renderHook(() => useSession());

    await waitFor(() => {
      expect(result.current[0]).toEqual({ status: "signed-out" });
    });
  });

  it("resolves to signed-out when the request itself fails", async () => {
    mockFetch.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => useSession());

    await waitFor(() => {
      expect(result.current[0]).toEqual({ status: "signed-out" });
    });
  });

  it("refresh() triggers another request", async () => {
    mockFetch.mockResolvedValue({ ok: false, json: async () => ({}) });

    const { result } = renderHook(() => useSession());
    await waitFor(() => expect(result.current[0].status).toBe("signed-out"));

    expect(mockFetch).toHaveBeenCalledTimes(1);

    result.current[1](); // refresh()

    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(2));
  });
});
