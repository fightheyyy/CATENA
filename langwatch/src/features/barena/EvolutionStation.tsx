import {
  Badge,
  Box,
  Button,
  Card,
  Code,
  Collapsible,
  Field,
  Grid,
  HStack,
  Icon,
  NativeSelect,
  Separator,
  Spinner,
  Text,
  Textarea,
  VStack,
} from "@chakra-ui/react";
import { useEffect, useMemo, useState } from "react";
import {
  LuArchiveRestore,
  LuChevronDown,
  LuCircleAlert,
  LuCircleCheckBig,
  LuCircleDashed,
  LuFileSearch,
  LuHistory,
  LuPlay,
  LuScanSearch,
  LuSparkles,
} from "react-icons/lu";
import { Link } from "~/components/ui/link";
import { toaster } from "~/components/ui/toaster";
import type {
  BarenaEvolutionJob,
  BarenaEvolutionStageName,
  BarenaRun,
} from "~/server/barena/contracts";
import { api } from "~/utils/api";
import { useBarenaI18n } from "./i18n";

type EvolutionStationProps = {
  projectId: string;
  projectSlug: string;
  runs: BarenaRun[];
  canManage: boolean;
  initialRunId?: string;
  reviewingProposal?: boolean;
  onReviewProposal?: (job: BarenaEvolutionJob) => void;
};

const stageOrder: BarenaEvolutionStageName[] = [
  "inspector-cat",
  "evolution-cat",
  "reviewer-cat",
];

export function EvolutionStation({
  projectId,
  projectSlug,
  runs,
  canManage,
  initialRunId,
  reviewingProposal = false,
  onReviewProposal,
}: EvolutionStationProps) {
  const { locale, t } = useBarenaI18n();
  const eligibleRuns = useMemo(
    () => runs.filter(isEligibleEvolutionRun),
    [runs],
  );
  const initialEligibleRunId = eligibleRuns.some(
    (run) => run.run_id === initialRunId,
  )
    ? initialRunId
    : eligibleRuns[0]?.run_id;
  const [runId, setRunId] = useState(initialEligibleRunId ?? "");
  const [objective, setObjective] = useState("");
  const [selectedJobId, setSelectedJobId] = useState<string>();

  useEffect(() => {
    if (initialEligibleRunId) setRunId(initialEligibleRunId);
  }, [initialEligibleRunId]);

  const jobs = api.barena.listEvolutionJobs.useQuery(
    { projectId },
    {
      enabled: !!projectId,
      refetchInterval: (data) =>
        data?.evolution_jobs.some((job) => !isTerminalEvolutionJob(job.state))
          ? 3_000
          : false,
      refetchOnWindowFocus: false,
    },
  );
  useEffect(() => {
    if (!selectedJobId && jobs.data?.evolution_jobs[0]?.job_id) {
      setSelectedJobId(jobs.data.evolution_jobs[0].job_id);
    }
  }, [jobs.data?.evolution_jobs, selectedJobId]);

  const jobDetail = api.barena.getEvolutionJob.useQuery(
    { projectId, jobId: selectedJobId ?? "" },
    {
      enabled: !!projectId && !!selectedJobId,
      refetchInterval: (data) =>
        !data || !isTerminalEvolutionJob(data.state) ? 1_500 : false,
      refetchOnWindowFocus: false,
    },
  );
  const selectedJob =
    jobDetail.data ??
    jobs.data?.evolution_jobs.find((job) => job.job_id === selectedJobId);
  const sourceRun = eligibleRuns.find((run) => run.run_id === runId);
  const sourceTraceId = sourceRun ? evolutionTraceId(sourceRun) : undefined;

  const utils = api.useUtils();
  const startJob = api.barena.startEvolutionJob.useMutation({
    onSuccess: (job) => {
      setSelectedJobId(job.job_id);
      void utils.barena.listEvolutionJobs.invalidate({ projectId });
      toaster.create({
        title: t("Evolution started"),
        description: t(
          "The three-role review is now running against retained evidence.",
        ),
        type: "success",
      });
    },
    onError: (error) => {
      toaster.create({
        title: t("Evolution could not start"),
        description: error.message,
        type: "error",
      });
    },
  });

  return (
    <VStack align="stretch" gap={5} paddingY={5}>
      <HStack justify="space-between" align="center" gap={4} flexWrap="wrap">
        <Text fontWeight="semibold">{t("Choose source evidence")}</Text>
        {(jobs.data?.evolution_jobs.length ?? 0) > 0 && (
          <NativeSelect.Root size="sm" width={{ base: "100%", md: "280px" }}>
            <NativeSelect.Field
              aria-label={t("Evolution history")}
              value={selectedJobId ?? ""}
              onChange={(event) => setSelectedJobId(event.target.value)}
            >
              {jobs.data?.evolution_jobs.map((job) => (
                <option key={job.job_id} value={job.job_id}>
                  {jobHistoryLabel(job, locale)}
                </option>
              ))}
            </NativeSelect.Field>
            <NativeSelect.Indicator>
              <LuHistory />
            </NativeSelect.Indicator>
          </NativeSelect.Root>
        )}
      </HStack>

      {jobs.isError && (
        <InlineError
          title={t("Evolution jobs unavailable")}
          detail={jobs.error.message}
        />
      )}

      <Card.Root variant="outline" borderRadius="xl">
        <Card.Body padding={{ base: 4, md: 5 }}>
          <VStack align="stretch" gap={4}>
            <Grid
              templateColumns={{
                base: "1fr",
                lg: "minmax(280px, 0.9fr) minmax(0, 1.1fr)",
              }}
              gap={4}
              alignItems="start"
            >
              <Field.Root required>
                <Field.Label>{t("Explore Run + Trace")}</Field.Label>
                <NativeSelect.Root>
                  <NativeSelect.Field
                    value={runId}
                    onChange={(event) => setRunId(event.target.value)}
                  >
                    <option value="" disabled>
                      {t("Select retained evidence")}
                    </option>
                    {eligibleRuns.map((run) => (
                      <option key={run.run_id} value={run.run_id}>
                        {runLabel(run)}
                      </option>
                    ))}
                  </NativeSelect.Field>
                  <NativeSelect.Indicator />
                </NativeSelect.Root>
                {eligibleRuns.length === 0 && (
                  <Field.HelperText>
                    {t(
                      "Run Explore and retain its OTLP Trace before starting evolution.",
                    )}
                  </Field.HelperText>
                )}
              </Field.Root>

              <Field.Root>
                <Field.Label>{t("Evolution focus (optional)")}</Field.Label>
                <Textarea
                  value={objective}
                  onChange={(event) => setObjective(event.target.value)}
                  maxLength={4000}
                  rows={2}
                  placeholder={t(
                    "For example: find why the Agent skipped clarification and propose the smallest safe fix.",
                  )}
                />
              </Field.Root>
            </Grid>

            <HStack
              justify="space-between"
              align={{ base: "stretch", sm: "center" }}
              gap={4}
              flexDirection={{ base: "column", sm: "row" }}
            >
              {sourceTraceId ? (
                <HStack gap={2} minWidth={0}>
                  <Text textStyle="xs" color="fg.muted" flexShrink={0}>
                    {t("Retained OTLP Trace")}
                  </Text>
                  <Code size="sm" lineClamp={1}>
                    {shortId(sourceTraceId)}
                  </Code>
                  <Link
                    href={`/${projectSlug}/traces/${encodeURIComponent(sourceTraceId)}`}
                    color="orange.600"
                    textStyle="xs"
                    fontWeight="semibold"
                    flexShrink={0}
                  >
                    {t("Open Trace")}
                  </Link>
                </HStack>
              ) : (
                <Text textStyle="xs" color="fg.muted">
                  {t(
                    "Only completed Explore runs with a retained Trace are eligible.",
                  )}
                </Text>
              )}
              <Button
                colorPalette="orange"
                minWidth={{ base: "100%", sm: "150px" }}
                disabled={
                  !canManage ||
                  !sourceRun ||
                  !sourceTraceId ||
                  startJob.isLoading
                }
                onClick={() => {
                  if (!sourceRun || !sourceTraceId) return;
                  startJob.mutate({
                    projectId,
                    runId: sourceRun.run_id,
                    traceId: sourceTraceId,
                    objective: objective.trim() || undefined,
                    idempotencyKey: `evolution-${sourceRun.run_id}-${globalThis.crypto.randomUUID()}`,
                  });
                }}
              >
                {startJob.isLoading ? <Spinner size="xs" /> : <LuPlay />}
                {startJob.isLoading
                  ? t("Starting evolution…")
                  : t("Start evolution")}
              </Button>
            </HStack>
            {!canManage && (
              <Text textStyle="xs" color="fg.muted">
                {t("Manage evaluation permission is required to start a job.")}
              </Text>
            )}
          </VStack>
        </Card.Body>
      </Card.Root>

      {selectedJobId && (
        <Box borderTopWidth="1px" borderColor="border" paddingTop={5}>
          {jobDetail.isLoading && !selectedJob ? (
            <HStack justify="center" paddingY={8}>
              <Spinner size="sm" />
              <Text color="fg.muted">{t("Loading evolution job…")}</Text>
            </HStack>
          ) : jobDetail.isError ? (
            <InlineError
              title={t("Evolution job unavailable")}
              detail={jobDetail.error.message}
            />
          ) : selectedJob ? (
            <VStack align="stretch" gap={4}>
              <HStack justify="space-between" gap={3} flexWrap="wrap">
                <Box>
                  <Text fontWeight="semibold">
                    {t("Follow the evolution review")}
                  </Text>
                  <JobProvenance job={selectedJob} projectSlug={projectSlug} />
                </Box>
                <JobStateBadge state={selectedJob.state} />
              </HStack>
              <StageRail job={selectedJob} />
              <RoleOutputDetails job={selectedJob} />
              {selectedJob.error && (
                <InlineError
                  title={t("Evolution stopped")}
                  detail={selectedJob.error}
                />
              )}
            </VStack>
          ) : null}
        </Box>
      )}

      {selectedJob && hasOutputs(selectedJob) && (
        <>
          <Separator />
          <Box>
            <Text fontSize="lg" fontWeight="semibold">
              {t("Review the proposed outputs")}
            </Text>
            <Text textStyle="sm" color="fg.muted" marginTop={1}>
              {t(
                "These outputs are evidence and proposals. Verification still happens through Case Replay and Release Gate.",
              )}
            </Text>
          </Box>
          <OutputGrid
            job={selectedJob}
            canManage={canManage}
            reviewingProposal={reviewingProposal}
            onReviewProposal={onReviewProposal}
          />
        </>
      )}
    </VStack>
  );
}

function StageRail({ job }: { job: BarenaEvolutionJob }) {
  const { t } = useBarenaI18n();
  const definitions = [
    {
      id: "inspector-cat" as const,
      label: "InspectorCat",
      description: t("Find the failure mode in Trace evidence"),
      icon: LuScanSearch,
    },
    {
      id: "evolution-cat" as const,
      label: "EvolutionCat",
      description: t(
        "Propose the smallest Role, Skill, Memory, or Harness change",
      ),
      icon: LuSparkles,
    },
    {
      id: "reviewer-cat" as const,
      label: "ReviewerCat",
      description: t("Review the proposal and expose its risks"),
      icon: LuFileSearch,
    },
  ];
  return (
    <Grid
      templateColumns={{ base: "1fr", md: "repeat(3, minmax(0, 1fr))" }}
      borderWidth="1px"
      borderColor="border"
      borderRadius="xl"
      overflow="hidden"
    >
      {definitions.map((definition, index) => {
        const state = resolvedStageState(job, definition.id);
        const StageIcon = definition.icon;
        return (
          <HStack
            key={definition.id}
            minWidth={0}
            gap={3}
            padding={3.5}
            borderRightWidth={{
              base: 0,
              md: index < definitions.length - 1 ? 1 : 0,
            }}
            borderBottomWidth={{
              base: index < definitions.length - 1 ? 1 : 0,
              md: 0,
            }}
            borderColor="border"
          >
            <Box
              display="grid"
              placeItems="center"
              width="30px"
              height="30px"
              borderRadius="lg"
              background={state === "running" ? "orange.500" : "bg.subtle"}
              color={state === "running" ? "white" : "fg.muted"}
              flexShrink={0}
            >
              <StageIcon size={15} />
            </Box>
            <Box minWidth={0} flex="1">
              <Text textStyle="sm" fontWeight="semibold">
                {definition.label}
              </Text>
              <Text textStyle="xs" color="fg.muted" lineClamp={1}>
                {definition.description}
              </Text>
            </Box>
            <StageStateIcon state={state} />
          </HStack>
        );
      })}
    </Grid>
  );
}

function RoleOutputDetails({ job }: { job: BarenaEvolutionJob }) {
  const { t } = useBarenaI18n();
  const stages = job.stages.filter(
    (stage) => stage.error || stage.raw_output !== undefined,
  );
  if (stages.length === 0) return null;

  return (
    <Disclosure
      label={t("Role outputs")}
      badge={
        <Badge size="sm" variant="subtle">
          {stages.length}
        </Badge>
      }
    >
      <VStack
        align="stretch"
        gap={0}
        borderWidth="1px"
        borderColor="border"
        borderRadius="lg"
        overflow="hidden"
      >
        {stages.map((stage, index) => (
          <Box
            key={`${stage.role}-${stage.name}`}
            padding={3}
            borderBottomWidth={index < stages.length - 1 ? 1 : 0}
            borderColor="border"
          >
            <HStack justify="space-between" gap={3} marginBottom={2}>
              <Text textStyle="sm" fontWeight="semibold">
                {stageRoleLabel(stage.role)}
              </Text>
              <Badge
                size="sm"
                variant="subtle"
                colorPalette={
                  stage.state === "completed"
                    ? "green"
                    : stage.state === "failed"
                      ? "red"
                      : "orange"
                }
              >
                {stage.state}
              </Badge>
            </HStack>
            {stage.error ? (
              <Text textStyle="xs" color="red.600">
                {stage.error}
              </Text>
            ) : (
              <Box
                as="pre"
                whiteSpace="pre-wrap"
                wordBreak="break-word"
                fontFamily="mono"
                textStyle="xs"
                maxHeight="220px"
                overflowY="auto"
                background="bg.subtle"
                borderRadius="md"
                padding={3}
              >
                {formatJSON(stage.raw_output)}
              </Box>
            )}
          </Box>
        ))}
      </VStack>
    </Disclosure>
  );
}

function OutputGrid({
  job,
  canManage,
  reviewingProposal,
  onReviewProposal,
}: {
  job: BarenaEvolutionJob;
  canManage: boolean;
  reviewingProposal: boolean;
  onReviewProposal?: (job: BarenaEvolutionJob) => void;
}) {
  const { t } = useBarenaI18n();
  return (
    <VStack align="stretch" gap={4}>
      <Grid
        templateColumns={{ base: "1fr", lg: "repeat(2, minmax(0, 1fr))" }}
        gap={4}
      >
        {job.finding && (
          <OutputCard
            icon={LuScanSearch}
            eyebrow={t("Finding")}
            title={job.finding.title}
          >
            <Text textStyle="sm">{job.finding.summary}</Text>
            <HStack marginTop={3} gap={2} flexWrap="wrap">
              <Badge
                colorPalette={severityPalette(job.finding.severity)}
                variant="subtle"
              >
                {t(severityLabel(job.finding.severity))}
              </Badge>
            </HStack>
            {job.finding.evidence.length > 0 && (
              <Box marginTop={3}>
                <Disclosure
                  label={t("Evidence")}
                  badge={
                    <Badge size="sm" variant="subtle">
                      {job.finding.evidence.length}
                    </Badge>
                  }
                >
                  <VStack align="stretch" gap={2}>
                    {job.finding.evidence.map((evidence) => (
                      <HStack key={evidence} align="start" gap={2}>
                        <Box
                          width="4px"
                          height="4px"
                          borderRadius="full"
                          background="fg.subtle"
                          marginTop="7px"
                          flexShrink={0}
                        />
                        <Text textStyle="xs">{evidence}</Text>
                      </HStack>
                    ))}
                  </VStack>
                </Disclosure>
              </Box>
            )}
          </OutputCard>
        )}

        {job.candidate && (
          <OutputCard
            icon={LuSparkles}
            eyebrow={t("Candidate")}
            title={job.candidate.title}
            accent
            badge={t("Draft · Unverified")}
          >
            <Text textStyle="sm">{job.candidate.summary}</Text>
            <HStack gap={2} marginTop={3} flexWrap="wrap">
              <Badge variant="outline">
                {candidateKindLabel(job.candidate.kind, t)}
              </Badge>
              <Code size="sm">{job.candidate.candidate_id}</Code>
            </HStack>
            {job.candidate.content !== undefined && (
              <Box marginTop={3}>
                <Disclosure label={t("Candidate details")}>
                  <Box
                    as="pre"
                    whiteSpace="pre-wrap"
                    wordBreak="break-word"
                    fontFamily="mono"
                    textStyle="xs"
                    maxHeight="220px"
                    overflowY="auto"
                    background="bg.subtle"
                    borderRadius="lg"
                    padding={3}
                  >
                    {formatJSON(job.candidate.content)}
                  </Box>
                </Disclosure>
              </Box>
            )}
            <HStack
              gap={2}
              borderRadius="lg"
              background="orange.subtle"
              color="orange.fg"
              padding={2.5}
              marginTop={3}
            >
              <LuCircleDashed size={14} />
              <Text textStyle="xs" fontWeight="medium">
                {t(
                  "Not applied, published, replayed, or cleared by Release Gate.",
                )}
              </Text>
            </HStack>
          </OutputCard>
        )}
      </Grid>

      {(job.case_proposal || job.review) && (
        <Card.Root variant="outline" borderRadius="xl">
          <Card.Body padding={2}>
            {job.case_proposal && (
              <Disclosure
                label={`${t("Case proposal")} · ${job.case_proposal.title}`}
                badge={
                  job.case_proposal.requires_human_review ? (
                    <Badge size="sm" variant="subtle" colorPalette="orange">
                      {t("Advisory only")}
                    </Badge>
                  ) : undefined
                }
              >
                <OutputLabel
                  label={t("Replay prompt")}
                  value={job.case_proposal.replay_prompt}
                />
                <OutputLabel
                  label={t("Success criteria")}
                  value={job.case_proposal.success_criteria}
                />
                <OutputLabel
                  label={t("Suggested verifier")}
                  value={formatJSON(job.case_proposal.verifier)}
                />
                <Text textStyle="xs" color="fg.muted" marginTop={3}>
                  {job.case_proposal.requires_human_review
                    ? t(
                        "Human review is required before this proposal becomes an immutable Case.",
                      )
                    : t(
                        "Review this proposal before turning it into an immutable Case.",
                      )}
                </Text>
              </Disclosure>
            )}
            {job.case_proposal && job.review && <Separator />}
            {job.review && (
              <Disclosure
                label={`${t("Review")} · ${reviewTitle(job.review.verdict, t)}`}
                badge={
                  <Badge size="sm" variant="subtle">
                    {t("Advisory only")}
                  </Badge>
                }
              >
                <Text textStyle="sm">{job.review.summary}</Text>
                <Text textStyle="xs" color="fg.muted" marginTop={3}>
                  {t(
                    "ReviewerCat feedback is not a Release Gate decision. Promote the Case proposal, Replay it, then inspect the gate.",
                  )}
                </Text>
              </Disclosure>
            )}
            {job.finding && job.case_proposal && (
              <HStack
                justify="space-between"
                align={{ base: "stretch", sm: "center" }}
                flexDirection={{ base: "column", sm: "row" }}
                gap={3}
                borderTopWidth="1px"
                borderColor="border"
                paddingX={2}
                paddingTop={3}
                paddingBottom={2}
              >
                <Box>
                  <Text textStyle="sm" fontWeight="semibold">
                    {t("Turn this finding into a Regression Case")}
                  </Text>
                  <Text textStyle="xs" color="fg.muted">
                    {t(
                      "Review and edit the proposed prompt and verifier before anything becomes immutable.",
                    )}
                  </Text>
                </Box>
                <Button
                  colorPalette="orange"
                  size="sm"
                  flexShrink={0}
                  disabled={
                    !canManage || reviewingProposal || !onReviewProposal
                  }
                  onClick={() => onReviewProposal?.(job)}
                >
                  {reviewingProposal ? (
                    <Spinner size="xs" />
                  ) : (
                    <LuArchiveRestore />
                  )}
                  {reviewingProposal
                    ? t("Retaining finding…")
                    : job.review?.verdict === "pass"
                      ? t("Review proposed Case")
                      : t("Fix and review Case")}
                </Button>
              </HStack>
            )}
          </Card.Body>
        </Card.Root>
      )}
    </VStack>
  );
}

function OutputCard({
  icon,
  eyebrow,
  title,
  badge,
  accent = false,
  children,
}: {
  icon: typeof LuSparkles;
  eyebrow: string;
  title: string;
  badge?: string;
  accent?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Card.Root variant="outline" borderRadius="xl">
      <Card.Header paddingBottom={3}>
        <HStack justify="space-between" align="start" gap={3}>
          <HStack align="start" gap={3} minWidth={0}>
            <Box
              padding={2}
              borderRadius="lg"
              background={accent ? "orange.subtle" : "bg.subtle"}
              color={accent ? "orange.fg" : "fg.muted"}
            >
              <Icon as={icon} boxSize={4} />
            </Box>
            <Box minWidth={0}>
              <Text textStyle="xs" color="fg.muted">
                {eyebrow}
              </Text>
              <Text fontWeight="semibold">{title}</Text>
            </Box>
          </HStack>
          {badge && (
            <Badge colorPalette={accent ? "orange" : "gray"} variant="subtle">
              {badge}
            </Badge>
          )}
        </HStack>
      </Card.Header>
      <Card.Body paddingTop={0}>{children}</Card.Body>
    </Card.Root>
  );
}

function Disclosure({
  label,
  badge,
  children,
}: {
  label: string;
  badge?: React.ReactNode;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible.Root open={open} onOpenChange={({ open }) => setOpen(open)}>
      <Collapsible.Trigger asChild>
        <Button
          width="full"
          size="sm"
          variant="ghost"
          justifyContent="space-between"
          paddingX={2}
          color="fg.muted"
        >
          <HStack gap={2} minWidth={0}>
            <Text textStyle="sm" fontWeight="medium" lineClamp={1}>
              {label}
            </Text>
            {badge}
          </HStack>
          <Icon
            as={LuChevronDown}
            boxSize={4}
            flexShrink={0}
            transform={open ? "rotate(180deg)" : undefined}
            transition="transform 150ms ease"
          />
        </Button>
      </Collapsible.Trigger>
      <Collapsible.Content>
        <Box paddingX={2} paddingTop={2} paddingBottom={1}>
          {children}
        </Box>
      </Collapsible.Content>
    </Collapsible.Root>
  );
}

function JobProvenance({
  job,
  projectSlug,
}: {
  job: BarenaEvolutionJob;
  projectSlug: string;
}) {
  const { t } = useBarenaI18n();
  return (
    <HStack gap={2} flexWrap="wrap" paddingTop={1}>
      <Text textStyle="xs" color="fg.muted">
        {t("Source")}
      </Text>
      <Code size="sm">{shortId(job.source_run_id)}</Code>
      <Link
        href={`/${projectSlug}/traces/${encodeURIComponent(job.source_trace_id)}`}
        color="orange.600"
        textStyle="xs"
        fontWeight="semibold"
      >
        {t("Open Trace")}
      </Link>
      {job.objective && (
        <Text
          textStyle="xs"
          color="fg.muted"
          lineClamp={1}
          flex="1"
          minWidth="220px"
        >
          {job.objective}
        </Text>
      )}
    </HStack>
  );
}

function OutputLabel({ label, value }: { label: string; value: string }) {
  return (
    <Box marginTop={3}>
      <Text textStyle="xs" color="fg.muted" marginBottom={1}>
        {label}
      </Text>
      <Text textStyle="sm">{value}</Text>
    </Box>
  );
}

function InlineError({ title, detail }: { title: string; detail: string }) {
  return (
    <HStack
      align="start"
      gap={3}
      borderWidth="1px"
      borderColor="red.300"
      background="red.50"
      color="red.700"
      _dark={{
        background: "red.950",
        color: "red.200",
        borderColor: "red.700",
      }}
      borderRadius="lg"
      padding={4}
    >
      <LuCircleAlert size={18} />
      <Box>
        <Text fontWeight="semibold">{title}</Text>
        <Text textStyle="sm">{detail}</Text>
      </Box>
    </HStack>
  );
}

function JobStateBadge({ state }: { state: BarenaEvolutionJob["state"] }) {
  const { t } = useBarenaI18n();
  const label =
    state === "queued"
      ? t("Queued")
      : state === "running"
        ? t("Running")
        : state === "completed"
          ? t("Done")
          : t("Failed");
  return (
    <Badge
      colorPalette={
        state === "completed" ? "green" : state === "failed" ? "red" : "orange"
      }
      variant="subtle"
    >
      {state === "running" && <Spinner size="xs" />} {label}
    </Badge>
  );
}

function StageStateIcon({
  state,
}: {
  state: ReturnType<typeof resolvedStageState>;
}) {
  if (state === "completed")
    return <Icon as={LuCircleCheckBig} color="green.500" boxSize={5} />;
  if (state === "running") return <Spinner size="sm" color="orange.500" />;
  if (state === "failed")
    return <Icon as={LuCircleAlert} color="red.500" boxSize={5} />;
  return <Icon as={LuCircleDashed} color="fg.muted" boxSize={5} />;
}

export function evolutionTraceId(run: BarenaRun): string | undefined {
  const primary = run.input.primary_trace_id;
  if (typeof primary === "string" && primary.trim()) return primary;
  const evidence = asRecord(run.input.evidence);
  if (
    typeof evidence?.primary_trace_id === "string" &&
    evidence.primary_trace_id.trim()
  ) {
    return evidence.primary_trace_id;
  }
  const traces = run.input.trace_ids;
  if (Array.isArray(traces)) {
    const retained = traces.findLast(
      (value): value is string => typeof value === "string" && !!value.trim(),
    );
    if (retained) return retained;
  }
  const evidenceTraceIds = evidence?.trace_ids;
  return Array.isArray(evidenceTraceIds)
    ? evidenceTraceIds.findLast(
        (value): value is string => typeof value === "string" && !!value.trim(),
      )
    : undefined;
}

export function isEligibleEvolutionRun(run: BarenaRun): boolean {
  return (
    run.operation === "explore" &&
    run.state === "completed" &&
    evolutionTraceId(run) !== undefined
  );
}

export function resolvedStageState(
  job: BarenaEvolutionJob,
  stageName: BarenaEvolutionStageName,
): "pending" | "running" | "completed" | "failed" {
  const retained = job.stages.find((stage) => stage.role === stageName);
  if (retained) return retained.state === "queued" ? "pending" : retained.state;
  if (job.state === "completed") return "completed";
  const currentRole = job.stages.find(
    (stage) =>
      stage.name === job.current_stage || stage.role === job.current_stage,
  )?.role;
  const currentIndex = currentRole ? stageOrder.indexOf(currentRole) : -1;
  const stageIndex = stageOrder.indexOf(stageName);
  if (job.state === "failed" && stageName === currentRole) return "failed";
  if (job.state === "running" && stageName === currentRole) return "running";
  if (currentIndex > stageIndex) return "completed";
  return "pending";
}

function hasOutputs(job: BarenaEvolutionJob) {
  return !!(job.finding || job.case_proposal || job.candidate || job.review);
}

function runLabel(run: BarenaRun) {
  const scenario = asRecord(run.input.scenario);
  const objective =
    typeof scenario?.objective === "string" ? scenario.objective : "";
  return objective
    ? `${objective.slice(0, 64)} · ${shortId(run.run_id)}`
    : `${shortId(run.run_id)} · ${shortId(evolutionTraceId(run) ?? "")}`;
}

function jobHistoryLabel(job: BarenaEvolutionJob, locale: "en" | "zh-CN") {
  const stamp = new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(job.created_at));
  return `${stamp} · ${job.state} · ${shortId(job.source_run_id)}`;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function shortId(value: string) {
  return value.length > 16 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value;
}

function severityPalette(
  value: "unknown" | "low" | "medium" | "high" | "critical",
) {
  if (value === "critical") return "red";
  if (value === "high") return "orange";
  if (value === "medium") return "yellow";
  return "gray";
}

function severityLabel(
  value: "unknown" | "low" | "medium" | "high" | "critical",
): "Unknown" | "Low" | "Medium" | "High" | "Critical" {
  if (value === "critical") return "Critical";
  if (value === "high") return "High";
  if (value === "medium") return "Medium";
  if (value === "unknown") return "Unknown";
  return "Low";
}

function candidateKindLabel(
  value: "role" | "skill" | "memory" | "harness",
  t: ReturnType<typeof useBarenaI18n>["t"],
) {
  if (value === "role") return t("Role");
  if (value === "skill") return t("Skill");
  if (value === "memory") return t("Memory");
  return t("Harness");
}

function reviewTitle(
  verdict: string,
  t: ReturnType<typeof useBarenaI18n>["t"],
) {
  if (["pass", "approve", "approved", "accept", "accepted"].includes(verdict))
    return t("Proposal looks ready for verification");
  if (["revise", "revision_required"].includes(verdict))
    return t("Revise before verification");
  if (verdict === "blocked") return t("Review blocked by missing evidence");
  return t("Proposal rejected by ReviewerCat");
}

function formatJSON(value: unknown): string {
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function stageRoleLabel(role: BarenaEvolutionStageName): string {
  if (role === "inspector-cat") return "InspectorCat";
  if (role === "evolution-cat") return "EvolutionCat";
  return "ReviewerCat";
}

function isTerminalEvolutionJob(state: BarenaEvolutionJob["state"]): boolean {
  return state === "completed" || state === "failed";
}
