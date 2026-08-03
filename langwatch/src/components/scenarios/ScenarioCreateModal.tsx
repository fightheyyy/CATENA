import { useCallback } from "react";
import { useDrawer } from "~/hooks/useDrawer";
import { useLicenseEnforcement } from "~/hooks/useLicenseEnforcement";
import { useModelProvidersSettings } from "~/hooks/useModelProvidersSettings";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import { usePublicEnv } from "~/hooks/usePublicEnv";
import { api } from "~/utils/api";
import { isHandledByGlobalHandler } from "~/utils/trpcError";
import { AICreateModal, type ExampleTemplate } from "../shared/AICreateModal";
import { ModelProviderRequiredModal } from "./ModelProviderRequiredModal";
import { ResolvedModelCaption } from "./ResolvedModelCaption";
import type { ScenarioFormData, ScenarioInitialData } from "./ScenarioForm";
import { generateScenarioWithAI } from "./services/scenarioGeneration";
import { storePromptForScenario } from "./services/scenarioPromptStorage";
import { getDefaultModelState } from "./utils/defaultModelState";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface ScenarioCreateModalProps {
  /** Controls modal visibility */
  open: boolean;
  /** Called when modal is closed */
  onClose: () => void;
}

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

const MODAL_TITLE = "Create new scenario";
const MODAL_PLACEHOLDER =
  "Explain your agent, its goals and what behavior you want to test.";
const GENERATING_TEXT = "Drafting your scenario…";

const EXAMPLE_TEMPLATES: ExampleTemplate[] = [
  {
    label: "Customer Support",
    text: "A customer support agent that handles complaints. Test an angry customer who was charged twice and wants a refund.",
  },
  {
    label: "RAG Q&A",
    text: "A knowledge bot that answers questions from documentation. Test a question that requires combining info from multiple sources.",
  },
  {
    label: "Tool-calling Agent",
    text: "An agent that uses tools to complete tasks. Test a request that requires calling multiple tools in sequence.",
  },
];

// ─────────────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Modal for creating a new scenario with AI assistance.
 *
 * Opens the ScenarioFormDrawer with initial data via complexProps.
 * No DB record is created until the user clicks "Save" in the drawer.
 */
export function ScenarioCreateModal({
  open,
  onClose,
}: ScenarioCreateModalProps) {
  const { project } = useOrganizationTeamProject();
  const publicEnv = usePublicEnv();
  const isSpiralMode = publicEnv.data?.IS_BARENA_MODE ?? false;
  const { openDrawer } = useDrawer();
  const { checkAndProceed } = useLicenseEnforcement("scenarios");

  // Check if any model providers are configured
  const { hasEnabledProviders, providers } = useModelProvidersSettings({
    projectId: project?.id,
  });

  // Cascade-resolved model for scenario generation.
  const resolvedDefault = api.modelProvider.getResolvedDefault.useQuery(
    { projectId: project?.id ?? "", featureKey: "scenarios.generator" },
    { enabled: !!project?.id },
  );

  const defaultModelState = getDefaultModelState({
    hasEnabledProviders,
    providers,
    defaultModel: resolvedDefault.data?.model,
  });

  const openEditorWithData = useCallback(
    (formData: Partial<ScenarioFormData>) => {
      const initialData: ScenarioInitialData = { initialFormData: formData };
      openDrawer(
        "scenarioEditor",
        {
          ...initialData,
        },
        { resetStack: true },
      );
      onClose();
    },
    [openDrawer, onClose],
  );

  const handleGenerate = useCallback(
    async (description: string) => {
      if (!project?.id) {
        throw new Error("No project selected");
      }

      try {
        if (isSpiralMode && !defaultModelState.ok) {
          const normalized = description.replace(/\s+/g, " ").trim();
          storePromptForScenario(description);
          openEditorWithData({
            name:
              normalized.length <= 80
                ? normalized
                : `${normalized.slice(0, 79)}…`,
            situation: description.trim(),
            criteria: [
              "The Agent completes the described user goal without unsupported claims.",
            ],
          });
          return;
        }
        const generatedData = await generateScenarioWithAI(
          description,
          project.id,
        );
        storePromptForScenario(description);
        openEditorWithData(generatedData);
      } catch (error) {
        if (isHandledByGlobalHandler(error)) return;
        throw error;
      }
    },
    [project?.id, isSpiralMode, defaultModelState.ok, openEditorWithData],
  );

  const handleSkip = useCallback(() => {
    openEditorWithData({
      name: "",
      situation: "",
      criteria: [],
    });
  }, [openEditorWithData]);

  if (!defaultModelState.ok && !isSpiralMode) {
    return (
      <ModelProviderRequiredModal
        open={open}
        onClose={onClose}
        onProceedAnyway={() => checkAndProceed(handleSkip)}
      />
    );
  }

  return (
    <AICreateModal
      open={open}
      onClose={onClose}
      title={MODAL_TITLE}
      placeholder={MODAL_PLACEHOLDER}
      exampleTemplates={EXAMPLE_TEMPLATES}
      onGenerate={(desc) => checkAndProceed(() => handleGenerate(desc))}
      onSkip={() => checkAndProceed(handleSkip)}
      generatingText={GENERATING_TEXT}
      footerHint={
        defaultModelState.ok ? (
          <ResolvedModelCaption model={resolvedDefault.data?.model} />
        ) : undefined
      }
      assistant={{
        name: isSpiralMode && !defaultModelState.ok ? "Barena" : "AI",
        description:
          "Describe the behavior you care about. AI will turn it into an editable situation and success criteria.",
        promptLabel: "What should this simulation prove?",
        generateLabel:
          isSpiralMode && !defaultModelState.ok
            ? "Review scenario"
            : "Draft with AI",
        reviewHint:
          "AI is shaping the situation and criteria. You will review everything before it is saved.",
      }}
    />
  );
}
