import assert from "node:assert/strict";
import test from "node:test";
import {
  canAnalyzeAgent,
  registeredAgentSummaries,
} from "../src/agentConnection.ts";

test("the Agent registry hides telemetry-only aliases", () => {
  const agents = [
    { agent_id: "stable", display_name: "大狗", registered: true },
    { agent_id: "codex", display_name: "Codex", registered: false },
  ];
  assert.deepEqual(registeredAgentSummaries(agents).map((agent) => agent.agent_id), ["stable"]);
});

test("Trace Farm requires a connected multi-Trace Agent", () => {
  assert.equal(canAnalyzeAgent({ connected: false, trace_count: 8 }), false);
  assert.equal(canAnalyzeAgent({ connected: true, trace_count: 1 }), false);
  assert.equal(canAnalyzeAgent({ connected: true, trace_count: 2 }), true);
});
