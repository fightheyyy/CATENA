import type { TraceSpan, TraceSummary } from "./types";

export type TraceFilter = "all" | "errors" | "multi";
export type TraceLens = "agent" | "tools" | "errors" | "raw";
export type TraceSpanSemanticKind = "turn" | "model" | "tool" | "artifact" | "error" | "internal";

export type SemanticTraceSpan = {
  span: TraceSpan;
  kind: TraceSpanSemanticKind;
};

export type TraceSemanticView = {
  agentSteps: SemanticTraceSpan[];
  toolSteps: SemanticTraceSpan[];
  errorSteps: SemanticTraceSpan[];
  rawSteps: SemanticTraceSpan[];
  counts: Record<TraceSpanSemanticKind, number>;
  turnCount: number;
  foldedInternalCount: number;
};

export const TRACE_STEP_PAGE_SIZE = 200;

export function tracesForAgentSelection(
  workspaceTraces: TraceSummary[],
  agentID: string,
  agentTraces: TraceSummary[],
) {
  // The Agent endpoint resolves canonical identity to every underlying
  // telemetry source. Keep that server-owned result intact: a canonical ID
  // such as `codex` intentionally does not equal either raw service.name.
  return agentID ? agentTraces : workspaceTraces;
}

export function filterTraceSummaries(
  traces: TraceSummary[],
  query: string,
  filter: TraceFilter,
) {
  const needle = query.trim().toLocaleLowerCase();
  return traces.filter((trace) => {
    if (filter === "errors" && trace.error_count === 0) return false;
    if (filter === "multi" && trace.span_count <= 1) return false;
    if (!needle) return true;
    return [trace.root_name, trace.service_name, trace.model, trace.trace_id]
      .filter((value): value is string => Boolean(value))
      .some((value) => value.toLocaleLowerCase().includes(needle));
  });
}

export function traceSpanDepth(span: TraceSpan, spansByID: Map<string, TraceSpan>) {
  let depth = 0;
  let parentID = span.parent_span_id;
  const visited = new Set<string>();
  while (parentID && depth < 8 && !visited.has(parentID)) {
    visited.add(parentID);
    const parent = spansByID.get(parentID);
    if (!parent) break;
    depth += 1;
    parentID = parent.parent_span_id;
  }
  return depth;
}

export function traceSpanToolName(span: TraceSpan) {
  for (const key of ["tool.name", "gen_ai.tool.name", "tool.call.name", "xiaoba.tool.name"]) {
    const value = span.attributes[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

export function traceSpanAttributeString(span: TraceSpan, ...keys: string[]) {
  for (const key of keys) {
    const value = span.attributes[key] ?? span.resource_attributes[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

export function traceSpanSemanticKind(span: TraceSpan): TraceSpanSemanticKind {
  if (span.status_code === 2) return "error";

  const name = span.name.trim().toLocaleLowerCase();
  const toolName = traceSpanToolName(span).toLocaleLowerCase();
  const artifactHint = traceSpanAttributeString(
    span,
    "artifact.path",
    "file.path",
    "gen_ai.output.file",
  );
  if (
    artifactHint
    || /(^|[._/ -])(artifact|apply_patch|write_file|create_file|send_file|upload_file|download_file)([._/ -]|$)/.test(toolName || name)
  ) return "artifact";

  if (
    toolName
    || /(^|[._/])(handle_tool_call|dispatch_tool_call|execute_tool|invoke_tool|tool_call)([._/]|$)/.test(name)
  ) return "tool";

  const model = span.model || traceSpanAttributeString(
    span,
    "gen_ai.request.model",
    "gen_ai.response.model",
    "llm.request.model",
    "model",
  );
  if (
    model
    && /(^|[._/])(run_sampling_request|try_run_sampling_request|stream_responses|responses_websocket|chat|completion|generate|inference|llm|model_client)([._/]|$)/.test(name)
  ) return "model";
  if (/^(chat|llm|gen_ai|model)[._/ -]/.test(name)) return "model";

  if (
    !span.parent_span_id
    || /^(turn(?:\/start)?|agent_run|agent[./]run|session(?:[./](?:start|run))?)$/.test(name)
  ) return "turn";

  return "internal";
}

export function buildTraceSemanticView(spans: TraceSpan[]): TraceSemanticView {
  const counts: Record<TraceSpanSemanticKind, number> = {
    turn: 0,
    model: 0,
    tool: 0,
    artifact: 0,
    error: 0,
    internal: 0,
  };
  const rawSteps: SemanticTraceSpan[] = [];
  const agentSteps: SemanticTraceSpan[] = [];
  const toolSteps: SemanticTraceSpan[] = [];
  const errorSteps: SemanticTraceSpan[] = [];
  const turnIDs = new Set<string>();

  for (const span of spans) {
    const kind = traceSpanSemanticKind(span);
    const semanticSpan = { span, kind };
    rawSteps.push(semanticSpan);
    counts[kind] += 1;
    if (kind !== "internal") agentSteps.push(semanticSpan);
    if (kind === "tool" || kind === "artifact") toolSteps.push(semanticSpan);
    if (kind === "error") errorSteps.push(semanticSpan);
    const turnID = traceSpanAttributeString(span, "turn_id", "gen_ai.conversation.id", "gen_ai.session.id");
    if (turnID) turnIDs.add(turnID);
  }

  return {
    agentSteps,
    toolSteps,
    errorSteps,
    rawSteps,
    counts,
    turnCount: turnIDs.size || counts.turn,
    foldedInternalCount: counts.internal,
  };
}

export function traceStepsForLens(view: TraceSemanticView, lens: TraceLens) {
  switch (lens) {
    case "tools": return view.toolSteps;
    case "errors": return view.errorSteps;
    case "raw": return view.rawSteps;
    default: return view.agentSteps;
  }
}

export function boundedTraceSteps(view: TraceSemanticView, lens: TraceLens, limit: number) {
  const allSteps = traceStepsForLens(view, lens);
  const safeLimit = Math.max(0, limit);
  return {
    steps: allSteps.slice(0, safeLimit),
    hiddenCount: Math.max(0, allSteps.length - safeLimit),
    totalCount: allSteps.length,
  };
}

export function preferredTraceSpan(view: TraceSemanticView) {
  return view.errorSteps[0]?.span
    ?? view.toolSteps.find(({ span }) => Boolean(span.input || span.output))?.span
    ?? view.toolSteps[0]?.span
    ?? view.agentSteps.find(({ kind }) => kind === "model")?.span
    ?? view.agentSteps[0]?.span
    ?? view.rawSteps[0]?.span;
}

export function formatTraceEvidence(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return value;
  try {
    let parsed: unknown = JSON.parse(trimmed);
    if (typeof parsed === "string") {
      try {
        parsed = JSON.parse(parsed);
      } catch {
        return typeof parsed === "string" ? parsed : trimmed;
      }
    }
    return typeof parsed === "string" ? parsed : JSON.stringify(parsed, null, 2);
  } catch {
    return value;
  }
}

export function shortTraceID(traceID: string) {
  if (traceID.length <= 18) return traceID;
  return `${traceID.slice(0, 8)}…${traceID.slice(-6)}`;
}
