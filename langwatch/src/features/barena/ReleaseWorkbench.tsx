import {
  Badge,
  Box,
  Button,
  Code,
  HStack,
  Icon,
  Spinner,
  Table,
  Tabs,
  Text,
  VStack,
} from "@chakra-ui/react";
import { useEffect, useState } from "react";
import {
  LuArchiveRestore,
  LuCircleCheckBig,
  LuFlag,
  LuGauge,
  LuGitCompareArrows,
  LuPlay,
  LuRefreshCw,
  LuShieldCheck,
  LuSlidersHorizontal,
} from "react-icons/lu";
import { Link } from "~/components/ui/link";
import { toaster } from "~/components/ui/toaster";
import { useBarenaI18n } from "~/features/barena/i18n";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import type {
  BarenaCase,
  BarenaEvaluation,
  BarenaEvolutionJob,
  BarenaIssue,
  BarenaRelease,
  BarenaRun,
} from "~/server/barena/contracts";
import { api } from "~/utils/api";
import { useRouter } from "~/utils/compat/next-router";
import { CompareRunsPanel } from "./CompareRunsPanel";
import { CreateIssuePanel } from "./CreateIssuePanel";
import { EvolutionRuntimeStatus } from "./EvolutionRuntimeStatus";
import { EvolutionStation } from "./EvolutionStation";
import {
  type PromoteIssueInitialValues,
  PromoteIssuePanel,
} from "./PromoteIssuePanel";

type WorkbenchTab =
  | "evolve"
  | "issues"
  | "cases"
  | "evaluations"
  | "compare"
  | "releases";

export function ReleaseWorkbench() {
  const { project, hasPermission } = useOrganizationTeamProject();
  const { t } = useBarenaI18n();
  const router = useRouter();
  const [tab, setTab] = useState<WorkbenchTab>("evolve");
  const [showCreateIssue, setShowCreateIssue] = useState(false);
  const [reviewIssueId, setReviewIssueId] = useState<string | null>(null);
  const [proposalPrefill, setProposalPrefill] =
    useState<PromoteIssueInitialValues>();
  const projectId = project?.id ?? "";
  const utils = api.useUtils();

  const runs = api.barena.listRuns.useQuery(
    { projectId },
    {
      enabled: !!projectId,
      refetchInterval: 2_000,
      refetchOnWindowFocus: false,
    },
  );
  const issues = api.barena.listIssues.useQuery(
    { projectId },
    { enabled: !!projectId, refetchOnWindowFocus: false },
  );
  const cases = api.barena.listCases.useQuery(
    { projectId },
    { enabled: !!projectId, refetchOnWindowFocus: false },
  );
  const evaluations = api.barena.listEvaluations.useQuery(
    { projectId },
    {
      enabled: !!projectId,
      refetchInterval: hasActiveReplay(runs.data?.runs) ? 2_000 : false,
      refetchOnWindowFocus: false,
    },
  );
  const releases = api.barena.listReleases.useQuery(
    { projectId },
    {
      enabled: !!projectId,
      refetchInterval: hasActiveReplay(runs.data?.runs) ? 2_000 : false,
      refetchOnWindowFocus: false,
    },
  );

  const terminalReplayEvidenceKey =
    runs.data?.runs
      .filter((run) => run.operation === "replay" && isTerminalRun(run.state))
      .map((run) => run.run_id)
      .sort()
      .join(",") ?? "";

  // A terminal Run and its Evaluation/Release are committed together, but the
  // independent queries can finish in the opposite order. Refresh the evidence
  // queries once whenever the terminal Replay set changes so the UI never
  // mislabels a successful Replay as missing evidence.
  useEffect(() => {
    if (!projectId || !terminalReplayEvidenceKey) return;
    void Promise.all([
      utils.barena.listEvaluations.invalidate({ projectId }),
      utils.barena.listReleases.invalidate({ projectId }),
    ]);
  }, [projectId, terminalReplayEvidenceKey, utils]);

  const prefillRunId = queryValue(router.query.runId);
  const prefillTraceId = queryValue(router.query.traceId);
  useEffect(() => {
    if (queryValue(router.query.newIssue) === "1") {
      setShowCreateIssue(true);
      setTab("issues");
    }
  }, [router.query.newIssue]);

  const canManage = hasPermission("evaluations:manage");
  const replay = api.barena.replayCase.useMutation({
    onSuccess: (run) => {
      toaster.create({
        title: t("Replay started"),
        description: t("{runId} is now producing release evidence.", {
          runId: run.run_id,
        }),
        type: "success",
      });
      void utils.barena.listRuns.invalidate({ projectId });
      setTab("evaluations");
    },
    onError: (error) => {
      toaster.create({
        title: t("Replay could not start"),
        description: error.message,
        type: "error",
      });
    },
  });
  const retainEvolutionFinding = api.barena.createIssue.useMutation();

  const allQueries = [runs, issues, cases, evaluations, releases];
  const firstError = allQueries.find((query) => query.isError)?.error;
  const loading = allQueries.some((query) => query.isLoading);
  const reviewedIssue = issues.data?.issues.find(
    (issue) => issue.issue_id === reviewIssueId,
  );
  const evaluationRunIds = new Set(
    evaluations.data?.evaluations.map((evaluation) => evaluation.run_id) ?? [],
  );
  const replayRunsWithoutEvaluation =
    runs.data?.runs.filter(
      (run) => run.operation === "replay" && !evaluationRunIds.has(run.run_id),
    ) ?? [];

  const refreshAll = () => {
    void Promise.all(allQueries.map((query) => query.refetch()));
  };

  const reviewEvolutionProposal = async (job: BarenaEvolutionJob) => {
    if (!job.finding || !job.case_proposal) return;
    const existing = issues.data?.issues.find(
      (issue) =>
        issue.source_run_id === job.source_run_id &&
        issue.title === job.finding?.title,
    );
    if (existing?.status === "promoted") {
      setTab("cases");
      toaster.create({
        title: t("Finding already promoted"),
        description: t("Its immutable Case is ready for Replay."),
        type: "info",
      });
      return;
    }
    try {
      const issue =
        existing ??
        (await retainEvolutionFinding.mutateAsync({
          projectId,
          runId: job.source_run_id,
          traceId: job.source_trace_id,
          title: job.finding.title,
          summary: job.finding.summary,
          severity:
            job.finding.severity === "unknown"
              ? "medium"
              : job.finding.severity,
        }));
      setProposalPrefill(caseProposalPrefill(job));
      setReviewIssueId(issue.issue_id);
      setTab("issues");
      void utils.barena.listIssues.invalidate({ projectId });
      toaster.create({
        title: t("Finding retained"),
        description: t(
          "Review the proposed prompt and verifier before creating the Case.",
        ),
        type: "success",
      });
    } catch (error) {
      toaster.create({
        title: t("Finding could not be retained"),
        description:
          error instanceof Error ? error.message : t("Request failed"),
        type: "error",
      });
    }
  };

  if (!project) return null;

  return (
    <VStack
      align="stretch"
      gap={4}
      width="full"
      maxWidth="1180px"
      marginX="auto"
      padding={{ base: 4, md: 6 }}
    >
      <HStack justify="space-between" align="start" gap={4}>
        <Box>
          <HStack gap={2} marginBottom={1}>
            <Badge colorPalette="orange" variant="subtle">
              Barena
            </Badge>
            <Text textStyle="sm" color="fg.muted">
              {t("Agent evolution control plane")}
            </Text>
          </HStack>
          <Text fontSize="2xl" fontWeight="semibold" letterSpacing="-0.02em">
            {t("Turn one real Trace into a reviewable change")}
          </Text>
          <Text color="fg.muted" textStyle="sm" marginTop={1} maxWidth="680px">
            {t(
              "Select retained runtime evidence. XiaoBaOS inspects the failure, proposes the smallest change, then reviews the proposal before any verification or release.",
            )}
          </Text>
        </Box>
        <HStack gap={1} flexShrink={0}>
          <Button asChild size="sm" variant="ghost">
            <Link
              href={`/${project.slug}/prompts`}
              textDecoration="none"
              color="fg.muted"
            >
              <LuSlidersHorizontal /> {t("Advanced role prompts")}
            </Link>
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-label={t("Refresh workbench")}
            title={t("Refresh")}
            onClick={refreshAll}
            paddingX={2}
          >
            <LuRefreshCw />
          </Button>
        </HStack>
      </HStack>

      <EvolutionRuntimeStatus compact />

      {firstError && (
        <Box
          borderWidth="1px"
          borderColor="red.300"
          bg="red.50"
          color="red.700"
          _dark={{ bg: "red.950", color: "red.200", borderColor: "red.700" }}
          borderRadius="lg"
          padding={4}
        >
          <Text fontWeight="semibold">{t("Control plane unavailable")}</Text>
          <Text textStyle="sm">{firstError.message}</Text>
        </Box>
      )}

      {showCreateIssue && canManage && (
        <CreateIssuePanel
          projectId={projectId}
          runs={runs.data?.runs ?? []}
          initialRunId={prefillRunId}
          initialTraceId={prefillTraceId}
          onClose={() => setShowCreateIssue(false)}
          onCreated={() => {
            setShowCreateIssue(false);
            void utils.barena.listIssues.invalidate({ projectId });
            void router.replace(
              { pathname: `/${project.slug}/evolution` },
              undefined,
              { shallow: true },
            );
          }}
        />
      )}

      {reviewedIssue && reviewedIssue.status === "open" && canManage && (
        <PromoteIssuePanel
          key={reviewedIssue.issue_id}
          projectId={projectId}
          issue={reviewedIssue}
          initialValues={proposalPrefill}
          onClose={() => {
            setReviewIssueId(null);
            setProposalPrefill(undefined);
          }}
          onPromoted={() => {
            setReviewIssueId(null);
            setProposalPrefill(undefined);
            void Promise.all([
              utils.barena.listIssues.invalidate({ projectId }),
              utils.barena.listCases.invalidate({ projectId }),
            ]);
            setTab("cases");
          }}
        />
      )}

      <Tabs.Root
        value={tab}
        onValueChange={({ value }) => setTab(value as WorkbenchTab)}
        variant="line"
      >
        <Box
          borderBottomWidth="1px"
          borderColor="border"
          overflowX="auto"
          overflowY="hidden"
        >
          <Tabs.List minWidth="max-content">
            <Tab value="evolve" label={t("Evolution")} />
            <Tab
              value="issues"
              label={t("Issues")}
              count={issues.data?.issues.length}
            />
            <Tab
              value="cases"
              label={t("Regression Cases")}
              count={cases.data?.cases.length}
            />
            <Tab
              value="evaluations"
              label={t("Evaluations")}
              count={
                (evaluations.data?.evaluations.length ?? 0) +
                replayRunsWithoutEvaluation.length
              }
            />
            <Tab
              value="compare"
              label={t("Compare")}
              count={
                runs.data?.runs.filter(
                  (run) =>
                    run.origin === "platform" &&
                    run.operation === "explore" &&
                    run.state === "completed",
                ).length
              }
            />
            <Tab
              value="releases"
              label={t("Release gates")}
              count={releases.data?.releases.length}
            />
          </Tabs.List>
        </Box>
        <Box minHeight="320px">
          {loading && !firstError ? (
            <HStack justify="center" padding={12}>
              <Spinner size="sm" />
              <Text color="fg.muted">{t("Loading retained evidence…")}</Text>
            </HStack>
          ) : (
            <>
              <Tabs.Content value="evolve" padding={0}>
                <EvolutionStation
                  projectId={projectId}
                  projectSlug={project.slug}
                  runs={runs.data?.runs ?? []}
                  canManage={canManage}
                  initialRunId={prefillRunId}
                  reviewingProposal={retainEvolutionFinding.isLoading}
                  onReviewProposal={(job) => void reviewEvolutionProposal(job)}
                />
              </Tabs.Content>
              <Tabs.Content value="issues" padding={0}>
                {canManage && (
                  <HStack justify="flex-end" paddingTop={4}>
                    <Button
                      size="xs"
                      variant="outline"
                      onClick={() => setShowCreateIssue(true)}
                    >
                      <LuFlag /> {t("New issue")}
                    </Button>
                  </HStack>
                )}
                <Box
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="xl"
                  overflow="hidden"
                  marginTop={4}
                >
                  <IssuesTable
                    issues={issues.data?.issues ?? []}
                    projectSlug={project.slug}
                    canManage={canManage}
                    onReview={(issueId) => {
                      setProposalPrefill(undefined);
                      setReviewIssueId(issueId);
                    }}
                  />
                </Box>
              </Tabs.Content>
              <Tabs.Content value="cases" paddingTop={4}>
                <Box
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="xl"
                  overflow="hidden"
                >
                  <CasesTable
                    cases={cases.data?.cases ?? []}
                    projectSlug={project.slug}
                    canManage={canManage}
                    replaying={replay.isLoading}
                    onReplay={(value) =>
                      replay.mutate({
                        projectId,
                        caseId: value.case_id,
                        idempotencyKey: `${value.case_id}-${globalThis.crypto.randomUUID()}`,
                      })
                    }
                  />
                </Box>
              </Tabs.Content>
              <Tabs.Content value="evaluations" paddingTop={4}>
                <Box
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="xl"
                  overflow="hidden"
                >
                  <EvaluationsTable
                    evaluations={evaluations.data?.evaluations ?? []}
                    unmatchedRuns={replayRunsWithoutEvaluation}
                    projectSlug={project.slug}
                  />
                </Box>
              </Tabs.Content>
              <Tabs.Content value="compare" paddingTop={4}>
                <Box
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="xl"
                  overflow="hidden"
                >
                  <CompareRunsPanel
                    runs={runs.data?.runs ?? []}
                    projectSlug={project.slug}
                  />
                </Box>
              </Tabs.Content>
              <Tabs.Content value="releases" paddingTop={4}>
                <Box
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="xl"
                  overflow="hidden"
                >
                  <ReleasesTable
                    releases={releases.data?.releases ?? []}
                    projectSlug={project.slug}
                  />
                </Box>
              </Tabs.Content>
            </>
          )}
        </Box>
      </Tabs.Root>
    </VStack>
  );
}

function Tab({
  value,
  label,
  count,
}: {
  value: WorkbenchTab;
  label: string;
  count?: number;
}) {
  return (
    <Tabs.Trigger
      value={value}
      gap={2}
      color="fg.muted"
      fontWeight="medium"
      _selected={{ color: "orange.fg", borderColor: "orange.solid" }}
    >
      {label}
      {count !== undefined && count > 0 && (
        <Badge size="sm" variant="subtle">
          {count}
        </Badge>
      )}
    </Tabs.Trigger>
  );
}

function IssuesTable({
  issues,
  projectSlug,
  canManage,
  onReview,
}: {
  issues: BarenaIssue[];
  projectSlug: string;
  canManage: boolean;
  onReview: (issueId: string) => void;
}) {
  const { t } = useBarenaI18n();
  if (issues.length === 0) {
    return (
      <EmptyState
        icon={LuFlag}
        title={t("No retained issues")}
        description={t(
          "Open a Barena-linked trace or choose New issue to turn runtime evidence into a reviewable failure.",
        )}
      />
    );
  }
  return (
    <Table.Root variant="line">
      <Table.Header>
        <Table.Row>
          <Table.ColumnHeader>{t("Issue")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Evidence")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Severity")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Status")}</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">{t("Action")}</Table.ColumnHeader>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {issues.map((issue) => (
          <Table.Row key={issue.issue_id}>
            <Table.Cell maxWidth="420px">
              <Text fontWeight="semibold">{issue.title}</Text>
              <Text textStyle="sm" color="fg.muted" lineClamp={2}>
                {issue.summary}
              </Text>
            </Table.Cell>
            <Table.Cell>
              <VStack align="start" gap={1}>
                <Code size="sm">{shortId(issue.source_run_id)}</Code>
                {issue.source_trace_id && (
                  <TraceLink
                    projectSlug={projectSlug}
                    traceId={issue.source_trace_id}
                  />
                )}
              </VStack>
            </Table.Cell>
            <Table.Cell>
              <Badge colorPalette={severityPalette(issue.severity)}>
                {t(severityMessage(issue.severity))}
              </Badge>
            </Table.Cell>
            <Table.Cell>
              <Badge
                colorPalette={issue.status === "promoted" ? "green" : "gray"}
                variant="subtle"
              >
                {t(issue.status === "promoted" ? "Promoted" : "Open")}
              </Badge>
            </Table.Cell>
            <Table.Cell textAlign="end">
              {canManage && issue.status === "open" ? (
                <Button
                  size="xs"
                  variant="outline"
                  onClick={() => onReview(issue.issue_id)}
                >
                  {t("Review as Case")}
                </Button>
              ) : issue.promoted_case_id ? (
                <Code size="sm">{shortId(issue.promoted_case_id)}</Code>
              ) : (
                "—"
              )}
            </Table.Cell>
          </Table.Row>
        ))}
      </Table.Body>
    </Table.Root>
  );
}

function CasesTable({
  cases,
  projectSlug,
  canManage,
  replaying,
  onReplay,
}: {
  cases: BarenaCase[];
  projectSlug: string;
  canManage: boolean;
  replaying: boolean;
  onReplay: (value: BarenaCase) => void;
}) {
  const { t } = useBarenaI18n();
  if (cases.length === 0) {
    return (
      <EmptyState
        icon={LuArchiveRestore}
        title={t("No immutable Cases")}
        description={t(
          "Review one Issue and define its deterministic artifact verifier.",
        )}
      />
    );
  }
  return (
    <Table.Root variant="line">
      <Table.Header>
        <Table.Row>
          <Table.ColumnHeader>{t("Case")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Success contract")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Provenance")}</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">{t("Replay")}</Table.ColumnHeader>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {cases.map((value) => (
          <Table.Row key={value.case_id}>
            <Table.Cell>
              <HStack gap={2}>
                <Text fontWeight="semibold">{value.title}</Text>
                <Badge variant="outline">
                  {t("revision {revision}", { revision: value.revision })}
                </Badge>
              </HStack>
              <Code size="sm">{shortId(value.case_id)}</Code>
            </Table.Cell>
            <Table.Cell maxWidth="430px">
              <Text textStyle="sm" lineClamp={2}>
                {value.success_criteria}
              </Text>
              <Text textStyle="xs" color="fg.muted" fontFamily="mono">
                {value.verifier.artifacts[0]?.path}
              </Text>
            </Table.Cell>
            <Table.Cell>
              <VStack align="start" gap={1}>
                <Code size="sm">{shortId(value.source_run_id)}</Code>
                {value.source_trace_id && (
                  <TraceLink
                    projectSlug={projectSlug}
                    traceId={value.source_trace_id}
                  />
                )}
              </VStack>
            </Table.Cell>
            <Table.Cell textAlign="end">
              {replayCapability(value).reason && (
                <Text
                  textStyle="xs"
                  color="fg.muted"
                  marginBottom={2}
                  maxWidth="260px"
                  marginLeft="auto"
                >
                  {t("Replay unavailable: {reason}", {
                    reason: replayCapability(value).reason ?? "",
                  })}
                </Text>
              )}
              <Button
                size="xs"
                colorPalette="orange"
                disabled={
                  !canManage || replaying || !replayCapability(value).supported
                }
                title={replayCapability(value).reason}
                onClick={() => onReplay(value)}
              >
                <LuPlay /> {t("Replay Case")}
              </Button>
            </Table.Cell>
          </Table.Row>
        ))}
      </Table.Body>
    </Table.Root>
  );
}

function EvaluationsTable({
  evaluations,
  unmatchedRuns,
  projectSlug,
}: {
  evaluations: BarenaEvaluation[];
  unmatchedRuns: BarenaRun[];
  projectSlug: string;
}) {
  const { locale, t } = useBarenaI18n();
  if (evaluations.length === 0 && unmatchedRuns.length === 0) {
    return (
      <EmptyState
        icon={LuGitCompareArrows}
        title={t("No Replay evaluations")}
        description={t(
          "Replay an immutable Case. The TypeScript Engine's terminal decision will appear here unchanged.",
        )}
      />
    );
  }
  return (
    <VStack align="stretch" gap={0}>
      {unmatchedRuns.map((run) => (
        <HStack
          key={run.run_id}
          padding={4}
          borderBottomWidth="1px"
          borderColor="border"
          bg={isTerminalRun(run.state) ? "red.50" : "orange.50"}
          _dark={{
            bg: isTerminalRun(run.state) ? "red.950" : "orange.950",
          }}
        >
          {isTerminalRun(run.state) ? (
            <Icon color="red.500">
              <LuFlag />
            </Icon>
          ) : (
            <Spinner size="xs" />
          )}
          <Text fontWeight="medium">
            {isTerminalRun(run.state)
              ? t("Replay failed")
              : t("Replay in progress")}
          </Text>
          <Code size="sm">{shortId(run.run_id)}</Code>
          <Text color="fg.muted" textStyle="sm">
            {isTerminalRun(run.state)
              ? run.error || t("Replay ended without evaluation evidence")
              : run.current_actor || run.current_phase || run.state}
          </Text>
        </HStack>
      ))}
      {evaluations.length > 0 && (
        <Table.Root variant="line">
          <Table.Header>
            <Table.Row>
              <Table.ColumnHeader>{t("Decision")}</Table.ColumnHeader>
              <Table.ColumnHeader>{t("Case / Run")}</Table.ColumnHeader>
              <Table.ColumnHeader>{t("Trace evidence")}</Table.ColumnHeader>
              <Table.ColumnHeader>{t("Engine result")}</Table.ColumnHeader>
              <Table.ColumnHeader>{t("Created")}</Table.ColumnHeader>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {evaluations.map((value) => (
              <Table.Row key={value.evaluation_id}>
                <Table.Cell>
                  <DecisionBadge value={value.decision} />
                </Table.Cell>
                <Table.Cell>
                  <Code size="sm">{shortId(value.case_id)}</Code>
                  <Text textStyle="xs" color="fg.muted" fontFamily="mono">
                    {shortId(value.run_id)}
                  </Text>
                </Table.Cell>
                <Table.Cell>
                  <HStack gap={3}>
                    {value.source_trace_id && (
                      <TraceLink
                        projectSlug={projectSlug}
                        traceId={value.source_trace_id}
                        label={t("source")}
                      />
                    )}
                    {value.replay_trace_id && (
                      <TraceLink
                        projectSlug={projectSlug}
                        traceId={value.replay_trace_id}
                        label={t("replay")}
                      />
                    )}
                  </HStack>
                </Table.Cell>
                <Table.Cell>
                  <Text fontWeight="medium">
                    {value.result_status || value.package_status}
                  </Text>
                  <Text textStyle="xs" color="fg.muted" lineClamp={1}>
                    {value.summary || value.result_ref}
                  </Text>
                </Table.Cell>
                <Table.Cell>{formatDate(value.created_at, locale)}</Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      )}
    </VStack>
  );
}

function ReleasesTable({
  releases,
  projectSlug,
}: {
  releases: BarenaRelease[];
  projectSlug: string;
}) {
  const { locale, t } = useBarenaI18n();
  if (releases.length === 0) {
    return (
      <EmptyState
        icon={LuShieldCheck}
        title={t("No release decisions")}
        description={t(
          "A Release is persisted only after a completed Replay produces a valid terminal Engine decision.",
        )}
      />
    );
  }
  return (
    <Table.Root variant="line">
      <Table.Header>
        <Table.Row>
          <Table.ColumnHeader>{t("Gate")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Lineage")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Evidence")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Summary")}</Table.ColumnHeader>
          <Table.ColumnHeader>{t("Created")}</Table.ColumnHeader>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {releases.map((value) => (
          <Table.Row key={value.release_id}>
            <Table.Cell>
              <DecisionBadge value={value.decision} />
            </Table.Cell>
            <Table.Cell>
              <Text textStyle="xs" color="fg.muted">
                {t("Evaluation")}
              </Text>
              <Code size="sm">{shortId(value.evaluation_id)}</Code>
              <Text textStyle="xs" color="fg.muted">
                {t("Harness")} {shortId(value.harness_version_id)}
              </Text>
            </Table.Cell>
            <Table.Cell>
              <HStack gap={3}>
                {value.source_trace_id && (
                  <TraceLink
                    projectSlug={projectSlug}
                    traceId={value.source_trace_id}
                    label={t("source")}
                  />
                )}
                {value.replay_trace_id && (
                  <TraceLink
                    projectSlug={projectSlug}
                    traceId={value.replay_trace_id}
                    label={t("replay")}
                  />
                )}
              </HStack>
            </Table.Cell>
            <Table.Cell maxWidth="420px">
              <Text lineClamp={2}>
                {value.summary || t("Terminal Engine decision retained.")}
              </Text>
            </Table.Cell>
            <Table.Cell>{formatDate(value.created_at, locale)}</Table.Cell>
          </Table.Row>
        ))}
      </Table.Body>
    </Table.Root>
  );
}

function EmptyState({
  icon,
  title,
  description,
}: {
  icon: typeof LuFlag;
  title: string;
  description: string;
}) {
  return (
    <VStack padding={12} gap={3} textAlign="center">
      <Box
        display="grid"
        placeItems="center"
        width="44px"
        height="44px"
        borderRadius="xl"
        bg="bg.subtle"
        borderWidth="1px"
        borderColor="border"
      >
        <Icon as={icon} boxSize={5} color="fg.muted" />
      </Box>
      <Text fontWeight="semibold">{title}</Text>
      <Text color="fg.muted" textStyle="sm" maxWidth="520px">
        {description}
      </Text>
    </VStack>
  );
}

function DecisionBadge({ value }: { value: BarenaEvaluation["decision"] }) {
  const { t } = useBarenaI18n();
  const icon =
    value === "cleared"
      ? LuCircleCheckBig
      : value === "held"
        ? LuGauge
        : LuShieldCheck;
  return (
    <Badge
      colorPalette={
        value === "cleared" ? "green" : value === "held" ? "yellow" : "red"
      }
      size="lg"
      variant="subtle"
    >
      <Icon as={icon} boxSize={3.5} />
      {t(decisionMessage(value))}
    </Badge>
  );
}

function TraceLink({
  projectSlug,
  traceId,
  label = "open trace",
}: {
  projectSlug: string;
  traceId: string;
  label?: string;
}) {
  const { t } = useBarenaI18n();
  return (
    <Link
      href={`/${projectSlug}/traces/${encodeURIComponent(traceId)}`}
      color="orange.600"
      textStyle="xs"
      fontWeight="medium"
    >
      {label === "open trace" ? t("open trace") : label}
    </Link>
  );
}

function severityPalette(severity: BarenaIssue["severity"]) {
  if (severity === "critical") return "red";
  if (severity === "high") return "orange";
  if (severity === "medium") return "yellow";
  return "gray";
}

function severityMessage(
  severity: BarenaIssue["severity"],
): "Low" | "Medium" | "High" | "Critical" {
  if (severity === "critical") return "Critical";
  if (severity === "high") return "High";
  if (severity === "medium") return "Medium";
  return "Low";
}

function decisionMessage(
  decision: BarenaEvaluation["decision"],
): "Cleared" | "Held" | "Rejected" {
  if (decision === "cleared") return "Cleared";
  if (decision === "held") return "Held";
  return "Rejected";
}

function hasActiveReplay(runs: BarenaRun[] | undefined) {
  return runs?.some(
    (run) => run.operation === "replay" && !isTerminalRun(run.state),
  );
}

function replayCapability(value: BarenaCase): {
  supported: boolean;
  reason?: string;
} {
  const runtime = value.runtime;
  if (runtime?.schema !== "barena.platform_http_runtime.v1") {
    return { supported: true };
  }
  const replay =
    runtime.replay && typeof runtime.replay === "object"
      ? (runtime.replay as Record<string, unknown>)
      : undefined;
  if (replay?.supported === true) return { supported: true };
  return {
    supported: false,
    reason:
      typeof replay?.reason === "string" && replay.reason.trim()
        ? replay.reason.trim()
        : "This Agent does not satisfy the deterministic Replay contract.",
  };
}

export function caseProposalPrefill(
  job: BarenaEvolutionJob,
): PromoteIssueInitialValues {
  const proposal = job.case_proposal;
  if (!proposal) return {};
  const verifier = recordValue(proposal.verifier);
  const artifacts = Array.isArray(verifier?.artifacts)
    ? verifier.artifacts
    : [];
  const firstArtifact = recordValue(artifacts[0]);
  return {
    successCriteria: proposal.success_criteria,
    replayPrompt: proposal.replay_prompt,
    ...(typeof firstArtifact?.path === "string" && {
      artifactPath: firstArtifact.path,
    }),
    ...(typeof firstArtifact?.contains === "string" && {
      expectedText: firstArtifact.contains,
    }),
  };
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function isTerminalRun(state: BarenaRun["state"]) {
  return ["completed", "interrupted", "cancelled", "failed"].includes(state);
}

function shortId(value: string) {
  return value.length > 16 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value;
}

function formatDate(value: string, locale: "en" | "zh-CN") {
  return new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function queryValue(value: string | string[] | undefined) {
  return Array.isArray(value) ? value[0] : value;
}
