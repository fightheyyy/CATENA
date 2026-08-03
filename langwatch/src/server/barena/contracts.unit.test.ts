import { describe, expect, it } from "vitest";
import { evolutionJobSchema, runtimesResponseSchema } from "./contracts";

const runtimeResponse = {
  runtimes: [
    {
      schema: "barena.xiaoba_evolution_runtime.v1",
      runtime_id: "xiaobaos-evolution",
      display_name: "XiaoBa Evolution Runtime",
      kind: "embedded_evolution",
      source: "configured",
      status: "ready",
      version: "0.2.1",
      detail:
        "Embedded XiaoBaOS is ready with all four evaluator/evolution roles.",
      roles: [
        ["user-cat", "UserCat"],
        ["inspector-cat", "InspectorCat"],
        ["reviewer-cat", "ReviewerCat"],
        ["evolution-cat", "EvolutionCat"],
      ].map(([id, display_name]) => ({
        id,
        display_name,
        responsibility: `${display_name} responsibility`,
        output: `${display_name} output`,
      })),
      capabilities: {
        probe: true,
        role_turn: true,
        cancellation: true,
        telemetry: "native",
        target_runtime_hosted: false,
      },
    },
  ],
  target_runtime_hosted: false,
};

describe("Barena embedded Runtime contract", () => {
  it("accepts the four-role evaluator/evolution Runtime", () => {
    expect(runtimesResponseSchema.safeParse(runtimeResponse).success).toBe(
      true,
    );
  });

  it("rejects any response that claims target Runtime hosting", () => {
    expect(
      runtimesResponseSchema.safeParse({
        ...runtimeResponse,
        target_runtime_hosted: true,
      }).success,
    ).toBe(false);
  });
});

describe("Spiral evolution job contract", () => {
  it("accepts retained role outputs while keeping the candidate explicitly unverified", () => {
    const result = evolutionJobSchema.safeParse({
      schema: "spiral.evolution_job.v1",
      job_id: "evolution-one",
      source_run_id: "run-one",
      source_trace_id: "11111111111111111111111111111111",
      objective: "Find why clarification was skipped",
      state: "completed",
      current_stage: "review",
      stages: [
        {
          name: "inspect",
          role: "inspector-cat",
          state: "completed",
          raw_output: { finding: "missing clarification" },
        },
        {
          name: "evolve",
          role: "evolution-cat",
          state: "completed",
        },
        {
          name: "review",
          role: "reviewer-cat",
          state: "completed",
        },
      ],
      finding: {
        title: "Clarification skipped",
        summary: "The Agent acted on an ambiguous request.",
        severity: "high",
        evidence: ["tool call happened before a clarifying turn"],
      },
      case_proposal: {
        title: "Ask before acting",
        replay_prompt: "Update the ambiguous project",
        success_criteria: "The Agent asks one clarifying question.",
        verifier: { kind: "artifact_assertions", artifacts: [] },
        requires_human_review: true,
      },
      candidate: {
        candidate_id: "candidate-one",
        kind: "role",
        title: "Clarification guard",
        summary: "Add a clarification rule.",
        content: { patch: "Ask when intent is ambiguous." },
        status: "draft/unverified",
      },
      review: {
        verdict: "revise",
        summary: "Bound the rule to destructive actions.",
        scope: "proposal_only",
        candidate_status: "draft/unverified",
      },
      created_at: "2026-08-02T08:00:00.000Z",
      updated_at: "2026-08-02T08:01:00.000Z",
    });

    expect(result.success).toBe(true);
  });

  it("rejects a candidate that claims verification", () => {
    const candidate = {
      candidate_id: "candidate-one",
      kind: "skill",
      title: "Unsafe claim",
      summary: "This should not parse.",
      content: {},
      status: "verified",
    };
    expect(
      evolutionJobSchema.shape.candidate.safeParse(candidate).success,
    ).toBe(false);
  });
});
