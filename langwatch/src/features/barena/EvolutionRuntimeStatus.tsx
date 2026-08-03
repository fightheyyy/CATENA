import {
  Badge,
  Box,
  Card,
  HStack,
  Icon,
  Skeleton,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  LuBot,
  LuCircleAlert,
  LuCircleCheckBig,
  LuCloud,
} from "react-icons/lu";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import type { BarenaEvolutionRuntime } from "~/server/barena/contracts";
import { api } from "~/utils/api";
import { useBarenaI18n } from "./i18n";

export function EvolutionRuntimeStatus({
  compact = false,
}: {
  compact?: boolean;
}) {
  const { project } = useOrganizationTeamProject();
  const { t } = useBarenaI18n();
  const runtimeQuery = api.barena.listRuntimes.useQuery(
    { projectId: project?.id ?? "" },
    {
      enabled: !!project?.id,
      refetchInterval: 10_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  );
  if (!project) return null;

  const runtime = runtimeQuery.data?.runtimes[0];
  const view = runtimeView({
    runtime,
    loading: runtimeQuery.isLoading,
    failed: runtimeQuery.isError,
    labels: {
      checking: t("Checking Runtime"),
      unavailable: t("Unavailable"),
      ready: t("Ready"),
      blocked: t("Blocked"),
    },
  });
  const StatusIcon = view.icon;
  const description = compact
    ? t("Target Agents stay external")
    : t(
        "Runs XiaoBaOS's evaluation and evolution roles in the Barena control plane. Your target Agents remain external and connect through HTTP or OTLP.",
      );

  if (compact) {
    return (
      <HStack
        minHeight="36px"
        gap={2}
        paddingX={1}
        color="fg.muted"
        flexWrap="wrap"
      >
        <Box
          width="7px"
          height="7px"
          borderRadius="full"
          background={view.ready ? "green.500" : "orange.500"}
          flexShrink={0}
        />
        <Text textStyle="sm" fontWeight="medium" color="fg">
          {t("Cloud evaluator Runtime")}
        </Text>
        <Text textStyle="sm">· {view.label}</Text>
        {runtime?.version && (
          <Text fontFamily="mono" fontSize="2xs" color="fg.subtle">
            {runtime.version}
          </Text>
        )}
        <Text textStyle="xs" marginLeft={{ base: 0, sm: "auto" }}>
          {description}
        </Text>
      </HStack>
    );
  }

  return (
    <Card.Root
      variant="outline"
      borderColor={view.ready ? "green.muted" : "border.muted"}
      background={view.ready ? "green.subtle" : "bg.surface"}
    >
      <Card.Body padding={5}>
        <HStack align="start" justify="space-between" gap={4} flexWrap="wrap">
          <HStack align="start" gap={3} minWidth={0}>
            <Box
              padding={2.5}
              borderRadius="xl"
              background={view.ready ? "green.emphasized" : "bg.muted"}
              color={view.ready ? "green.contrast" : "fg.muted"}
              flexShrink={0}
            >
              <Icon as={LuBot} boxSize={5} />
            </Box>
            <VStack align="start" gap={1} minWidth={0}>
              <HStack gap={2} flexWrap="wrap">
                <Text fontWeight="semibold">
                  {t("Cloud evaluator Runtime")}
                </Text>
                <Badge colorPalette={view.palette} variant="subtle">
                  <StatusIcon size={11} /> {view.label}
                </Badge>
                {runtime?.version && (
                  <Text fontFamily="mono" fontSize="2xs" color="fg.subtle">
                    {runtime.version}
                  </Text>
                )}
              </HStack>
              <Text color="fg.muted" textStyle="sm" maxWidth="760px">
                {description}
              </Text>
            </VStack>
          </HStack>

          <HStack
            gap={2}
            flexWrap="wrap"
            justify={{ base: "start", md: "end" }}
          >
            <RuntimeRoleBadges
              loading={runtimeQuery.isLoading}
              runtime={runtime}
              ready={view.ready}
            />
          </HStack>
        </HStack>
      </Card.Body>
    </Card.Root>
  );
}

function RuntimeRoleBadges({
  loading,
  runtime,
  ready,
}: {
  loading: boolean;
  runtime?: BarenaEvolutionRuntime;
  ready: boolean;
}) {
  if (loading) {
    return Array.from({ length: 4 }, (_, index) => (
      <Skeleton key={index} height="24px" width="88px" borderRadius="full" />
    ));
  }
  return (runtime?.roles ?? []).map((role) => (
    <Badge
      key={role.id}
      variant="outline"
      colorPalette={ready ? "green" : "gray"}
      title={role.responsibility}
    >
      {role.display_name}
    </Badge>
  ));
}

function runtimeView({
  runtime,
  loading,
  failed,
  labels,
}: {
  runtime: BarenaEvolutionRuntime | undefined;
  loading: boolean;
  failed: boolean;
  labels: {
    checking: string;
    unavailable: string;
    ready: string;
    blocked: string;
  };
}) {
  if (loading) {
    return {
      ready: false,
      label: labels.checking,
      icon: LuCloud,
      palette: "gray",
    } as const;
  }
  if (failed) {
    return {
      ready: false,
      label: labels.unavailable,
      icon: LuCircleAlert,
      palette: "orange",
    } as const;
  }
  if (runtime?.status === "ready") {
    return {
      ready: true,
      label: labels.ready,
      icon: LuCircleCheckBig,
      palette: "green",
    } as const;
  }
  return {
    ready: false,
    label: labels.blocked,
    icon: LuCircleAlert,
    palette: "orange",
  } as const;
}
