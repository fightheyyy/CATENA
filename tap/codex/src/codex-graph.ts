import { createHash } from "node:crypto";
import * as fs from "node:fs/promises";
import * as path from "node:path";

import {
  CANONICAL_SCHEMA_VERSION,
  canonicalString,
  millisecondsToNanoseconds,
  type CanonicalEventGraph,
  type CanonicalNode,
  type CanonicalState,
  type CanonicalTrace,
  type SourceAccounting,
} from "./canonical.js";
import { parseSession } from "./langfuse-derived/parse.js";
import type {
  ModelStep,
  RolloutLine,
  SessionMeta,
  ToolCall,
  Turn,
} from "./langfuse-derived/types.js";

export const CODEX_UPSTREAM_COMMIT = "7500867afecf963d1cf83bf2b860a659591ace18";
export const CODEX_PARSER_NAME = "langfuse-codex-derived@7500867";

type SourceRecord = { eventId: string; type: string };
type RolloutDocument = {
  file: string;
  lines: RolloutLine[];
  records: SourceRecord[];
};

type TraceBuild = {
  trace: CanonicalTrace;
  sourceRecords: SourceRecord[];
};

function isTraceId(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{32}$/.test(value) && !/^0+$/.test(value);
}

function traceIdFromRuntimeCorrelation(sessionId: string, turn: Turn): string {
  if (isTraceId(turn.traceId)) return turn.traceId;
  return createHash("sha256")
    .update(`catena:codex:${sessionId}:${turn.turnId}`)
    .digest("hex")
    .slice(0, 32);
}

function unique(values: Array<string | undefined>): string[] {
  return [...new Set(values.filter((value): value is string => typeof value === "string" && value.length > 0))];
}

function safeEnd(start: number, end: number | undefined): number {
  return Math.max(start, end ?? start);
}

function toolType(tool: ToolCall): string {
  if (tool.mcp) return "mcp";
  const name = tool.name.toLowerCase();
  if (tool.type !== "function") return tool.type;
  if (name.includes("web_search")) return "web_search";
  if (name.includes("file_search")) return "file_search";
  if (name.includes("computer")) return "computer";
  if (name.includes("shell") || name.includes("exec_command")) return "local_shell";
  return "function";
}

function toolName(tool: ToolCall): string {
  return tool.mcp ? `${tool.mcp.server}.${tool.mcp.tool}` : tool.name || "tool";
}

function usage(step: ModelStep): Record<string, number> | undefined {
  if (!step.usage) return undefined;
  const values: Record<string, number> = {};
  for (const [key, value] of Object.entries(step.usage)) {
    if (typeof value === "number" && Number.isSafeInteger(value) && value >= 0) values[key] = value;
  }
  return Object.keys(values).length > 0 ? values : undefined;
}

function stepOutput(step: ModelStep): Record<string, unknown> | undefined {
  const value: Record<string, unknown> = {};
  if (step.text) value.content = step.text;
  if (step.reasoning) value.reasoning = step.reasoning;
  if (step.toolCalls.length > 0) {
    value.tool_calls = step.toolCalls.map((tool) => ({
      id: tool.callId || undefined,
      name: toolName(tool),
      type: toolType(tool),
      arguments: tool.args,
    }));
  }
  return Object.keys(value).length > 0 ? value : undefined;
}

function toolResultInput(tools: ToolCall[]): Array<Record<string, unknown>> | undefined {
  if (tools.length === 0) return undefined;
  return tools.map((tool) => ({
    call_id: tool.callId || undefined,
    name: toolName(tool),
    output: tool.output,
    error: tool.error,
  }));
}

function appendModelAndToolNodes(
  nodes: CanonicalNode[],
  turn: Turn,
  parentKey: string,
  keyPrefix: string,
  modelName: string,
): { toolKeys: Map<string, string>; failed: boolean; incomplete: boolean } {
  const toolKeys = new Map<string, string>();
  let previousTools: ToolCall[] = [];
  let failed = false;
  let incomplete = false;

  turn.steps.forEach((step, stepIndex) => {
    const modelIdentity = step.modelCallId || `step:${stepIndex + 1}`;
    const modelKey = `${keyPrefix}:model:${modelIdentity}`;
    const modelState: CanonicalState = step.state ?? "ok";
    if (modelState === "error") failed = true;
    const modelNode: CanonicalNode = {
      key: modelKey,
      parent_key: parentKey,
      kind: step.state === "retry" ? "retry" : "model",
      name: step.state === "retry" ? "gen_ai.model.retry" : "gen_ai.model.call",
      ...(step.modelCallId ? { runtime_id: step.modelCallId } : {}),
      start_time_unix_nano: millisecondsToNanoseconds(step.startTime),
      end_time_unix_nano: millisecondsToNanoseconds(safeEnd(step.startTime, step.endTime)),
      state: modelState,
      ...(step.statusMessage ? { status_message: step.statusMessage } : {}),
      input: stepIndex === 0 ? turn.userInput : toolResultInput(previousTools),
      output: stepOutput(step),
      model: modelName,
      ...(usage(step) ? { usage: usage(step) } : {}),
      attributes: { "catena.model.step.index": stepIndex },
      source_event_ids: unique(step.sourceEventIds),
    };
    nodes.push(modelNode);

    step.toolCalls.forEach((tool, toolIndex) => {
      const identity = tool.callId || tool.responseItemId || tool.sourceEventIds[0] || `index:${toolIndex}`;
      const key = `${keyPrefix}:tool:${identity}`;
      if (tool.callId) toolKeys.set(tool.callId, key);
      const missingResult = tool.endTime == null;
      const state: CanonicalState = tool.error ? "error" : missingResult ? "incomplete" : "ok";
      if (tool.error) failed = true;
      if (missingResult) incomplete = true;
      nodes.push({
        key,
        parent_key: modelKey,
        kind: "tool",
        name: `agent.tool.call ${toolName(tool)}`,
        ...(tool.callId ? { runtime_id: tool.callId } : {}),
        start_time_unix_nano: millisecondsToNanoseconds(tool.startTime),
        end_time_unix_nano: millisecondsToNanoseconds(safeEnd(tool.startTime, tool.endTime ?? step.endTime)),
        state,
        ...(tool.error
          ? { status_message: tool.error }
          : missingResult
            ? { status_message: "tool result not present in rollout" }
            : {}),
        input: tool.args,
        output: tool.output,
        attributes: {
          "gen_ai.tool.type": toolType(tool),
          "gen_ai.tool.name": toolName(tool),
          ...(tool.callId ? { "gen_ai.tool.call.id": tool.callId } : {}),
          ...(tool.responseItemId ? { "codex.response_item.id": tool.responseItemId } : {}),
          ...(tool.mcp
            ? { "mcp.server": tool.mcp.server, "mcp.tool": tool.mcp.tool }
            : {}),
        },
        source_event_ids: unique(tool.sourceEventIds),
      });
    });
    previousTools = step.toolCalls;
  });

  return { toolKeys, failed, incomplete };
}

async function readRollout(file: string, prefix = "main"): Promise<RolloutDocument> {
  const raw = await fs.readFile(file, "utf-8");
  const lines: RolloutLine[] = [];
  const records: SourceRecord[] = [];
  raw.split("\n").forEach((text, index) => {
    if (!text.trim()) return;
    const eventId = `${prefix}:line:${index + 1}`;
    try {
      const value = JSON.parse(text) as RolloutLine;
      if (!value || typeof value !== "object") {
        records.push({ eventId, type: "malformed:not-object" });
        return;
      }
      value.sourceEventId = eventId;
      lines.push(value);
      const payloadType =
        value.payload && typeof value.payload === "object" && "type" in value.payload
          ? String((value.payload as { type?: unknown }).type ?? "")
          : "";
      records.push({ eventId, type: payloadType ? `${value.type}:${payloadType}` : value.type });
    } catch {
      records.push({ eventId, type: "malformed:json" });
    }
  });
  return { file, lines, records };
}

async function findSubagentRollout(parentFile: string, threadId: string): Promise<string | undefined> {
  const suffix = `-${threadId}.jsonl`;
  const root = path.resolve(path.dirname(parentFile), "../../..");
  async function walk(directory: string): Promise<string | undefined> {
    let entries;
    try {
      entries = await fs.readdir(directory, { withFileTypes: true });
    } catch {
      return undefined;
    }
    for (const entry of entries) {
      const full = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        const nested = await walk(full);
        if (nested) return nested;
      } else if (entry.isFile() && entry.name.endsWith(suffix)) {
        return full;
      }
    }
    return undefined;
  }
  return walk(root);
}

async function appendSubagent(
  build: TraceBuild,
  parentRollout: string,
  link: Turn["subagents"][number],
  parentTurn: Turn,
  parentKey: string,
  toolKeys: Map<string, string>,
  depth: number,
): Promise<void> {
  const threadKey = `${parentKey}:subagent-thread:${link.threadId}`;
  if (link.callId && !toolKeys.has(link.callId)) {
    throw new Error(`subagent ${link.threadId} references unknown spawning call_id ${link.callId}`);
  }
  const parent = link.callId ? toolKeys.get(link.callId)! : parentKey;
  const childFile = await findSubagentRollout(parentRollout, link.threadId);
  const threadNode: CanonicalNode = {
    key: threadKey,
    parent_key: parent,
    kind: "subagent",
    name: "agent.subagent.thread",
    runtime_id: link.threadId,
    start_time_unix_nano: millisecondsToNanoseconds(parentTurn.startTime),
    end_time_unix_nano: millisecondsToNanoseconds(parentTurn.endTime),
    state: childFile ? "ok" : "incomplete",
    ...(!childFile ? { status_message: "subagent rollout not found" } : {}),
    attributes: {
      "agent.subagent.thread.id": link.threadId,
      ...(link.callId ? { "agent.subagent.spawn.call_id": link.callId } : {}),
    },
    source_event_ids: unique(link.sourceEventIds),
  };
  build.trace.nodes.push(threadNode);
  if (!childFile || depth >= 8) return;

  const document = await readRollout(childFile, `subagent:${link.threadId}`);
  build.sourceRecords.push(...document.records);
  const parsed = parseSession(document.lines);
  if (parsed.turns.length > 0) {
    threadNode.start_time_unix_nano = millisecondsToNanoseconds(
      Math.min(...parsed.turns.map((childTurn) => childTurn.startTime)),
    );
    threadNode.end_time_unix_nano = millisecondsToNanoseconds(
      Math.max(...parsed.turns.map((childTurn) => childTurn.endTime)),
    );
    const states = parsed.turns.map(turnState);
    threadNode.state = states.includes("aborted")
      ? "aborted"
      : states.includes("error")
        ? "error"
        : states.includes("incomplete")
          ? "incomplete"
          : "ok";
  }
  for (const [turnIndex, childTurn] of parsed.turns.entries()) {
    if (!childTurn.turnId) {
      throw new Error(`Codex subagent rollout ${childFile} turn ${turnIndex} has no Runtime turn_id`);
    }
    const childTurnId = childTurn.turnId;
    const childKey = `${threadKey}:turn:${childTurnId}`;
    const childState = turnState(childTurn);
    build.trace.nodes.push({
      key: childKey,
      parent_key: threadKey,
      kind: "subagent",
      name: "agent.subagent.turn",
      ...(childTurn.turnId ? { runtime_id: childTurn.turnId } : {}),
      start_time_unix_nano: millisecondsToNanoseconds(childTurn.startTime),
      end_time_unix_nano: millisecondsToNanoseconds(childTurn.endTime),
      state: childState,
      ...(childTurn.aborted ? { status_message: "subagent turn aborted" } : {}),
      input: childTurn.userInput,
      output: childTurn.finalOutput,
      attributes: {
        "agent.subagent.thread.id": parsed.sessionMeta.sessionId,
        ...(childTurn.turnId ? { "agent.turn.id": childTurn.turnId } : {}),
        ...(childTurn.traceId ? { "codex.trace_id": childTurn.traceId } : {}),
        "catena.trace.correlation": `${parsed.sessionMeta.sessionId}:${childTurnId}`,
      },
      source_event_ids: unique([
        ...parsed.sessionMeta.sourceEventIds,
        ...childTurn.sourceEventIds,
      ]),
    });
    const children = appendModelAndToolNodes(
      build.trace.nodes,
      childTurn,
      childKey,
      childKey,
      childTurn.model ?? "unknown",
    );
    for (const nested of childTurn.subagents) {
      await appendSubagent(
        build,
        childFile,
        nested,
        childTurn,
        childKey,
        children.toolKeys,
        depth + 1,
      );
    }
  }
}

function mapMainEventsToTurns(lines: RolloutLine[], turns: Turn[]): Map<string, string> {
  const result = new Map<string, string>();
  let current = turns[0]?.turnId ?? "";
  let turnIndex = 0;
  for (const line of lines) {
    const eventId = line.sourceEventId;
    const payload = line.payload as { type?: unknown; turn_id?: unknown };
    if (line.type === "event_msg" && payload.type === "task_started") {
      current = typeof payload.turn_id === "string" ? payload.turn_id : turns[turnIndex]?.turnId ?? "";
      turnIndex += 1;
    }
    if (eventId && current) result.set(eventId, current);
  }
  return result;
}

function finalizeAccounting(trace: CanonicalTrace, records: SourceRecord[]): void {
  const priorities: Record<CanonicalNode["kind"], number> = {
    unmatched_tool_result: 7,
    tool: 6,
    context_compact: 5,
    retry: 4,
    model: 3,
    subagent: 2,
    turn: 1,
  };
  const primary = new Map<string, string>();
  for (const node of [...trace.nodes].sort((a, b) => priorities[b.kind] - priorities[a.kind])) {
    for (const eventId of node.source_event_ids) {
      if (!primary.has(eventId)) primary.set(eventId, node.key);
    }
  }
  const seen = new Set<string>();
  const accounting: SourceAccounting[] = [];
  for (const record of records) {
    if (seen.has(record.eventId)) continue;
    seen.add(record.eventId);
    const nodeKey = primary.get(record.eventId);
    accounting.push(
      nodeKey
        ? { event_id: record.eventId, disposition: "span", node_key: nodeKey }
        : {
            event_id: record.eventId,
            disposition: "ignored",
            reason: `record ${record.type} has no canonical semantic node`,
          },
    );
  }
  trace.accounting = accounting;
}

function turnState(turn: Turn): CanonicalState {
  if (turn.aborted) return "aborted";
  if (!turn.completed) return "incomplete";
  const tools = turn.steps.flatMap((step) => step.toolCalls);
  if (turn.unmatchedToolResults.length > 0 || tools.some((tool) => Boolean(tool.error))) return "error";
  if (tools.some((tool) => tool.endTime == null)) return "incomplete";
  return "ok";
}

export async function parseCodexRollout(rolloutFile: string): Promise<CanonicalEventGraph> {
  const document = await readRollout(rolloutFile);
  const { sessionMeta, turns } = parseSession(document.lines);
  if (!sessionMeta.sessionId || sessionMeta.sessionId === "unknown") {
    throw new Error("Codex rollout has no Runtime session id");
  }
  const eventTurns = mapMainEventsToTurns(document.lines, turns);
  const builds: TraceBuild[] = [];

  for (const [turnIndex, turn] of turns.entries()) {
    if (!turn.turnId) {
      throw new Error(`Codex rollout turn ${turnIndex} has no Runtime turn_id`);
    }
    const turnId = turn.turnId;
    const traceId = traceIdFromRuntimeCorrelation(sessionMeta.sessionId, turn);
    const state = turnState(turn);
    const turnKey = `turn:${turnId}`;
    const rootSources = unique([
      ...(turnIndex === 0 ? sessionMeta.sourceEventIds : []),
      ...turn.sourceEventIds,
    ]);
    const root: CanonicalNode = {
      key: turnKey,
      kind: "turn",
      name: "agent.turn",
      ...(turn.turnId ? { runtime_id: turn.turnId } : {}),
      start_time_unix_nano: millisecondsToNanoseconds(turn.startTime),
      end_time_unix_nano: millisecondsToNanoseconds(turn.endTime),
      state,
      ...(state === "aborted"
        ? { status_message: "turn aborted by Runtime" }
        : state === "incomplete"
          ? { status_message: "turn is incomplete" }
          : state === "error"
            ? { status_message: "turn contains failed or unmatched tool evidence" }
            : {}),
      input: turn.userInput,
      output: turn.finalOutput,
      attributes: {
        "agent.session.id": sessionMeta.sessionId,
        ...(turn.turnId ? { "agent.turn.id": turn.turnId } : {}),
        ...(turn.traceId ? { "codex.trace_id": turn.traceId } : {}),
        "catena.trace.correlation": `${sessionMeta.sessionId}:${turnId}`,
        "catena.trace.id.source": isTraceId(turn.traceId)
          ? "runtime_trace_id"
          : "runtime_session_turn_correlation",
        "codex.cli.version": sessionMeta.cliVersion ?? "",
        "codex.model.provider": sessionMeta.modelProvider ?? "",
        "codex.aborted": turn.aborted,
        "codex.completed": turn.completed,
      },
      source_event_ids: rootSources,
    };
    const trace: CanonicalTrace = { trace_id: traceId, turn_id: turnId, state, nodes: [root], accounting: [] };
    const build: TraceBuild = { trace, sourceRecords: [] };
    const children = appendModelAndToolNodes(trace.nodes, turn, turnKey, turnKey, turn.model ?? "unknown");

    turn.contextCompactions.forEach((compaction, index) => {
      trace.nodes.push({
        key: `${turnKey}:compact:${compaction.sourceEventIds[0] || index}`,
        parent_key: turnKey,
        kind: "context_compact",
        name: "agent.context.compact",
        start_time_unix_nano: millisecondsToNanoseconds(compaction.timestamp),
        end_time_unix_nano: millisecondsToNanoseconds(compaction.timestamp),
        state: "ok",
        input: compaction.payload,
        attributes: { "codex.compaction.event_type": compaction.eventType },
        source_event_ids: unique(compaction.sourceEventIds),
      });
    });

    turn.unmatchedToolResults.forEach((result, index) => {
      trace.nodes.push({
        key: `${turnKey}:unmatched:${result.callId || result.sourceEventIds[0] || index}`,
        parent_key: turnKey,
        kind: "unmatched_tool_result",
        name: "agent.tool.result.unmatched",
        ...(result.callId ? { runtime_id: result.callId } : {}),
        start_time_unix_nano: millisecondsToNanoseconds(result.timestamp),
        end_time_unix_nano: millisecondsToNanoseconds(result.timestamp),
        state: "error",
        status_message: result.error ?? "unmatched tool result",
        output: result.output,
        attributes: {
          "gen_ai.tool.result.type": result.resultType,
          ...(result.callId ? { "gen_ai.tool.call.id": result.callId } : {}),
        },
        source_event_ids: unique(result.sourceEventIds),
      });
    });

    for (const link of turn.subagents) {
      await appendSubagent(build, rolloutFile, link, turn, turnKey, children.toolKeys, 0);
    }
    builds.push(build);
  }

  const firstTurnId = turns[0]?.turnId ?? "";
  for (const record of document.records) {
    const mappedTurn = eventTurns.get(record.eventId) || firstTurnId;
    const build = builds.find((candidate) => candidate.trace.turn_id === mappedTurn) ?? builds[0];
    if (build) build.sourceRecords.push(record);
  }
  for (const build of builds) finalizeAccounting(build.trace, build.sourceRecords);

  return {
    schema_version: CANONICAL_SCHEMA_VERSION,
    runtime: "codex",
    session_id: sessionMeta.sessionId,
    source: {
      format: "codex-rollout-jsonl",
      path: path.basename(rolloutFile),
      parser: CODEX_PARSER_NAME,
      upstream_commit: CODEX_UPSTREAM_COMMIT,
    },
    traces: builds.map((build) => build.trace),
  };
}

export function canonicalGraphJSON(graph: CanonicalEventGraph): string {
  return `${JSON.stringify(graph, null, 2)}\n`;
}

export function sourceEvidenceText(value: unknown): string {
  return canonicalString(value);
}
