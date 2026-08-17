import { useEffect, useState } from "react";

// A handful of views doesn't justify a router dependency -- this is just
// the History API plus a popstate listener, mirroring apiKey.ts's
// window-event pattern for cross-component state.
export type Route = "/" | "/signin" | "/signup" | "/account" | "/about";

const VALID_ROUTES: readonly Route[] = ["/", "/signin", "/signup", "/account", "/about"];

function currentRoute(): Route {
  const path = window.location.pathname;
  return (VALID_ROUTES as readonly string[]).includes(path) ? (path as Route) : "/";
}

export function navigate(route: Route): void {
  if (window.location.pathname === route) return;
  window.history.pushState(null, "", route);
  window.dispatchEvent(new PopStateEvent("popstate"));
}

export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(currentRoute);

  useEffect(() => {
    const onPopState = () => setRoute(currentRoute());
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  return route;
}
