import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getApiKey } from "./apiKey.ts";

export function useSSE() {
  const queryClient = useQueryClient();

  useEffect(() => {
    // Browsers' EventSource can't send custom headers, so the API key goes
    // as a query param here -- the one route the server accepts that from.
    const apiKey = getApiKey();
    const url = apiKey
      ? `/api/events?api_key=${encodeURIComponent(apiKey)}`
      : "/api/events";
    const es = new EventSource(url);

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
