import { TRPCError } from "@trpc/server";
import { z } from "zod";
import { AgentService } from "~/server/agents/agent.service";
import { getApp } from "~/server/app-layer/app";
import { BarenaControlPlaneError, requestBarena } from "~/server/barena/client";
import {
  artifactAssertionSchema,
  caseSchema,
  casesResponseSchema,
  evaluationSchema,
  evaluationsResponseSchema,
  evolutionJobSchema,
  evolutionJobsResponseSchema,
  issueSchema,
  issuesResponseSchema,
  releaseSchema,
  releasesResponseSchema,
  runSchema,
  runsResponseSchema,
  runtimesResponseSchema,
  scenarioAdoptionResponseSchema,
} from "~/server/barena/contracts";
import { buildScenarioRunAdoption } from "~/server/barena/scenario-adoption";
import { ScenarioService } from "~/server/scenarios/scenario.service";
import { checkProjectPermission } from "../rbac";
import { createTRPCRouter, protectedProcedure } from "../trpc";

const projectInput = z.object({ projectId: z.string().min(1) });

function controlContext(projectId: string, actorId: string) {
  return { projectId, actorId };
}

export const barenaRouter = createTRPCRouter({
  listRuntimes: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena("/v1/runtimes", runtimesResponseSchema, {
          ...controlContext(input.projectId, ctx.session.user.id),
        }),
      ),
    ),

  adoptScenarioRun: protectedProcedure
    .input(
      projectInput.extend({
        scenarioRunId: z.string().min(1).max(256),
      }),
    )
    .use(checkProjectPermission("scenarios:view"))
    .use(checkProjectPermission("traces:view"))
    .use(checkProjectPermission("evaluations:manage"))
    .mutation(async ({ ctx, input }) => {
      const run = await getApp().simulations.runs.getScenarioRunData({
        projectId: input.projectId,
        scenarioRunId: input.scenarioRunId,
      });
      if (!run) {
        throw new TRPCError({
          code: "NOT_FOUND",
          message: "Explore run not found",
        });
      }
      const scenario = await ScenarioService.create(
        ctx.prisma,
      ).getByIdIncludingArchived({
        id: run.scenarioId,
        projectId: input.projectId,
      });
      if (!scenario) {
        throw new TRPCError({
          code: "PRECONDITION_FAILED",
          message: "The retained Scenario definition is unavailable",
        });
      }
      const targetReferenceId = run.metadata?.langwatch?.targetReferenceId;
      if (
        run.metadata?.langwatch?.targetType !== "http" ||
        !targetReferenceId
      ) {
        throw new TRPCError({
          code: "PRECONDITION_FAILED",
          message:
            "Only registered HTTP Agent Explore runs can be adopted here",
        });
      }
      const agent = await AgentService.create(ctx.prisma).getById({
        id: targetReferenceId,
        projectId: input.projectId,
      });
      if (agent?.type !== "http") {
        throw new TRPCError({
          code: "PRECONDITION_FAILED",
          message: "The registered HTTP Agent is unavailable",
        });
      }
      let adoption;
      try {
        adoption = buildScenarioRunAdoption({
          projectId: input.projectId,
          run,
          scenario,
          agent,
        });
      } catch (error) {
        throw new TRPCError({
          code: "PRECONDITION_FAILED",
          message:
            error instanceof Error
              ? error.message
              : "Explore evidence is incomplete",
        });
      }
      let retainedTrace;
      try {
        retainedTrace = await getApp().traces.summary.getByTraceId(
          input.projectId,
          adoption.primary_trace_id,
          {},
        );
      } catch {
        retainedTrace = undefined;
      }
      if (!retainedTrace) {
        throw new TRPCError({
          code: "PRECONDITION_FAILED",
          message:
            "The HTTP Agent Trace has not reached the project yet; wait for OTLP ingestion and retry",
        });
      }
      return forward(() =>
        requestBarena(
          "/v1/platform/scenario-runs/adopt",
          scenarioAdoptionResponseSchema,
          {
            method: "POST",
            body: adoption,
            ...controlContext(input.projectId, ctx.session.user.id),
          },
        ),
      );
    }),

  listRuns: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena("/v1/runs?limit=100", runsResponseSchema, {
          ...controlContext(input.projectId, ctx.session.user.id),
        }),
      ),
    ),

  listEvolutionJobs: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          "/v1/evolution-jobs?limit=100",
          evolutionJobsResponseSchema,
          {
            ...controlContext(input.projectId, ctx.session.user.id),
          },
        ),
      ),
    ),

  getEvolutionJob: protectedProcedure
    .input(projectInput.extend({ jobId: z.string().min(1) }))
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/evolution-jobs/${encodeURIComponent(input.jobId)}`,
          evolutionJobSchema,
          controlContext(input.projectId, ctx.session.user.id),
        ),
      ),
    ),

  startEvolutionJob: protectedProcedure
    .input(
      projectInput.extend({
        runId: z.string().min(1),
        traceId: z.string().min(1).max(256),
        objective: z.string().max(4000).optional(),
        idempotencyKey: z.string().min(1).max(200),
      }),
    )
    .use(checkProjectPermission("evaluations:manage"))
    .mutation(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/runs/${encodeURIComponent(input.runId)}/evolution-jobs`,
          evolutionJobSchema,
          {
            method: "POST",
            idempotencyKey: input.idempotencyKey,
            body: {
              trace_id: input.traceId,
              objective: input.objective || undefined,
            },
            ...controlContext(input.projectId, ctx.session.user.id),
          },
        ),
      ),
    ),

  listIssues: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena("/v1/issues?limit=100", issuesResponseSchema, {
          ...controlContext(input.projectId, ctx.session.user.id),
        }),
      ),
    ),

  createIssue: protectedProcedure
    .input(
      projectInput.extend({
        runId: z.string().min(1),
        traceId: z.string().max(256).optional(),
        title: z.string().min(3).max(160),
        summary: z.string().min(1).max(4000),
        severity: z.enum(["low", "medium", "high", "critical"]),
      }),
    )
    .use(checkProjectPermission("evaluations:manage"))
    .mutation(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/runs/${encodeURIComponent(input.runId)}/issues`,
          issueSchema,
          {
            method: "POST",
            body: {
              trace_id: input.traceId || undefined,
              title: input.title,
              summary: input.summary,
              severity: input.severity,
            },
            ...controlContext(input.projectId, ctx.session.user.id),
          },
        ),
      ),
    ),

  promoteIssue: protectedProcedure
    .input(
      projectInput.extend({
        issueId: z.string().min(1),
        replayPrompt: z.string().max(24_000).optional(),
        successCriteria: z.string().min(1).max(4000),
        artifacts: z.array(artifactAssertionSchema).min(1),
      }),
    )
    .use(checkProjectPermission("evaluations:manage"))
    .mutation(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/issues/${encodeURIComponent(input.issueId)}/promote`,
          caseSchema,
          {
            method: "POST",
            body: {
              replay_prompt: input.replayPrompt || undefined,
              success_criteria: input.successCriteria,
              verifier: {
                kind: "artifact_assertions",
                artifacts: input.artifacts,
              },
            },
            ...controlContext(input.projectId, ctx.session.user.id),
          },
        ),
      ),
    ),

  listCases: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena("/v1/cases?limit=100", casesResponseSchema, {
          ...controlContext(input.projectId, ctx.session.user.id),
        }),
      ),
    ),

  replayCase: protectedProcedure
    .input(
      projectInput.extend({
        caseId: z.string().min(1),
        idempotencyKey: z.string().min(1).max(200),
      }),
    )
    .use(checkProjectPermission("evaluations:manage"))
    .mutation(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/cases/${encodeURIComponent(input.caseId)}/replay`,
          runSchema,
          {
            method: "POST",
            idempotencyKey: input.idempotencyKey,
            ...controlContext(input.projectId, ctx.session.user.id),
          },
        ),
      ),
    ),

  listEvaluations: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena("/v1/evaluations?limit=100", evaluationsResponseSchema, {
          ...controlContext(input.projectId, ctx.session.user.id),
        }),
      ),
    ),

  getEvaluation: protectedProcedure
    .input(projectInput.extend({ evaluationId: z.string().min(1) }))
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/evaluations/${encodeURIComponent(input.evaluationId)}`,
          evaluationSchema,
          controlContext(input.projectId, ctx.session.user.id),
        ),
      ),
    ),

  listReleases: protectedProcedure
    .input(projectInput)
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena("/v1/releases?limit=100", releasesResponseSchema, {
          ...controlContext(input.projectId, ctx.session.user.id),
        }),
      ),
    ),

  getRelease: protectedProcedure
    .input(projectInput.extend({ releaseId: z.string().min(1) }))
    .use(checkProjectPermission("evaluations:view"))
    .query(async ({ ctx, input }) =>
      forward(() =>
        requestBarena(
          `/v1/releases/${encodeURIComponent(input.releaseId)}`,
          releaseSchema,
          controlContext(input.projectId, ctx.session.user.id),
        ),
      ),
    ),
});

async function forward<T>(request: () => Promise<T>): Promise<T> {
  try {
    return await request();
  } catch (error) {
    if (error instanceof BarenaControlPlaneError) {
      throw new TRPCError({
        code: trpcCode(error.status),
        message: error.message,
      });
    }
    throw new TRPCError({
      code: "INTERNAL_SERVER_ERROR",
      message: "Barena request failed",
    });
  }
}

function trpcCode(status: number) {
  if (status === 400) return "BAD_REQUEST" as const;
  if (status === 401) return "UNAUTHORIZED" as const;
  if (status === 403) return "FORBIDDEN" as const;
  if (status === 404) return "NOT_FOUND" as const;
  if (status === 409) return "CONFLICT" as const;
  if (status === 503) return "INTERNAL_SERVER_ERROR" as const;
  return "INTERNAL_SERVER_ERROR" as const;
}
