import {
  Box,
  Button,
  Field,
  HStack,
  Input,
  Text,
  Textarea,
  VStack,
} from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { LuArchiveRestore, LuX } from "react-icons/lu";
import { toaster } from "~/components/ui/toaster";
import { useBarenaI18n } from "~/features/barena/i18n";
import type { BarenaIssue } from "~/server/barena/contracts";
import { api } from "~/utils/api";

type PromoteIssuePanelProps = {
  projectId: string;
  issue: BarenaIssue;
  onClose: () => void;
  onPromoted: () => void;
  initialValues?: PromoteIssueInitialValues;
};

export type PromoteIssueInitialValues = {
  successCriteria?: string;
  replayPrompt?: string;
  artifactPath?: string;
  expectedText?: string;
};

export function PromoteIssuePanel({
  projectId,
  issue,
  onClose,
  onPromoted,
  initialValues,
}: PromoteIssuePanelProps) {
  const { t } = useBarenaI18n();
  const [successCriteria, setSuccessCriteria] = useState(
    initialValues?.successCriteria ?? "",
  );
  const [replayPrompt, setReplayPrompt] = useState(
    initialValues?.replayPrompt ?? "",
  );
  const [artifactPath, setArtifactPath] = useState(
    initialValues?.artifactPath ?? "",
  );
  const [expectedText, setExpectedText] = useState(
    initialValues?.expectedText ?? "",
  );

  useEffect(() => {
    setSuccessCriteria(initialValues?.successCriteria ?? "");
    setReplayPrompt(initialValues?.replayPrompt ?? "");
    setArtifactPath(initialValues?.artifactPath ?? "");
    setExpectedText(initialValues?.expectedText ?? "");
  }, [initialValues, issue.issue_id]);

  const mutation = api.barena.promoteIssue.useMutation({
    onSuccess: () => {
      toaster.create({
        title: t("Immutable Case created"),
        description: t("Replay now uses the reviewed prompt and verifier."),
        type: "success",
      });
      onPromoted();
    },
    onError: (error) => {
      toaster.create({
        title: t("Issue could not be promoted"),
        description: error.message,
        type: "error",
      });
    },
  });
  const canSubmit =
    successCriteria.trim().length > 0 && artifactPath.trim().length > 0;

  return (
    <Box
      borderWidth="1px"
      borderColor="orange.300"
      borderRadius="xl"
      bg="orange.50"
      _dark={{ bg: "orange.950", borderColor: "orange.700" }}
      padding={5}
    >
      <HStack justify="space-between" align="start" marginBottom={5}>
        <Box>
          <HStack gap={2}>
            <LuArchiveRestore size={17} />
            <Text fontWeight="semibold">
              {t("Review before turning into a Case")}
            </Text>
          </HStack>
          <Text color="fg.muted" textStyle="sm" marginTop={1}>
            {issue.title}. {t("Once created, revision 1 is immutable.")}
          </Text>
        </Box>
        <Button
          size="xs"
          variant="ghost"
          aria-label={t("Close case review")}
          onClick={onClose}
        >
          <LuX />
        </Button>
      </HStack>

      <VStack align="stretch" gap={4}>
        <Field.Root required>
          <Field.Label>{t("Success criteria")}</Field.Label>
          <Textarea
            rows={3}
            value={successCriteria}
            onChange={(event) => setSuccessCriteria(event.target.value)}
            placeholder={t(
              "The Agent completes the task and leaves verifiable evidence.",
            )}
          />
        </Field.Root>

        <Field.Root>
          <Field.Label>{t("Replay prompt")}</Field.Label>
          <Textarea
            rows={4}
            value={replayPrompt}
            onChange={(event) => setReplayPrompt(event.target.value)}
            placeholder={t(
              "Leave blank to reuse the original Explore objective.",
            )}
          />
          <Field.HelperText>
            {t("This freezes the user request that will be replayed.")}
          </Field.HelperText>
        </Field.Root>

        <Field.Root required>
          <Field.Label>{t("Required artifact path")}</Field.Label>
          <Input
            fontFamily="mono"
            value={artifactPath}
            onChange={(event) => setArtifactPath(event.target.value)}
            placeholder="output/result.md"
          />
        </Field.Root>

        <Field.Root>
          <Field.Label>{t("Artifact must contain")}</Field.Label>
          <Input
            value={expectedText}
            onChange={(event) => setExpectedText(event.target.value)}
            placeholder={t("Optional deterministic content check")}
          />
        </Field.Root>

        <HStack justify="end">
          <Button variant="ghost" onClick={onClose}>
            {t("Keep as issue")}
          </Button>
          <Button
            colorPalette="orange"
            disabled={!canSubmit || mutation.isLoading}
            onClick={() =>
              mutation.mutate({
                projectId,
                issueId: issue.issue_id,
                replayPrompt: replayPrompt.trim() || undefined,
                successCriteria: successCriteria.trim(),
                artifacts: [
                  {
                    path: artifactPath.trim(),
                    ...(expectedText.trim()
                      ? { contains: expectedText.trim() }
                      : {}),
                  },
                ],
              })
            }
          >
            {mutation.isLoading ? t("Creating…") : t("Create immutable Case")}
          </Button>
        </HStack>
      </VStack>
    </Box>
  );
}
