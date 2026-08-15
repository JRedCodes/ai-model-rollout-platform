import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

export function useSSE() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const es = new EventSource("/api/events");

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
