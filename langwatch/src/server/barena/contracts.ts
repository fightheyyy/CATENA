import { z } from "zod";

const isoDate = z.string().datetime({ offset: true });
const jsonObject = z.record(z.unknown());

export const runStateSchema = z.enum([
  "queued",
  "running",
  "completed",
  "interrupted",
  "cancelled",
  "failed",
]);

export const runSchema = z.object({
  run_id: z.string().min(1),
  request_id: z.string().min(1),
  origin: z.enum(["local", "edge", "platform"]),
  operation: z.enum(["explore", "replay", "compare"]),
  state: runStateSchema,
  current_phase: z.string().optional(),
  current_actor: z.string().optional(),
  input: jsonObject,
  runtime: jsonObject.optional(),
  cancel_requested: z.boolean(),
  error: z.string().optional(),
  created_at: isoDate,
  updated_at: isoDate,
});

export const issueSchema = z.object({
  issue_id: z.string().min(1),
  source_run_id: z.string().min(1),
  source_trace_id: z.string().optional(),
  title: z.string(),
  summary: z.string(),
  severity: z.enum(["low", "medium", "high", "critical"]),
  status: z.enum(["open", "promoted", "dismissed"]),
  promoted_case_id: z.string().optional(),
  created_at: isoDate,
  updated_at: isoDate,
});

export const artifactAssertionSchema = z
  .object({
    path: z.string().min(1),
    exists: z.boolean().optional(),
    contains: z.string().optional(),
  })
  .strict();

export const caseSchema = z.object({
  schema: z.literal("barena.case.v1"),
  case_id: z.string().min(1),
  revision: z.number().int().positive(),
  source_issue_id: z.string().min(1),
  source_run_id: z.string().min(1),
  source_trace_id: z.string().optional(),
  title: z.string(),
  operation: z.literal("explore"),
  input: jsonObject,
  runtime: jsonObject.optional(),
  replay_prompt: z.string(),
  success_criteria: z.string(),
  verifier: z
    .object({
      kind: z.literal("artifact_assertions"),
      artifacts: z.array(artifactAssertionSchema).min(1),
    })
    .passthrough(),
  created_at: isoDate,
});

export const releaseDecisionSchema = z.enum(["cleared", "held", "rejected"]);

export const evaluationSchema = z.object({
  evaluation_id: z.string().min(1),
  harness_version_id: z.string().min(1),
  case_id: z.string().min(1),
  run_id: z.string().min(1),
  source_run_id: z.string().min(1),
  source_trace_id: z.string().optional(),
  replay_trace_id: z.string().optional(),
  terminal_event_id: z.string().min(1),
  package_status: z.string().min(1),
  result_status: z.string().optional(),
  decision: releaseDecisionSchema,
  summary: z.string().optional(),
  result_ref: z.string().min(1),
  created_at: isoDate,
});

export const releaseSchema = z.object({
  release_id: z.string().min(1),
  harness_version_id: z.string().min(1),
  evaluation_id: z.string().min(1),
  case_id: z.string().min(1),
  run_id: z.string().min(1),
  source_run_id: z.string().min(1),
  source_trace_id: z.string().optional(),
  replay_trace_id: z.string().optional(),
  terminal_event_id: z.string().min(1),
  decision: releaseDecisionSchema,
  summary: z.string().optional(),
  created_at: isoDate,
});

export const evolutionRuntimeRoleSchema = z.object({
  id: z.enum(["user-cat", "inspector-cat", "reviewer-cat", "evolution-cat"]),
  display_name: z.string().min(1).max(80),
  responsibility: z.string().min(1).max(500),
  output: z.string().min(1).max(120),
});

export const evolutionRuntimeSchema = z.object({
  schema: z.literal("barena.xiaoba_evolution_runtime.v1"),
  runtime_id: z.literal("xiaobaos-evolution"),
  display_name: z.literal("XiaoBa Evolution Runtime"),
  kind: z.literal("embedded_evolution"),
  source: z.literal("configured"),
  status: z.enum(["ready", "blocked"]),
  version: z.string().max(120).optional(),
  reason_code: z.string().max(120).optional(),
  detail: z.string().min(1).max(500),
  roles: z.array(evolutionRuntimeRoleSchema).length(4),
  capabilities: z.object({
    probe: z.literal(true),
    role_turn: z.literal(true),
    cancellation: z.literal(true),
    telemetry: z.literal("native"),
    target_runtime_hosted: z.literal(false),
  }),
});

export const evolutionJobStateSchema = z.enum([
  "queued",
  "running",
  "completed",
  "failed",
]);

export const evolutionStageNameSchema = z.enum([
  "inspector-cat",
  "evolution-cat",
  "reviewer-cat",
]);

export const evolutionStageSchema = z.object({
  name: z.string().min(1),
  role: evolutionStageNameSchema,
  state: z.enum(["queued", "running", "completed", "failed"]),
  raw_output: z.unknown().optional(),
  error: z.string().optional(),
  started_at: isoDate.optional(),
  finished_at: isoDate.optional(),
});

export const evolutionFindingSchema = z.object({
  title: z.string().min(1),
  summary: z.string().min(1),
  severity: z.enum(["unknown", "low", "medium", "high", "critical"]),
  evidence: z.array(z.string()),
});

export const evolutionCaseProposalSchema = z.object({
  title: z.string().min(1),
  replay_prompt: z.string().min(1),
  success_criteria: z.string().min(1),
  verifier: z.unknown(),
  requires_human_review: z.boolean(),
});

export const evolutionCandidateSchema = z.object({
  candidate_id: z.string().min(1),
  kind: z.enum(["role", "skill", "memory", "harness"]),
  title: z.string().min(1),
  summary: z.string().min(1),
  content: z.unknown(),
  status: z.literal("draft/unverified"),
});

export const evolutionReviewSchema = z.object({
  verdict: z.string().min(1),
  summary: z.string().min(1),
  scope: z.literal("proposal_only"),
  candidate_status: z.literal("draft/unverified"),
});

export const evolutionJobSchema = z.object({
  schema: z.literal("spiral.evolution_job.v1"),
  job_id: z.string().min(1),
  source_run_id: z.string().min(1),
  source_trace_id: z.string().min(1),
  objective: z.string().optional(),
  state: evolutionJobStateSchema,
  current_stage: z.string().optional(),
  stages: z.array(evolutionStageSchema),
  finding: evolutionFindingSchema.optional(),
  case_proposal: evolutionCaseProposalSchema.optional(),
  candidate: evolutionCandidateSchema.optional(),
  review: evolutionReviewSchema.optional(),
  error: z.string().optional(),
  created_at: isoDate,
  updated_at: isoDate,
});

export const runsResponseSchema = z.object({ runs: z.array(runSchema) });
export const issuesResponseSchema = z.object({ issues: z.array(issueSchema) });
export const casesResponseSchema = z.object({ cases: z.array(caseSchema) });
export const evaluationsResponseSchema = z.object({
  evaluations: z.array(evaluationSchema),
});
export const releasesResponseSchema = z.object({
  releases: z.array(releaseSchema),
});
export const runtimesResponseSchema = z.object({
  runtimes: z.array(evolutionRuntimeSchema).length(1),
  target_runtime_hosted: z.literal(false),
});
export const evolutionJobsResponseSchema = z.object({
  evolution_jobs: z.array(evolutionJobSchema),
});
export const scenarioAdoptionResponseSchema = z.object({
  run: runSchema,
  created: z.boolean(),
  trace_ids: z.array(z.string().regex(/^[a-f0-9]{32}$/i)).min(1),
  primary_trace_id: z.string().regex(/^[a-f0-9]{32}$/i),
});

export const problemSchema = z.object({
  status: z.number().optional(),
  detail: z.string().optional(),
});

export type BarenaRun = z.infer<typeof runSchema>;
export type BarenaIssue = z.infer<typeof issueSchema>;
export type BarenaCase = z.infer<typeof caseSchema>;
export type BarenaEvaluation = z.infer<typeof evaluationSchema>;
export type BarenaRelease = z.infer<typeof releaseSchema>;
export type BarenaEvolutionRuntime = z.infer<typeof evolutionRuntimeSchema>;
export type BarenaEvolutionJob = z.infer<typeof evolutionJobSchema>;
export type BarenaEvolutionStageName = z.infer<typeof evolutionStageNameSchema>;
