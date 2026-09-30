import type { Route } from "./navigation";
import type { WorkspaceData } from "./types";

export const workspaceResources = {
  system: "/v1/system/status",
  agents: "/v1/agents?limit=100",
  traces: "/v1/traces?limit=100",
  evolutionJobs: "/v1/evolution-jobs?limit=40",
} as const;

export const routeResources: Record<Route, Array<keyof typeof workspaceResources>> = {
  home: ["agents", "traces", "evolutionJobs"],
  agents: ["agents"], apiKeys: ["agents"], traces: ["agents", "traces"],
  evolution: ["agents", "evolutionJobs"], conversations: ["system"], memory: ["system"], settings: [],
};

export function emptyWorkspace(): WorkspaceData {
  return {
    system: { status: "unknown", auth_mode: "", edge_ingest: "", run_bundle: "barena.run_bundle.v1",
      evolution_protocol: "barena.xiaoba_evolution_request.v1", evolution_runtime: "", trace_store: "unknown", memory_store: "unknown" },
    runtimes: [], runs: [], evolutionJobs: [], issues: [], cases: [], releases: [],
    traceAvailable: false, agentAvailable: false, traces: [], agents: [],
  };
}
