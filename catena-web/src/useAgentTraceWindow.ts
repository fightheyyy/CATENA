import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import { agentEvolutionWindow } from "./traceFarm";
import type { TraceSummary } from "./types";

type AgentTraceState = {
  agentID: string;
  traces: TraceSummary[];
  loading: boolean;
  error: string;
};

const emptyState: AgentTraceState = {
  agentID: "",
  traces: [],
  loading: false,
  error: "",
};

export function useAgentTraceWindow(agentID: string, limit: number) {
  const [state, setState] = useState<AgentTraceState>(emptyState);
  const [requestVersion, setRequestVersion] = useState(0);
  const retry = useCallback(() => setRequestVersion((value) => value + 1), []);

  useEffect(() => {
    if (!agentID) {
      setState(emptyState);
      return;
    }
    let active = true;
    const evidenceWindow = agentEvolutionWindow(new Date(), "30d");
    setState({ agentID, traces: [], loading: true, error: "" });
    void api.agentTraces(
      agentID,
      evidenceWindow.window_start,
      evidenceWindow.window_end,
      Math.max(1, Math.min(500, Math.trunc(limit))),
    ).then((result) => {
      if (active) setState({ agentID, traces: result.traces, loading: false, error: "" });
    }).catch((cause) => {
      if (!active) return;
      setState({
        agentID,
        traces: [],
        loading: false,
        error: cause instanceof Error ? cause.message : "Agent Trace query failed",
      });
    });
    return () => { active = false; };
  }, [agentID, limit, requestVersion]);

  if (!agentID) return { ...emptyState, retry };
  if (state.agentID !== agentID) {
    return { agentID, traces: [], loading: true, error: "", retry };
  }
  return { ...state, retry };
}
