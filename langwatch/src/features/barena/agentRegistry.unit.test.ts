import { describe, expect, it } from "vitest";
import {
  type AgentRegistryObservation,
  agentTraceQuery,
  buildAgentRegistry,
  detectRuntime,
  summarizeAgentRegistry,
} from "./agentRegistry";

const observation = (
  overrides: Partial<AgentRegistryObservation> = {},
): AgentRegistryObservation => ({
  serviceName: "xiaoba-cli",
  agentName: "researcher",
  agentId: null,
  connectorId: "xiaoba",
  serviceVersion: "1.4.0",
  serviceInstanceId: "macbook-1",
  deploymentEnvironment: "local",
  traceCount: 7,
  errorCount: 1,
  warningCount: 0,
  lastSeenAt: 200,
  latestStatus: "ok",
  models: ["gpt-5.6-sol"],
  origins: ["application"],
  ...overrides,
});

describe("Catena Agent Registry", () => {
  it("keeps separate XiaoBa deployments and hides platform services", () => {
    const entries = buildAgentRegistry([
      observation(),
      observation({ serviceInstanceId: "macbook-2", lastSeenAt: 100 }),
      observation({ serviceName: "barena-evaluator" }),
      observation({ serviceName: "catena-evolution-runtime" }),
    ]);

    expect(entries).toHaveLength(2);
    expect(entries[0]).toMatchObject({
      name: "researcher",
      runtime: "XiaoBaOS",
      traceCount: 7,
      serviceInstanceId: "macbook-1",
    });
    expect(entries[0]?.key).not.toBe(entries[1]?.key);
  });

  it("falls back to service identity and detects common runtimes", () => {
    const [entry] = buildAgentRegistry([
      observation({
        serviceName: "openclaw",
        agentName: null,
        connectorId: null,
      }),
    ]);

    expect(entry).toMatchObject({ name: "openclaw", runtime: "OpenClaw" });
    expect(detectRuntime({ serviceName: "barena-codex" })).toBe("Codex");
  });

  it("builds a trace filter and compact fleet totals", () => {
    const entries = buildAgentRegistry([
      observation(),
      observation({
        serviceName: "openclaw",
        agentName: null,
        traceCount: 3,
        errorCount: 2,
        models: ["claude-sonnet-4"],
      }),
    ]);

    expect(
      agentTraceQuery(
        entries.find((entry) => entry.serviceName === "xiaoba-cli")!,
      ),
    ).toBe(
      'service:"xiaoba-cli" AND (attribute.gen_ai.agent.name:"researcher" OR attribute.xiaoba.role.name:"researcher")',
    );
    expect(summarizeAgentRegistry(entries)).toEqual({
      agents: 2,
      traces: 10,
      errors: 3,
      models: 2,
    });
  });
});
