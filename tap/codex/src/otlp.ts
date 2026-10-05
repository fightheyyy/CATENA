import { createHash } from "node:crypto";

import { canonicalString, type CanonicalEventGraph, type CanonicalNode, type CanonicalTrace } from "./canonical.js";

export const CATENA_RUNTIME_VERSION = "0.2.0";
const MAX_PAYLOAD_BYTES = 8 * 1024 * 1024;

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

export function splitOTLPPayload(payload: OTLPTracePayload): OTLPTracePayload[] {
  const resource = payload.resourceSpans[0];
  const scopes = resource?.scopeSpans as Array<Record<string, unknown>> | undefined;
  const scope = scopes?.[0];
  const spans = scope?.spans as OTLPSpan[] | undefined;
  if (!resource || !scope || !spans) throw new Error("unexpected OTLP trace payload shape");
  const frame = (selected: OTLPSpan[]): OTLPTracePayload => ({
    resourceSpans: [{ ...resource, scopeSpans: [{ ...scope, spans: selected }] }],
  });
  const overhead = Buffer.byteLength(JSON.stringify(frame([])), "utf-8");
  const chunks: OTLPTracePayload[] = [];
  let current: OTLPSpan[] = [];
  let size = overhead;
  for (const original of spans) {
    const span = fitOversizedSpan(original, overhead);
    const spanBytes = Buffer.byteLength(JSON.stringify(span), "utf-8") + 1;
    if (overhead + spanBytes > MAX_PAYLOAD_BYTES) throw new Error("one OTLP span exceeds upload limit after attribute truncation");
    if (current.length && size + spanBytes > MAX_PAYLOAD_BYTES) {
      chunks.push(frame(current));
      current = [];
      size = overhead;
    }
    current.push(span);
    size += spanBytes;
  }
  if (current.length || chunks.length === 0) chunks.push(frame(current));
  return chunks;
}

function fitOversizedSpan(span: OTLPSpan, overhead: number): OTLPSpan {
  const limit = MAX_PAYLOAD_BYTES - overhead - 1;
  if (Buffer.byteLength(JSON.stringify(span), "utf-8") <= limit) return span;
  const original = (span.attributes as OTLPAttribute[] | undefined) ?? [];
  const copied = original.map((attribute) => ({ key: attribute.key, value: { ...attribute.value } }));
  const result: OTLPSpan = { ...span, attributes: copied };
  const metadata = new Map<string, OTLPAttribute[]>();
  const candidates = original
    .map((attribute, index) => ({ attribute, index,
      bytes: "stringValue" in attribute.value ? Buffer.byteLength(attribute.value.stringValue, "utf-8") : 0 }))
    .filter((item) => item.bytes > 1024)
    .sort((left, right) => right.bytes - left.bytes);
  for (const cap of [128 * 1024, 16 * 1024, 1024]) {
    for (const { attribute, index, bytes } of candidates) {
      if (bytes <= cap || !("stringValue" in attribute.value)) continue;
      const current = copied[index].value;
      if (!("stringValue" in current) || Buffer.byteLength(current.stringValue, "utf-8") <= cap) continue;
      const preview = Buffer.from(attribute.value.stringValue).subarray(0, cap).toString("utf-8");
      copied[index].value = { stringValue: `${preview}\n[Catena truncated this attribute for OTLP; original bytes: ${bytes}]` };
      if (!metadata.has(attribute.key)) {
        metadata.set(attribute.key, [
          { key: `catena.truncated.${attribute.key}.original_bytes`, value: { intValue: String(bytes) } },
          { key: `catena.truncated.${attribute.key}.sha256`, value: { stringValue: createHash("sha256").update(attribute.value.stringValue).digest("hex") } },
        ]);
      }
      result.attributes = [...copied, ...Array.from(metadata.values()).flat()];
      if (Buffer.byteLength(JSON.stringify(result), "utf-8") <= limit) return result;
    }
  }
  throw new Error("one OTLP span exceeds upload limit after selective attribute truncation");
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
    let ok = true;
    try {
      for (const payload of splitOTLPPayload(traceToOTLP(graph, trace))) {
        if (!(await sendOTLP(payload, options))) {
          ok = false;
          break;
        }
      }
    } catch (error) {
      if (options.debug) console.error("[catena-runtime] OTLP preparation failed", error);
      ok = false;
    }
    (ok ? uploaded : failed).push(trace.turn_id);
  }
  return { uploaded, failed };
}
