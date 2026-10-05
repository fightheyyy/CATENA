export const primaryNavigationRoutes = ["home", "traces", "memory", "cases", "evolution"] as const;

export const routePaths = {
  home: "/", assistant: "/assistant", agents: "/agents", apiKeys: "/api-keys", conversations: "/conversations",
  traces: "/traces", cases: "/cases", evolution: "/evolution", memory: "/memory", settings: "/settings",
} as const;

export type Route = keyof typeof routePaths;
export type EvidenceSelection = { agentID?: string; traceID?: string; jobID?: string };

export function readNavigation(location: { pathname: string; search: string }) {
  const matched = (Object.keys(routePaths) as Route[]).find((key) => routePaths[key] === location.pathname) ?? "home";
  const query = new URLSearchParams(location.search);
  const route = matched === "assistant" || (matched === "evolution" && query.has("agent") && !query.has("job")) ? "home" : matched;
  return { route, agentID: query.get("agent") || "", traceID: query.get("trace") || "", jobID: query.get("job") || "" };
}

export function navigationURL(route: Route, selection: EvidenceSelection = {}) {
  const query = new URLSearchParams();
  if (selection.agentID && ["home", "agents", "assistant", "traces", "evolution"].includes(route)) query.set("agent", selection.agentID);
  if (selection.traceID && route === "traces") query.set("trace", selection.traceID);
  if (selection.jobID && ["home", "evolution"].includes(route)) query.set("job", selection.jobID);
  return routePaths[route === "assistant" ? "home" : route] + (query.size ? `?${query}` : "");
}
