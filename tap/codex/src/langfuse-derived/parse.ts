import type {
  EventMsgPayload,
  MessageContentPart,
  ModelStep,
  ResponseItemBuiltinToolCall,
  ResponseItemFunctionCall,
  ResponseItemFunctionCallOutput,
  ResponseItemCustomToolCall,
  ResponseItemLocalShellCall,
  ResponseItemMessage,
  ResponseItemWebSearchCall,
  RolloutLine,
  SessionMeta,
  TokenUsage,
  ToolCall,
  Turn,
} from "./types.js";
import { isPrimitive, toText } from "./utils.js";

/** Extract printable text from a Codex message `content` array. */
function extractMessageText(content: MessageContentPart[] | undefined): string {
  if (!Array.isArray(content)) return "";
  return content
    .map((part) => {
      if (!part || typeof part !== "object") return "";
      if (part.type === "input_text" || part.type === "output_text" || part.type === "text") {
        return typeof part.text === "string" ? part.text : "";
      }
      return "";
    })
    .filter(Boolean)
    .join("\n");
}

/** Extract reasoning text, skipping encrypted-only reasoning items. */
function extractReasoning(item: {
  content?: unknown[] | string | null;
  summary?: unknown[];
}): string {
  if (typeof item.content === "string") return item.content;
  if (Array.isArray(item.content)) {
    return item.content
      .map((c) =>
        c && typeof c === "object" && "text" in c
          ? toText((c as { text: unknown }).text)
          : toText(c),
      )
      .filter(Boolean)
      .join("\n");
  }
  if (Array.isArray(item.summary) && item.summary.length > 0) {
    return item.summary
      .map((s) => toText(s))
      .filter(Boolean)
      .join("\n");
  }
  return "";
}

function parseArgs(raw: string): unknown {
  if (typeof raw !== "string") return raw;
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

function extractToolError(payload: EventMsgPayload): string | undefined {
  const explicit = payload.error ?? payload.codex_error_info;
  if (explicit != null) {
    return isPrimitive(explicit) ? String(explicit) : JSON.stringify(explicit);
  }
  const streams = [payload.stdout, payload.stderr]
    .filter((s): s is string => typeof s === "string" && s.length > 0)
    .join("\n");
  if (typeof payload.aggregated_output === "string" && payload.aggregated_output) {
    return payload.aggregated_output;
  }
  if (streams) return streams;
  if (typeof payload.exit_code === "number") return `Exit code: ${payload.exit_code}`;
  return undefined;
}

function sourceEventId(line: RolloutLine): string {
  return line.sourceEventId ?? `timestamp:${line.timestamp}:${line.type}`;
}

function appendSource(target: { sourceEventIds: string[] }, eventId: string): void {
  if (!target.sourceEventIds.includes(eventId)) target.sourceEventIds.push(eventId);
}

function runtimeTimestamp(value: unknown, fallback: number): number {
  if (typeof value !== "string") return fallback;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function responseItemId(value: Record<string, unknown>): string | undefined {
  return typeof value.id === "string" && value.id ? value.id : undefined;
}

/** A turn that is still being assembled. */
type MutableTurn = Turn & { lastAgentMessage?: string; userInputFallback?: string };

function newTurn(startTime: number): MutableTurn {
  return {
    turnId: undefined,
    startTime,
    endTime: startTime,
    steps: [],
    subagentThreadIds: [],
    subagents: [],
    contextCompactions: [],
    unmatchedToolResults: [],
    sourceEventIds: [],
    completed: false,
    aborted: false,
  };
}

/**
 * Parse a Codex rollout into session metadata and a list of fully assembled
 * turns.
 *
 * Codex interleaves model I/O (`response_item`) with lifecycle events
 * (`event_msg`). We reconstruct each turn as a sequence of model steps (one per
 * model response, delimited by `token_count` events) plus the tool calls each
 * step issued. Tool execution details (status, exit code, output) arrive later
 * as `*_end` events and are matched back to their call by `call_id`.
 */
export function parseSession(lines: RolloutLine[]): {
  sessionMeta: SessionMeta;
  turns: Turn[];
} {
  let sessionMeta: SessionMeta = { sessionId: "unknown", sourceEventIds: [] };
  const turns: Turn[] = [];

  let turn: MutableTurn | null = null;
  let step: ModelStep | null = null;
  let toolCallsById = new Map<string, ToolCall>();
  let lastTimestamp = Date.now();

  function newStep(startTime: number): ModelStep {
    return { startTime, endTime: startTime, toolCalls: [], sourceEventIds: [] };
  }

  const ensureTurn = (ts: number): MutableTurn => (turn ??= newTurn(ts));
  const ensureStep = (ts: number) => (step ??= newStep(ts));

  // Rollouts from the transition period can carry both spawn-event formats
  // for the same child; the thread must be nested exactly once.
  const recordSubagentThread = (threadId: string, callId: string | undefined, eventId: string) => {
    if (!turn!.subagentThreadIds.includes(threadId)) {
      turn!.subagentThreadIds.push(threadId);
    }
    const existing = turn!.subagents.find((item) => item.threadId === threadId);
    if (existing) {
      existing.callId ??= callId;
      if (!existing.sourceEventIds.includes(eventId)) existing.sourceEventIds.push(eventId);
      return;
    }
    turn!.subagents.push({ threadId, callId, sourceEventIds: [eventId] });
  };

  const closeStep = (ts: number, usage?: TokenUsage) => {
    if (!step) return;
    step.endTime = Math.max(step.endTime, ts);
    if (usage) step.usage = usage;
    turn!.steps.push(step);
    step = null;
  };

  const finishTurn = (ts: number, opts: { completed: boolean; aborted: boolean }) => {
    if (!turn) return;
    closeStep(ts);
    turn.endTime = Math.max(turn.endTime, ts);
    turn.completed = opts.completed;
    turn.aborted = opts.aborted;
    turn.userInput = turn.userInput ?? turn.userInputFallback;
    turn.finalOutput = turn.lastAgentMessage ?? turn.steps.filter((s) => s.text).at(-1)?.text;
    delete turn.lastAgentMessage;
    delete turn.userInputFallback;
    turns.push(turn);
    turn = null;
    toolCallsById = new Map();
  };

  for (const line of lines) {
    const ts = Number.isFinite(Date.parse(line.timestamp))
      ? Date.parse(line.timestamp)
      : lastTimestamp;
    lastTimestamp = ts;
    const eventId = sourceEventId(line);

    if (line.type === "session_meta") {
      const p = line.payload as RolloutLine["payload"] & {
        id?: string;
        cli_version?: string;
        model_provider?: string | null;
        base_instructions?: { text?: string } | null;
        parent_thread_id?: string | null;
        thread_source?: string | null;
      };
      sessionMeta = {
        sessionId: typeof p.id === "string" ? p.id : sessionMeta.sessionId,
        cliVersion: p.cli_version,
        modelProvider: p.model_provider ?? undefined,
        baseInstructions: p.base_instructions?.text,
        isSubagentThread: typeof p.parent_thread_id === "string" || p.thread_source === "subagent",
        parentThreadId: typeof p.parent_thread_id === "string" ? p.parent_thread_id : undefined,
        sourceEventIds: [...sessionMeta.sourceEventIds, eventId],
      };
      continue;
    }

    if (line.type === "turn_context") {
      const t = ensureTurn(ts);
      const p = line.payload as { model?: string };
      t.model = p.model ?? t.model;
      t.invocationParams = line.payload as Record<string, unknown>;
      appendSource(t, eventId);
      continue;
    }

    if (line.type === "response_item") {
      // `payload.type` is an open string set across Codex versions, so we
      // switch on it and cast into the concrete shape per branch rather than
      // relying on discriminated-union narrowing.
      const p = line.payload as { type?: string } & Record<string, unknown>;
      ensureTurn(ts);

      if (p.type === "message") {
        const msg = p as unknown as ResponseItemMessage;
        const text = extractMessageText(msg.content as MessageContentPart[]);
        if (msg.role === "assistant") {
          const s = ensureStep(ts);
          s.modelCallId ??= responseItemId(p);
          appendSource(s, eventId);
          if (text) s.text = s.text ? `${s.text}\n${text}` : text;
        } else if (msg.role === "user" && text) {
          appendSource(turn!, eventId);
          // Codex injects <environment_context>/<user_instructions> as user
          // messages; keep only the first that does not look like wrapper XML.
          if (
            !turn!.userInputFallback &&
            !/^<(environment_context|user_instructions)/.test(text.trim())
          ) {
            turn!.userInputFallback = text;
          }
        }
      } else if (p.type === "function_call") {
        const call = p as unknown as ResponseItemFunctionCall;
        const s = ensureStep(ts);
        const tc: ToolCall = {
          callId: call.call_id,
          responseItemId: call.id ?? undefined,
          name: call.name,
          type: "function",
          args: parseArgs(call.arguments),
          startTime: ts,
          sourceEventIds: [eventId],
        };
        s.modelCallId ??= call.id ?? undefined;
        appendSource(s, eventId);
        s.toolCalls.push(tc);
        if (tc.callId) toolCallsById.set(tc.callId, tc);
      } else if (p.type === "custom_tool_call") {
        const call = p as unknown as ResponseItemCustomToolCall;
        const s = ensureStep(ts);
        const tc: ToolCall = {
          callId: call.call_id,
          responseItemId: call.id ?? undefined,
          name: call.name,
          type: "custom",
          args: parseArgs(call.input),
          startTime: ts,
          sourceEventIds: [eventId],
        };
        s.modelCallId ??= call.id ?? undefined;
        appendSource(s, eventId);
        s.toolCalls.push(tc);
        if (tc.callId) toolCallsById.set(tc.callId, tc);
      } else if (p.type === "local_shell_call") {
        // Built-in local shell tool: the command lives in `action`, and
        // exec_command_end / function_call_output enrich it like any function
        // call.
        const call = p as unknown as ResponseItemLocalShellCall;
        const s = ensureStep(ts);
        const tc: ToolCall = {
          callId: call.call_id ?? call.id ?? "",
          responseItemId: call.id ?? undefined,
          name: "local_shell",
          type: "local_shell",
          args: call.action ?? undefined,
          startTime: ts,
          sourceEventIds: [eventId],
        };
        s.modelCallId ??= call.id ?? undefined;
        appendSource(s, eventId);
        s.toolCalls.push(tc);
        if (tc.callId) toolCallsById.set(tc.callId, tc);
      } else if (p.type === "web_search_call") {
        // Server-side web search: there is no output item, and the
        // web_search_end event may be recorded before or after this item, so
        // merge with an existing call when one was already registered.
        const call = p as unknown as ResponseItemWebSearchCall;
        const callId = call.id ?? "";
        const existing = callId ? toolCallsById.get(callId) : undefined;
        if (existing) {
          existing.args = existing.args ?? call.action ?? undefined;
          existing.endTime = Math.max(existing.endTime ?? ts, ts);
          if (call.status === "failed") existing.error = "web_search_call failed";
          appendSource(existing, eventId);
        } else {
          const s = ensureStep(ts);
          const tc: ToolCall = {
            callId,
            responseItemId: call.id ?? undefined,
            name: "web_search",
            type: "web_search",
            args: call.action ?? undefined,
            startTime: ts,
            ...(call.status === "completed" ? { endTime: ts } : {}),
            ...(call.status === "failed" ? { endTime: ts, error: "web_search_call failed" } : {}),
            sourceEventIds: [eventId],
          };
          s.modelCallId ??= call.id ?? undefined;
          appendSource(s, eventId);
          s.toolCalls.push(tc);
          if (callId) toolCallsById.set(callId, tc);
        }
      } else if (p.type === "file_search_call" || p.type === "computer_call") {
        const call = p as unknown as ResponseItemBuiltinToolCall;
        const s = ensureStep(ts);
        const callId = call.call_id ?? call.id ?? "";
        const tc: ToolCall = {
          callId,
          responseItemId: call.id ?? undefined,
          name: p.type === "file_search_call" ? "file_search" : "computer",
          type: p.type === "file_search_call" ? "file_search" : "computer",
          args: call.action ?? call.input,
          startTime: ts,
          ...(call.status === "completed" ? { endTime: ts } : {}),
          ...(call.results !== undefined ? { output: call.results } : {}),
          ...(call.status === "failed" ? { error: `${p.type} failed` } : {}),
          sourceEventIds: [eventId],
        };
        s.modelCallId ??= call.id ?? undefined;
        appendSource(s, eventId);
        s.toolCalls.push(tc);
        if (callId) toolCallsById.set(callId, tc);
      } else if (p.type === "function_call_output" || p.type === "custom_tool_call_output") {
        const out = p as unknown as ResponseItemFunctionCallOutput;
        const tc = toolCallsById.get(out.call_id);
        if (tc) {
          if (tc.output == null) tc.output = out.output;
          tc.endTime = Math.max(tc.endTime ?? ts, ts);
          appendSource(tc, eventId);
        } else {
          turn!.unmatchedToolResults.push({
            callId: out.call_id,
            resultType: String(p.type),
            output: out.output,
            error: out.call_id ? `unknown call_id ${out.call_id}` : "tool result has no call_id",
            timestamp: ts,
            sourceEventIds: [eventId],
          });
        }
      } else if (p.type === "reasoning") {
        const reasoning = extractReasoning(
          p as { content?: unknown[] | string | null; summary?: unknown[] },
        );
        if (reasoning) {
          const s = ensureStep(ts);
          s.modelCallId ??= responseItemId(p);
          appendSource(s, eventId);
          s.reasoning = s.reasoning ? `${s.reasoning}\n${reasoning}` : reasoning;
        }
      } else {
        appendSource(turn!, eventId);
      }
      continue;
    }

    if (line.type === "compacted") {
      const t = ensureTurn(ts);
      t.contextCompactions.push({
        timestamp: ts,
        eventType: "compacted",
        payload: line.payload as Record<string, unknown>,
        sourceEventIds: [eventId],
      });
      continue;
    }

    if (line.type === "event_msg") {
      const p = line.payload as EventMsgPayload;
      const et = p.type;

      if (et === "task_started") {
        if (turn) finishTurn(ts, { completed: false, aborted: false });
        const startedAt = runtimeTimestamp(p.started_at, ts);
        turn = newTurn(startedAt);
        turn.turnId = typeof p.turn_id === "string" ? p.turn_id : undefined;
        turn.traceId = typeof p.trace_id === "string" ? p.trace_id.toLowerCase() : undefined;
        appendSource(turn, eventId);
        continue;
      }

      // Session-level lifecycle records can appear between completed turns
      // (notably after `codex resume`). They are source-accounted but must not
      // synthesize a prompt-less turn before the next authoritative
      // task_started event.
      if (!turn) continue;

      if (et === "user_message" && typeof p.message === "string") {
        appendSource(turn!, eventId);
        if (!turn!.userInput) turn!.userInput = p.message;
      } else if (et === "agent_message" && typeof p.message === "string") {
        appendSource(turn!, eventId);
        turn!.lastAgentMessage = p.message;
      } else if (et === "token_count") {
        if (step) appendSource(step, eventId);
        if (p.info?.total_token_usage) turn!.totalUsage = p.info.total_token_usage;
        closeStep(ts, p.info?.last_token_usage ?? undefined);
      } else if (et === "task_complete") {
        appendSource(turn!, eventId);
        finishTurn(runtimeTimestamp(p.completed_at, ts), { completed: true, aborted: false });
      } else if (et === "turn_aborted") {
        appendSource(turn!, eventId);
        finishTurn(ts, { completed: true, aborted: true });
      } else if (et === "context_compacted") {
        turn!.contextCompactions.push({
          timestamp: ts,
          eventType: et,
          payload: p as Record<string, unknown>,
          sourceEventIds: [eventId],
        });
      } else if (et === "stream_error" || et === "model_retry" || et === "retry") {
        const s = ensureStep(ts);
        s.state = "retry";
        s.statusMessage = extractToolError(p) ?? toText(p.message ?? p.error ?? et);
        appendSource(s, eventId);
      } else {
        // A subagent spawn records the child thread *and* (since it carries a
        // call_id ending in "_end") enriches the spawning tool call below.
        if (et === "collab_agent_spawn_end" && typeof p.new_thread_id === "string") {
          recordSubagentThread(p.new_thread_id, p.call_id, eventId);
        }
        // Codex multi-agent v2 persists the spawn as sub_agent_activity
        // instead. Only kind "started" marks a spawn — "interacted" and
        // "interrupted" reference an existing child and would nest it under
        // the wrong (later) turn.
        if (
          et === "sub_agent_activity" &&
          p.kind === "started" &&
          typeof p.agent_thread_id === "string"
        ) {
          recordSubagentThread(
            p.agent_thread_id,
            typeof p.event_id === "string" ? p.event_id : p.call_id,
            eventId,
          );
        }
        // MCP tool calls are function calls with a mangled name
        // (`server__tool`); the begin/end events carry the clean server/tool
        // split, which makes a much better observation name.
        if (
          (et === "mcp_tool_call_begin" || et === "mcp_tool_call_end") &&
          typeof p.call_id === "string"
        ) {
          const tc = toolCallsById.get(p.call_id);
          const inv = p.invocation;
          if (tc && typeof inv?.server === "string" && typeof inv?.tool === "string") {
            tc.mcp = { server: inv.server, tool: inv.tool };
            tc.type = "mcp";
            appendSource(tc, eventId);
          }
        }
        // Web searches run server-side inside the model response. Lifecycle
        // events can be recorded before the response item, so register at the
        // begin event and enrich at end without guessing another identity.
        if (et === "web_search_begin" && typeof p.call_id === "string") {
          let tc = toolCallsById.get(p.call_id);
          if (!tc) {
            tc = {
              callId: p.call_id,
              name: "web_search",
              type: "web_search",
              args: p.action ?? (typeof p.query === "string" ? { query: p.query } : undefined),
              startTime: ts,
              sourceEventIds: [eventId],
            };
            const s = ensureStep(ts);
            appendSource(s, eventId);
            s.toolCalls.push(tc);
            toolCallsById.set(tc.callId, tc);
          } else {
            tc.startTime = Math.min(tc.startTime, ts);
            appendSource(tc, eventId);
          }
        }
        if (et === "web_search_end" && typeof p.call_id === "string") {
          let tc = toolCallsById.get(p.call_id);
          if (!tc) {
            tc = {
              callId: p.call_id,
              name: "web_search",
              type: "web_search",
              args: undefined,
              startTime: ts,
              sourceEventIds: [eventId],
            };
            const s = ensureStep(ts);
            appendSource(s, eventId);
            s.toolCalls.push(tc);
            toolCallsById.set(tc.callId, tc);
          } else {
            appendSource(tc, eventId);
          }
          tc.args =
            tc.args ?? p.action ?? (typeof p.query === "string" ? { query: p.query } : undefined);
          tc.endTime = Math.max(tc.endTime ?? ts, ts);
        }
        if (typeof p.call_id === "string" && et.endsWith("_begin") && et !== "web_search_begin") {
          const tc = toolCallsById.get(p.call_id);
          if (tc) {
            tc.startTime = Math.min(tc.startTime, ts);
            appendSource(tc, eventId);
          }
        }
        // Tool execution lifecycle events (exec_command_end, patch_apply_end,
        // mcp_tool_call_end, collab_*_end, …) match a call by id and add
        // timing, status, and output.
        if (typeof p.call_id === "string" && et.endsWith("_end")) {
          const tc = toolCallsById.get(p.call_id);
          if (tc) {
            appendSource(tc, eventId);
            tc.endTime = Math.max(tc.endTime ?? ts, ts);
            if (p.status === "failed" || p.status === "declined") {
              tc.error = extractToolError(p) ?? `tool ${p.status}`;
            }
            if (tc.output == null) {
              tc.output = p.aggregated_output ?? p.stdout ?? (p as { result?: unknown }).result;
            }
          } else if (p.call_id && et !== "collab_agent_spawn_end") {
            turn!.unmatchedToolResults.push({
              callId: p.call_id,
              resultType: et,
              output: p.aggregated_output ?? p.stdout ?? (p as { result?: unknown }).result,
              error: extractToolError(p) ?? `unknown call_id ${p.call_id}`,
              timestamp: ts,
              sourceEventIds: [eventId],
            });
          }
        }
      }
      continue;
    }
  }

  // Trailing, not-yet-completed turn (e.g. session ended mid-response).
  if (turn) finishTurn(lastTimestamp, { completed: false, aborted: false });

  return { sessionMeta, turns };
}
