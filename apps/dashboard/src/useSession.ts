import { useCallback, useEffect, useState } from "react";

export interface Session {
  id: string;
  email: string;
}

export type SessionState =
  { status: "loading" } | { status: "signed-out" } | { status: "signed-in"; session: Session };

// httpOnly cookies aren't readable from JS (that's the point), so "am I
// signed in" has to be a server round-trip via GET /auth/me instead of a
// client-side localStorage read like the old ApiKeyGate used.
export function useSession(): [SessionState, () => void] {
  const [state, setState] = useState<SessionState>({ status: "loading" });
  const [refreshToken, setRefreshToken] = useState(0);

  const refresh = useCallback(() => setRefreshToken((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;

    async function check() {
      setState({ status: "loading" });
      try {
        const res = await fetch("/api/auth/me", { credentials: "include" });
        if (cancelled) return;
        if (res.ok) {
          const session = (await res.json()) as Session;
          setState({ status: "signed-in", session });
        } else {
          setState({ status: "signed-out" });
        }
      } catch {
        if (!cancelled) setState({ status: "signed-out" });
      }
    }

    void check();
    return () => {
      cancelled = true;
    };
  }, [refreshToken]);

  return [state, refresh];
}
