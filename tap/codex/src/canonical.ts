export const CANONICAL_SCHEMA_VERSION = "catena.coding_agent.event_graph.v1" as const;

export type RuntimeKind = "codex" | "claude-code";
export type CanonicalState = "ok" | "error" | "retry" | "aborted" | "incomplete";
export type CanonicalNodeKind =
  | "turn"
  | "model"
  | "tool"
  | "subagent"
  | "context_compact"
  | "retry"
  | "unmatched_tool_result";

export type CanonicalSource = {
  format: "codex-rollout-jsonl" | "claude-transcript-jsonl";
  path: string;
  parser: string;
  upstream_commit: string;
};

export type CanonicalNode = {
  key: string;
  parent_key?: string;
  kind: CanonicalNodeKind;
  name: string;
  runtime_id?: string;
  start_time_unix_nano: string;
  end_time_unix_nano: string;
  state: CanonicalState;
  status_message?: string;
  input?: unknown;
  output?: unknown;
  model?: string;
  usage?: Record<string, number>;
  attributes: Record<string, unknown>;
  source_event_ids: string[];
};

export type SourceAccounting = {
  event_id: string;
  disposition: "span" | "ignored";
  node_key?: string;
  reason?: string;
};

export type CanonicalTrace = {
  trace_id: string;
  turn_id: string;
  state: CanonicalState;
  nodes: CanonicalNode[];
  accounting: SourceAccounting[];
};

export type CanonicalEventGraph = {
  schema_version: typeof CANONICAL_SCHEMA_VERSION;
  runtime: RuntimeKind;
  session_id: string;
  source: CanonicalSource;
  traces: CanonicalTrace[];
};

export function millisecondsToNanoseconds(value: number): string {
  const finite = Number.isFinite(value) ? Math.max(1, Math.round(value)) : 1;
  return (BigInt(finite) * 1_000_000n).toString();
}

export function canonicalString(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}
