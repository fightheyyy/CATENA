import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  TRACE_STEP_PAGE_SIZE,
  boundedTraceSteps,
  buildTraceNarrative,
  buildTraceSemanticView,
  filterTraceSummaries,
  groupTraceSummariesBySession,
  formatTraceEvidence,
  presentTraceEvidence,
  preferredTraceSpan,
  shortTraceID,
  traceSessionKey,
  traceSessionTitle,
  traceSpanDepth,
  traceSpanSemanticKind,
  traceSpanState,
  traceSpanTokenUsage,
  traceSpanToolName,
  traceSummaryTitle,
} from "../src/traceView.ts";

const traces = [
  {
    trace_id: "trace-codex",
    root_name: "codex.turn.response",
    service_name: "catena-runtime-codex",
    model: "gpt-5.6-sol",
    span_count: 21,
    error_count: 0,
  },
  {
    trace_id: "trace-barena",
    root_name: "barena.explore",
    service_name: "barena-explore-engine",
    span_count: 13,
    error_count: 1,
  },
  {
    trace_id: "trace-xiaoba",
    root_name: "xiaoba.session",
    service_name: "xiaoba-target",
    span_count: 1,
    error_count: 0,
  },
];

test("Trace list search stays focused on root Trace identity", () => {
  assert.deepEqual(filterTraceSummaries(traces, "gpt-5.6", "all").map((trace) => trace.trace_id), ["trace-codex"]);
  assert.deepEqual(filterTraceSummaries(traces, "barena", "all").map((trace) => trace.trace_id), ["trace-barena"]);
  assert.deepEqual(filterTraceSummaries(traces, "TRACE-XIAOBA", "all").map((trace) => trace.trace_id), ["trace-xiaoba"]);
});

test("Trace rows use the real user request instead of a Runtime fallback label", () => {
  const direct = { ...traces[0], root_name: "agent.turn", input_preview: "剩下两题过完更新吧" };
  assert.equal(traceSummaryTitle(direct, "zh"), "剩下两题过完更新吧");

  const wrapped = {
    ...direct,
    input_preview: JSON.stringify({
      type: "chat_messages",
      value: [
        { role: "assistant", content: "上一轮回答" },
        { role: "user", content: "检查一下发布结果" },
      ],
    }),
  };
  assert.equal(traceSummaryTitle(wrapped, "zh"), "检查一下发布结果");
  assert.equal(traceSummaryTitle({ ...direct, input_preview: "" }, "zh"), "用户请求");
  assert.equal(traceSummaryTitle({ ...direct, input_preview: "" }, "en"), "User request");
  assert.deepEqual(filterTraceSummaries([wrapped], "发布结果", "all").map((trace) => trace.trace_id), ["trace-codex"]);
});

test("Trace filters expose errors and multi-step runs without a query builder", () => {
  assert.deepEqual(filterTraceSummaries(traces, "", "errors").map((trace) => trace.trace_id), ["trace-barena"]);
  assert.deepEqual(filterTraceSummaries(traces, "", "multi").map((trace) => trace.trace_id), ["trace-codex", "trace-barena"]);
});

test("Trace filtering preserves raw service names from a canonical Agent query", () => {
  const canonicalCodexTraces = [
    traces[0],
    { ...traces[0], trace_id: "trace-codex-live", service_name: "catena-runtime-codex-live" },
  ];
  assert.deepEqual(
    filterTraceSummaries(canonicalCodexTraces, "", "all").map((trace) => trace.service_name),
    ["catena-runtime-codex", "catena-runtime-codex-live"],
  );
  assert.deepEqual(
    filterTraceSummaries(canonicalCodexTraces, "tool", "all").map((trace) => trace.trace_id),
    [],
  );
});

test("Trace summaries preserve Agent, Session, Trace, Span hierarchy", () => {
  const sessionTraces = [
    { ...traces[0], agent_id: "codex", session_id: "session-a", start_time: "2026-08-11T01:00:00Z", end_time: "2026-08-11T01:02:00Z" },
    { ...traces[0], trace_id: "trace-codex-2", agent_id: "codex", session_id: "session-a", span_count: 4, start_time: "2026-08-11T01:03:00Z", end_time: "2026-08-11T01:04:00Z" },
    { ...traces[2], agent_id: "xiaoba", session_id: "session-x", start_time: "2026-08-11T02:00:00Z", end_time: "2026-08-11T02:01:00Z" },
  ];
  const groups = groupTraceSummariesBySession(sessionTraces);
  assert.deepEqual(groups.map((group) => [group.agentID, group.sessionID, group.traces.length]), [
    ["xiaoba", "session-x", 1],
    ["codex", "session-a", 2],
  ]);
  assert.equal(groups[1].spanCount, 25);
  assert.equal(traceSessionKey(sessionTraces[0]), "codex\u0000session-a");
});

test("Session titles use the earliest retained user request without an LLM", () => {
  const groups = groupTraceSummariesBySession([
    {
      ...traces[0],
      trace_id: "trace-later",
      agent_id: "codex",
      session_id: "session-a",
      input_preview: "继续下一题",
      start_time: "2026-08-11T01:03:00Z",
      end_time: "2026-08-11T01:04:00Z",
    },
    {
      ...traces[0],
      trace_id: "trace-first",
      agent_id: "codex",
      session_id: "session-a",
      input_preview: JSON.stringify({ type: "chat_messages", value: [{ role: "user", content: "帮我讲解回溯算法" }] }),
      start_time: "2026-08-11T01:00:00Z",
      end_time: "2026-08-11T01:02:00Z",
    },
  ]);
  assert.equal(traceSessionTitle(groups[0]), "帮我讲解回溯算法");
  assert.equal(traceSessionTitle({ ...groups[0], traces: groups[0].traces.map((trace) => ({ ...trace, input_preview: "" })) }), "");
});

test("missing Session identity stays explicitly ungrouped per Agent", () => {
  const groups = groupTraceSummariesBySession([
    { ...traces[0], agent_id: "codex", start_time: "2026-08-11T01:00:00Z", end_time: "2026-08-11T01:01:00Z" },
    { ...traces[0], trace_id: "trace-codex-2", start_time: "2026-08-11T01:02:00Z", end_time: "2026-08-11T01:03:00Z" },
  ], "codex");
  assert.equal(groups.length, 1);
  assert.equal(groups[0].sessionID, "");
  assert.equal(groups[0].traces.length, 2);
  assert.equal(groups[0].key, "codex\u0000__ungrouped__");
});

test("Span depth follows parent links and fails closed on cycles", () => {
  const root = { span_id: "root" };
  const child = { span_id: "child", parent_span_id: "root" };
  const grandchild = { span_id: "grandchild", parent_span_id: "child" };
  const byID = new Map([["root", root], ["child", child], ["grandchild", grandchild]]);
  assert.equal(traceSpanDepth(root, byID), 0);
  assert.equal(traceSpanDepth(child, byID), 1);
  assert.equal(traceSpanDepth(grandchild, byID), 2);

  const cycleA = { span_id: "a", parent_span_id: "b" };
  const cycleB = { span_id: "b", parent_span_id: "a" };
  assert.equal(traceSpanDepth(cycleA, new Map([["a", cycleA], ["b", cycleB]])), 2);
});

test("Tool names and evidence are rendered semantically", () => {
  assert.equal(traceSpanToolName({ attributes: { "gen_ai.tool.name": " search_repo " } }), "search_repo");
  assert.equal(formatTraceEvidence('{"query":"trace","limit":3}'), '{\n  "query": "trace",\n  "limit": 3\n}');
  assert.equal(formatTraceEvidence("plain result"), "plain result");
});

test("Token usage accepts both Codex and Claude Runtime attribute names", () => {
  assert.deepEqual(traceSpanTokenUsage({
    attributes: {
      "gen_ai.usage.input_tokens": 19154,
      "gen_ai.usage.output_tokens": 86,
      "gen_ai.usage.total_tokens": 19240,
    },
    resource_attributes: {},
  }), { input: 19154, output: 86, total: 19240 });
  assert.deepEqual(traceSpanTokenUsage({
    attributes: {
      "gen_ai.usage.input": 8119,
      "gen_ai.usage.output": 151,
    },
    resource_attributes: {},
  }), { input: 8119, output: 151, total: 8270 });
});

test("Codex model requests render the visible conversation instead of raw request JSON", () => {
  const evidence = presentTraceEvidence(JSON.stringify({
    model: "gpt-5.6-sol",
    input: [
      { type: "additional_tools", role: "developer", tools: [{ name: "exec" }] },
      { type: "message", role: "developer", content: [{ type: "input_text", text: "system instructions" }] },
      { type: "message", role: "user", content: [{ type: "input_text", text: "<recommended_plugins>hidden</recommended_plugins>" }] },
      { type: "message", role: "user", content: [{ type: "input_text", text: "Run pwd once." }] },
      { type: "message", role: "assistant", content: [{ type: "output_text", text: "I will run it." }] },
      { type: "custom_tool_call", name: "exec", input: "pwd" },
    ],
  }), "model", "input");

  assert.equal(evidence.kind, "messages");
  assert.deepEqual(evidence.messages, [
    { role: "user", text: "Run pwd once." },
    { role: "assistant", text: "I will run it." },
  ]);
  assert.equal(evidence.hiddenContextCount, 4);
  assert.equal(evidence.structured, true);
});

test("Codex turn inputs unwrap chat_messages instead of exposing protocol fields", () => {
  const evidence = presentTraceEvidence(JSON.stringify({
    type: "chat_messages",
    value: [
      { role: "user", content: [{ type: "input_text", text: "帮我检查部署异常。" }] },
      { role: "assistant", content: [{ type: "output_text", text: "我先检查服务状态。" }] },
    ],
  }), "turn", "input");

  assert.equal(evidence.kind, "messages");
  assert.deepEqual(evidence.messages, [
    { role: "user", text: "帮我检查部署异常。" },
    { role: "assistant", text: "我先检查服务状态。" },
  ]);
  assert.deepEqual(evidence.fields, []);
  assert.equal(evidence.structured, true);
});

test("Codex exec wrappers render arguments and terminal output without JSON chrome", () => {
  const input = presentTraceEvidence(
    'const r = await tools.exec_command({"cmd":"pwd","workdir":"/workspace","yield_time_ms":10000});\ntext(r.output);',
    "tool",
    "input",
  );
  const output = presentTraceEvidence(
    '[{"type":"input_text","text":"Script completed\\nOutput:\\n"},{"type":"input_text","text":"/workspace\\n"}]',
    "tool",
    "output",
  );

  assert.equal(input.kind, "fields");
  assert.deepEqual(input.fields.slice(0, 2), [
    { key: "cmd", value: "pwd", code: true },
    { key: "workdir", value: "/workspace", code: true },
  ]);
  assert.equal(output.kind, "terminal");
  assert.equal(output.text, "Script completed\nOutput:\n/workspace\n");
});

test("truncated Codex requests fail closed instead of rendering giant JSON as a user message", () => {
  const evidence = presentTraceEvidence(
    '{"model":"gpt-5.6-sol","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"system"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"unfinished',
    "model",
    "input",
  );
  assert.equal(evidence.kind, "fields");
  assert.deepEqual(evidence.fields, [{ key: "model", value: "gpt-5.6-sol", code: false }]);
  assert.deepEqual(evidence.messages, []);
  assert.equal(evidence.hiddenContextCount, 2);
  assert.equal(evidence.structured, true);
  assert.equal(evidence.truncated, true);
});

test("Long Trace IDs are shortened without losing both ends", () => {
  assert.equal(shortTraceID("c2d7cd176b43d718a9135442b0ddcd36"), "c2d7cd17…ddcd36");
  assert.equal(shortTraceID("trace-short"), "trace-short");
});

function span(overrides) {
  return {
    trace_id: "trace-semantic",
    span_id: overrides.span_id,
    parent_span_id: overrides.parent_span_id ?? "root",
    name: overrides.name,
    kind: 1,
    service_name: "catena-runtime-codex",
    start_time: "2026-08-06T00:00:00Z",
    end_time: "2026-08-06T00:00:01Z",
    status_code: overrides.status_code ?? 0,
    attributes: overrides.attributes ?? {},
    resource_attributes: {},
    input: overrides.input,
    output: overrides.output,
    model: overrides.model,
  };
}

test("Agent semantic lens separates useful steps from Runtime internals", () => {
  const root = span({ span_id: "root", parent_span_id: "", name: "turn/start", attributes: { turn_id: "turn-1" } });
  const model = span({ span_id: "model", name: "run_sampling_request", model: "gpt-5.6-sol", attributes: { turn_id: "turn-1" } });
  const tool = span({ span_id: "tool", name: "handle_tool_call_with_source", attributes: { turn_id: "turn-1", "gen_ai.tool.name": "exec_command" }, input: "pwd" });
  const artifact = span({ span_id: "artifact", name: "apply_patch", attributes: { turn_id: "turn-1", "gen_ai.tool.name": "apply_patch" } });
  const error = span({ span_id: "error", name: "provider.request", status_code: 2 });
  const internal = span({ span_id: "internal", name: "persist_rollout_items", attributes: { "code.file.path": "core/src/rollout.rs" } });
  const view = buildTraceSemanticView([root, model, tool, artifact, error, internal]);

  assert.deepEqual(view.agentSteps.map((item) => item.kind), ["turn", "model", "tool", "artifact", "error"]);
  assert.equal(view.foldedInternalCount, 1);
  assert.equal(view.turnCount, 1);
  assert.deepEqual(view.toolSteps.map((item) => item.span.span_id), ["tool", "artifact"]);
  assert.equal(preferredTraceSpan(view).span_id, "error");
  assert.equal(traceSpanSemanticKind(internal), "internal");
});

test("Catena canonical states keep failed tools visible in both tool and error lenses", () => {
  const turn = span({
    span_id: "canonical-turn",
    parent_span_id: "",
    name: "agent.turn",
    status_code: 2,
    attributes: { "catena.node.kind": "turn", "catena.state": "aborted", "agent.turn.id": "turn-abort" },
  });
  const failedTool = span({
    span_id: "canonical-tool",
    parent_span_id: "canonical-turn",
    name: "agent.tool.call Bash",
    status_code: 2,
    attributes: {
      "catena.node.kind": "tool",
      "catena.state": "error",
      "gen_ai.tool.name": "Bash",
      "gen_ai.tool.call.id": "toolu_failure",
    },
  });
  const retry = span({
    span_id: "canonical-retry",
    parent_span_id: "canonical-turn",
    name: "gen_ai.model.retry",
    status_code: 2,
    attributes: { "catena.node.kind": "retry", "catena.state": "retry" },
  });
  const view = buildTraceSemanticView([turn, failedTool, retry]);

  assert.deepEqual(view.agentSteps.map((item) => item.kind), ["turn", "tool", "retry"]);
  assert.deepEqual(view.toolSteps.map((item) => item.span.span_id), ["canonical-tool"]);
  assert.deepEqual(view.errorSteps.map((item) => item.span.span_id), ["canonical-turn", "canonical-tool", "canonical-retry"]);
});

test("Canonical narrative preserves parallel tools, Subagents, and distinct state events", () => {
  const turn = span({
    span_id: "narrative-turn",
    parent_span_id: "",
    name: "agent.turn",
    input: "Inspect the repository",
    output: "Inspection complete",
    attributes: { "catena.node.kind": "turn", "catena.state": "ok", "agent.turn.id": "turn-narrative" },
  });
  const model = span({
    span_id: "narrative-model",
    parent_span_id: turn.span_id,
    name: "gen_ai.model.call",
    attributes: { "catena.node.kind": "model", "catena.model.step.index": "0" },
  });
  const firstTool = span({
    span_id: "parallel-a",
    parent_span_id: model.span_id,
    name: "agent.tool.call file_search",
    attributes: { "catena.node.kind": "tool", "gen_ai.tool.name": "file_search", "gen_ai.tool.call.id": "call-a" },
  });
  const secondTool = span({
    span_id: "parallel-b",
    parent_span_id: model.span_id,
    name: "agent.tool.call web_search",
    attributes: { "catena.node.kind": "tool", "gen_ai.tool.name": "web_search", "gen_ai.tool.call.id": "call-b" },
  });
  const subagent = span({
    span_id: "subagent-thread",
    parent_span_id: firstTool.span_id,
    name: "agent.subagent.thread",
    attributes: { "catena.node.kind": "subagent", "agent.subagent.thread.id": "thread-child" },
  });
  const compact = span({
    span_id: "compact",
    parent_span_id: turn.span_id,
    name: "agent.context.compact",
    attributes: { "catena.node.kind": "context_compact", "catena.state": "ok" },
  });
  const retry = span({
    span_id: "retry",
    parent_span_id: turn.span_id,
    name: "gen_ai.model.retry",
    status_code: 2,
    attributes: { "catena.node.kind": "retry", "catena.state": "retry" },
  });
  const wrapper = span({ span_id: "runtime-wrapper", parent_span_id: turn.span_id, name: "runtime.persist" });
  const finalModel = span({
    span_id: "final-model",
    parent_span_id: wrapper.span_id,
    name: "gen_ai.model.call",
    attributes: { "catena.node.kind": "model", "catena.model.step.index": "1" },
  });

  const narrative = buildTraceNarrative([turn, model, firstTool, secondTool, subagent, compact, retry, wrapper, finalModel]);
  assert.equal(narrative.primaryTurn?.span.span_id, turn.span_id);
  assert.deepEqual(narrative.primaryTurn?.children.map((node) => node.kind), ["model", "compact", "retry", "model"]);
  assert.deepEqual(narrative.primaryTurn?.children[0].children.map((node) => node.span.span_id), ["parallel-a", "parallel-b"]);
  assert.equal(narrative.primaryTurn?.children[0].children[0].children[0].kind, "subagent");
  assert.equal(traceSpanSemanticKind(compact), "compact");
  assert.equal(traceSpanSemanticKind(retry), "retry");
  assert.equal(traceSpanState(retry), "retry");
});

test("Codex and Claude OTLP goldens render as Agent, Tool, Subagent and Error evidence", () => {
  for (const runtime of ["codex", "claude"]) {
    const payloads = JSON.parse(readFileSync(
      new URL(`../../tap/fixtures/golden/${runtime}.otlp.json`, import.meta.url),
      "utf8",
    ));
    const spans = payloads.flatMap((payload) => payload.resourceSpans.flatMap((resource) => (
      resource.scopeSpans.flatMap((scope) => scope.spans.map((value) => {
        const attributes = Object.fromEntries(value.attributes.map((attribute) => [
          attribute.key,
          Object.values(attribute.value)[0],
        ]));
        return {
          trace_id: value.traceId,
          span_id: value.spanId,
          parent_span_id: value.parentSpanId ?? "",
          name: value.name,
          kind: value.kind,
          service_name: `catena-runtime-${runtime}`,
          start_time: value.startTimeUnixNano,
          end_time: value.endTimeUnixNano,
          status_code: value.status.code,
          attributes,
          resource_attributes: {},
          input: attributes["input.value"] ?? "",
          output: attributes["output.value"] ?? "",
        };
      }))
    )));
    const view = buildTraceSemanticView(spans);
    assert.ok(view.counts.turn >= payloads.length, `${runtime} turn count`);
    assert.ok(view.toolSteps.some(({ span }) => traceSpanToolName(span)), `${runtime} tool evidence`);
    assert.ok(view.errorSteps.some(({ span }) => span.attributes["catena.state"] === "aborted"), `${runtime} abort`);
    assert.ok(view.errorSteps.some(({ span }) => span.attributes["catena.node.kind"] === "unmatched_tool_result"), `${runtime} unmatched result`);
    assert.ok(view.agentSteps.some(({ span }) => span.attributes["catena.node.kind"] === "subagent"), `${runtime} subagent`);
  }
});

test("Runtime turn-context helpers stay folded instead of masquerading as Agent turns", () => {
  const helper = span({
    span_id: "turn-context-helper",
    name: "turn_context.build",
    attributes: { turn_id: "turn-1" },
  });

  assert.equal(traceSpanSemanticKind(helper), "internal");
});

test("Barena and XiaoBaOS spans render as one readable cross-process Agent chain", () => {
  const root = span({ span_id: "run", parent_span_id: "", name: "barena.simulation" });
  const turn = span({
    span_id: "barena-turn",
    parent_span_id: "run",
    name: "barena.turn",
    attributes: { "agent.turn.id": "turn-1" },
    input: "ambiguous user request",
    output: "agent answer",
  });
  const session = span({
    span_id: "xiaoba-session",
    parent_span_id: "barena-turn",
    name: "xiaoba.session",
  });
  const model = span({
    span_id: "xiaoba-model",
    parent_span_id: "xiaoba-session",
    name: "xiaoba.model.call",
  });
  const assertion = span({
    span_id: "assertion",
    parent_span_id: "run",
    name: "barena.assertion",
    attributes: { "barena.assertion.kind": "excludes", "barena.assertion.status": "pass" },
    input: '["forbidden"]',
    output: "excluded forbidden text",
  });

  const view = buildTraceSemanticView([root, turn, session, model, assertion]);
  assert.deepEqual(view.agentSteps.map((item) => [item.span.span_id, item.kind]), [
    ["run", "run"],
    ["barena-turn", "turn"],
    ["xiaoba-model", "model"],
    ["assertion", "check"],
  ]);
  assert.equal(view.turnCount, 1);
  assert.equal(view.counts.model, 1);
  assert.equal(view.counts.check, 1);
  assert.equal(view.foldedInternalCount, 1);
  assert.equal(preferredTraceSpan(view).span_id, "barena-turn");
});

test("Raw Span disclosure is bounded even for a 30k Span Trace", () => {
  const spans = Array.from({ length: 30_000 }, (_, index) => span({
    span_id: `internal-${index}`,
    name: "receiving",
  }));
  const view = buildTraceSemanticView(spans);
  const page = boundedTraceSteps(view, "raw", TRACE_STEP_PAGE_SIZE);

  assert.equal(page.steps.length, TRACE_STEP_PAGE_SIZE);
  assert.equal(page.hiddenCount, 29_800);
  assert.equal(page.totalCount, 30_000);
  assert.equal(view.agentSteps.length, 0);
});
