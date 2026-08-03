import {
  Badge,
  Box,
  Card,
  Code,
  Grid,
  HStack,
  Icon,
  NativeSelect,
  Text,
  VStack,
} from "@chakra-ui/react";
import { useEffect, useMemo, useState } from "react";
import { LuGitCompareArrows, LuInfo, LuWaypoints } from "react-icons/lu";
import { Link } from "~/components/ui/link";
import { useBarenaI18n } from "~/features/barena/i18n";
import type { BarenaRun } from "~/server/barena/contracts";

const PLATFORM_EXPLORE_SCHEMA = "barena.platform_explore_scenario.v1";

export interface PlatformExploreFacts {
  runId: string;
  scenarioId: string;
  scenarioName: string;
  targetType: string;
  targetReferenceId: string;
  targetName: string;
  sourceStatus: string;
  judgeVerdict?: string;
  judgeReasoning?: string;
  metCriteria: string[];
  unmetCriteria: string[];
  durationInMs?: number;
  primaryTraceId: string;
  completedAt: string;
}

export type CompareCompatibility =
  | { compatible: true }
  | {
      compatible: false;
      reason: "same_run" | "different_scenario" | "different_target";
    };

export function readPlatformExploreFacts(
  run: BarenaRun,
): PlatformExploreFacts | undefined {
  if (
    run.origin !== "platform" ||
    run.operation !== "explore" ||
    run.state !== "completed" ||
    run.input.schema !== PLATFORM_EXPLORE_SCHEMA
  ) {
    return undefined;
  }

  const source = asRecord(run.input.source);
  const scenario = asRecord(run.input.scenario);
  const target = asRecord(run.input.target);
  const execution = asRecord(run.input.execution);
  const evidence = asRecord(run.input.evidence);
  const judge = asRecord(run.input.judge);
  const scenarioId = stringValue(source?.scenario_id);
  const scenarioName = stringValue(scenario?.name);
  const targetType = stringValue(target?.type);
  const targetReferenceId = stringValue(target?.reference_id);
  const targetName = stringValue(target?.name);
  const sourceStatus = stringValue(execution?.status);
  const primaryTraceId = stringValue(evidence?.primary_trace_id);

  if (
    !scenarioId ||
    !scenarioName ||
    !targetType ||
    !targetReferenceId ||
    !targetName ||
    !sourceStatus ||
    !primaryTraceId ||
    !/^[a-f0-9]{32}$/i.test(primaryTraceId)
  ) {
    return undefined;
  }

  return {
    runId: run.run_id,
    scenarioId,
    scenarioName,
    targetType,
    targetReferenceId,
    targetName,
    sourceStatus,
    judgeVerdict: stringValue(judge?.verdict),
    judgeReasoning: stringValue(judge?.reasoning),
    metCriteria: stringArray(judge?.met_criteria),
    unmetCriteria: stringArray(judge?.unmet_criteria),
    durationInMs: finiteNumber(execution?.duration_in_ms),
    primaryTraceId: primaryTraceId.toLowerCase(),
    completedAt: stringValue(execution?.completed_at) ?? run.updated_at,
  };
}

export function compareCompatibility(
  left: PlatformExploreFacts,
  right: PlatformExploreFacts,
): CompareCompatibility {
  if (left.runId === right.runId) {
    return { compatible: false, reason: "same_run" };
  }
  if (left.scenarioId !== right.scenarioId) {
    return { compatible: false, reason: "different_scenario" };
  }
  if (
    left.targetType !== right.targetType ||
    left.targetReferenceId !== right.targetReferenceId
  ) {
    return { compatible: false, reason: "different_target" };
  }
  return { compatible: true };
}

export function CompareRunsPanel({
  runs,
  projectSlug,
}: {
  runs: BarenaRun[];
  projectSlug: string;
}) {
  const { locale, t } = useBarenaI18n();
  const eligibleRuns = useMemo(
    () =>
      runs
        .map(readPlatformExploreFacts)
        .filter((run): run is PlatformExploreFacts => run !== undefined)
        .sort((left, right) =>
          right.completedAt.localeCompare(left.completedAt),
        ),
    [runs],
  );
  const leftChoices = useMemo(
    () =>
      eligibleRuns.filter((left) =>
        eligibleRuns.some(
          (right) => compareCompatibility(left, right).compatible,
        ),
      ),
    [eligibleRuns],
  );
  const [leftRunId, setLeftRunId] = useState("");
  const [rightRunId, setRightRunId] = useState("");
  const left =
    leftChoices.find((candidate) => candidate.runId === leftRunId) ??
    leftChoices[0];
  const rightChoices = left
    ? eligibleRuns.filter(
        (candidate) => compareCompatibility(left, candidate).compatible,
      )
    : [];
  const right =
    rightChoices.find((candidate) => candidate.runId === rightRunId) ??
    rightChoices[0];

  useEffect(() => {
    if (left && leftRunId !== left.runId) setLeftRunId(left.runId);
  }, [left, leftRunId]);
  useEffect(() => {
    if (right && rightRunId !== right.runId) setRightRunId(right.runId);
  }, [right, rightRunId]);

  if (!left || !right) {
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
          <Icon as={LuGitCompareArrows} boxSize={5} color="fg.muted" />
        </Box>
        <Text fontWeight="semibold">{t("No compatible Explore runs")}</Text>
        <Text color="fg.muted" textStyle="sm" maxWidth="560px">
          {t(
            "Complete the same Scenario against the same HTTP Agent at least twice, then retain both runs in Evolution.",
          )}
        </Text>
      </VStack>
    );
  }

  return (
    <VStack align="stretch" gap={5} padding={5}>
      <HStack
        align="start"
        gap={3}
        padding={4}
        borderRadius="lg"
        borderWidth="1px"
        borderColor="blue.200"
        bg="blue.50"
        color="blue.900"
        _dark={{
          bg: "blue.950",
          color: "blue.100",
          borderColor: "blue.800",
        }}
      >
        <Icon as={LuInfo} boxSize={4} marginTop={0.5} flexShrink={0} />
        <Box>
          <Text fontWeight="semibold">
            {t("Evidence comparison, not a release decision")}
          </Text>
          <Text textStyle="sm">
            {t(
              "Compare two completed runs from the same Scenario and target Agent. Only Replay can produce a Release Gate.",
            )}
          </Text>
        </Box>
      </HStack>

      <Grid templateColumns={{ base: "1fr", lg: "1fr auto 1fr" }} gap={4}>
        <RunSelector
          label={t("Run A")}
          value={left.runId}
          runs={leftChoices}
          onChange={(runId) => {
            setLeftRunId(runId);
            setRightRunId("");
          }}
          locale={locale}
        />
        <Box
          display="grid"
          placeItems="center"
          color="fg.muted"
          paddingTop={{ base: 0, lg: 7 }}
        >
          <Icon as={LuGitCompareArrows} boxSize={5} />
        </Box>
        <RunSelector
          label={t("Run B")}
          value={right.runId}
          runs={rightChoices}
          onChange={setRightRunId}
          locale={locale}
        />
      </Grid>

      <Card.Root variant="outline">
        <Card.Header paddingBottom={3}>
          <HStack justify="space-between" align="start" gap={4}>
            <Box>
              <Text fontWeight="semibold">{left.scenarioName}</Text>
              <Text color="fg.muted" textStyle="sm">
                {left.targetName} · {left.targetType}
              </Text>
            </Box>
            <Badge variant="subtle" colorPalette="blue">
              <LuWaypoints /> {t("Compatible pair")}
            </Badge>
          </HStack>
        </Card.Header>
        <Card.Body paddingTop={0}>
          <VStack align="stretch" gap={0}>
            <ComparisonRow
              label={t("Judge verdict")}
              left={<VerdictBadge verdict={left.judgeVerdict} />}
              right={<VerdictBadge verdict={right.judgeVerdict} />}
            />
            <ComparisonRow
              label={t("Criteria met")}
              left={formatCriteria(left)}
              right={formatCriteria(right)}
            />
            <ComparisonRow
              label={t("Source status")}
              left={<Code>{left.sourceStatus}</Code>}
              right={<Code>{right.sourceStatus}</Code>}
            />
            <ComparisonRow
              label={t("Duration")}
              left={formatDuration(left.durationInMs)}
              right={formatDuration(right.durationInMs)}
            />
            <ComparisonRow
              label={t("Completed")}
              left={formatDate(left.completedAt, locale)}
              right={formatDate(right.completedAt, locale)}
            />
            <ComparisonRow
              label={t("Trace evidence")}
              left={
                <TraceLink
                  projectSlug={projectSlug}
                  traceId={left.primaryTraceId}
                />
              }
              right={
                <TraceLink
                  projectSlug={projectSlug}
                  traceId={right.primaryTraceId}
                />
              }
            />
          </VStack>
        </Card.Body>
      </Card.Root>

      <Grid templateColumns={{ base: "1fr", lg: "1fr 1fr" }} gap={4}>
        <JudgeEvidence label={t("Run A Judge evidence")} run={left} />
        <JudgeEvidence label={t("Run B Judge evidence")} run={right} />
      </Grid>
    </VStack>
  );
}

function RunSelector({
  label,
  value,
  runs,
  onChange,
  locale,
}: {
  label: string;
  value: string;
  runs: PlatformExploreFacts[];
  onChange: (value: string) => void;
  locale: "en" | "zh-CN";
}) {
  return (
    <Box>
      <Text textStyle="xs" color="fg.muted" fontWeight="semibold" mb={1.5}>
        {label}
      </Text>
      <NativeSelect.Root size="sm">
        <NativeSelect.Field
          value={value}
          onChange={(event) => onChange(event.target.value)}
          aria-label={label}
        >
          {runs.map((run) => (
            <option key={run.runId} value={run.runId}>
              {formatDate(run.completedAt, locale)} · {shortId(run.runId)}
            </option>
          ))}
        </NativeSelect.Field>
        <NativeSelect.Indicator />
      </NativeSelect.Root>
    </Box>
  );
}

function ComparisonRow({
  label,
  left,
  right,
}: {
  label: string;
  left: React.ReactNode;
  right: React.ReactNode;
}) {
  return (
    <Grid
      templateColumns={{ base: "1fr 1fr", md: "160px 1fr 1fr" }}
      gap={4}
      alignItems="center"
      minHeight="52px"
      borderTopWidth="1px"
      borderColor="border"
      paddingY={2.5}
    >
      <Text
        gridColumn={{ base: "1 / -1", md: "auto" }}
        color="fg.muted"
        textStyle="sm"
      >
        {label}
      </Text>
      <Box minWidth={0} textStyle="sm">
        {left}
      </Box>
      <Box minWidth={0} textStyle="sm">
        {right}
      </Box>
    </Grid>
  );
}

function JudgeEvidence({
  label,
  run,
}: {
  label: string;
  run: PlatformExploreFacts;
}) {
  const { t } = useBarenaI18n();
  return (
    <Card.Root variant="outline" height="full">
      <Card.Header paddingBottom={2}>
        <Text fontWeight="semibold" textStyle="sm">
          {label}
        </Text>
      </Card.Header>
      <Card.Body paddingTop={0}>
        <Text textStyle="sm" color={run.judgeReasoning ? "fg" : "fg.muted"}>
          {run.judgeReasoning ?? t("No Judge reasoning was retained.")}
        </Text>
        {run.unmetCriteria.length > 0 && (
          <Box marginTop={4}>
            <Text textStyle="xs" color="fg.muted" fontWeight="semibold">
              {t("Unmet criteria")}
            </Text>
            <VStack align="stretch" gap={1} marginTop={1.5}>
              {run.unmetCriteria.map((criterion) => (
                <Text key={criterion} textStyle="sm">
                  · {criterion}
                </Text>
              ))}
            </VStack>
          </Box>
        )}
      </Card.Body>
    </Card.Root>
  );
}

function VerdictBadge({ verdict }: { verdict?: string }) {
  const value = verdict?.toLowerCase();
  return (
    <Badge
      variant="subtle"
      colorPalette={
        value === "success" ? "green" : value === "failure" ? "red" : "gray"
      }
    >
      {value ?? "unknown"}
    </Badge>
  );
}

function TraceLink({
  projectSlug,
  traceId,
}: {
  projectSlug: string;
  traceId: string;
}) {
  const { t } = useBarenaI18n();
  return (
    <Link
      href={`/${projectSlug}/traces/${encodeURIComponent(traceId)}`}
      color="blue.600"
      fontWeight="medium"
    >
      {t("Open trace")} · {shortId(traceId)}
    </Link>
  );
}

function formatCriteria(run: PlatformExploreFacts) {
  const total = run.metCriteria.length + run.unmetCriteria.length;
  return total === 0 ? "—" : `${run.metCriteria.length} / ${total}`;
}

function formatDuration(value: number | undefined) {
  if (value === undefined) return "—";
  if (value < 1_000) return `${value} ms`;
  return `${(value / 1_000).toFixed(value < 10_000 ? 1 : 0)} s`;
}

function formatDate(value: string, locale: "en" | "zh-CN") {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  return new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(timestamp);
}

function shortId(value: string) {
  return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is string => typeof item === "string" && !!item.trim(),
      )
    : [];
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}
