import { createHash } from "node:crypto";

import { canonicalString, type CanonicalEventGraph, type CanonicalNode, type CanonicalTrace } from "./canonical.js";

export const CATENA_RUNTIME_VERSION = "0.2.0";

type AnyValue =
  | { stringValue: string }
  | { boolValue: boolean }
  | { intValue: string }
  | { doubleValue: number };

type OTLPAttribute = { key: string; value: AnyValue };
type OTLPSpan = Record<string, unknown>;
export type OTLPTracePayload = { resourceSpans: Array<Record<string, unknown>> };

export type ExportOptions = {
  endpoint: string;
  apiKey: string;
  timeoutMs?: number;
  attempts?: number;
  debug?: boolean;
};

function stableSpanId(runtime: string, sessionId: string, traceId: string, key: string): string {
  return createHash("sha256")
    .update(`catena:${runtime}:${sessionId}:${traceId}:${key}`)
    .digest("hex")
    .slice(0, 16);
}

function protoBytes(hex: string): string {
  return Buffer.from(hex, "hex").toString("base64");
}

function anyValue(value: unknown): AnyValue {
  if (typeof value === "boolean") return { boolValue: value };
  if (typeof value === "number" && Number.isSafeInteger(value)) return { intValue: String(value) };
  if (typeof value === "number") return { doubleValue: value };
  return { stringValue: typeof value === "string" ? value : canonicalString(value) };
}

function attributes(values: Record<string, unknown>): OTLPAttribute[] {
  return Object.entries(values)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => ({ key, value: anyValue(value) }));
}

function nodeAttributes(graph: CanonicalEventGraph, trace: CanonicalTrace, node: CanonicalNode) {
  const values: Record<string, unknown> = {
    "agent.runtime": graph.runtime,
    "agent.session.id": graph.session_id,
    "agent.turn.id": trace.turn_id,
    "catena.canonical.schema": graph.schema_version,
    "catena.node.key": node.key,
    "catena.node.kind": node.kind,
    "catena.state": node.state,
    "catena.source.event.ids": JSON.stringify(node.source_event_ids),
    ...node.attributes,
  };
  if (node.runtime_id) values["catena.runtime.id"] = node.runtime_id;
  if (node.input !== undefined) values["input.value"] = canonicalString(node.input);
  if (node.output !== undefined) values["output.value"] = canonicalString(node.output);
  if (node.model) {
    values["gen_ai.request.model"] = node.model;
    values["gen_ai.response.model"] = node.model;
  }
  if (node.kind === "tool" || node.kind === "unmatched_tool_result") {
    const callId = node.attributes["gen_ai.tool.call.id"] ?? node.runtime_id;
    if (callId) values["gen_ai.tool.call.id"] = callId;
    if (node.input !== undefined) {
      values["gen_ai.tool.call.arguments"] = canonicalString(node.input);
      values["tool.call.arguments"] = canonicalString(node.input);
    }
    if (node.output !== undefined) {
      values["gen_ai.tool.call.result"] = canonicalString(node.output);
      values["tool.call.result"] = canonicalString(node.output);
    }
  }
  for (const [key, value] of Object.entries(node.usage ?? {})) {
    values[`gen_ai.usage.${key}`] = value;
  }
  return values;
}

function spanKind(node: CanonicalNode): number {
  return node.kind === "model" || node.kind === "retry" ? 3 : 1;
}

export function traceToOTLP(graph: CanonicalEventGraph, trace: CanonicalTrace): OTLPTracePayload {
  const spanIds = new Map<string, string>();
  for (const node of trace.nodes) {
    if (spanIds.has(node.key)) throw new Error(`duplicate canonical node key ${node.key}`);
    spanIds.set(node.key, stableSpanId(graph.runtime, graph.session_id, trace.trace_id, node.key));
  }
  const spans: OTLPSpan[] = trace.nodes.map((node) => {
    const parentSpanId = node.parent_key ? spanIds.get(node.parent_key) : undefined;
    if (node.parent_key && !parentSpanId) {
      throw new Error(`canonical parent ${node.parent_key} is missing for ${node.key}`);
    }
    const value: OTLPSpan = {
      traceId: protoBytes(trace.trace_id),
      spanId: protoBytes(spanIds.get(node.key)!),
      name: node.name,
      kind: spanKind(node),
      startTimeUnixNano: node.start_time_unix_nano,
      endTimeUnixNano: node.end_time_unix_nano,
      attributes: attributes(nodeAttributes(graph, trace, node)),
      status: {
        code: node.state === "ok" ? 1 : 2,
        ...(node.status_message ? { message: node.status_message } : {}),
      },
    };
    if (parentSpanId) value.parentSpanId = protoBytes(parentSpanId);
    return value;
  });
  return {
    resourceSpans: [
      {
        resource: {
          attributes: attributes({
            "service.name": `catena-runtime-${graph.runtime}`,
            "agent.runtime": graph.runtime,
            "agent.session.id": graph.session_id,
            "telemetry.sdk.name": "catena-runtime",
            "telemetry.sdk.language": graph.runtime === "codex" ? "typescript" : "python",
            "telemetry.sdk.version": CATENA_RUNTIME_VERSION,
          }),
        },
        scopeSpans: [
          {
            scope: { name: "catena.runtime", version: CATENA_RUNTIME_VERSION },
            spans,
          },
        ],
      },
    ],
  };
}

export function endpointFromEnvironment(environment = process.env): string {
  const explicit = environment.CATENA_OTLP_ENDPOINT?.trim();
  if (explicit) return explicit;
  const base = environment.CATENA_URL?.trim() || "http://127.0.0.1:5570";
  return `${base.replace(/\/$/, "")}/v1/otlp/v1/traces`;
}

function transientStatus(status: number): boolean {
  return status === 408 || status === 425 || status === 429 || status >= 500;
}

async function delay(milliseconds: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, milliseconds));
}

export async function sendOTLP(payload: OTLPTracePayload, options: ExportOptions): Promise<boolean> {
  const attempts = Math.max(1, options.attempts ?? 3);
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const response = await fetch(options.endpoint, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${options.apiKey}`,
          "Content-Type": "application/json",
          "User-Agent": `catena-runtime/${CATENA_RUNTIME_VERSION}`,
        },
        body: JSON.stringify(payload),
        signal: AbortSignal.timeout(options.timeoutMs ?? 4_000),
      });
      if (response.ok) return true;
      if (!transientStatus(response.status) || attempt === attempts) {
        if (options.debug) console.error(`[catena-runtime] OTLP HTTP ${response.status}`);
        return false;
      }
    } catch (error) {
      if (attempt === attempts) {
        if (options.debug) console.error("[catena-runtime] OTLP upload failed", error);
        return false;
      }
    }
    await delay(100 * attempt);
  }
  return false;
}

export async function exportGraph(
  graph: CanonicalEventGraph,
  options: ExportOptions,
  traces: CanonicalTrace[] = graph.traces,
): Promise<{ uploaded: string[]; failed: string[] }> {
  const uploaded: string[] = [];
  const failed: string[] = [];
  for (const trace of traces) {
    const ok = await sendOTLP(traceToOTLP(graph, trace), options);
    (ok ? uploaded : failed).push(trace.turn_id);
  }
  return { uploaded, failed };
}
