import type { TraceSpan, TraceSummary } from "./types";

export type TraceFilter = "all" | "errors" | "multi";
export type TraceLens = "agent" | "tools" | "errors" | "raw";
export type TraceSpanSemanticKind = "run" | "turn" | "model" | "tool" | "artifact" | "check" | "error" | "internal";

export type TraceEvidenceRole = "user" | "assistant" | "system" | "tool";

export type TraceEvidencePresentation = {
  kind: "messages" | "fields" | "text" | "terminal";
  messages: Array<{ role: TraceEvidenceRole; text: string }>;
  fields: Array<{ key: string; value: string; code: boolean }>;
  text: string;
  hiddenContextCount: number;
  hiddenFieldCount: number;
  structured: boolean;
  truncated: boolean;
};

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

export type TraceSessionGroup = {
  key: string;
  agentID: string;
  sessionID: string;
  traces: TraceSummary[];
  spanCount: number;
  errorCount: number;
  startedAt: string;
  endedAt: string;
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
    return [trace.root_name, trace.service_name, trace.model, trace.trace_id, trace.agent_id, trace.session_id]
      .filter((value): value is string => Boolean(value))
      .some((value) => value.toLocaleLowerCase().includes(needle));
  });
}

export function traceSessionKey(trace: TraceSummary, fallbackAgentID = "") {
  return `${trace.agent_id || fallbackAgentID}\u0000${trace.session_id || "__ungrouped__"}`;
}

export function groupTraceSummariesBySession(traces: TraceSummary[], fallbackAgentID = ""): TraceSessionGroup[] {
  const groups = new Map<string, TraceSessionGroup>();
  for (const trace of traces) {
    const key = traceSessionKey(trace, fallbackAgentID);
    const existing = groups.get(key);
    if (existing) {
      existing.traces.push(trace);
      existing.spanCount += trace.span_count;
      existing.errorCount += trace.error_count;
      if (trace.start_time < existing.startedAt) existing.startedAt = trace.start_time;
      if (trace.end_time > existing.endedAt) existing.endedAt = trace.end_time;
      continue;
    }
    groups.set(key, {
      key,
      agentID: trace.agent_id || fallbackAgentID,
      sessionID: trace.session_id || "",
      traces: [trace],
      spanCount: trace.span_count,
      errorCount: trace.error_count,
      startedAt: trace.start_time,
      endedAt: trace.end_time,
    });
  }
  return [...groups.values()].sort((left, right) => right.endedAt.localeCompare(left.endedAt));
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
    if (typeof value === "number" || typeof value === "boolean") return String(value);
  }
  return "";
}

export function traceSpanSemanticKind(span: TraceSpan): TraceSpanSemanticKind {
  const name = span.name.trim().toLocaleLowerCase();
  if (/^barena[._/]simulation$/.test(name)) return "run";
  if (/^barena[._/]assertion$/.test(name)) return "check";
  if (span.status_code === 2) return "error";

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
  if (/^(?:xiaoba|xiaobaos)[._/]model[._/]call$/.test(name)) return "model";

  if (
    !span.parent_span_id
    || /^barena[._/]turn$/.test(name)
    || /^(turn(?:\/start)?|agent_run|agent[./]run|session(?:[./](?:start|run))?)$/.test(name)
  ) return "turn";

  return "internal";
}

export function buildTraceSemanticView(spans: TraceSpan[]): TraceSemanticView {
  const counts: Record<TraceSpanSemanticKind, number> = {
    run: 0,
    turn: 0,
    model: 0,
    tool: 0,
    artifact: 0,
    check: 0,
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
    if (span.status_code === 2) {
      errorSteps.push(semanticSpan);
      if (kind !== "error") counts.error += 1;
    }
    const turnID = traceSpanAttributeString(
      span,
      "agent.turn.id",
      "turn_id",
      "gen_ai.conversation.id",
      "gen_ai.session.id",
    );
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
    ?? view.agentSteps.find(({ span, kind }) => kind === "turn" && Boolean(span.input || span.output))?.span
    ?? view.toolSteps.find(({ span }) => Boolean(span.input || span.output))?.span
    ?? view.toolSteps[0]?.span
    ?? view.agentSteps.find(({ span }) => Boolean(span.input || span.output))?.span
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

export function presentTraceEvidence(
  value: string,
  kind: TraceSpanSemanticKind,
  direction: "input" | "output",
): TraceEvidencePresentation {
  const embeddedToolArguments = kind === "tool" || kind === "artifact"
    ? parseEmbeddedToolArguments(value)
    : undefined;
  const parsed = embeddedToolArguments ?? parseTraceJSON(value);
  const structured = parsed !== undefined;

  if (kind === "model" && direction === "input" && !structured) {
    const partial = partialModelRequest(value);
    if (partial) {
      return presentation({
        kind: partial.messages.length > 0 ? "messages" : "fields",
        messages: partial.messages,
        fields: partial.model ? [{ key: "model", value: partial.model, code: false }] : [],
        hiddenContextCount: partial.hiddenContextCount,
        structured: true,
        truncated: true,
      });
    }
  }

  if ((kind === "model" || kind === "turn") && direction === "input" && structured) {
    const request = modelRequestMessages(parsed);
    if (request.messages.length > 0) {
      return presentation({
        kind: "messages",
        messages: request.messages,
        hiddenContextCount: request.hiddenContextCount,
        structured,
      });
    }
  }

  if (structured && (kind === "tool" || kind === "artifact")) {
    if (direction === "output") {
      const result = contentText(parsed);
      if (result) return presentation({ kind: "terminal", text: result, structured });
    }
    const summarized = evidenceFields(parsed);
    if (summarized.fields.length > 0) {
      return presentation({
        kind: "fields",
        fields: summarized.fields,
        hiddenFieldCount: summarized.hiddenCount,
        structured,
      });
    }
  }

  if (structured && direction === "output") {
    const response = responseText(parsed);
    if (response) {
      return presentation({
        kind: kind === "model" || kind === "turn" ? "messages" : "text",
        messages: kind === "model" || kind === "turn" ? [{ role: "assistant", text: response }] : [],
        text: kind === "model" || kind === "turn" ? "" : response,
        structured,
      });
    }
  }

  if (structured) {
    const summarized = evidenceFields(parsed);
    if (summarized.fields.length > 0) {
      return presentation({
        kind: "fields",
        fields: summarized.fields,
        hiddenFieldCount: summarized.hiddenCount,
        structured,
      });
    }
  }

  const text = typeof parsed === "string" ? parsed : value;
  if (kind === "turn" || kind === "model") {
    return presentation({
      kind: "messages",
      messages: [{ role: direction === "input" ? "user" : "assistant", text }],
      structured,
    });
  }
  return presentation({
    kind: kind === "tool" || kind === "artifact" ? "terminal" : "text",
    text,
    structured,
  });
}

function presentation(
  partial: Partial<TraceEvidencePresentation> & Pick<TraceEvidencePresentation, "kind">,
): TraceEvidencePresentation {
  return {
    kind: partial.kind,
    messages: partial.messages ?? [],
    fields: partial.fields ?? [],
    text: partial.text ?? "",
    hiddenContextCount: partial.hiddenContextCount ?? 0,
    hiddenFieldCount: partial.hiddenFieldCount ?? 0,
    structured: partial.structured ?? false,
    truncated: partial.truncated ?? false,
  };
}

function partialModelRequest(value: string) {
  const trimmed = value.trimStart();
  if (!trimmed.startsWith("{") || !/"input"\s*:/.test(trimmed)) return undefined;

  const inputKey = trimmed.search(/"input"\s*:/);
  const inputStart = trimmed.indexOf("[", inputKey);
  if (inputStart < 0) return undefined;

  const items: unknown[] = [];
  let arrayDepth = 1;
  let objectDepth = 0;
  let objectStart = -1;
  let inString = false;
  let escaped = false;
  for (let index = inputStart + 1; index < trimmed.length; index += 1) {
    const character = trimmed[index];
    if (inString) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === '"') inString = false;
      continue;
    }
    if (character === '"') {
      inString = true;
      continue;
    }
    if (character === "[") arrayDepth += 1;
    else if (character === "]") arrayDepth -= 1;
    else if (character === "{") {
      if (arrayDepth === 1 && objectDepth === 0) objectStart = index;
      objectDepth += 1;
    } else if (character === "}") {
      objectDepth -= 1;
      if (objectDepth === 0 && objectStart >= 0) {
        try {
          items.push(JSON.parse(trimmed.slice(objectStart, index + 1)));
        } catch {
          // A complete-looking item can still contain exporter corruption.
        }
        objectStart = -1;
      }
    }
  }

  const request = modelRequestMessages({ input: items });
  const modelMatch = trimmed.match(/"model"\s*:\s*"((?:\\.|[^"\\])*)"/);
  let model = "";
  if (modelMatch) {
    try {
      model = JSON.parse(`"${modelMatch[1]}"`);
    } catch {
      model = modelMatch[1];
    }
  }
  return {
    model,
    messages: request.messages,
    hiddenContextCount: request.hiddenContextCount + (objectStart >= 0 ? 1 : 0),
  };
}

function parseTraceJSON(value: string): unknown | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  try {
    let parsed: unknown = JSON.parse(trimmed);
    if (typeof parsed === "string") {
      try {
        parsed = JSON.parse(parsed);
      } catch {
        return parsed;
      }
    }
    return parsed;
  } catch {
    return undefined;
  }
}

function parseEmbeddedToolArguments(value: string): unknown | undefined {
  for (const marker of ["tools.exec_command(", "tools.write_stdin("]) {
    const start = value.indexOf(marker);
    if (start < 0) continue;
    const payloadStart = start + marker.length;
    const payloadEnd = value.indexOf(");", payloadStart);
    if (payloadEnd < 0) continue;
    try {
      return JSON.parse(value.slice(payloadStart, payloadEnd));
    } catch {
      continue;
    }
  }
  return parseTraceJSON(value);
}

function modelRequestMessages(value: unknown) {
  if (!isRecord(value)) return { messages: [], hiddenContextCount: 0 };
  const source = value.type === "chat_messages"
    ? value.value
    : value.input ?? value.messages;
  if (typeof source === "string") {
    return { messages: [{ role: "user" as const, text: source }], hiddenContextCount: 0 };
  }
  if (!Array.isArray(source)) return { messages: [], hiddenContextCount: 0 };

  const messages: Array<{ role: TraceEvidenceRole; text: string }> = [];
  let hiddenContextCount = typeof value.instructions === "string" && value.instructions.trim() ? 1 : 0;
  for (const item of source) {
    if (typeof item === "string") {
      messages.push({ role: "user", text: item });
      continue;
    }
    if (!isRecord(item)) {
      hiddenContextCount += 1;
      continue;
    }
    const role = normalizeEvidenceRole(item.role);
    const type = typeof item.type === "string" ? item.type : "";
    const text = contentText(item.content ?? item.text ?? item.output);
    if (
      role === "system"
      || type === "additional_tools"
      || type.includes("tool_call")
      || isInjectedContext(text)
    ) {
      hiddenContextCount += 1;
      continue;
    }
    if (text) messages.push({ role, text });
    else hiddenContextCount += 1;
  }
  return {
    messages: messages.slice(-6),
    hiddenContextCount: hiddenContextCount + Math.max(0, messages.length - 6),
  };
}

function normalizeEvidenceRole(value: unknown): TraceEvidenceRole {
  if (value === "assistant") return "assistant";
  if (value === "tool") return "tool";
  if (value === "system" || value === "developer") return "system";
  return "user";
}

function isInjectedContext(value: string) {
  const trimmed = value.trimStart();
  return [
    "<recommended_plugins>",
    "<environment_context>",
    "<in-app-browser-context",
    "<skills_instructions>",
    "<multi_agent_mode>",
  ].some((prefix) => trimmed.startsWith(prefix));
}

function responseText(value: unknown): string {
  if (typeof value === "string") return value;
  if (!isRecord(value)) return contentText(value);
  return contentText(value.output_text ?? value.output ?? value.content ?? value.message);
}

function contentText(value: unknown): string {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) {
    return value.map(contentText).filter(Boolean).join("");
  }
  if (!isRecord(value)) return "";
  if (typeof value.text === "string") return value.text;
  if (typeof value.output_text === "string") return value.output_text;
  if (value.content !== undefined) return contentText(value.content);
  if (value.output !== undefined) return contentText(value.output);
  return "";
}

function evidenceFields(value: unknown) {
  if (!isRecord(value)) {
    if (Array.isArray(value)) {
      return {
        fields: [{ key: "items", value: `${value.length}`, code: false }],
        hiddenCount: 0,
      };
    }
    return { fields: [], hiddenCount: 0 };
  }
  const preferredKeys = ["cmd", "command", "workdir", "path", "query", "url", "method"];
  const entries = Object.entries(value).sort(([left], [right]) => {
    const leftOrder = preferredKeys.indexOf(left);
    const rightOrder = preferredKeys.indexOf(right);
    if (leftOrder >= 0 || rightOrder >= 0) {
      return (leftOrder >= 0 ? leftOrder : preferredKeys.length) - (rightOrder >= 0 ? rightOrder : preferredKeys.length);
    }
    return left.localeCompare(right);
  });
  const fields = entries.slice(0, 10).map(([key, item]) => ({
    key,
    value: summarizeFieldValue(item),
    code: typeof item === "string" && (key === "cmd" || key === "command" || key === "path" || key === "workdir"),
  }));
  return { fields, hiddenCount: Math.max(0, entries.length - fields.length) };
}

function summarizeFieldValue(value: unknown): string {
  if (typeof value === "string") return truncateEvidenceText(value, 4_000);
  if (typeof value === "number" || typeof value === "boolean" || value === null) return String(value);
  if (Array.isArray(value)) return `${value.length} items`;
  if (isRecord(value)) return `${Object.keys(value).length} fields`;
  return String(value ?? "");
}

function truncateEvidenceText(value: string, limit: number) {
  if (value.length <= limit) return value;
  return `${value.slice(0, limit)}\n…`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function shortTraceID(traceID: string) {
  if (traceID.length <= 18) return traceID;
  return `${traceID.slice(0, 8)}…${traceID.slice(-6)}`;
}
