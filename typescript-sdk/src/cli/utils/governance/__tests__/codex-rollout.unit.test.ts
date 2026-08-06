import { describe, expect, it } from "vitest";
import {
  type CodexChatMessage,
  parseCodexRollout,
  syntheticCodexTraceId,
} from "../codex-rollout";
import { buildCodexIOExportRequest } from "../codex-rollout-otlp";

const line = (obj: unknown) => JSON.stringify(obj);

function rollout(...objs: unknown[]): string {
  return objs.map(line).join("\n");
}

const taskStarted = (traceId: string, turnId: string, startedAt = 1_780_000_000) =>
  ({ type: "event_msg", payload: { type: "task_started", turn_id: turnId, trace_id: traceId, started_at: startedAt } });
const turnContext = (turnId: string, model = "gpt-5.5") =>
  ({ type: "turn_context", payload: { turn_id: turnId, model } });
const developerMsg = (text: string) =>
  ({ type: "response_item", payload: { type: "message", role: "developer", content: [{ type: "input_text", text }] } });
const userMsg = (text: string) =>
  ({ type: "response_item", payload: { type: "message", role: "user", content: [{ type: "input_text", text }] } });
const assistantMsg = (text: string) =>
  ({ type: "response_item", payload: { type: "message", role: "assistant", content: [{ type: "output_text", text }] } });
const agentMessage = (message: string) =>
  ({ type: "event_msg", payload: { type: "agent_message", message, phase: "final_answer" } });
const taskComplete = (turnId: string, completedAt: number) =>
  ({ type: "event_msg", payload: { type: "task_complete", turn_id: turnId, completed_at: completedAt } });
const functionCall = (
  name: string,
  args: string,
  callId: string,
  timestamp?: string,
) => ({
  type: "response_item",
  ...(timestamp ? { timestamp } : {}),
  payload: { type: "function_call", name, arguments: args, call_id: callId },
});
const functionCallOutput = (
  callId: string,
  output: string,
  timestamp?: string,
) => ({
  type: "response_item",
  ...(timestamp ? { timestamp } : {}),
  payload: { type: "function_call_output", call_id: callId, output },
});

const lastUser = (messages: CodexChatMessage[]) =>
  [...messages].reverse().find((m) => m.role === "user")?.content;

describe("parseCodexRollout", () => {
  describe("given a single-turn rollout", () => {
    describe("when it is parsed", () => {
      /** @scenario "A single-turn rollout yields the request body as chat messages on the turn's trace" */
      it("produces one turn carrying the turn's trace_id, request messages, and output", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("abc123", "t1"),
            turnContext("t1"),
            userMsg("list the files"),
            assistantMsg("a.txt b.txt"),
          ),
        );

        expect(turns).toHaveLength(1);
        expect(turns[0]).toMatchObject({
          traceId: "abc123",
          output: "a.txt b.txt",
          model: "gpt-5.5",
        });
        expect(turns[0]!.inputMessages).toEqual([
          { role: "user", content: "list the files" },
        ]);
        expect(turns[0]!.startedAtMs).toBe(1_780_000_000 * 1000);
      });
    });
  });

  describe("given a developer message", () => {
    describe("when it is parsed", () => {
      /** @scenario "The developer message becomes the system prompt in the request body" */
      it("maps the developer role to a system message at the head of input", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("abc123", "t1"),
            developerMsg("You are codex. Use the tools."),
            userMsg("hi"),
            assistantMsg("hello"),
          ),
        );

        expect(turns[0]!.inputMessages[0]).toEqual({
          role: "system",
          content: "You are codex. Use the tools.",
        });
        expect(lastUser(turns[0]!.inputMessages)).toBe("hi");
      });
    });
  });

  describe("given codex's injected environment_context user turn", () => {
    describe("when it is parsed", () => {
      /** @scenario "The environment_context is preserved in the request body but the prompt is the headline" */
      it("keeps the environment_context as a message while the last user message is the real prompt", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("abc123", "t1"),
            userMsg("<environment_context>\n  <cwd>/tmp</cwd>\n</environment_context>"),
            userMsg("fix the bug"),
            assistantMsg("fixed"),
          ),
        );

        expect(turns).toHaveLength(1);
        expect(turns[0]!.inputMessages).toHaveLength(2);
        expect(turns[0]!.inputMessages[0]!.content).toContain(
          "environment_context",
        );
        expect(lastUser(turns[0]!.inputMessages)).toBe("fix the bug");
      });
    });
  });

  describe("given a multi-turn rollout", () => {
    describe("when it is parsed", () => {
      /** @scenario "A multi-turn rollout accumulates prior turns into each turn's request body" */
      it("produces one turn per task_started trace_id and folds prior turns into the next input", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("t-one", "turn1"),
            userMsg("first question"),
            assistantMsg("first answer"),
            taskStarted("t-two", "turn2"),
            userMsg("second question"),
            assistantMsg("second answer"),
          ),
        );

        expect(turns.map((t) => t.traceId)).toEqual(["t-one", "t-two"]);
        expect(turns[0]!.inputMessages).toEqual([
          { role: "user", content: "first question" },
        ]);
        expect(turns[0]!.output).toBe("first answer");
        // Turn two carries the full prior conversation, as sent to the model.
        expect(turns[1]!.inputMessages).toEqual([
          { role: "user", content: "first question" },
          { role: "assistant", content: "first answer" },
          { role: "user", content: "second question" },
        ]);
        expect(turns[1]!.output).toBe("second answer");
      });

      /** @scenario "Historical backfill keeps each trace self-contained instead of copying every prior turn" */
      it("can retain only the current turn while live parsing keeps full history", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("t-one", "turn1"),
            userMsg("first question"),
            assistantMsg("first answer"),
            taskStarted("t-two", "turn2"),
            userMsg("second question"),
            assistantMsg("second answer"),
          ),
          { includePriorHistory: false },
        );

        expect(turns[1]!.inputMessages).toEqual([
          { role: "user", content: "second question" },
        ]);
      });
    });
  });

  describe("given a turn that calls a tool", () => {
    describe("when it is parsed", () => {
      /** @scenario "Tool calls and their results are captured in the request body" */
      it("records the function_call as an assistant tool_call and the output as a tool message", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("abc123", "t1"),
            userMsg("run ls"),
            assistantMsg("I'll list the files."),
            functionCall(
              "exec_command",
              '{"cmd":"ls"}',
              "call_1",
              "2026-08-04T10:00:01.000Z",
            ),
            functionCallOutput(
              "call_1",
              "a.txt\nb.txt",
              "2026-08-04T10:00:02.250Z",
            ),
            agentMessage("Here are the files: a.txt, b.txt"),
          ),
        );

        const input = turns[0]!.inputMessages;
        expect(input).toEqual([
          { role: "user", content: "run ls" },
          { role: "assistant", content: "I'll list the files." },
          {
            role: "assistant",
            tool_calls: [
              {
                id: "call_1",
                type: "function",
                function: { name: "exec_command", arguments: '{"cmd":"ls"}' },
              },
            ],
          },
          { role: "tool", tool_call_id: "call_1", content: "a.txt\nb.txt" },
        ]);
        expect(turns[0]!.toolCalls).toEqual([
          {
            callId: "call_1",
            name: "exec_command",
            arguments: '{"cmd":"ls"}',
            output: "a.txt\nb.txt",
            startedAtMs: Date.parse("2026-08-04T10:00:01.000Z"),
            completedAtMs: Date.parse("2026-08-04T10:00:02.250Z"),
          },
        ]);
        expect(turns[0]!.output).toBe("Here are the files: a.txt, b.txt");
      });
    });
  });

  describe("given a tool call codex emitted without a call_id", () => {
    describe("when it is parsed", () => {
      /** @scenario "An id-less tool call and its output share one synthetic id so they still pair" */
      it("mints one stable id for the call and reuses it on the output", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("abc123", "t1"),
            userMsg("run ls"),
            // codex can omit call_id; the call and its output must still pair.
            {
              type: "response_item",
              payload: {
                type: "function_call",
                name: "exec_command",
                arguments: '{"cmd":"ls"}',
              },
            },
            {
              type: "response_item",
              payload: { type: "function_call_output", output: "a.txt" },
            },
            agentMessage("done"),
          ),
        );

        const input = turns[0]!.inputMessages;
        const call = input.find((m) => m.tool_calls);
        const result = input.find((m) => m.role === "tool");
        const callId = call?.tool_calls?.[0]?.id;
        expect(callId).toBeTruthy();
        // The output carries the SAME id, so the pair joins instead of drifting
        // apart as the running history grows.
        expect(result?.tool_call_id).toBe(callId);
      });
    });
  });

  describe("given parallel tool calls whose results complete out of order", () => {
    /** @scenario "Tool result spans pair by call_id instead of completion order" */
    it("keeps each result on the correct reconstructed tool execution", () => {
      const turns = parseCodexRollout(
        rollout(
          taskStarted("abc123", "t1"),
          userMsg("inspect both files"),
          functionCall("read_file", '{"path":"a.txt"}', "call_a"),
          functionCall("read_file", '{"path":"b.txt"}', "call_b"),
          functionCallOutput("call_b", "B"),
          functionCallOutput("call_a", "A"),
          agentMessage("done"),
        ),
      );

      expect(turns[0]!.toolCalls).toMatchObject([
        { callId: "call_a", name: "read_file", output: "A" },
        { callId: "call_b", name: "read_file", output: "B" },
      ]);
    });
  });

  describe("given an id-less tool call left unmatched when its turn ends", () => {
    describe("when a later turn has its own id-less tool output", () => {
      /** @scenario "A synthetic tool-call id does not leak across the turn boundary" */
      it("does not pair the later output to the previous turn's orphaned call", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("trace-1", "t1"),
            userMsg("first"),
            // codex omitted the id and the matching output never arrives this
            // turn; the queued synthetic id must not survive the boundary.
            {
              type: "response_item",
              payload: { type: "function_call", name: "exec_command", arguments: "{}" },
            },
            agentMessage("answered one"),
            taskStarted("trace-2", "t2"),
            userMsg("second"),
            {
              type: "response_item",
              payload: { type: "function_call_output", output: "late result" },
            },
            agentMessage("answered two"),
          ),
        );

        // The second turn's cumulative history holds both the orphaned call
        // (from turn one) and its own late output. They must carry DIFFERENT
        // synthetic ids — the cleared queue stops the stale id leaking in.
        const msgs = turns[1]!.inputMessages;
        const orphanCall = msgs.find((m) => m.tool_calls);
        const laterOutput = msgs.find((m) => m.role === "tool");
        expect(orphanCall?.tool_calls?.[0]?.id).toBeTruthy();
        expect(laterOutput?.tool_call_id).toBeTruthy();
        expect(laterOutput?.tool_call_id).not.toBe(
          orphanCall?.tool_calls?.[0]?.id,
        );
      });
    });
  });

  describe("given both an agent_message and a response_item assistant message", () => {
    describe("when it is parsed", () => {
      /** @scenario "The assistant final answer is taken from the agent_message when present" */
      it("prefers the agent_message final answer and keeps it out of the input", () => {
        const turns = parseCodexRollout(
          rollout(
            taskStarted("abc123", "t1"),
            userMsg("hi"),
            agentMessage("done"),
            assistantMsg("raw scaffold text"),
          ),
        );

        expect(turns[0]!.output).toBe("done");
        expect(turns[0]!.inputMessages).toEqual([
          { role: "user", content: "hi" },
        ]);
      });
    });
  });

  describe("given a turn with no assistant reply", () => {
    describe("when it is parsed", () => {
      /** @scenario "A turn with no assistant reply is dropped rather than emitting an empty span" */
      it("drops the turn entirely", () => {
        const turns = parseCodexRollout(
          rollout(taskStarted("abc123", "t1"), userMsg("are you there?")),
        );

        expect(turns).toHaveLength(0);
      });
    });
  });

  describe("given a current Codex Desktop rollout without trace_id", () => {
    /** @scenario "History backfill derives a stable OTel trace id from the persisted turn id" */
    it("drops it during live parsing but imports it with an original completion time when opted in", () => {
      const content = rollout(
        {
          type: "session_meta",
          payload: { session_id: "session-1", base_instructions: "Use tools." },
        },
        {
          type: "event_msg",
          payload: {
            type: "task_started",
            turn_id: "550e8400-e29b-41d4-a716-446655440000",
            started_at: 1_780_000_000,
          },
        },
        userMsg("inspect the repository"),
        agentMessage("done"),
        taskComplete("550e8400-e29b-41d4-a716-446655440000", 1_780_000_042),
      );

      expect(parseCodexRollout(content)).toHaveLength(0);
      const imported = parseCodexRollout(content, {
        synthesizeMissingTraceIds: true,
      });

      expect(imported).toHaveLength(1);
      expect(imported[0]).toMatchObject({
        traceId: syntheticCodexTraceId(
          "550e8400-e29b-41d4-a716-446655440000",
        ),
        traceIdSource: "synthetic",
        sessionId: "session-1",
        completedAtMs: 1_780_000_042_000,
      });
      expect(imported[0]!.traceId).toMatch(/^[0-9a-f]{32}$/);
    });
  });

  describe("given a long accumulated conversation", () => {
    /** @scenario "Input capping keeps system instructions and the newest turns in linear time" */
    it("drops the oldest non-system messages while staying below the total cap", () => {
      const oldest = `oldest-${"a".repeat(29_000)}`;
      const newest = `newest-${"z".repeat(29_000)}`;
      const turns = parseCodexRollout(
        rollout(
          taskStarted("trace-long", "turn-long"),
          developerMsg(`system-${"s".repeat(29_000)}`),
          userMsg(oldest),
          userMsg(`middle-1-${"b".repeat(29_000)}`),
          userMsg(`middle-2-${"c".repeat(29_000)}`),
          userMsg(`middle-3-${"d".repeat(29_000)}`),
          userMsg(newest),
          assistantMsg("done"),
        ),
      );

      expect(JSON.stringify(turns[0]!.inputMessages).length).toBeLessThanOrEqual(
        120_000,
      );
      expect(turns[0]!.inputMessages[0]?.role).toBe("system");
      expect(turns[0]!.inputMessages.some((message) => message.content === oldest)).toBe(
        false,
      );
      expect(turns[0]!.inputMessages.some((message) => message.content === newest)).toBe(
        true,
      );
    });
  });
});

describe("buildCodexIOExportRequest", () => {
  describe("given a parsed turn", () => {
    describe("when the I/O spans are built", () => {
      /** @scenario "Parsed turns become OTLP spans carrying a chat_messages request body on the codex trace_id" */
      it("emits a span on the codex trace_id with a chat_messages langwatch.input and llm type", () => {
        const traceId = "00112233445566778899aabbccddeeff";
        const otlpTraceId = Buffer.from(traceId, "hex").toString("base64");
        const req = buildCodexIOExportRequest(
          [
            {
              traceId,
              turnId: "t1",
              model: "gpt-5.5",
              inputMessages: [
                { role: "system", content: "You are codex" },
                { role: "user", content: "hi" },
              ],
              output: "hello",
              toolCalls: [
                {
                  callId: "call_1",
                  name: "exec_command",
                  arguments: '{"cmd":"ls"}',
                  output: "a.txt",
                  startedAtMs: 1_780_000_010_000,
                  completedAtMs: 1_780_000_012_500,
                },
              ],
              startedAtMs: 1_780_000_000_000,
              completedAtMs: 1_780_000_100_000,
              traceIdSource: "synthetic",
              sessionId: "session-1",
            },
          ],
          1_780_000_500_000,
        );

        const span = (req.resourceSpans as any[])[0].scopeSpans[0].spans[0];
        expect(span.traceId).toBe(otlpTraceId);
        expect(Buffer.from(span.traceId, "base64")).toHaveLength(16);
        expect(Buffer.from(span.spanId, "base64")).toHaveLength(8);
        expect((req.resourceSpans as any[])[0].scopeSpans[0].scope.name).toBe(
          "langwatch.codex.rollout",
        );
        const attrs = Object.fromEntries(
          span.attributes.map((a: any) => [a.key, a.value.stringValue]),
        );
        expect(attrs["langwatch.span.type"]).toBe("llm");
        expect(JSON.parse(attrs["langwatch.input"])).toEqual({
          type: "chat_messages",
          value: [
            { role: "system", content: "You are codex" },
            { role: "user", content: "hi" },
          ],
        });
        expect(attrs["langwatch.output"]).toBe("hello");
        expect(attrs["gen_ai.response.model"]).toBe("gpt-5.5");
        expect(attrs["gen_ai.conversation.id"]).toBe("session-1");
        expect(attrs["codex.trace_id.source"]).toBe("synthetic");
        expect(span.endTimeUnixNano).toBe("1780000100000000000");

        const toolSpan = (req.resourceSpans as any[])[0].scopeSpans[0]
          .spans[1];
        expect(toolSpan).toMatchObject({
          traceId: otlpTraceId,
          parentSpanId: span.spanId,
          name: "codex.tool.exec_command",
          startTimeUnixNano: "1780000010000000000",
          endTimeUnixNano: "1780000012500000000",
        });
        const toolAttrs = Object.fromEntries(
          toolSpan.attributes.map((a: any) => [a.key, a.value.stringValue]),
        );
        expect(toolAttrs).toMatchObject({
          "langwatch.span.type": "tool",
          "gen_ai.tool.name": "exec_command",
          "gen_ai.tool.call.id": "call_1",
          "gen_ai.tool.call.arguments": '{"cmd":"ls"}',
          "gen_ai.tool.call.result": "a.txt",
          "codex.history.reconstructed": "true",
        });

        const secondBuild = buildCodexIOExportRequest(
          [
            {
              traceId,
              turnId: "t1",
              model: "gpt-5.5",
              inputMessages: [],
              output: "hello",
              toolCalls: [
                {
                  callId: "call_1",
                  name: "exec_command",
                  arguments: '{"cmd":"ls"}',
                  output: "a.txt",
                  startedAtMs: 1_780_000_010_000,
                  completedAtMs: 1_780_000_012_500,
                },
              ],
              startedAtMs: 1_780_000_000_000,
              completedAtMs: 1_780_000_100_000,
            },
          ],
          1_780_000_500_000,
        );
        expect(
          (secondBuild.resourceSpans as any[])[0].scopeSpans[0].spans[1]
            .spanId,
        ).toBe(toolSpan.spanId);
      });
    });
  });
});
