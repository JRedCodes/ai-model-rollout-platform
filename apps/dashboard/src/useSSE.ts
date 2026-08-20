import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getApiKey } from "./apiKey.ts";
import { API_BASE } from "./apiBase.ts";

export function useSSE() {
  const queryClient = useQueryClient();

  useEffect(() => {
    // Browsers' EventSource can't send custom headers, so a stored API key
    // goes as a query param here -- the one route the server accepts that
    // from. Without one, withCredentials carries the session cookie
    // instead (EventSource doesn't send cookies by default).
    const apiKey = getApiKey();
    const eventsUrl = `${API_BASE}/events`;
    const url = apiKey ? `${eventsUrl}?api_key=${encodeURIComponent(apiKey)}` : eventsUrl;
    const es = new EventSource(url, { withCredentials: !apiKey });

    es.onmessage = () => {
      void queryClient.invalidateQueries({ queryKey: ["rollout"] });
      void queryClient.invalidateQueries({ queryKey: ["metrics"] });
      void queryClient.invalidateQueries({ queryKey: ["decisions"] });
      void queryClient.invalidateQueries({ queryKey: ["modelConfigs"] });
    };

    es.onerror = () => {
      // EventSource auto-reconnects on error; no action needed
    };

    return () => es.close();
  }, [queryClient]);
}
