import type { AgentSummary } from "./types";

export function registeredAgentSummaries(agents: AgentSummary[]) {
  return agents.filter((agent) => agent.registered);
}

export function canAnalyzeAgent(agent: AgentSummary) {
  return agent.connected && agent.trace_count >= 2;
}
