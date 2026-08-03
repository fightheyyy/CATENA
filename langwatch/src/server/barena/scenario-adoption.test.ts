import { describe, expect, it } from "vitest";
import type { ScenarioRunData } from "~/server/scenarios/scenario-event.types";
import { buildScenarioRunAdoption, classifyReplay } from "./scenario-adoption";

describe("Barena Scenario adoption", () => {
  it("builds terminal tenant-scoped facts and freezes only a safe HTTP replay seam", () => {
    const body = buildScenarioRunAdoption({
      projectId: "project-one",
      run: runFixture(),
      scenario: {
        id: "scenario-one",
        name: "Clarification",
        situation: "Ask before acting",
        criteria: ["asks one question"],
      },
      agent: {
        id: "agent-one",
        name: "XiaoBa HTTP",
        type: "http",
        config: {
          url: "https://agent.example.test/chat",
          method: "POST",
          auth: { type: "none" },
          headers: [{ key: "Content-Type", value: "application/json" }],
          bodyTemplate: `{
            "thread_id": "{{threadId}}",
            "messages": {{messages}}
          }`,
          outputPath: "$.response",
          timeoutMs: 5_000,
        },
      },
    });

    expect(body.source_project_id).toBe("project-one");
    expect(body.source_status).toBe("FAILED");
    expect(body.primary_trace_id).toBe("22222222222222222222222222222222");
    expect(body.trace_ids).toEqual([
      "11111111111111111111111111111111",
      "22222222222222222222222222222222",
    ]);
    expect(body.replay).toEqual({
      supported: true,
      url: "https://agent.example.test/chat",
      method: "POST",
      output_path: "$.response",
      timeout_ms: 5_000,
    });
    expect(JSON.stringify(body)).not.toContain("authorization");
    expect(JSON.stringify(body)).not.toContain("token");
  });

  it("never serializes credentials for unsupported Replay", () => {
    const replay = classifyReplay({
      url: "https://agent.example.test/chat",
      method: "POST",
      auth: { type: "bearer", token: "super-secret" },
    });
    expect(replay.supported).toBe(false);
    expect(JSON.stringify(replay)).not.toContain("super-secret");
    expect(JSON.stringify(replay)).not.toContain("agent.example.test");
  });

  it("rejects genuinely custom body templates while accepting the platform default", () => {
    const standard = classifyReplay({
      url: "https://agent.example.test/chat",
      method: "POST",
      bodyTemplate: '{ "thread_id": "{{threadId}}", "messages": {{messages}} }',
    });
    expect(standard.supported).toBe(true);

    const custom = classifyReplay({
      url: "https://agent.example.test/chat",
      method: "POST",
      bodyTemplate: '{"input":"{{input}}"}',
    });
    expect(custom).toEqual({
      supported: false,
      reason:
        "Custom HTTP body templates are not supported by deterministic Replay.",
    });
  });

  it("fails closed for a non-terminal run or mismatched target", () => {
    const running = runFixture();
    running.status = "IN_PROGRESS" as ScenarioRunData["status"];
    expect(() =>
      buildScenarioRunAdoption({
        projectId: "project-one",
        run: running,
        scenario: {
          id: "scenario-one",
          name: "Clarification",
          situation: "Ask before acting",
          criteria: [],
        },
        agent: {
          id: "agent-one",
          name: "XiaoBa HTTP",
          type: "http",
          config: {},
        },
      }),
    ).toThrow(/terminal Explore run/);
  });
});

function runFixture(): ScenarioRunData {
  return {
    scenarioId: "scenario-one",
    batchRunId: "batch-one",
    scenarioRunId: "scenario-run-one",
    name: "Clarification",
    description: "Ask before acting",
    metadata: {
      langwatch: {
        targetReferenceId: "agent-one",
        targetType: "http",
      },
    },
    status: "FAILED",
    results: {
      verdict: "failure",
      reasoning: "The Agent acted immediately.",
      metCriteria: [],
      unmetCriteria: ["asks one question"],
    },
    messages: [
      {
        role: "assistant",
        content: "done",
        trace_id: "22222222222222222222222222222222",
      },
    ],
    traceIds: [
      "11111111111111111111111111111111",
      "22222222222222222222222222222222",
    ],
    timestamp: Date.parse("2026-07-31T08:00:00.000Z"),
    updatedAt: Date.parse("2026-07-31T08:00:02.000Z"),
    durationInMs: 2_000,
  } as unknown as ScenarioRunData;
}
