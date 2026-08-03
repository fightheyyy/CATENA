/** @vitest-environment node */

import type { AgentInput } from "@langwatch/scenario";
import { AgentRole } from "@langwatch/scenario";
import { describe, expect, it, vi } from "vitest";
import {
  type SpiralScenarioRoleClient,
  XiaobaScenarioEvaluatorClient,
  XiaobaScenarioReviewer,
  XiaobaScenarioUserSimulator,
} from "../xiaoba-scenario-evaluator";

describe("XiaoBaOS Scenario evaluator adapters", () => {
  it("sends a versioned, project-correlated role turn to spiral-runner", async () => {
    let requestBody: Record<string, unknown> | undefined;
    const fetchImpl = vi.fn(async (_url: unknown, init?: RequestInit) => {
      requestBody = JSON.parse(String(init?.body)) as Record<string, unknown>;
      return new Response(
        JSON.stringify({
          schema: "barena.xiaoba_scenario_response.v1",
          request_id: requestBody.request_id,
          status: "ok",
          result: {
            status: "completed",
            detail: "ok",
            assistant: { role: "assistant", content: '{"message":"hello"}' },
          },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }) as unknown as typeof fetch;
    const client = new XiaobaScenarioEvaluatorClient({
      endpoint: "http://spiral-runner:8790/",
      projectId: "project-1",
      scenarioId: "scenario-1",
      runId: "run-1",
      telemetryEndpoint: "http://spiral-app:5560",
      apiKey: "project-key",
      fetchImpl,
    });

    await expect(
      client.runRole({
        role: "user-cat",
        prompt: "one user turn",
        threadId: "thread-1",
      }),
    ).resolves.toBe('{"message":"hello"}');

    expect(requestBody).toMatchObject({
      schema: "barena.xiaoba_scenario_request.v1",
      project_id: "project-1",
      scenario_id: "scenario-1",
      run_id: "run-1",
      thread_id: "thread-1",
      role: "user-cat",
      telemetry: {
        traces_endpoint: "http://spiral-app:5560/api/otel/v1/traces",
        headers: { "x-auth-token": "project-key" },
      },
    });
  });

  it("maps UserCat and ReviewerCat outputs to Scenario's unchanged agent contract", async () => {
    const calls: string[] = [];
    const client: SpiralScenarioRoleClient = {
      runRole: vi.fn(async ({ role, prompt }) => {
        calls.push(`${role}:${prompt}`);
        if (role === "user-cat") {
          return '{"message":"can you check my order?"}';
        }
        if (calls.filter((call) => call.startsWith("reviewer-cat:")).length === 1) {
          return '{"action":"continue","reasoning":"need the agent response"}';
        }
        return JSON.stringify({
          action: "finish",
          verdict: "success",
          reasoning: "The agent resolved the order question.",
          criteria: [{ criterion: "resolve the order", status: "met" }],
        });
      }),
    };
    const user = new XiaobaScenarioUserSimulator(client);
    const reviewer = new XiaobaScenarioReviewer({
      client,
      criteria: ["resolve the order"],
      projectId: "project-1",
      querySpans: vi.fn().mockResolvedValue([]),
    });

    await expect(user.call(agentInput({ currentTurn: 0 }))).resolves.toBe(
      "can you check my order?",
    );
    await expect(reviewer.call(agentInput({ currentTurn: 0 }))).resolves.toBeNull();
    await expect(
      reviewer.call(agentInput({ currentTurn: 2, maxTurns: 3 })),
    ).resolves.toEqual({
      success: true,
      reasoning: "The agent resolved the order question.",
      metCriteria: ["resolve the order"],
      unmetCriteria: [],
    });
    expect(calls.some((call) => call.includes("<opentelemetry_traces>"))).toBe(
      true,
    );
  });
});

function agentInput(params: {
  currentTurn: number;
  maxTurns?: number;
}): AgentInput {
  const config = {
    name: "Order support",
    description: "The user needs help resolving an order question.",
    agents: [],
    maxTurns: params.maxTurns ?? 4,
  };
  return {
    threadId: "thread-1",
    messages: [
      { role: "user", content: "where is order 42" },
      { role: "assistant", content: "I can check that" },
    ],
    newMessages: [],
    requestedRole: AgentRole.USER,
    scenarioConfig: config,
    scenarioState: {
      config,
      description: config.description,
      currentTurn: params.currentTurn,
      threadId: "thread-1",
      messages: [],
    },
  } as unknown as AgentInput;
}
