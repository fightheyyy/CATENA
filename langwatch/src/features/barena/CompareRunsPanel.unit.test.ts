import { describe, expect, it } from "vitest";
import type { BarenaRun } from "~/server/barena/contracts";
import {
  compareCompatibility,
  readPlatformExploreFacts,
} from "./CompareRunsPanel";

describe("Barena Compare", () => {
  it("extracts only completed adopted Platform Explore evidence", () => {
    const run = runFixture();
    expect(readPlatformExploreFacts(run)).toMatchObject({
      runId: "run-one",
      scenarioId: "scenario-one",
      targetReferenceId: "agent-one",
      judgeVerdict: "failure",
      metCriteria: [],
      unmetCriteria: ["writes DONE"],
      primaryTraceId: "11111111111111111111111111111111",
    });

    expect(
      readPlatformExploreFacts({ ...run, origin: "local" }),
    ).toBeUndefined();
    expect(
      readPlatformExploreFacts({ ...run, state: "failed" }),
    ).toBeUndefined();
  });

  it("allows only distinct runs from the same Scenario and registered target", () => {
    const left = readPlatformExploreFacts(runFixture())!;
    const right = readPlatformExploreFacts(runFixture({ run_id: "run-two" }))!;
    expect(compareCompatibility(left, right)).toEqual({ compatible: true });
    expect(compareCompatibility(left, left)).toEqual({
      compatible: false,
      reason: "same_run",
    });

    const otherScenario = {
      ...right,
      scenarioId: "scenario-two",
    };
    expect(compareCompatibility(left, otherScenario)).toEqual({
      compatible: false,
      reason: "different_scenario",
    });
    const otherTarget = {
      ...right,
      targetReferenceId: "agent-two",
    };
    expect(compareCompatibility(left, otherTarget)).toEqual({
      compatible: false,
      reason: "different_target",
    });
  });
});

function runFixture(overrides: Partial<BarenaRun> = {}): BarenaRun {
  return {
    run_id: "run-one",
    request_id: "request-one",
    origin: "platform",
    operation: "explore",
    state: "completed",
    input: {
      schema: "barena.platform_explore_scenario.v1",
      source: { scenario_id: "scenario-one" },
      scenario: { name: "Write a file", criteria: ["writes DONE"] },
      target: {
        type: "http",
        reference_id: "agent-one",
        name: "XiaoBa HTTP",
      },
      execution: {
        status: "FAILED",
        completed_at: "2026-07-31T08:00:02.000Z",
        duration_in_ms: 2_000,
      },
      judge: {
        verdict: "failure",
        reasoning: "The requested file was missing.",
        met_criteria: [],
        unmet_criteria: ["writes DONE"],
      },
      evidence: {
        primary_trace_id: "11111111111111111111111111111111",
      },
    },
    cancel_requested: false,
    created_at: "2026-07-31T08:00:00.000Z",
    updated_at: "2026-07-31T08:00:02.000Z",
    ...overrides,
  };
}
