import assert from "node:assert/strict";
import test from "node:test";
import {
  agentEvolutionRequestSignature,
  agentEvolutionTraceSelection,
  agentEvolutionWindow,
  canStartAgentEvolution,
} from "../src/traceFarm.ts";

test("Agent evolution presets produce exact UTC windows", () => {
  const now = new Date("2026-08-05T12:30:00.000Z");
  assert.deepEqual(agentEvolutionWindow(now, "24h"), {
    window_start: "2026-08-04T12:30:00.000Z",
    window_end: "2026-08-05T12:30:00.000Z",
  });
  assert.deepEqual(agentEvolutionWindow(now, "7d"), {
    window_start: "2026-07-29T12:30:00.000Z",
    window_end: "2026-08-05T12:30:00.000Z",
  });
  assert.equal(agentEvolutionWindow(now, "30d").window_start, "2026-07-06T12:30:00.000Z");
});

test("Agent evolution requires at least two matching Traces", () => {
  assert.equal(canStartAgentEvolution("agent-a", 0), false);
  assert.equal(canStartAgentEvolution("agent-a", 1), false);
  assert.equal(canStartAgentEvolution("agent-a", 2), true);
  assert.equal(canStartAgentEvolution("", 2), false);
  assert.equal(canStartAgentEvolution("agent-a", 2, true), false);
});

test("Agent evolution makes the twelve-Trace freeze cap explicit", () => {
  assert.deepEqual(agentEvolutionTraceSelection(29), { matched: 29, frozen: 12, truncated: true });
  assert.deepEqual(agentEvolutionTraceSelection(2), { matched: 2, frozen: 2, truncated: false });
});

test("Agent evolution request identity changes with Agent, window, or objective", () => {
  const window = agentEvolutionWindow(new Date("2026-08-05T12:30:00.000Z"), "7d");
  const base = agentEvolutionRequestSignature("agent-a", window, "reduce failures");
  assert.notEqual(base, agentEvolutionRequestSignature("agent-b", window, "reduce failures"));
  assert.notEqual(base, agentEvolutionRequestSignature("agent-a", agentEvolutionWindow(new Date("2026-08-05T12:30:00.000Z"), "24h"), "reduce failures"));
  assert.notEqual(base, agentEvolutionRequestSignature("agent-a", window, "find latency"));
  assert.notEqual(base, agentEvolutionRequestSignature("agent-a", window, "reduce failures", "en"));
});
