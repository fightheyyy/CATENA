#!/usr/bin/env node
import { spawn } from "node:child_process";
import * as fs from "node:fs/promises";
import { chmod, mkdir, readFile, rename, stat, unlink, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import * as path from "node:path";
import { join } from "node:path";
import { createHash, randomUUID } from "node:crypto";

//#region src/canonical.ts
const CANONICAL_SCHEMA_VERSION = "catena.coding_agent.event_graph.v1";
function millisecondsToNanoseconds(value) {
	const finite = Number.isFinite(value) ? Math.max(1, Math.round(value)) : 1;
	return (BigInt(finite) * 1000000n).toString();
}
function canonicalString(value) {
	if (value == null) return "";
	if (typeof value === "string") return value;
	return JSON.stringify(value);
}

//#endregion
//#region src/langfuse-derived/utils.ts
function isPrimitive(value) {
	const t = typeof value;
	return t === "string" || t === "number" || t === "boolean";
}
/** Stringify a value for display, leaving strings untouched. */
function toText(value) {
	if (value == null) return "";
	if (typeof value === "string") return value;
	if (isPrimitive(value)) return String(value);
	try {
		return JSON.stringify(value);
	} catch {
		return String(value);
	}
}

//#endregion
//#region src/langfuse-derived/parse.ts
/** Extract printable text from a Codex message `content` array. */
function extractMessageText(content) {
	if (!Array.isArray(content)) return "";
	return content.map((part) => {
		if (!part || typeof part !== "object") return "";
		if (part.type === "input_text" || part.type === "output_text" || part.type === "text") return typeof part.text === "string" ? part.text : "";
		return "";
	}).filter(Boolean).join("\n");
}
/** Extract reasoning text, skipping encrypted-only reasoning items. */
function extractReasoning(item) {
	if (typeof item.content === "string") return item.content;
	if (Array.isArray(item.content)) return item.content.map((c) => c && typeof c === "object" && "text" in c ? toText(c.text) : toText(c)).filter(Boolean).join("\n");
	if (Array.isArray(item.summary) && item.summary.length > 0) return item.summary.map((s) => toText(s)).filter(Boolean).join("\n");
	return "";
}
function parseArgs(raw) {
	if (typeof raw !== "string") return raw;
	try {
		return JSON.parse(raw);
	} catch {
		return raw;
	}
}
function extractToolError(payload) {
	const explicit = payload.error ?? payload.codex_error_info;
	if (explicit != null) return isPrimitive(explicit) ? String(explicit) : JSON.stringify(explicit);
	const streams = [payload.stdout, payload.stderr].filter((s) => typeof s === "string" && s.length > 0).join("\n");
	if (typeof payload.aggregated_output === "string" && payload.aggregated_output) return payload.aggregated_output;
	if (streams) return streams;
	if (typeof payload.exit_code === "number") return `Exit code: ${payload.exit_code}`;
}
function sourceEventId(line) {
	return line.sourceEventId ?? `timestamp:${line.timestamp}:${line.type}`;
}
function appendSource(target, eventId) {
	if (!target.sourceEventIds.includes(eventId)) target.sourceEventIds.push(eventId);
}
function runtimeTimestamp(value, fallback) {
	if (typeof value !== "string") return fallback;
	const parsed = Date.parse(value);
	return Number.isFinite(parsed) ? parsed : fallback;
}
function responseItemId(value) {
	return typeof value.id === "string" && value.id ? value.id : void 0;
}
function newTurn(startTime) {
	return {
		turnId: void 0,
		startTime,
		endTime: startTime,
		steps: [],
		subagentThreadIds: [],
		subagents: [],
		contextCompactions: [],
		unmatchedToolResults: [],
		sourceEventIds: [],
		completed: false,
		aborted: false
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
function parseSession(lines) {
	let sessionMeta = {
		sessionId: "unknown",
		sourceEventIds: []
	};
	const turns = [];
	let turn = null;
	let step = null;
	let toolCallsById = /* @__PURE__ */ new Map();
	let lastTimestamp = Date.now();
	function newStep(startTime) {
		return {
			startTime,
			endTime: startTime,
			toolCalls: [],
			sourceEventIds: []
		};
	}
	const ensureTurn = (ts) => turn ??= newTurn(ts);
	const ensureStep = (ts) => step ??= newStep(ts);
	const recordSubagentThread = (threadId, callId, eventId) => {
		if (!turn.subagentThreadIds.includes(threadId)) turn.subagentThreadIds.push(threadId);
		const existing = turn.subagents.find((item) => item.threadId === threadId);
		if (existing) {
			existing.callId ??= callId;
			if (!existing.sourceEventIds.includes(eventId)) existing.sourceEventIds.push(eventId);
			return;
		}
		turn.subagents.push({
			threadId,
			callId,
			sourceEventIds: [eventId]
		});
	};
	const closeStep = (ts, usage$2) => {
		if (!step) return;
		step.endTime = Math.max(step.endTime, ts);
		if (usage$2) step.usage = usage$2;
		turn.steps.push(step);
		step = null;
	};
	const finishTurn = (ts, opts) => {
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
		toolCallsById = /* @__PURE__ */ new Map();
	};
	for (const line of lines) {
		const ts = Number.isFinite(Date.parse(line.timestamp)) ? Date.parse(line.timestamp) : lastTimestamp;
		lastTimestamp = ts;
		const eventId = sourceEventId(line);
		if (line.type === "session_meta") {
			const p = line.payload;
			sessionMeta = {
				sessionId: typeof p.id === "string" ? p.id : sessionMeta.sessionId,
				cliVersion: p.cli_version,
				modelProvider: p.model_provider ?? void 0,
				baseInstructions: p.base_instructions?.text,
				isSubagentThread: typeof p.parent_thread_id === "string" || p.thread_source === "subagent",
				parentThreadId: typeof p.parent_thread_id === "string" ? p.parent_thread_id : void 0,
				sourceEventIds: [...sessionMeta.sourceEventIds, eventId]
			};
			continue;
		}
		if (line.type === "turn_context") {
			const t = ensureTurn(ts);
			t.model = line.payload.model ?? t.model;
			t.invocationParams = line.payload;
			appendSource(t, eventId);
			continue;
		}
		if (line.type === "response_item") {
			const p = line.payload;
			ensureTurn(ts);
			if (p.type === "message") {
				const msg = p;
				const text = extractMessageText(msg.content);
				if (msg.role === "assistant") {
					const s = ensureStep(ts);
					s.modelCallId ??= responseItemId(p);
					appendSource(s, eventId);
					if (text) s.text = s.text ? `${s.text}\n${text}` : text;
				} else if (msg.role === "user" && text) {
					appendSource(turn, eventId);
					if (!turn.userInputFallback && !/^<(environment_context|user_instructions)/.test(text.trim())) turn.userInputFallback = text;
				}
			} else if (p.type === "function_call") {
				const call = p;
				const s = ensureStep(ts);
				const tc = {
					callId: call.call_id,
					responseItemId: call.id ?? void 0,
					name: call.name,
					type: "function",
					args: parseArgs(call.arguments),
					startTime: ts,
					sourceEventIds: [eventId]
				};
				s.modelCallId ??= call.id ?? void 0;
				appendSource(s, eventId);
				s.toolCalls.push(tc);
				if (tc.callId) toolCallsById.set(tc.callId, tc);
			} else if (p.type === "custom_tool_call") {
				const call = p;
				const s = ensureStep(ts);
				const tc = {
					callId: call.call_id,
					responseItemId: call.id ?? void 0,
					name: call.name,
					type: "custom",
					args: parseArgs(call.input),
					startTime: ts,
					sourceEventIds: [eventId]
				};
				s.modelCallId ??= call.id ?? void 0;
				appendSource(s, eventId);
				s.toolCalls.push(tc);
				if (tc.callId) toolCallsById.set(tc.callId, tc);
			} else if (p.type === "local_shell_call") {
				const call = p;
				const s = ensureStep(ts);
				const tc = {
					callId: call.call_id ?? call.id ?? "",
					responseItemId: call.id ?? void 0,
					name: "local_shell",
					type: "local_shell",
					args: call.action ?? void 0,
					startTime: ts,
					sourceEventIds: [eventId]
				};
				s.modelCallId ??= call.id ?? void 0;
				appendSource(s, eventId);
				s.toolCalls.push(tc);
				if (tc.callId) toolCallsById.set(tc.callId, tc);
			} else if (p.type === "web_search_call") {
				const call = p;
				const callId = call.id ?? "";
				const existing = callId ? toolCallsById.get(callId) : void 0;
				if (existing) {
					existing.args = existing.args ?? call.action ?? void 0;
					existing.endTime = Math.max(existing.endTime ?? ts, ts);
					if (call.status === "failed") existing.error = "web_search_call failed";
					appendSource(existing, eventId);
				} else {
					const s = ensureStep(ts);
					const tc = {
						callId,
						responseItemId: call.id ?? void 0,
						name: "web_search",
						type: "web_search",
						args: call.action ?? void 0,
						startTime: ts,
						...call.status === "completed" ? { endTime: ts } : {},
						...call.status === "failed" ? {
							endTime: ts,
							error: "web_search_call failed"
						} : {},
						sourceEventIds: [eventId]
					};
					s.modelCallId ??= call.id ?? void 0;
					appendSource(s, eventId);
					s.toolCalls.push(tc);
					if (callId) toolCallsById.set(callId, tc);
				}
			} else if (p.type === "file_search_call" || p.type === "computer_call") {
				const call = p;
				const s = ensureStep(ts);
				const callId = call.call_id ?? call.id ?? "";
				const tc = {
					callId,
					responseItemId: call.id ?? void 0,
					name: p.type === "file_search_call" ? "file_search" : "computer",
					type: p.type === "file_search_call" ? "file_search" : "computer",
					args: call.action ?? call.input,
					startTime: ts,
					...call.status === "completed" ? { endTime: ts } : {},
					...call.results !== void 0 ? { output: call.results } : {},
					...call.status === "failed" ? { error: `${p.type} failed` } : {},
					sourceEventIds: [eventId]
				};
				s.modelCallId ??= call.id ?? void 0;
				appendSource(s, eventId);
				s.toolCalls.push(tc);
				if (callId) toolCallsById.set(callId, tc);
			} else if (p.type === "function_call_output" || p.type === "custom_tool_call_output") {
				const out = p;
				const tc = toolCallsById.get(out.call_id);
				if (tc) {
					if (tc.output == null) tc.output = out.output;
					tc.endTime = Math.max(tc.endTime ?? ts, ts);
					appendSource(tc, eventId);
				} else turn.unmatchedToolResults.push({
					callId: out.call_id,
					resultType: String(p.type),
					output: out.output,
					error: out.call_id ? `unknown call_id ${out.call_id}` : "tool result has no call_id",
					timestamp: ts,
					sourceEventIds: [eventId]
				});
			} else if (p.type === "reasoning") {
				const reasoning = extractReasoning(p);
				if (reasoning) {
					const s = ensureStep(ts);
					s.modelCallId ??= responseItemId(p);
					appendSource(s, eventId);
					s.reasoning = s.reasoning ? `${s.reasoning}\n${reasoning}` : reasoning;
				}
			} else appendSource(turn, eventId);
			continue;
		}
		if (line.type === "compacted") {
			ensureTurn(ts).contextCompactions.push({
				timestamp: ts,
				eventType: "compacted",
				payload: line.payload,
				sourceEventIds: [eventId]
			});
			continue;
		}
		if (line.type === "event_msg") {
			const p = line.payload;
			const et = p.type;
			if (et === "task_started") {
				if (turn) finishTurn(ts, {
					completed: false,
					aborted: false
				});
				turn = newTurn(runtimeTimestamp(p.started_at, ts));
				turn.turnId = typeof p.turn_id === "string" ? p.turn_id : void 0;
				turn.traceId = typeof p.trace_id === "string" ? p.trace_id.toLowerCase() : void 0;
				appendSource(turn, eventId);
				continue;
			}
			if (!turn) continue;
			if (et === "user_message" && typeof p.message === "string") {
				appendSource(turn, eventId);
				if (!turn.userInput) turn.userInput = p.message;
			} else if (et === "agent_message" && typeof p.message === "string") {
				appendSource(turn, eventId);
				turn.lastAgentMessage = p.message;
			} else if (et === "token_count") {
				if (step) appendSource(step, eventId);
				if (p.info?.total_token_usage) turn.totalUsage = p.info.total_token_usage;
				closeStep(ts, p.info?.last_token_usage ?? void 0);
			} else if (et === "task_complete") {
				appendSource(turn, eventId);
				finishTurn(runtimeTimestamp(p.completed_at, ts), {
					completed: true,
					aborted: false
				});
			} else if (et === "turn_aborted") {
				appendSource(turn, eventId);
				finishTurn(ts, {
					completed: true,
					aborted: true
				});
			} else if (et === "context_compacted") turn.contextCompactions.push({
				timestamp: ts,
				eventType: et,
				payload: p,
				sourceEventIds: [eventId]
			});
			else if (et === "stream_error" || et === "model_retry" || et === "retry") {
				const s = ensureStep(ts);
				s.state = "retry";
				s.statusMessage = extractToolError(p) ?? toText(p.message ?? p.error ?? et);
				appendSource(s, eventId);
			} else {
				if (et === "collab_agent_spawn_end" && typeof p.new_thread_id === "string") recordSubagentThread(p.new_thread_id, p.call_id, eventId);
				if (et === "sub_agent_activity" && p.kind === "started" && typeof p.agent_thread_id === "string") recordSubagentThread(p.agent_thread_id, typeof p.event_id === "string" ? p.event_id : p.call_id, eventId);
				if ((et === "mcp_tool_call_begin" || et === "mcp_tool_call_end") && typeof p.call_id === "string") {
					const tc = toolCallsById.get(p.call_id);
					const inv = p.invocation;
					if (tc && typeof inv?.server === "string" && typeof inv?.tool === "string") {
						tc.mcp = {
							server: inv.server,
							tool: inv.tool
						};
						tc.type = "mcp";
						appendSource(tc, eventId);
					}
				}
				if (et === "web_search_begin" && typeof p.call_id === "string") {
					let tc = toolCallsById.get(p.call_id);
					if (!tc) {
						tc = {
							callId: p.call_id,
							name: "web_search",
							type: "web_search",
							args: p.action ?? (typeof p.query === "string" ? { query: p.query } : void 0),
							startTime: ts,
							sourceEventIds: [eventId]
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
							args: void 0,
							startTime: ts,
							sourceEventIds: [eventId]
						};
						const s = ensureStep(ts);
						appendSource(s, eventId);
						s.toolCalls.push(tc);
						toolCallsById.set(tc.callId, tc);
					} else appendSource(tc, eventId);
					tc.args = tc.args ?? p.action ?? (typeof p.query === "string" ? { query: p.query } : void 0);
					tc.endTime = Math.max(tc.endTime ?? ts, ts);
				}
				if (typeof p.call_id === "string" && et.endsWith("_begin") && et !== "web_search_begin") {
					const tc = toolCallsById.get(p.call_id);
					if (tc) {
						tc.startTime = Math.min(tc.startTime, ts);
						appendSource(tc, eventId);
					}
				}
				if (typeof p.call_id === "string" && et.endsWith("_end")) {
					const tc = toolCallsById.get(p.call_id);
					if (tc) {
						appendSource(tc, eventId);
						tc.endTime = Math.max(tc.endTime ?? ts, ts);
						if (p.status === "failed" || p.status === "declined") tc.error = extractToolError(p) ?? `tool ${p.status}`;
						if (tc.output == null) tc.output = p.aggregated_output ?? p.stdout ?? p.result;
					} else if (p.call_id && et !== "collab_agent_spawn_end") turn.unmatchedToolResults.push({
						callId: p.call_id,
						resultType: et,
						output: p.aggregated_output ?? p.stdout ?? p.result,
						error: extractToolError(p) ?? `unknown call_id ${p.call_id}`,
						timestamp: ts,
						sourceEventIds: [eventId]
					});
				}
			}
			continue;
		}
	}
	if (turn) finishTurn(lastTimestamp, {
		completed: false,
		aborted: false
	});
	return {
		sessionMeta,
		turns
	};
}

//#endregion
//#region src/codex-graph.ts
const CODEX_UPSTREAM_COMMIT = "7500867afecf963d1cf83bf2b860a659591ace18";
const CODEX_PARSER_NAME = "langfuse-codex-derived@7500867";
function isTraceId(value) {
	return typeof value === "string" && /^[0-9a-f]{32}$/.test(value) && !/^0+$/.test(value);
}
function traceIdFromRuntimeCorrelation(sessionId, turn) {
	if (isTraceId(turn.traceId)) return turn.traceId;
	return createHash("sha256").update(`catena:codex:${sessionId}:${turn.turnId}`).digest("hex").slice(0, 32);
}
function unique(values) {
	return [...new Set(values.filter((value) => typeof value === "string" && value.length > 0))];
}
function safeEnd(start, end) {
	return Math.max(start, end ?? start);
}
function toolType(tool) {
	if (tool.mcp) return "mcp";
	const name = tool.name.toLowerCase();
	if (tool.type !== "function") return tool.type;
	if (name.includes("web_search")) return "web_search";
	if (name.includes("file_search")) return "file_search";
	if (name.includes("computer")) return "computer";
	if (name.includes("shell") || name.includes("exec_command")) return "local_shell";
	return "function";
}
function toolName(tool) {
	return tool.mcp ? `${tool.mcp.server}.${tool.mcp.tool}` : tool.name || "tool";
}
function usage$1(step) {
	if (!step.usage) return void 0;
	const values = {};
	for (const [key, value] of Object.entries(step.usage)) if (typeof value === "number" && Number.isSafeInteger(value) && value >= 0) values[key] = value;
	return Object.keys(values).length > 0 ? values : void 0;
}
function stepOutput(step) {
	const value = {};
	if (step.text) value.content = step.text;
	if (step.reasoning) value.reasoning = step.reasoning;
	if (step.toolCalls.length > 0) value.tool_calls = step.toolCalls.map((tool) => ({
		id: tool.callId || void 0,
		name: toolName(tool),
		type: toolType(tool),
		arguments: tool.args
	}));
	return Object.keys(value).length > 0 ? value : void 0;
}
function toolResultInput(tools) {
	if (tools.length === 0) return void 0;
	return tools.map((tool) => ({
		call_id: tool.callId || void 0,
		name: toolName(tool),
		output: tool.output,
		error: tool.error
	}));
}
function appendModelAndToolNodes(nodes, turn, parentKey, keyPrefix, modelName) {
	const toolKeys = /* @__PURE__ */ new Map();
	let previousTools = [];
	let failed = false;
	let incomplete = false;
	turn.steps.forEach((step, stepIndex) => {
		const modelKey = `${keyPrefix}:model:${step.modelCallId || `step:${stepIndex + 1}`}`;
		const modelState = step.state ?? "ok";
		if (modelState === "error") failed = true;
		const modelNode = {
			key: modelKey,
			parent_key: parentKey,
			kind: step.state === "retry" ? "retry" : "model",
			name: step.state === "retry" ? "gen_ai.model.retry" : "gen_ai.model.call",
			...step.modelCallId ? { runtime_id: step.modelCallId } : {},
			start_time_unix_nano: millisecondsToNanoseconds(step.startTime),
			end_time_unix_nano: millisecondsToNanoseconds(safeEnd(step.startTime, step.endTime)),
			state: modelState,
			...step.statusMessage ? { status_message: step.statusMessage } : {},
			input: stepIndex === 0 ? turn.userInput : toolResultInput(previousTools),
			output: stepOutput(step),
			model: modelName,
			...usage$1(step) ? { usage: usage$1(step) } : {},
			attributes: { "catena.model.step.index": stepIndex },
			source_event_ids: unique(step.sourceEventIds)
		};
		nodes.push(modelNode);
		step.toolCalls.forEach((tool, toolIndex) => {
			const key = `${keyPrefix}:tool:${tool.callId || tool.responseItemId || tool.sourceEventIds[0] || `index:${toolIndex}`}`;
			if (tool.callId) toolKeys.set(tool.callId, key);
			const missingResult = tool.endTime == null;
			const state = tool.error ? "error" : missingResult ? "incomplete" : "ok";
			if (tool.error) failed = true;
			if (missingResult) incomplete = true;
			nodes.push({
				key,
				parent_key: modelKey,
				kind: "tool",
				name: `agent.tool.call ${toolName(tool)}`,
				...tool.callId ? { runtime_id: tool.callId } : {},
				start_time_unix_nano: millisecondsToNanoseconds(tool.startTime),
				end_time_unix_nano: millisecondsToNanoseconds(safeEnd(tool.startTime, tool.endTime ?? step.endTime)),
				state,
				...tool.error ? { status_message: tool.error } : missingResult ? { status_message: "tool result not present in rollout" } : {},
				input: tool.args,
				output: tool.output,
				attributes: {
					"gen_ai.tool.type": toolType(tool),
					"gen_ai.tool.name": toolName(tool),
					...tool.callId ? { "gen_ai.tool.call.id": tool.callId } : {},
					...tool.responseItemId ? { "codex.response_item.id": tool.responseItemId } : {},
					...tool.mcp ? {
						"mcp.server": tool.mcp.server,
						"mcp.tool": tool.mcp.tool
					} : {}
				},
				source_event_ids: unique(tool.sourceEventIds)
			});
		});
		previousTools = step.toolCalls;
	});
	return {
		toolKeys,
		failed,
		incomplete
	};
}
async function readRollout(file, prefix = "main") {
	const raw = await fs.readFile(file, "utf-8");
	const lines = [];
	const records = [];
	raw.split("\n").forEach((text, index) => {
		if (!text.trim()) return;
		const eventId = `${prefix}:line:${index + 1}`;
		try {
			const value = JSON.parse(text);
			if (!value || typeof value !== "object") {
				records.push({
					eventId,
					type: "malformed:not-object"
				});
				return;
			}
			value.sourceEventId = eventId;
			lines.push(value);
			const payloadType = value.payload && typeof value.payload === "object" && "type" in value.payload ? String(value.payload.type ?? "") : "";
			records.push({
				eventId,
				type: payloadType ? `${value.type}:${payloadType}` : value.type
			});
		} catch {
			records.push({
				eventId,
				type: "malformed:json"
			});
		}
	});
	return {
		file,
		lines,
		records
	};
}
async function findSubagentRollout(parentFile, threadId) {
	const suffix = `-${threadId}.jsonl`;
	const root = path.resolve(path.dirname(parentFile), "../../..");
	async function walk(directory) {
		let entries;
		try {
			entries = await fs.readdir(directory, { withFileTypes: true });
		} catch {
			return;
		}
		for (const entry of entries) {
			const full = path.join(directory, entry.name);
			if (entry.isDirectory()) {
				const nested = await walk(full);
				if (nested) return nested;
			} else if (entry.isFile() && entry.name.endsWith(suffix)) return full;
		}
	}
	return walk(root);
}
async function appendSubagent(build, parentRollout, link, parentTurn, parentKey, toolKeys, depth) {
	const threadKey = `${parentKey}:subagent-thread:${link.threadId}`;
	if (link.callId && !toolKeys.has(link.callId)) throw new Error(`subagent ${link.threadId} references unknown spawning call_id ${link.callId}`);
	const parent = link.callId ? toolKeys.get(link.callId) : parentKey;
	const childFile = await findSubagentRollout(parentRollout, link.threadId);
	const threadNode = {
		key: threadKey,
		parent_key: parent,
		kind: "subagent",
		name: "agent.subagent.thread",
		runtime_id: link.threadId,
		start_time_unix_nano: millisecondsToNanoseconds(parentTurn.startTime),
		end_time_unix_nano: millisecondsToNanoseconds(parentTurn.endTime),
		state: childFile ? "ok" : "incomplete",
		...!childFile ? { status_message: "subagent rollout not found" } : {},
		attributes: {
			"agent.subagent.thread.id": link.threadId,
			...link.callId ? { "agent.subagent.spawn.call_id": link.callId } : {}
		},
		source_event_ids: unique(link.sourceEventIds)
	};
	build.trace.nodes.push(threadNode);
	if (!childFile || depth >= 8) return;
	const document = await readRollout(childFile, `subagent:${link.threadId}`);
	build.sourceRecords.push(...document.records);
	const parsed = parseSession(document.lines);
	if (parsed.turns.length > 0) {
		threadNode.start_time_unix_nano = millisecondsToNanoseconds(Math.min(...parsed.turns.map((childTurn) => childTurn.startTime)));
		threadNode.end_time_unix_nano = millisecondsToNanoseconds(Math.max(...parsed.turns.map((childTurn) => childTurn.endTime)));
		const states = parsed.turns.map(turnState);
		threadNode.state = states.includes("aborted") ? "aborted" : states.includes("error") ? "error" : states.includes("incomplete") ? "incomplete" : "ok";
	}
	for (const [turnIndex, childTurn] of parsed.turns.entries()) {
		if (!childTurn.turnId) throw new Error(`Codex subagent rollout ${childFile} turn ${turnIndex} has no Runtime turn_id`);
		const childTurnId = childTurn.turnId;
		const childKey = `${threadKey}:turn:${childTurnId}`;
		const childState = turnState(childTurn);
		build.trace.nodes.push({
			key: childKey,
			parent_key: threadKey,
			kind: "subagent",
			name: "agent.subagent.turn",
			...childTurn.turnId ? { runtime_id: childTurn.turnId } : {},
			start_time_unix_nano: millisecondsToNanoseconds(childTurn.startTime),
			end_time_unix_nano: millisecondsToNanoseconds(childTurn.endTime),
			state: childState,
			...childTurn.aborted ? { status_message: "subagent turn aborted" } : {},
			input: childTurn.userInput,
			output: childTurn.finalOutput,
			attributes: {
				"agent.subagent.thread.id": parsed.sessionMeta.sessionId,
				...childTurn.turnId ? { "agent.turn.id": childTurn.turnId } : {},
				...childTurn.traceId ? { "codex.trace_id": childTurn.traceId } : {},
				"catena.trace.correlation": `${parsed.sessionMeta.sessionId}:${childTurnId}`
			},
			source_event_ids: unique([...parsed.sessionMeta.sourceEventIds, ...childTurn.sourceEventIds])
		});
		const children = appendModelAndToolNodes(build.trace.nodes, childTurn, childKey, childKey, childTurn.model ?? "unknown");
		for (const nested of childTurn.subagents) await appendSubagent(build, childFile, nested, childTurn, childKey, children.toolKeys, depth + 1);
	}
}
function mapMainEventsToTurns(lines, turns) {
	const result = /* @__PURE__ */ new Map();
	let current = turns[0]?.turnId ?? "";
	let turnIndex = 0;
	for (const line of lines) {
		const eventId = line.sourceEventId;
		const payload = line.payload;
		if (line.type === "event_msg" && payload.type === "task_started") {
			current = typeof payload.turn_id === "string" ? payload.turn_id : turns[turnIndex]?.turnId ?? "";
			turnIndex += 1;
		}
		if (eventId && current) result.set(eventId, current);
	}
	return result;
}
function finalizeAccounting(trace, records) {
	const priorities = {
		unmatched_tool_result: 7,
		tool: 6,
		context_compact: 5,
		retry: 4,
		model: 3,
		subagent: 2,
		turn: 1
	};
	const primary = /* @__PURE__ */ new Map();
	for (const node of [...trace.nodes].sort((a, b) => priorities[b.kind] - priorities[a.kind])) for (const eventId of node.source_event_ids) if (!primary.has(eventId)) primary.set(eventId, node.key);
	const seen = /* @__PURE__ */ new Set();
	const accounting = [];
	for (const record of records) {
		if (seen.has(record.eventId)) continue;
		seen.add(record.eventId);
		const nodeKey = primary.get(record.eventId);
		accounting.push(nodeKey ? {
			event_id: record.eventId,
			disposition: "span",
			node_key: nodeKey
		} : {
			event_id: record.eventId,
			disposition: "ignored",
			reason: `record ${record.type} has no canonical semantic node`
		});
	}
	trace.accounting = accounting;
}
function turnState(turn) {
	if (turn.aborted) return "aborted";
	if (!turn.completed) return "incomplete";
	const tools = turn.steps.flatMap((step) => step.toolCalls);
	if (turn.unmatchedToolResults.length > 0 || tools.some((tool) => Boolean(tool.error))) return "error";
	if (tools.some((tool) => tool.endTime == null)) return "incomplete";
	return "ok";
}
async function parseCodexRollout(rolloutFile) {
	const document = await readRollout(rolloutFile);
	const { sessionMeta, turns } = parseSession(document.lines);
	if (!sessionMeta.sessionId || sessionMeta.sessionId === "unknown") throw new Error("Codex rollout has no Runtime session id");
	const eventTurns = mapMainEventsToTurns(document.lines, turns);
	const builds = [];
	for (const [turnIndex, turn] of turns.entries()) {
		if (!turn.turnId) throw new Error(`Codex rollout turn ${turnIndex} has no Runtime turn_id`);
		const turnId = turn.turnId;
		const traceId = traceIdFromRuntimeCorrelation(sessionMeta.sessionId, turn);
		const state = turnState(turn);
		const turnKey = `turn:${turnId}`;
		const rootSources = unique([...turnIndex === 0 ? sessionMeta.sourceEventIds : [], ...turn.sourceEventIds]);
		const trace = {
			trace_id: traceId,
			turn_id: turnId,
			state,
			nodes: [{
				key: turnKey,
				kind: "turn",
				name: "agent.turn",
				...turn.turnId ? { runtime_id: turn.turnId } : {},
				start_time_unix_nano: millisecondsToNanoseconds(turn.startTime),
				end_time_unix_nano: millisecondsToNanoseconds(turn.endTime),
				state,
				...state === "aborted" ? { status_message: "turn aborted by Runtime" } : state === "incomplete" ? { status_message: "turn is incomplete" } : state === "error" ? { status_message: "turn contains failed or unmatched tool evidence" } : {},
				input: turn.userInput,
				output: turn.finalOutput,
				attributes: {
					"agent.session.id": sessionMeta.sessionId,
					...turn.turnId ? { "agent.turn.id": turn.turnId } : {},
					...turn.traceId ? { "codex.trace_id": turn.traceId } : {},
					"catena.trace.correlation": `${sessionMeta.sessionId}:${turnId}`,
					"catena.trace.id.source": isTraceId(turn.traceId) ? "runtime_trace_id" : "runtime_session_turn_correlation",
					"codex.cli.version": sessionMeta.cliVersion ?? "",
					"codex.model.provider": sessionMeta.modelProvider ?? "",
					"codex.aborted": turn.aborted,
					"codex.completed": turn.completed
				},
				source_event_ids: rootSources
			}],
			accounting: []
		};
		const build = {
			trace,
			sourceRecords: []
		};
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
				source_event_ids: unique(compaction.sourceEventIds)
			});
		});
		turn.unmatchedToolResults.forEach((result, index) => {
			trace.nodes.push({
				key: `${turnKey}:unmatched:${result.callId || result.sourceEventIds[0] || index}`,
				parent_key: turnKey,
				kind: "unmatched_tool_result",
				name: "agent.tool.result.unmatched",
				...result.callId ? { runtime_id: result.callId } : {},
				start_time_unix_nano: millisecondsToNanoseconds(result.timestamp),
				end_time_unix_nano: millisecondsToNanoseconds(result.timestamp),
				state: "error",
				status_message: result.error ?? "unmatched tool result",
				output: result.output,
				attributes: {
					"gen_ai.tool.result.type": result.resultType,
					...result.callId ? { "gen_ai.tool.call.id": result.callId } : {}
				},
				source_event_ids: unique(result.sourceEventIds)
			});
		});
		for (const link of turn.subagents) await appendSubagent(build, rolloutFile, link, turn, turnKey, children.toolKeys, 0);
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
			upstream_commit: CODEX_UPSTREAM_COMMIT
		},
		traces: builds.map((build) => build.trace)
	};
}
function canonicalGraphJSON(graph) {
	return `${JSON.stringify(graph, null, 2)}\n`;
}

//#endregion
//#region src/otlp.ts
const CATENA_RUNTIME_VERSION = "0.2.0";
function stableSpanId(runtime, sessionId, traceId, key) {
	return createHash("sha256").update(`catena:${runtime}:${sessionId}:${traceId}:${key}`).digest("hex").slice(0, 16);
}
function protoBytes(hex) {
	return Buffer.from(hex, "hex").toString("base64");
}
function anyValue(value) {
	if (typeof value === "boolean") return { boolValue: value };
	if (typeof value === "number" && Number.isSafeInteger(value)) return { intValue: String(value) };
	if (typeof value === "number") return { doubleValue: value };
	return { stringValue: typeof value === "string" ? value : canonicalString(value) };
}
function attributes(values) {
	return Object.entries(values).filter(([, value]) => value !== void 0 && value !== null && value !== "").map(([key, value]) => ({
		key,
		value: anyValue(value)
	}));
}
function nodeAttributes(graph, trace, node) {
	const values = {
		"agent.runtime": graph.runtime,
		"agent.session.id": graph.session_id,
		"agent.turn.id": trace.turn_id,
		"catena.canonical.schema": graph.schema_version,
		"catena.node.key": node.key,
		"catena.node.kind": node.kind,
		"catena.state": node.state,
		"catena.source.event.ids": JSON.stringify(node.source_event_ids),
		...node.attributes
	};
	if (node.runtime_id) values["catena.runtime.id"] = node.runtime_id;
	if (node.input !== void 0) values["input.value"] = canonicalString(node.input);
	if (node.output !== void 0) values["output.value"] = canonicalString(node.output);
	if (node.model) {
		values["gen_ai.request.model"] = node.model;
		values["gen_ai.response.model"] = node.model;
	}
	if (node.kind === "tool" || node.kind === "unmatched_tool_result") {
		const callId = node.attributes["gen_ai.tool.call.id"] ?? node.runtime_id;
		if (callId) values["gen_ai.tool.call.id"] = callId;
		if (node.input !== void 0) {
			values["gen_ai.tool.call.arguments"] = canonicalString(node.input);
			values["tool.call.arguments"] = canonicalString(node.input);
		}
		if (node.output !== void 0) {
			values["gen_ai.tool.call.result"] = canonicalString(node.output);
			values["tool.call.result"] = canonicalString(node.output);
		}
	}
	for (const [key, value] of Object.entries(node.usage ?? {})) values[`gen_ai.usage.${key}`] = value;
	return values;
}
function spanKind(node) {
	return node.kind === "model" || node.kind === "retry" ? 3 : 1;
}
function traceToOTLP(graph, trace) {
	const spanIds = /* @__PURE__ */ new Map();
	for (const node of trace.nodes) {
		if (spanIds.has(node.key)) throw new Error(`duplicate canonical node key ${node.key}`);
		spanIds.set(node.key, stableSpanId(graph.runtime, graph.session_id, trace.trace_id, node.key));
	}
	const spans = trace.nodes.map((node) => {
		const parentSpanId = node.parent_key ? spanIds.get(node.parent_key) : void 0;
		if (node.parent_key && !parentSpanId) throw new Error(`canonical parent ${node.parent_key} is missing for ${node.key}`);
		const value = {
			traceId: protoBytes(trace.trace_id),
			spanId: protoBytes(spanIds.get(node.key)),
			name: node.name,
			kind: spanKind(node),
			startTimeUnixNano: node.start_time_unix_nano,
			endTimeUnixNano: node.end_time_unix_nano,
			attributes: attributes(nodeAttributes(graph, trace, node)),
			status: {
				code: node.state === "ok" ? 1 : 2,
				...node.status_message ? { message: node.status_message } : {}
			}
		};
		if (parentSpanId) value.parentSpanId = protoBytes(parentSpanId);
		return value;
	});
	return { resourceSpans: [{
		resource: { attributes: attributes({
			"service.name": `catena-runtime-${graph.runtime}`,
			"agent.runtime": graph.runtime,
			"agent.session.id": graph.session_id,
			"telemetry.sdk.name": "catena-runtime",
			"telemetry.sdk.language": graph.runtime === "codex" ? "typescript" : "python",
			"telemetry.sdk.version": CATENA_RUNTIME_VERSION
		}) },
		scopeSpans: [{
			scope: {
				name: "catena.runtime",
				version: CATENA_RUNTIME_VERSION
			},
			spans
		}]
	}] };
}
function endpointFromEnvironment(environment = process.env) {
	const explicit = environment.CATENA_OTLP_ENDPOINT?.trim();
	if (explicit) return explicit;
	return `${(environment.CATENA_URL?.trim() || "http://127.0.0.1:5570").replace(/\/$/, "")}/v1/otlp/v1/traces`;
}
function transientStatus(status) {
	return status === 408 || status === 425 || status === 429 || status >= 500;
}
async function delay(milliseconds) {
	await new Promise((resolve) => setTimeout(resolve, milliseconds));
}
async function sendOTLP(payload, options) {
	const attempts = Math.max(1, options.attempts ?? 3);
	for (let attempt = 1; attempt <= attempts; attempt += 1) {
		try {
			const response = await fetch(options.endpoint, {
				method: "POST",
				headers: {
					Authorization: `Bearer ${options.apiKey}`,
					"Content-Type": "application/json",
					"User-Agent": `catena-runtime/${CATENA_RUNTIME_VERSION}`
				},
				body: JSON.stringify(payload),
				signal: AbortSignal.timeout(options.timeoutMs ?? 4e3)
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
async function exportGraph(graph, options, traces = graph.traces) {
	const uploaded = [];
	const failed = [];
	for (const trace of traces) (await sendOTLP(traceToOTLP(graph, trace), options) ? uploaded : failed).push(trace.turn_id);
	return {
		uploaded,
		failed
	};
}

//#endregion
//#region src/state.ts
function ledgerPath(rolloutFile) {
	return `${rolloutFile}.catena`;
}
function emptyLedger() {
	return {
		version: 1,
		completed: {},
		observed: {}
	};
}
async function loadLedger(rolloutFile) {
	try {
		const value = JSON.parse(await fs.readFile(ledgerPath(rolloutFile), "utf-8"));
		return {
			version: 1,
			completed: value.completed && typeof value.completed === "object" ? value.completed : {},
			observed: value.observed && typeof value.observed === "object" ? value.observed : {}
		};
	} catch (error) {
		if (error.code === "ENOENT") return emptyLedger();
		throw error;
	}
}
async function saveLedger(rolloutFile, ledger) {
	const file = ledgerPath(rolloutFile);
	const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
	await fs.writeFile(temporary, `${JSON.stringify(ledger, null, 2)}\n`, {
		encoding: "utf-8",
		mode: 384
	});
	await fs.rename(temporary, file);
}
function traceDigest(trace) {
	return createHash("sha256").update(JSON.stringify(trace)).digest("hex");
}
function tracesNeedingUpload(traces, ledger) {
	return traces.filter((trace) => {
		if (ledger.completed[trace.turn_id] === trace.trace_id) return false;
		const observed = ledger.observed[trace.turn_id];
		return !observed || observed.trace_id !== trace.trace_id || observed.digest !== traceDigest(trace);
	});
}
async function recordUploadedTraces(rolloutFile, ledger, traces) {
	for (const trace of traces) {
		ledger.observed[trace.turn_id] = {
			trace_id: trace.trace_id,
			digest: traceDigest(trace),
			state: trace.state
		};
		if (trace.state !== "incomplete") ledger.completed[trace.turn_id] = trace.trace_id;
	}
	await saveLedger(rolloutFile, ledger);
}
async function wait$1(milliseconds) {
	await new Promise((resolve) => setTimeout(resolve, milliseconds));
}
async function withLedgerLock(rolloutFile, callback) {
	const lockFile = `${ledgerPath(rolloutFile)}.lock`;
	let handle;
	for (let attempt = 0; attempt < 40; attempt += 1) try {
		handle = await fs.open(lockFile, "wx", 384);
		break;
	} catch (error) {
		if (error.code !== "EEXIST") throw error;
		try {
			const stat$1 = await fs.stat(lockFile);
			if (Date.now() - stat$1.mtimeMs > 6e4) await fs.unlink(lockFile);
		} catch {}
		await wait$1(50);
	}
	if (!handle) throw new Error(`could not acquire Catena rollout state lock ${lockFile}`);
	try {
		return await callback();
	} finally {
		await handle.close().catch(() => void 0);
		await fs.unlink(lockFile).catch(() => void 0);
	}
}

//#endregion
//#region src/runtime.ts
async function writePluginCredentials(pluginData, credentials) {
	const apiKey = credentials.apiKey.trim();
	const url = credentials.url.trim().replace(/\/$/, "");
	if (!apiKey) throw new Error("Catena Agent API key is required");
	if (!url || !["http:", "https:"].includes(new URL(url).protocol)) throw new Error("Catena URL must be an absolute HTTP(S) URL");
	await mkdir(pluginData, {
		recursive: true,
		mode: 448
	});
	if (process.platform !== "win32") await chmod(pluginData, 448);
	const destination = join(pluginData, "credentials.json");
	const temporary = join(pluginData, `.credentials.${process.pid}.${Date.now()}.tmp`);
	try {
		await writeFile(temporary, `${JSON.stringify({
			url,
			api_key: apiKey
		}, null, 2)}\n`, {
			encoding: "utf-8",
			flag: "wx",
			mode: 384
		});
		if (process.platform !== "win32") await chmod(temporary, 384);
		await rename(temporary, destination);
	} catch (error) {
		await unlink(temporary).catch(() => void 0);
		throw error;
	}
	return destination;
}
function credentialsPath(environment) {
	const explicit = environment.CATENA_CREDENTIALS_FILE?.trim();
	if (explicit) return explicit;
	const pluginData = environment.PLUGIN_DATA?.trim();
	return pluginData ? join(pluginData, "credentials.json") : void 0;
}
async function environmentWithCredentials(environment) {
	if (environment.CATENA_API_KEY?.trim()) return environment;
	const file = credentialsPath(environment);
	if (!file) return environment;
	let metadata;
	try {
		metadata = await stat(file);
	} catch (error) {
		if (error.code === "ENOENT") return environment;
		throw error;
	}
	if (!metadata.isFile()) throw new Error(`Catena credentials path is not a file: ${file}`);
	if (process.platform !== "win32" && (metadata.mode & 63) !== 0) throw new Error(`Catena credentials must be readable only by the current user (chmod 600): ${file}`);
	const credentials = JSON.parse(await readFile(file, "utf-8"));
	const apiKey = typeof credentials.api_key === "string" ? credentials.api_key.trim() : "";
	if (!apiKey) throw new Error(`Catena credentials do not contain api_key: ${file}`);
	const url = typeof credentials.url === "string" ? credentials.url.trim() : "";
	return {
		...environment,
		CATENA_API_KEY: apiKey,
		...!environment.CATENA_URL?.trim() && !environment.CATENA_OTLP_ENDPOINT?.trim() && url ? { CATENA_URL: url } : {}
	};
}
function wait(milliseconds) {
	return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
function debugEnabled(environment) {
	return [
		"1",
		"true",
		"yes",
		"on"
	].includes((environment.CATENA_TRACE_DEBUG ?? "").toLowerCase());
}
function exportOptions(environment) {
	const apiKey = environment.CATENA_API_KEY?.trim();
	if (!apiKey) return void 0;
	return {
		endpoint: endpointFromEnvironment(environment),
		apiKey,
		debug: debugEnabled(environment)
	};
}
async function runCodexHook(input, environment = process.env) {
	if (!input.transcript_path) return {
		parsed: 0,
		uploaded: 0,
		skipped: 0,
		failed: 0
	};
	const options = exportOptions(await environmentWithCredentials(environment));
	if (!options) return {
		parsed: 0,
		uploaded: 0,
		skipped: 0,
		failed: 0
	};
	return withLedgerLock(input.transcript_path, async () => {
		const graph = await parseCodexRollout(input.transcript_path);
		if (input.session_id && input.session_id !== graph.session_id) throw new Error(`Codex hook session_id ${input.session_id} does not match rollout ${graph.session_id}`);
		const ledger = await loadLedger(input.transcript_path);
		const candidates = tracesNeedingUpload(graph.traces, ledger);
		const result = await exportGraph(graph, options, candidates);
		const uploaded = candidates.filter((trace) => result.uploaded.includes(trace.turn_id));
		if (uploaded.length > 0) await recordUploadedTraces(input.transcript_path, ledger, uploaded);
		return {
			parsed: graph.traces.length,
			uploaded: uploaded.length,
			skipped: graph.traces.length - candidates.length,
			failed: result.failed.length
		};
	});
}
async function settleCodexHook(input, environment = process.env, attempts = 20, delayMs = 100) {
	if (!input.transcript_path || !input.turn_id) return;
	for (let attempt = 0; attempt < attempts; attempt += 1) {
		const target = (await parseCodexRollout(input.transcript_path)).traces.find((trace) => trace.turn_id === input.turn_id);
		if (target && target.state !== "incomplete") {
			await runCodexHook(input, environment);
			return;
		}
		await wait(delayMs);
	}
	await runCodexHook(input, environment);
}
async function importCodexRollout(rolloutFile, options = {}) {
	const graph = await parseCodexRollout(rolloutFile);
	const traces = options.traceId ? graph.traces.filter((trace) => trace.trace_id === options.traceId) : graph.traces;
	const output = options.output === "otlp" ? `${JSON.stringify(traces.map((trace) => traceToOTLP(graph, trace)), null, 2)}\n` : canonicalGraphJSON({
		...graph,
		traces
	});
	if (!options.upload) return {
		graph: {
			...graph,
			traces
		},
		output,
		uploaded: [],
		failed: []
	};
	const exporter = exportOptions(await environmentWithCredentials(options.environment ?? process.env));
	if (!exporter) throw new Error("CATENA_API_KEY is required for historical upload");
	const result = await exportGraph(graph, exporter, traces);
	return {
		graph: {
			...graph,
			traces
		},
		output,
		...result
	};
}

//#endregion
//#region src/index.ts
async function readStdin() {
	const chunks = [];
	for await (const chunk of process.stdin) chunks.push(Buffer.from(chunk));
	const text = Buffer.concat(chunks).toString("utf-8").trim();
	if (!text) throw new Error("hook stdin is empty");
	return JSON.parse(text);
}
function usage() {
	console.error("usage: catena-codex-hook import <rollout.jsonl> [--otlp] [--upload] [--trace-id <id>]\n       catena-codex-hook configure --plugin-data <directory> --url <catena-url>");
	process.exit(2);
}
function option(args, name) {
	const index = args.indexOf(name);
	return index >= 0 ? args[index + 1] : void 0;
}
async function runConfigure(args) {
	const pluginData = option(args, "--plugin-data") ?? process.env.PLUGIN_DATA;
	const url = option(args, "--url");
	if (!pluginData || !url) usage();
	const input = await readStdin();
	const destination = await writePluginCredentials(pluginData, {
		apiKey: typeof input.token === "string" ? input.token : typeof input.api_key === "string" ? input.api_key : "",
		url
	});
	process.stdout.write(`Catena Codex credentials configured at ${destination}\n`);
}
async function runImport(args) {
	const file = args[0];
	if (!file) usage();
	await readFile(file, "utf-8");
	const traceIndex = args.indexOf("--trace-id");
	const traceId = traceIndex >= 0 ? args[traceIndex + 1] : void 0;
	if (traceIndex >= 0 && !traceId) usage();
	const result = await importCodexRollout(file, {
		output: args.includes("--otlp") ? "otlp" : "canonical",
		upload: args.includes("--upload"),
		traceId
	});
	process.stdout.write(result.output);
	if (result.failed.length > 0) process.exitCode = 1;
}
function scheduleSettle(input) {
	if (!input.turn_id || (input.hook_event_name ?? "Stop").toLowerCase() !== "stop") return;
	const encoded = Buffer.from(JSON.stringify(input)).toString("base64url");
	spawn(process.execPath, [
		fileURLToPath(import.meta.url),
		"settle-hook",
		encoded
	], {
		detached: true,
		stdio: "ignore",
		env: process.env
	}).unref();
}
async function runSettle(encoded) {
	if (!encoded) return;
	await settleCodexHook(JSON.parse(Buffer.from(encoded, "base64url").toString("utf-8")));
}
async function main() {
	if (process.argv[2] === "configure") {
		await runConfigure(process.argv.slice(3));
		return;
	}
	if (process.argv[2] === "import") {
		await runImport(process.argv.slice(3));
		return;
	}
	if (process.argv[2] === "settle-hook") {
		await runSettle(process.argv[3]);
		return;
	}
	let input;
	try {
		input = await readStdin();
		await runCodexHook(input);
	} catch (error) {
		if (process.env.CATENA_TRACE_DEBUG === "true") console.error("[catena-runtime] Codex hook failed open", error);
	} finally {
		if (input) scheduleSettle(input);
	}
}
main().catch((error) => {
	if (["configure", "import"].includes(process.argv[2] ?? "")) {
		console.error(error instanceof Error ? error.message : String(error));
		process.exitCode = 1;
	}
});

//#endregion
export {  };