export type AnalysisWindowPreset = "24h" | "7d" | "30d";
export const AGENT_ANALYSIS_FREEZE_LIMIT = 12;

const presetDurationMS: Record<AnalysisWindowPreset, number> = {
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
  "30d": 30 * 24 * 60 * 60 * 1000,
};

export type AgentAnalysisWindow = {
  window_start: string;
  window_end: string;
};

export function agentAnalysisWindow(now: Date, preset: AnalysisWindowPreset): AgentAnalysisWindow {
  const windowEnd = now.getTime();
  if (!Number.isFinite(windowEnd)) throw new Error("A valid current time is required");
  return {
    window_start: new Date(windowEnd - presetDurationMS[preset]).toISOString(),
    window_end: new Date(windowEnd).toISOString(),
  };
}

export function canStartAgentAnalysis(agentID: string, traceCount: number, loading = false) {
  return Boolean(agentID.trim()) && Number.isInteger(traceCount) && traceCount >= 2 && !loading;
}

export function agentAnalysisTraceSelection(matchedTraceCount: number) {
  const matched = Number.isFinite(matchedTraceCount)
    ? Math.max(0, Math.trunc(matchedTraceCount))
    : 0;
  return {
    matched,
    frozen: Math.min(matched, AGENT_ANALYSIS_FREEZE_LIMIT),
    truncated: matched > AGENT_ANALYSIS_FREEZE_LIMIT,
  };
}

export function agentAnalysisRequestSignature(
  agentID: string,
  window: AgentAnalysisWindow,
  objective: string,
  outputLanguage: "zh-CN" | "en" = "zh-CN",
) {
  return JSON.stringify({
    agent_id: agentID.trim(),
    window_start: window.window_start,
    window_end: window.window_end,
    objective: objective.trim(),
    output_language: outputLanguage,
  });
}
