import type { Route } from "./navigation";
import type { WorkspaceData } from "./types";

export const workspaceResources = {
  system: "/v1/system/status",
  agents: "/v1/agents?limit=100",
  traces: "/v1/traces?limit=100",
  analysisJobs: "/v1/analysis-jobs?limit=40",
} as const;

export const routeResources: Record<Route, Array<keyof typeof workspaceResources>> = {
  home: ["agents", "traces", "analysisJobs"],
  assistant: ["agents", "traces", "analysisJobs"],
  agents: ["agents"], apiKeys: ["agents"], traces: ["agents", "traces"],
  evolution: ["agents", "analysisJobs"], conversations: ["system"], memory: ["system"], settings: [],
};

export function emptyWorkspace(): WorkspaceData {
  return {
    system: { status: "unknown", auth_mode: "", edge_ingest: "",
      analysis_protocol: "catena.analysis_request.v2", analysis_engine: "", trace_store: "unknown", memory_store: "unknown" },
    analysisJobs: [],
    traceAvailable: false, agentAvailable: false, traces: [], agents: [],
  };
}
