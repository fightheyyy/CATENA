import { describe, expect, it } from "vitest";
import type { BarenaEvolutionJob, BarenaRun } from "~/server/barena/contracts";
import {
  evolutionTraceId,
  isEligibleEvolutionRun,
  resolvedStageState,
} from "./EvolutionStation";
import { caseProposalPrefill } from "./ReleaseWorkbench";

describe("Spiral Evolution Station source selection", () => {
  it("accepts only completed Explore evidence with a retained Trace", () => {
    const run = runFixture();
    expect(evolutionTraceId(run)).toBe("11111111111111111111111111111111");
    expect(isEligibleEvolutionRun(run)).toBe(true);
    expect(isEligibleEvolutionRun({ ...run, state: "failed" })).toBe(false);
    expect(isEligibleEvolutionRun({ ...run, operation: "replay" })).toBe(false);
    expect(
      isEligibleEvolutionRun({
        ...run,
        input: { scenario: { objective: "missing trace" } },
      }),
    ).toBe(false);
  });

  it("supports the older top-level retained Trace shape", () => {
    const run = runFixture();
    run.input = {
      primary_trace_id: "22222222222222222222222222222222",
    };
    expect(evolutionTraceId(run)).toBe("22222222222222222222222222222222");
  });
});

describe("Spiral Evolution Station stage progress", () => {
  it("maps persisted queued/running/completed stages into the visible rail", () => {
    const job = jobFixture();
    expect(resolvedStageState(job, "inspector-cat")).toBe("completed");
    expect(resolvedStageState(job, "evolution-cat")).toBe("running");
    expect(resolvedStageState(job, "reviewer-cat")).toBe("pending");
  });

  it("marks all stages complete for a terminal completed job", () => {
    const job: BarenaEvolutionJob = {
      ...jobFixture(),
      state: "completed",
      stages: [],
    };
    expect(resolvedStageState(job, "reviewer-cat")).toBe("completed");
  });
});

describe("Evolution proposal handoff", () => {
  it("prefills the human Case review from the retained proposal", () => {
    const job: BarenaEvolutionJob = {
      ...jobFixture(),
      case_proposal: {
        title: "Clarify before planning",
        replay_prompt: "Ask one question, then create plan.md.",
        success_criteria: "The plan is specific and verifiable.",
        verifier: {
          kind: "artifact_assertions",
          artifacts: [{ path: "plan.md", contains: "Barena" }],
        },
        requires_human_review: true,
      },
    };

    expect(caseProposalPrefill(job)).toEqual({
      successCriteria: "The plan is specific and verifiable.",
      replayPrompt: "Ask one question, then create plan.md.",
      artifactPath: "plan.md",
      expectedText: "Barena",
    });
  });
});

function runFixture(): BarenaRun {
  return {
    run_id: "run-one",
    request_id: "request-one",
    origin: "platform",
    operation: "explore",
    state: "completed",
    input: {
      scenario: { objective: "Ask before acting" },
      evidence: {
        primary_trace_id: "11111111111111111111111111111111",
      },
    },
    cancel_requested: false,
    created_at: "2026-08-02T08:00:00.000Z",
    updated_at: "2026-08-02T08:00:02.000Z",
  };
}

function jobFixture(): BarenaEvolutionJob {
  return {
    schema: "spiral.evolution_job.v1",
    job_id: "evolution-one",
    source_run_id: "run-one",
    source_trace_id: "11111111111111111111111111111111",
    state: "running",
    current_stage: "evolve",
    stages: [
      {
        name: "inspect",
        role: "inspector-cat",
        state: "completed",
      },
      {
        name: "evolve",
        role: "evolution-cat",
        state: "running",
      },
      {
        name: "review",
        role: "reviewer-cat",
        state: "queued",
      },
    ],
    created_at: "2026-08-02T08:00:00.000Z",
    updated_at: "2026-08-02T08:00:02.000Z",
  };
}
