/**
 * Codex rollout transcript -> per-turn chat-message request body + reply.
 *
 * Codex's native OTLP spans (scope `codex_cli_rs`) carry tokens, model, and
 * timing but never the prompt, the system instructions, the tool calls, or the
 * assistant reply. Codex DOES persist the whole conversation to disk as a JSONL
 * "rollout" at `~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<sessionid>.jsonl`,
 * Older releases recorded the exact OTLP `trace_id` on each `task_started`;
 * current Codex Desktop releases keep `turn_id` but omit that join key. Live
 * harvesting uses only native ids. Historical backfill may derive a stable
 * trace id from the globally unique turn id so persisted conversations remain
 * observable without pretending they joined a native trace.
 *
 * The rollout is the running conversation state (the OpenAI Responses API
 * `input` array), append-only. We replay it into an accumulating chat history
 * and, at each turn boundary, snapshot that history as the turn's `input` (the
 * request actually sent to the model: system prompt + every prior message + the
 * current user prompt + any mid-turn tool calls/results) with the turn's final
 * assistant answer as `output`. This mirrors how the claude log-to-span fold
 * turns a `/v1/messages` body into `gen_ai.input.messages`, so a codex trace
 * renders the same full conversation a claude trace does.
 *
 * Rollout line shapes this parser relies on (codex 0.137):
 * - `{"type":"session_meta","payload":{"base_instructions":"...","cwd":"..."}}`
 * - `{"type":"turn_context","payload":{"model":"gpt-5.5"}}`
 * - `{"type":"event_msg","payload":{"type":"task_started","turn_id":"...","trace_id":"<hex32>"}}`
 * - `{"type":"response_item","payload":{"type":"message","role":"developer|user|assistant","content":[{"type":"input_text|output_text","text":"..."}]}}`
 * - `{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{...}","call_id":"..."}}`
 * - `{"type":"response_item","payload":{"type":"function_call_output","call_id":"...","output":"..."}}`
 * - `{"type":"event_msg","payload":{"type":"agent_message","message":"...","phase":"final_answer"}}`
 */

import { createHash } from "node:crypto";

/** Per-message content cap so a single huge tool output can't dominate the span. */
const MAX_CONTENT_CHARS = 30_000;
/** Whole-input cap (well under the 256KB ingestion attribute ceiling). */
const MAX_INPUT_CHARS = 120_000;
/** Final-answer cap. */
const MAX_OUTPUT_CHARS = 30_000;

/**
 * A LangWatch chat message. Roles map to the canonical chat roles
 * (`system|user|assistant|tool`); codex's `developer` role is folded into
 * `system`. Shapes a subset of the platform `chatMessageSchema` so the
 * receiver's LangWatch extractor canonicalises it to `gen_ai.input.messages`.
 */
export interface CodexChatMessage {
  role: "system" | "user" | "assistant" | "tool";
  content?: string;
  tool_calls?: {
    id: string;
    type: "function";
    function: { name: string; arguments: string };
  }[];
  tool_call_id?: string;
}

/** One tool execution reconstructed from a rollout call/result pair. */
export interface CodexToolCall {
  /** Stable join key persisted by Codex, or a deterministic parser fallback. */
  callId: string;
  name: string;
  arguments: string;
  /** Null when the rollout ended before Codex persisted a result. */
  output: string | null;
  /** Call and result envelope timestamps, when present in the rollout. */
  startedAtMs: number | null;
  completedAtMs: number | null;
}

export interface CodexTurnIO {
  /** Hex OTLP trace_id codex used for this turn's spans (the join key). */
  traceId: string;
  /** Whether Codex persisted the id or the history importer derived it. */
  traceIdSource?: "native" | "synthetic";
  turnId: string | null;
  sessionId?: string | null;
  model: string | null;
  /**
   * The full request body as sent to the model for this turn: the system
   * prompt, every prior message, the current user prompt, and any mid-turn
   * tool calls/results — everything except the turn's final assistant answer.
   */
  inputMessages: CodexChatMessage[];
  /** Tool executions from this turn only, in call order. */
  toolCalls?: CodexToolCall[];
  /** The assistant's final reply for the turn (plain text). */
  output: string;
  /** Turn start in unix ms, for a sane span start time (best-effort). */
  startedAtMs: number | null;
  /** Turn completion in unix ms; historical imports must not end at upload time. */
  completedAtMs?: number | null;
}

export interface ParseCodexRolloutOptions {
  /**
   * Codex Desktop no longer persists its native OTel trace_id in task_started.
   * Historical imports may opt into a stable id derived from the globally
   * unique turn_id. Live harvesting leaves this disabled so it never creates a
   * second trace beside Codex's native OTel trace.
   */
  synthesizeMissingTraceIds?: boolean;
  /**
   * Live content recovery mirrors Codex's full accumulated request. Historical
   * backfill can keep only the current turn plus the latest system instruction
   * so forked histories do not duplicate megabytes of earlier conversation.
   */
  includePriorHistory?: boolean;
}

/** Stable, valid 128-bit OTel trace id for one persisted Codex turn. */
export function syntheticCodexTraceId(turnId: string): string {
  return createHash("sha256")
    .update(`langwatch.codex.history:${turnId}`)
    .digest("hex")
    .slice(0, 32);
}

function epochMs(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value >= 1_000_000_000_000 ? value : value * 1000;
  }
  if (typeof value === "string") {
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…[truncated]` : text;
}

function textFromContent(content: unknown): string {
  if (!Array.isArray(content)) return "";
  const parts: string[] = [];
  for (const part of content) {
    if (part && typeof part === "object") {
      const t = (part as { text?: unknown }).text;
      const ot = (part as { output_text?: unknown }).output_text;
      if (typeof t === "string") parts.push(t);
      else if (typeof ot === "string") parts.push(ot);
    }
  }
  return parts.join("").trim();
}

function outputToText(output: unknown): string {
  if (typeof output === "string") return output;
  if (output && typeof output === "object") {
    // codex wraps exec output as { output: "...", metadata: {...} } sometimes
    const inner = (output as { output?: unknown }).output;
    if (typeof inner === "string") return inner;
    try {
      return JSON.stringify(output);
    } catch {
      return "";
    }
  }
  if (typeof output === "number" || typeof output === "boolean") {
    return String(output);
  }
  return "";
}

/**
 * Bound the serialized input: cap each message's content, then drop the oldest
 * NON-system messages until the whole array is under the total cap. System
 * messages (the prompt the user actually asked to see) are always preserved.
 */
function capInputMessages(messages: CodexChatMessage[]): CodexChatMessage[] {
  const capped = messages.map((m) =>
    typeof m.content === "string" && m.content.length > MAX_CONTENT_CHARS
      ? { ...m, content: truncate(m.content, MAX_CONTENT_CHARS) }
      : m,
  );
  const sizes = capped.map((message) => JSON.stringify(message).length);
  let keptCount = capped.length;
  let total =
    2 + sizes.reduce((sum, size) => sum + size, 0) + Math.max(0, keptCount - 1);
  if (total <= MAX_INPUT_CHARS) return capped;

  const keep = capped.map(() => true);
  for (let i = 0; total > MAX_INPUT_CHARS && i < capped.length; i++) {
    if (capped[i]?.role === "system") continue;
    keep[i] = false;
    total -= sizes[i] ?? 0;
    if (keptCount > 1) total--;
    keptCount--;
  }
  return capped.filter((_, index) => keep[index]);
}

/** One parsed rollout JSONL line: a tagged envelope with an opaque payload. */
interface RolloutLine {
  type?: string;
  timestamp?: unknown;
  payload?: Record<string, unknown>;
}

/** The tool call's explicit id (`call_id`, then `id`), or null if codex omitted both. */
function explicitToolCallId(payload: Record<string, unknown>): string | null {
  if (typeof payload.call_id === "string" && payload.call_id) {
    return payload.call_id;
  }
  if (typeof payload.id === "string" && payload.id) return payload.id;
  return null;
}

/**
 * Replays a codex rollout's events into a running chat history and snapshots one
 * {@link CodexTurnIO} per turn boundary. All the cross-event state (the running
 * history, the open turn, the held assistant text, the authoritative final
 * answer, the session model) lives here so {@link parseCodexRollout} stays a
 * thin parse-and-dispatch coordinator.
 */
class CodexTurnAccumulator {
  /** Emitted turns, in rollout order. */
  private readonly turns: CodexTurnIO[] = [];
  /** Accumulating conversation across the whole rollout (claude-style). */
  private readonly history: CodexChatMessage[] = [];
  private currentSystemMessage: CodexChatMessage | null = null;
  private sessionModel: string | null = null;
  private sessionId: string | null = null;
  private cur: {
    traceId: string;
    traceIdSource: "native" | "synthetic";
    turnId: string | null;
    model: string | null;
    startedAtMs: number | null;
  } | null = null;
  /** Latest assistant text not yet committed to history (the final-answer candidate). */
  private pendingAssistant: string | null = null;
  /** Authoritative final answer from the agent_message(final_answer) event. */
  private agentFinal: string | null = null;
  /**
   * Synthetic ids minted for tool calls that arrived without a `call_id`, queued
   * FIFO so the matching (also id-less) function_call_output pairs to the same id
   * instead of drifting as the running history grows.
   */
  private readonly pendingToolCallIds: string[] = [];
  /** Tool evidence belongs to the current turn, never accumulated history. */
  private readonly currentToolCalls: CodexToolCall[] = [];
  /** Unresolved calls by id; arrays preserve parallel/repeated call ordering. */
  private readonly unresolvedToolCallIndexes = new Map<string, number[]>();
  private autoToolCallSeq = 0;

  constructor(private readonly options: ParseCodexRolloutOptions) {}

  /** Route one parsed rollout line to the handler for its event type. */
  handle(obj: RolloutLine): void {
    const payload = obj.payload ?? {};
    const eventAtMs = epochMs(obj.timestamp);
    switch (obj.type) {
      case "session_meta":
        return this.onSessionMeta(payload);
      case "turn_context":
        return this.onTurnContext(payload);
      case "event_msg":
        return this.onEventMsg(payload, eventAtMs);
      case "response_item":
        return this.onResponseItem(payload, eventAtMs);
    }
  }

  /** Close the trailing open turn and return every emitted turn. */
  finish(): CodexTurnIO[] {
    this.closeTurn();
    return this.turns;
  }

  private onSessionMeta(payload: Record<string, unknown>): void {
    const sessionId = payload.session_id ?? payload.id;
    if (typeof sessionId === "string" && sessionId) {
      this.sessionId = sessionId;
    }
    const bi = payload.base_instructions;
    if (typeof bi === "string" && bi.trim()) {
      this.currentSystemMessage = { role: "system", content: bi.trim() };
      if (this.options.includePriorHistory !== false || this.cur) {
        this.history.push(this.currentSystemMessage);
      }
    }
  }

  private onTurnContext(payload: Record<string, unknown>): void {
    const m = payload.model;
    if (typeof m === "string" && m) {
      this.sessionModel = m;
      if (this.cur) this.cur.model = m;
    }
  }

  private onEventMsg(
    payload: Record<string, unknown>,
    eventAtMs: number | null,
  ): void {
    if (payload.type === "task_started") {
      return this.onTaskStarted(payload, eventAtMs);
    }
    if (payload.type === "task_complete") {
      return this.closeTurn(epochMs(payload.completed_at) ?? eventAtMs);
    }
    if (payload.type === "turn_aborted") {
      return this.closeTurn(epochMs(payload.completed_at) ?? eventAtMs);
    }
    // Everything below belongs to the open turn; ignore it outside one.
    if (!this.cur) return;
    if (payload.type === "agent_message") this.onAgentMessage(payload);
  }

  private onTaskStarted(
    payload: Record<string, unknown>,
    eventAtMs: number | null,
  ): void {
    this.closeTurn();
    if (this.options.includePriorHistory === false) {
      this.history.length = 0;
      if (this.currentSystemMessage) {
        this.history.push(this.currentSystemMessage);
      }
    }
    const turnId =
      typeof payload.turn_id === "string" ? payload.turn_id : null;
    const nativeTraceId =
      typeof payload.trace_id === "string" && payload.trace_id
        ? payload.trace_id
        : null;
    const traceId =
      nativeTraceId ??
      (this.options.synthesizeMissingTraceIds && turnId
        ? syntheticCodexTraceId(turnId)
        : null);
    if (!traceId) return;
    this.cur = {
      traceId,
      traceIdSource: nativeTraceId ? "native" : "synthetic",
      turnId,
      model: this.sessionModel,
      startedAtMs: epochMs(payload.started_at) ?? eventAtMs,
    };
  }

  private onAgentMessage(payload: Record<string, unknown>): void {
    // The clean final answer rides the agent_message(final_answer) event; prefer
    // it over the raw assistant response_item which can repeat tool scaffolding.
    const msg = payload.message;
    if (
      typeof msg === "string" &&
      msg.trim() &&
      payload.phase === "final_answer"
    ) {
      this.agentFinal = msg.trim();
    }
  }

  private onResponseItem(
    payload: Record<string, unknown>,
    eventAtMs: number | null,
  ): void {
    // response_items belong to the open turn; ignore them outside one.
    if (!this.cur) return;
    switch (payload.type) {
      case "message":
        return this.onMessage(payload);
      case "function_call":
        return this.onFunctionCall(payload, eventAtMs);
      case "function_call_output":
        return this.onFunctionCallOutput(payload, eventAtMs);
    }
  }

  private onMessage(payload: Record<string, unknown>): void {
    const role = payload.role;
    const text = textFromContent(payload.content);
    if (!text) return;
    if (role === "developer") {
      this.flushPendingAssistant();
      this.history.push({ role: "system", content: text });
    } else if (role === "user") {
      this.flushPendingAssistant();
      this.history.push({ role: "user", content: text });
    } else if (role === "assistant") {
      // Hold: this may be a mid-turn preamble (committed to history when the
      // next item arrives) or the turn's final answer (consumed by closeTurn).
      this.flushPendingAssistant();
      this.pendingAssistant = text;
    }
  }

  private onFunctionCall(
    payload: Record<string, unknown>,
    eventAtMs: number | null,
  ): void {
    this.flushPendingAssistant();
    let callId = explicitToolCallId(payload);
    if (!callId) {
      // codex omitted the id: mint a stable one and queue it for the output.
      callId = `call_auto_${this.autoToolCallSeq++}`;
      this.pendingToolCallIds.push(callId);
    }
    const name = typeof payload.name === "string" ? payload.name : "tool";
    const args =
      typeof payload.arguments === "string"
        ? payload.arguments
        : payload.arguments != null
          ? JSON.stringify(payload.arguments)
          : "";
    this.history.push({
      role: "assistant",
      tool_calls: [
        {
          id: callId,
          type: "function",
          function: { name, arguments: truncate(args, MAX_CONTENT_CHARS) },
        },
      ],
    });
    const index = this.currentToolCalls.length;
    this.currentToolCalls.push({
      callId,
      name,
      arguments: truncate(args, MAX_CONTENT_CHARS),
      output: null,
      startedAtMs: eventAtMs,
      completedAtMs: null,
    });
    const unresolved = this.unresolvedToolCallIndexes.get(callId) ?? [];
    unresolved.push(index);
    this.unresolvedToolCallIndexes.set(callId, unresolved);
  }

  private onFunctionCallOutput(
    payload: Record<string, unknown>,
    eventAtMs: number | null,
  ): void {
    this.flushPendingAssistant();
    // Reuse the id codex gave; else pair FIFO with the matching id-less call.
    const callId =
      explicitToolCallId(payload) ??
      this.pendingToolCallIds.shift() ??
      `call_auto_${this.autoToolCallSeq++}`;
    const output = truncate(outputToText(payload.output), MAX_CONTENT_CHARS);
    this.history.push({
      role: "tool",
      tool_call_id: callId,
      content: output,
    });
    const unresolved = this.unresolvedToolCallIndexes.get(callId);
    const index = unresolved?.shift();
    if (unresolved?.length === 0) {
      this.unresolvedToolCallIndexes.delete(callId);
    }
    if (index !== undefined) {
      const call = this.currentToolCalls[index];
      if (call) {
        call.output = output;
        call.completedAtMs = eventAtMs;
      }
    } else {
      // Keep an orphaned result observable instead of silently dropping the
      // only persisted evidence for a tool execution.
      this.currentToolCalls.push({
        callId,
        name: "tool",
        arguments: "",
        output,
        startedAtMs: eventAtMs,
        completedAtMs: eventAtMs,
      });
    }
  }

  private flushPendingAssistant(): void {
    if (this.pendingAssistant !== null) {
      this.history.push({ role: "assistant", content: this.pendingAssistant });
      this.pendingAssistant = null;
    }
  }

  private closeTurn(completedAtMs: number | null = null): void {
    if (this.cur) {
      const finalAnswer = this.agentFinal ?? this.pendingAssistant;
      if (finalAnswer?.trim()) {
        this.turns.push({
          traceId: this.cur.traceId,
          traceIdSource: this.cur.traceIdSource,
          turnId: this.cur.turnId,
          sessionId: this.sessionId,
          model: this.cur.model ?? this.sessionModel,
          inputMessages: capInputMessages([...this.history]),
          toolCalls: this.currentToolCalls.map((call) => ({ ...call })),
          output: truncate(finalAnswer.trim(), MAX_OUTPUT_CHARS),
          startedAtMs: this.cur.startedAtMs,
          completedAtMs,
        });
        if (this.options.includePriorHistory !== false) {
          this.history.push({ role: "assistant", content: finalAnswer.trim() });
        }
      }
    }
    // Synthetic fallback ids only pair a call with its output *within* a turn.
    // A call left unmatched at the boundary (output never arrived) must not
    // leak its queued id into the next turn, or that turn's first id-less
    // output would pair to the wrong call. `autoToolCallSeq` stays monotonic
    // so the ids themselves remain unique across the session.
    this.pendingToolCallIds.length = 0;
    this.currentToolCalls.length = 0;
    this.unresolvedToolCallIndexes.clear();
    this.cur = null;
    this.pendingAssistant = null;
    this.agentFinal = null;
  }
}

/**
 * Parse a codex rollout JSONL into one chat-message request/reply record per
 * turn. Turns with no assistant reply are dropped (an empty span helps no one).
 */
export function parseCodexRollout(
  content: string,
  options: ParseCodexRolloutOptions = {},
): CodexTurnIO[] {
  const parser = createCodexRolloutParser(options);
  for (const line of content.split("\n")) parser.pushLine(line);
  return parser.finish();
}

export interface CodexRolloutParser {
  pushLine(line: string): void;
  finish(): CodexTurnIO[];
}

/** Incremental parser used by history backfill for multi-gigabyte rollouts. */
export function createCodexRolloutParser(
  options: ParseCodexRolloutOptions = {},
): CodexRolloutParser {
  const acc = new CodexTurnAccumulator(options);
  return {
    pushLine(line: string): void {
      const trimmed = line.trim();
      if (!trimmed) return;
      let obj: RolloutLine;
      try {
        obj = JSON.parse(trimmed) as RolloutLine;
      } catch {
        return;
      }
      acc.handle(obj);
    },
    finish: () => acc.finish(),
  };
}
