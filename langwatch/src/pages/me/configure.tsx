import {
  Badge,
  Box,
  Button,
  Heading,
  HStack,
  Input,
  Spacer,
  Text,
  VStack,
} from "@chakra-ui/react";
import { ArrowLeft, Copy, Laptop, Monitor, Server } from "lucide-react";
import { useState } from "react";
import { AvatarUploadControl } from "~/components/me/avatar/AvatarUploadControl";
import { HomePagePicker } from "~/components/me/HomePagePicker";
import MyLayout from "~/components/me/MyLayout";
import { PersonalOtlpEndpointPanel } from "~/components/me/PersonalOtlpEndpointPanel";
import {
  type PersonalApiKeyRow,
  usePersonalContext,
} from "~/components/me/usePersonalContext";
import SettingsLayout from "~/components/SettingsLayout";
import { Checkbox } from "~/components/ui/checkbox";
import { Link } from "~/components/ui/link";
import { toaster } from "~/components/ui/toaster";
import { withFeatureFlagGuard } from "~/components/WithFeatureFlagGuard";
import {
  type PersonalSettingsTranslator,
  usePersonalSettingsI18n,
} from "~/features/barena/personalSettingsI18n";
import { showErrorToast } from "~/features/errors";
import { api } from "~/utils/api";
import Head from "~/utils/compat/next-head";

const fmtRelative = (
  iso: string | null,
  locale: "en" | "zh-CN",
  t: PersonalSettingsTranslator,
): string => {
  if (!iso) return t("never");
  const diffMs = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diffMs / 1000);
  if (sec < 60) return t("secondsAgo", { count: sec });
  const min = Math.floor(sec / 60);
  if (min < 60) return t("minutesAgo", { count: min });
  const hr = Math.floor(min / 60);
  if (hr < 24) return t("hoursAgo", { count: hr });
  const day = Math.floor(hr / 24);
  if (day < 30) return t("daysAgo", { count: day });
  return new Date(iso).toLocaleDateString(locale);
};

const fmtUsd = (amount: number): string =>
  amount === 0
    ? "$0.00"
    : `$${amount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

function MySettingsPage() {
  const ctx = usePersonalContext();
  const { isBarenaMode, t } = usePersonalSettingsI18n();
  const [newKeyLabel, setNewKeyLabel] = useState("");
  const [showAddForm, setShowAddForm] = useState(false);
  const [revealedSecret, setRevealedSecret] = useState<{
    label: string;
    secret: string;
    baseUrl: string;
  } | null>(null);
  const [pendingRevokeId, setPendingRevokeId] = useState<string | null>(null);

  const utils = api.useUtils();
  const issueMutation = api.personalVirtualKeys.issuePersonal.useMutation({
    onSuccess: (issued) => {
      setRevealedSecret({
        label: issued.label,
        secret: issued.secret,
        baseUrl: issued.baseUrl,
      });
      setNewKeyLabel("");
      setShowAddForm(false);
      void utils.personalVirtualKeys.list.invalidate({
        organizationId: ctx.organizationId,
      });
      toaster.create({
        title: t("issuedPersonalKey", { label: issued.label }),
        type: "success",
      });
    },
    onError: (err) =>
      showErrorToast({
        error: err,
        fallbackTitle: t("issuePersonalKeyError"),
      }),
  });

  const personalContextQuery = api.user.personalContext.useQuery(
    { organizationId: ctx.organizationId },
    { enabled: !!ctx.organizationId, refetchOnWindowFocus: false },
  );
  const personalProjectId =
    personalContextQuery.data?.workspace.project.id ?? null;

  const featuresQuery = api.personalWorkspaceFeatures.get.useQuery(
    { projectId: personalProjectId ?? "" },
    { enabled: !!personalProjectId, refetchOnWindowFocus: false },
  );
  const featuresEnabled = !!(
    featuresQuery.data?.evaluations &&
    featuresQuery.data?.datasets &&
    featuresQuery.data?.annotations &&
    featuresQuery.data?.automations
  );
  const enableAllMutation = api.personalWorkspaceFeatures.enableAll.useMutation(
    {
      onSuccess: () => {
        if (personalProjectId) {
          void utils.personalWorkspaceFeatures.get.invalidate({
            projectId: personalProjectId,
          });
        }
        toaster.create({
          title: t("advancedFeaturesEnabled"),
          description: t("advancedFeaturesEnabledDescription"),
          type: "success",
        });
      },
      onError: (err) =>
        showErrorToast({
          error: err,
          fallbackTitle: t("enableAdvancedFeaturesError"),
        }),
    },
  );
  const disableAllMutation =
    api.personalWorkspaceFeatures.disableAll.useMutation({
      onSuccess: () => {
        if (personalProjectId) {
          void utils.personalWorkspaceFeatures.get.invalidate({
            projectId: personalProjectId,
          });
        }
        toaster.create({
          title: t("advancedFeaturesDisabled"),
          description: t("advancedFeaturesDisabledDescription"),
          type: "success",
        });
      },
      onError: (err) =>
        showErrorToast({
          error: err,
          fallbackTitle: t("disableAdvancedFeaturesError"),
        }),
    });

  const revokeMutation = api.personalVirtualKeys.revokePersonal.useMutation({
    onSuccess: () => {
      void utils.personalVirtualKeys.list.invalidate({
        organizationId: ctx.organizationId,
      });
      setPendingRevokeId(null);
      toaster.create({
        title: t("keyRevoked"),
        description: t("keyRevokedDescription"),
        type: "success",
      });
    },
    onError: (err) =>
      showErrorToast({ error: err, fallbackTitle: t("revokeKeyError") }),
  });

  const onIssue = () => {
    if (!newKeyLabel.trim() || !ctx.organizationId) return;
    issueMutation.mutate({
      organizationId: ctx.organizationId,
      label: newKeyLabel.trim(),
    });
  };

  const onRevoke = (id: string) => {
    if (!ctx.organizationId) return;
    revokeMutation.mutate({ organizationId: ctx.organizationId, id });
  };

  const content = (
    <>
      <Head>
        <title>{t("pageTitle")}</title>
      </Head>

      <VStack align="stretch" gap={6} width="full">
        {isBarenaMode && (
          <Link href="/settings" alignSelf="flex-start">
            <Button variant="ghost" size="sm" marginLeft={-2}>
              <ArrowLeft size={14} /> {t("backToSettings")}
            </Button>
          </Link>
        )}
        <HStack alignItems="end">
          <VStack align="start" gap={0}>
            <Heading as="h2" size="lg">
              {t("heading")}
            </Heading>
            <Text color="fg.muted" fontSize="sm">
              {t("subtitle")}
            </Text>
          </VStack>
          <Spacer />
        </HStack>

        <SectionCard title={t("profile")}>
          <VStack align="stretch" gap={4}>
            {ctx.organizationId && (
              <AvatarUploadControl organizationId={ctx.organizationId} />
            )}
            <Field label={t("name")} value={ctx.fullName} />
            <Field
              label={t("email")}
              value={ctx.email}
              hint={t("managedByIT", {
                organization: ctx.organizationName,
              })}
            />
            <Field label={t("joined")} value={ctx.joinedOn} />
            {ctx.routingPolicyName && (
              <Field
                label={t("routing")}
                value={
                  <HStack gap={2}>
                    <Text>{ctx.routingPolicyName}</Text>
                    <Badge variant="surface" colorPalette="gray" size="sm">
                      {t("managedByOrg")}
                    </Badge>
                  </HStack>
                }
              />
            )}
          </VStack>
        </SectionCard>

        <SectionCard
          title={t("personalVirtualKeys")}
          description={t("personalVirtualKeysDescription")}
          action={
            !showAddForm && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setShowAddForm(true)}
              >
                {t("addNewKey")}
              </Button>
            )
          }
        >
          {revealedSecret && (
            <RevealedSecretBanner
              secret={revealedSecret}
              onDismiss={() => setRevealedSecret(null)}
            />
          )}

          {showAddForm && (
            <Box
              borderWidth="1px"
              borderColor="border.muted"
              borderRadius="sm"
              padding={3}
              marginBottom={3}
            >
              <VStack align="stretch" gap={2}>
                <Text fontSize="sm" fontWeight="medium">
                  {t("newPersonalKey")}
                </Text>
                <Input
                  placeholder={t("newKeyPlaceholder")}
                  size="sm"
                  value={newKeyLabel}
                  onChange={(e) => setNewKeyLabel(e.target.value)}
                />
                <Text fontSize="xs" color="fg.muted">
                  {t("newKeyHint")}
                </Text>
                <HStack gap={2}>
                  <Button
                    size="sm"
                    onClick={onIssue}
                    loading={issueMutation.isPending}
                    disabled={!newKeyLabel.trim()}
                  >
                    {t("createKey")}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setShowAddForm(false);
                      setNewKeyLabel("");
                    }}
                  >
                    {t("cancel")}
                  </Button>
                </HStack>
              </VStack>
            </Box>
          )}

          {ctx.apiKeys.length === 0 ? (
            <Text fontSize="sm" color="fg.muted">
              {t("noPersonalKeys")}
            </Text>
          ) : (
            <VStack align="stretch" gap={2}>
              {ctx.apiKeys.map((key) => (
                <ApiKeyRow
                  key={key.id}
                  apiKey={key}
                  isPendingRevoke={pendingRevokeId === key.id}
                  isRevoking={
                    revokeMutation.isPending && pendingRevokeId === key.id
                  }
                  onRequestRevoke={() => setPendingRevokeId(key.id)}
                  onCancelRevoke={() => setPendingRevokeId(null)}
                  onConfirmRevoke={() => onRevoke(key.id)}
                />
              ))}
            </VStack>
          )}
        </SectionCard>

        {ctx.organizationId ? (
          <SectionCard
            title={t("defaultLandingPage")}
            description={t("defaultLandingPageDescription")}
          >
            <HomePagePicker organizationId={ctx.organizationId} />
          </SectionCard>
        ) : null}

        {personalContextQuery.data?.workspace.project.apiKey ? (
          <SectionCard
            title={t("personalOtlpEndpoint")}
            description={t("personalOtlpEndpointDescription")}
          >
            <PersonalOtlpEndpointPanel
              apiKey={personalContextQuery.data.workspace.project.apiKey}
            />
          </SectionCard>
        ) : null}

        {personalProjectId ? (
          <SectionCard
            title={t("workspaceFeatures")}
            description={t("workspaceFeaturesDescription")}
          >
            <Checkbox
              checked={featuresEnabled}
              disabled={
                featuresQuery.isLoading ||
                enableAllMutation.isPending ||
                disableAllMutation.isPending
              }
              onCheckedChange={(details) => {
                if (!personalProjectId) return;
                if (details.checked) {
                  enableAllMutation.mutate({ projectId: personalProjectId });
                } else {
                  disableAllMutation.mutate({ projectId: personalProjectId });
                }
              }}
            >
              {t("enableAdvancedFeatures")}
            </Checkbox>
          </SectionCard>
        ) : null}

        <SectionCard title={t("personalBudget")}>
          {ctx.summary.budgetUsd === null ? (
            <VStack align="start" gap={1}>
              <Text fontSize="sm" color="fg.muted">
                {t("noPersonalBudget")}
              </Text>
              <Text fontSize="xs" color="fg.muted">
                {t("askAdminForBudget")}
              </Text>
            </VStack>
          ) : (
            <VStack align="stretch" gap={3}>
              <Field
                label={t("monthlyLimit")}
                value={fmtUsd(ctx.summary.budgetUsd)}
                hint={t("setByAdmin", {
                  organization: ctx.organizationName,
                })}
              />
              <Field
                label={t("currentSpend")}
                value={fmtUsd(ctx.summary.spentThisMonthUsd)}
              />
            </VStack>
          )}
        </SectionCard>
      </VStack>
    </>
  );

  return isBarenaMode ? (
    <SettingsLayout>{content}</SettingsLayout>
  ) : (
    <MyLayout>{content}</MyLayout>
  );
}

function SectionCard({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Box
      borderWidth="1px"
      borderColor="border.muted"
      borderRadius="md"
      padding={4}
    >
      <HStack alignItems="start" marginBottom={3}>
        <VStack align="start" gap={0}>
          <Text fontSize="sm" fontWeight="semibold">
            {title}
          </Text>
          {description && (
            <Text fontSize="xs" color="fg.muted">
              {description}
            </Text>
          )}
        </VStack>
        <Spacer />
        {action}
      </HStack>
      {children}
    </Box>
  );
}

function Field({
  label,
  value,
  hint,
}: {
  label: string;
  value: React.ReactNode;
  hint?: string;
}) {
  return (
    <HStack alignItems="start" gap={4}>
      <Text fontSize="sm" color="fg.muted" minWidth="100px" paddingTop={1}>
        {label}
      </Text>
      <VStack align="start" gap={0}>
        {typeof value === "string" ? <Text fontSize="sm">{value}</Text> : value}
        {hint && (
          <Text fontSize="xs" color="fg.muted">
            {hint}
          </Text>
        )}
      </VStack>
    </HStack>
  );
}

function ApiKeyRow({
  apiKey,
  isPendingRevoke,
  isRevoking,
  onRequestRevoke,
  onCancelRevoke,
  onConfirmRevoke,
}: {
  apiKey: PersonalApiKeyRow;
  isPendingRevoke: boolean;
  isRevoking: boolean;
  onRequestRevoke: () => void;
  onCancelRevoke: () => void;
  onConfirmRevoke: () => void;
}) {
  const { locale, t } = usePersonalSettingsI18n();
  const Icon =
    apiKey.os === "macOS" || apiKey.os === "Windows"
      ? Laptop
      : apiKey.os === "Linux"
        ? Monitor
        : Server;

  return (
    <VStack
      align="stretch"
      gap={2}
      borderWidth="1px"
      borderColor={isPendingRevoke ? "red.300" : "border.muted"}
      borderRadius="sm"
      padding={3}
    >
      <HStack gap={3}>
        <Box>
          <Icon size={20} />
        </Box>
        <VStack align="start" gap={0} flex={1}>
          <Text fontSize="sm" fontWeight="medium">
            {apiKey.label}
          </Text>
          <Text fontSize="xs" color="fg.muted">
            {apiKey.deviceHint} ·{" "}
            {t("lastUsed", {
              when: fmtRelative(apiKey.lastUsedAt, locale, t),
            })}
          </Text>
          <Text fontSize="xs" color="fg.muted">
            {t("created", {
              when: fmtRelative(apiKey.createdAt, locale, t),
            })}
          </Text>
        </VStack>
        {!isPendingRevoke && (
          <Button
            size="sm"
            variant="outline"
            colorPalette="red"
            onClick={onRequestRevoke}
          >
            {t("revoke")}
          </Button>
        )}
      </HStack>
      {isPendingRevoke && (
        <HStack
          gap={2}
          paddingY={2}
          paddingX={3}
          backgroundColor="red.50"
          borderRadius="sm"
        >
          <Text fontSize="xs" color="red.700" flex={1}>
            {t("revokeConfirmation")}
          </Text>
          <Button
            size="xs"
            variant="ghost"
            onClick={onCancelRevoke}
            disabled={isRevoking}
          >
            {t("cancel")}
          </Button>
          <Button
            size="xs"
            colorPalette="red"
            onClick={onConfirmRevoke}
            loading={isRevoking}
          >
            {t("confirmRevoke")}
          </Button>
        </HStack>
      )}
    </VStack>
  );
}

function RevealedSecretBanner({
  secret,
  onDismiss,
}: {
  secret: { label: string; secret: string; baseUrl: string };
  onDismiss: () => void;
}) {
  const { t } = usePersonalSettingsI18n();
  return (
    <Box
      borderWidth="1px"
      borderColor="green.300"
      backgroundColor="green.50"
      borderRadius="md"
      padding={3}
      marginBottom={3}
    >
      <VStack align="stretch" gap={2}>
        <HStack>
          <Text fontWeight="semibold" color="green.800">
            {t("newKeyCreated", { label: secret.label })}
          </Text>
          <Spacer />
          <Button size="xs" variant="ghost" onClick={onDismiss}>
            {t("dismiss")}
          </Button>
        </HStack>
        <Text fontSize="xs" color="green.800">
          {t("copySecretNow")}
        </Text>
        <HStack
          gap={2}
          paddingX={2}
          paddingY={2}
          backgroundColor="white"
          borderRadius="sm"
          borderWidth="1px"
          borderColor="border.muted"
        >
          <Text fontSize="xs" fontFamily="mono" flex={1} wordBreak="break-all">
            {secret.secret}
          </Text>
          <Button
            size="xs"
            variant="ghost"
            onClick={() => {
              void navigator.clipboard.writeText(secret.secret);
              toaster.create({
                title: t("secretCopied"),
                type: "success",
              });
            }}
          >
            <Copy size={14} /> {t("copy")}
          </Button>
        </HStack>
        <Text fontSize="xs" color="green.800">
          {t("gatewayBaseUrl")}: <code>{secret.baseUrl}</code>
        </Text>
      </VStack>
    </Box>
  );
}

export default withFeatureFlagGuard("release_ui_ai_governance_enabled", {
  bypassOnboardingRedirect: true,
})(MySettingsPage);
