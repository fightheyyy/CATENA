import assert from "node:assert/strict";
import test from "node:test";
import { agentIdentitySourceLabel, agentSources, agentSourceSummary } from "../src/agentView.ts";
import { tracesForAgentSelection } from "../src/traceView.ts";

const workspaceTraces = [{ trace_id: "global-1", service_name: "busy-agent" }];
const agentTraces = [
  { trace_id: "agent-1", service_name: "codex-app-server" },
  { trace_id: "agent-2", service_name: "Codex Desktop" },
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

test("one canonical Codex Agent presents live and historical sources", () => {
  const codex = {
    agent_id: "codex",
    display_name: "Codex",
    identity_source: "catena.alias",
    sources: [
      { service_name: "codex-app-server", kind: "native_live" },
      { service_name: "Codex Desktop", kind: "history_backfill" },
    ],
  };

  assert.deepEqual(agentSources(codex).map((source) => source.service_name), [
    "codex-app-server",
    "Codex Desktop",
  ]);
  assert.equal(agentSourceSummary(codex, "zh"), "实时 + 历史");
  assert.equal(agentSourceSummary(codex, "en"), "Live + History");
  assert.equal(agentIdentitySourceLabel(codex.identity_source, "zh"), "Catena Agent 归并");
});

test("source presentation remains compatible with legacy Agent summaries", () => {
  assert.deepEqual(agentSources({ agent_id: "legacy", display_name: "legacy" }), []);
});
