import assert from "node:assert/strict";
import test from "node:test";
import { agentIdentitySourceLabel, agentSources, agentSourceSummary } from "../src/agentView.ts";
import { tracesForAgentSelection } from "../src/traceView.ts";

const workspaceTraces = [{ trace_id: "global-1", service_name: "busy-agent" }];
const agentTraces = [
  { trace_id: "agent-1", service_name: "catena-runtime-codex" },
  { trace_id: "agent-2", service_name: "catena-runtime-claude-code" },
];

test("Agent selection uses its server window instead of inferring from global Top 100", () => {
  assert.deepEqual(
    tracesForAgentSelection(workspaceTraces, "codex", agentTraces).map((trace) => trace.trace_id),
    ["agent-1", "agent-2"],
  );
  assert.deepEqual(
    tracesForAgentSelection(workspaceTraces, "", agentTraces).map((trace) => trace.trace_id),
    ["global-1"],
  );
});

test("one registered Agent presents accepted runtime and generic OTel sources", () => {
  const codex = {
    agent_id: "agent-codex-runtime",
    display_name: "Codex",
    identity_source: "api_key",
    sources: [
      { service_name: "catena-runtime-codex", kind: "native_live" },
      { service_name: "custom-observer", kind: "otel" },
    ],
  };

  assert.deepEqual(agentSources(codex).map((source) => source.service_name), [
    "catena-runtime-codex",
    "custom-observer",
  ]);
  assert.equal(agentSourceSummary(codex, "zh"), "实时 + OTel");
  assert.equal(agentSourceSummary(codex, "en"), "Live + OTel");
  assert.equal(agentIdentitySourceLabel(codex.identity_source, "zh"), "Agent 接入密钥");
});

test("source presentation tolerates summaries without source details", () => {
  assert.deepEqual(agentSources({ agent_id: "legacy", display_name: "legacy" }), []);
});
