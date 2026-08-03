import {
  Box,
  Button,
  Field,
  HStack,
  Input,
  NativeSelect,
  Text,
  Textarea,
  VStack,
} from "@chakra-ui/react";
import { useEffect, useMemo, useState } from "react";
import { LuFlag, LuX } from "react-icons/lu";
import { toaster } from "~/components/ui/toaster";
import { useBarenaI18n } from "~/features/barena/i18n";
import type { BarenaRun } from "~/server/barena/contracts";
import { api } from "~/utils/api";

type CreateIssuePanelProps = {
  projectId: string;
  runs: BarenaRun[];
  initialRunId?: string;
  initialTraceId?: string;
  onClose: () => void;
  onCreated: () => void;
};

export function CreateIssuePanel({
  projectId,
  runs,
  initialRunId,
  initialTraceId,
  onClose,
  onCreated,
}: CreateIssuePanelProps) {
  const { t } = useBarenaI18n();
  const eligibleRuns = useMemo(
    () =>
      runs.filter(
        (run) => run.operation === "explore" && isTerminal(run.state),
      ),
    [runs],
  );
  const [runId, setRunId] = useState(initialRunId ?? eligibleRuns[0]?.run_id);
  const [traceId, setTraceId] = useState(initialTraceId ?? "");
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [severity, setSeverity] = useState<
    "low" | "medium" | "high" | "critical"
  >("medium");

  useEffect(() => {
    if (initialRunId) setRunId(initialRunId);
  }, [initialRunId]);
  useEffect(() => {
    if (initialTraceId) setTraceId(initialTraceId);
  }, [initialTraceId]);

  const mutation = api.barena.createIssue.useMutation({
    onSuccess: () => {
      toaster.create({
        title: t("Issue retained"),
        description: t("The trace evidence is ready for review."),
        type: "success",
      });
      onCreated();
    },
    onError: (error) => {
      toaster.create({
        title: t("Issue could not be created"),
        description: error.message,
        type: "error",
      });
    },
  });

  const canSubmit =
    !!runId && title.trim().length >= 3 && summary.trim().length > 0;

  return (
    <Box
      borderWidth="1px"
      borderColor="border"
      borderRadius="xl"
      bg="bg.surface"
      boxShadow="sm"
      padding={5}
    >
      <HStack justify="space-between" align="start" marginBottom={5}>
        <Box>
          <HStack gap={2}>
            <LuFlag size={17} />
            <Text fontWeight="semibold">{t("Create issue from evidence")}</Text>
          </HStack>
          <Text color="fg.muted" textStyle="sm" marginTop={1}>
            {t(
              "Keep a concrete failure or boundary before it disappears into run history.",
            )}
          </Text>
        </Box>
        <Button
          size="xs"
          variant="ghost"
          aria-label={t("Close issue form")}
          onClick={onClose}
        >
          <LuX />
        </Button>
      </HStack>

      <VStack align="stretch" gap={4}>
        <Field.Root required>
          <Field.Label>{t("Source Explore run")}</Field.Label>
          <NativeSelect.Root>
            <NativeSelect.Field
              value={runId ?? ""}
              onChange={(event) => setRunId(event.target.value)}
            >
              <option value="" disabled>
                {t("Select a completed Explore run")}
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
              {t("Complete one Explore run before creating an issue.")}
            </Field.HelperText>
          )}
        </Field.Root>

        <Field.Root>
          <Field.Label>{t("Trace ID")}</Field.Label>
          <Input
            fontFamily="mono"
            value={traceId}
            onChange={(event) => setTraceId(event.target.value)}
            placeholder={t("Prefilled when opened from a Barena trace")}
          />
          <Field.HelperText>
            {t("Optional, but when supplied it must be retained by this Run.")}
          </Field.HelperText>
        </Field.Root>

        <Field.Root required>
          <Field.Label>{t("What failed?")}</Field.Label>
          <Input
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            maxLength={160}
            placeholder={t(
              "Agent stopped before writing the requested artifact",
            )}
          />
        </Field.Root>

        <Field.Root required>
          <Field.Label>{t("Evidence and expected behavior")}</Field.Label>
          <Textarea
            value={summary}
            onChange={(event) => setSummary(event.target.value)}
            rows={4}
            maxLength={4000}
            placeholder={t(
              "Describe what the trace shows, and what should have happened.",
            )}
          />
        </Field.Root>

        <Field.Root>
          <Field.Label>{t("Severity")}</Field.Label>
          <NativeSelect.Root>
            <NativeSelect.Field
              value={severity}
              onChange={(event) =>
                setSeverity(
                  event.target.value as "low" | "medium" | "high" | "critical",
                )
              }
            >
              <option value="low">{t("Low")}</option>
              <option value="medium">{t("Medium")}</option>
              <option value="high">{t("High")}</option>
              <option value="critical">{t("Critical")}</option>
            </NativeSelect.Field>
            <NativeSelect.Indicator />
          </NativeSelect.Root>
        </Field.Root>

        <HStack justify="end">
          <Button variant="ghost" onClick={onClose}>
            {t("Cancel")}
          </Button>
          <Button
            colorPalette="orange"
            disabled={!canSubmit || mutation.isLoading}
            onClick={() => {
              if (!runId) return;
              mutation.mutate({
                projectId,
                runId,
                traceId: traceId.trim() || undefined,
                title: title.trim(),
                summary: summary.trim(),
                severity,
              });
            }}
          >
            {mutation.isLoading ? t("Retaining…") : t("Retain issue")}
          </Button>
        </HStack>
      </VStack>
    </Box>
  );
}

function isTerminal(state: BarenaRun["state"]) {
  return ["completed", "interrupted", "cancelled", "failed"].includes(state);
}

function runLabel(run: BarenaRun) {
  const objective =
    typeof run.input.scenario === "object" &&
    run.input.scenario !== null &&
    "objective" in run.input.scenario &&
    typeof run.input.scenario.objective === "string"
      ? run.input.scenario.objective
      : "";
  return objective
    ? `${objective.slice(0, 72)} · ${run.run_id.slice(-8)}`
    : `${run.run_id} · ${run.state}`;
}
