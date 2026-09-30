import assert from "node:assert/strict";
import test from "node:test";
import {
  canAnalyzeAgent,
  registeredAgentSummaries,
  agentConnectionConfig,
} from "../src/agentConnection.ts";

test("the Agent registry hides telemetry-only aliases", () => {
  const agents = [
    { agent_id: "stable", display_name: "大狗", registered: true },
    { agent_id: "codex", display_name: "Codex", registered: false },
  ];
  assert.deepEqual(registeredAgentSummaries(agents).map((agent) => agent.agent_id), ["stable"]);
});

test("connection previews contain a placeholder and copied shell values remain literal", () => {
  const name = "O'Brien $env:PATH $(whoami) `value`";
  const preview = agentConnectionConfig("https://catena.example/", name, "powershell");
  assert.ok(preview.includes("$env:CATENA_API_KEY = '<CATENA_API_KEY>'"));
  assert.ok(preview.includes("O''Brien $env:PATH $(whoami) `value`"));
  assert.ok(preview.includes("$env:CATENA_URL = 'https://catena.example'"));
  const copied = agentConnectionConfig("https://catena.example", name, "sh", "fixture'key");
  assert.ok(copied.includes("export CATENA_API_KEY='fixture'\\''key'"));
  assert.ok(copied.includes("O'\\''Brien $env:PATH $(whoami) `value`"));
  assert.ok(!copied.includes("<CATENA_API_KEY>"));
  assert.ok(copied.includes('/v1/otlp/v1/traces'));
});

test("Trace Farm requires a connected multi-Trace Agent", () => {
  assert.equal(canAnalyzeAgent({ connected: false, trace_count: 8 }), false);
  assert.equal(canAnalyzeAgent({ connected: true, trace_count: 1 }), false);
  assert.equal(canAnalyzeAgent({ connected: true, trace_count: 2 }), true);
});
