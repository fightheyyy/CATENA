import {
  Badge,
  Box,
  Button,
  HStack,
  Icon,
  Skeleton,
  Table,
  Text,
  VStack,
} from "@chakra-ui/react";
import { LuBot, LuCable, LuExternalLink, LuRadio } from "react-icons/lu";
import { Link } from "~/components/ui/link";
import { buildFragment } from "~/features/traces-v2/utils/urlState";
import { type AgentRegistryEntry, agentTraceQuery } from "./agentRegistry";
import { useBarenaI18n } from "./i18n";
import { useAgentRegistry } from "./useAgentRegistry";

export function AgentRegistry() {
  const { locale, t } = useBarenaI18n();
  const registry = useAgentRegistry();
  const project = registry.project;
  if (!project) return null;

  return (
    <VStack
      align="stretch"
      gap={5}
      width="full"
      maxWidth="1180px"
      marginX="auto"
      padding={{ base: 4, md: 6 }}
    >
      <HStack justify="space-between" align="start" gap={4} flexWrap="wrap">
        <Box>
          <HStack gap={2} marginBottom={1}>
            <Badge colorPalette="orange" variant="subtle">
              {t("OTLP-discovered")}
            </Badge>
            <HStack gap={1.5} color="green.fg" fontSize="xs">
              <LuRadio size={12} />
              <Text>{t("Observed through OTLP")}</Text>
            </HStack>
          </HStack>
          <Text fontSize="2xl" fontWeight="semibold" letterSpacing="-0.02em">
            {t("Agent Registry")}
          </Text>
          <Text color="fg.muted" textStyle="sm" marginTop={1} maxWidth="680px">
            {t(
              "Agents are discovered from standard OpenTelemetry identity. Trace remains the execution evidence, not the asset registry.",
            )}
          </Text>
        </Box>
        <Button asChild size="sm" colorPalette="orange" variant="outline">
          <Link href="/me/configure" textDecoration="none">
            <LuCable /> {t("Connect Agent")}
          </Link>
        </Button>
      </HStack>

      <HStack justify="space-between" minHeight="24px">
        <Text color="fg.muted" textStyle="xs">
          {t("Last 30 days")}
        </Text>
        {!registry.isLoading && registry.entries.length > 0 && (
          <Text color="fg.muted" textStyle="xs">
            {registry.entries.length} {t("Agents")}
          </Text>
        )}
      </HStack>

      {registry.isLoading ? (
        <VStack align="stretch" gap={2}>
          {[0, 1, 2].map((value) => (
            <Skeleton key={value} height="72px" borderRadius="lg" />
          ))}
        </VStack>
      ) : registry.isError ? (
        <Box
          borderWidth="1px"
          borderColor="red.300"
          borderRadius="xl"
          padding={5}
        >
          <Text color="red.fg">{registry.error.message}</Text>
        </Box>
      ) : registry.entries.length === 0 ? (
        <EmptyRegistry projectSlug={project.slug} />
      ) : (
        <Box
          borderWidth="1px"
          borderColor="border"
          borderRadius="xl"
          overflowX="auto"
        >
          <Table.Root variant="line" size="sm">
            <Table.Header>
              <Table.Row>
                <Table.ColumnHeader>{t("Agent")}</Table.ColumnHeader>
                <Table.ColumnHeader>{t("Runtime")}</Table.ColumnHeader>
                <Table.ColumnHeader>{t("Deployment")}</Table.ColumnHeader>
                <Table.ColumnHeader>{t("Activity")}</Table.ColumnHeader>
                <Table.ColumnHeader textAlign="end">
                  {t("Action")}
                </Table.ColumnHeader>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {registry.entries.map((entry) => (
                <AgentRow
                  key={entry.key}
                  entry={entry}
                  projectSlug={project.slug}
                  locale={locale}
                />
              ))}
            </Table.Body>
          </Table.Root>
        </Box>
      )}
    </VStack>
  );
}

function AgentRow({
  entry,
  projectSlug,
  locale,
}: {
  entry: AgentRegistryEntry;
  projectSlug: string;
  locale: "en" | "zh-CN";
}) {
  const { t } = useBarenaI18n();
  const traceHref = `/${projectSlug}/traces#${buildFragment("all-traces", {
    query: agentTraceQuery(entry),
    preset: "30d",
  })}`;

  return (
    <Table.Row>
      <Table.Cell minWidth="230px">
        <HStack align="start" gap={3}>
          <Box
            padding={2}
            borderRadius="lg"
            background="orange.subtle"
            color="orange.fg"
          >
            <Icon as={LuBot} boxSize={4} />
          </Box>
          <Box>
            <Text fontWeight="semibold">{entry.name}</Text>
            <Text color="fg.muted" textStyle="xs" fontFamily="mono">
              {entry.serviceName}
            </Text>
          </Box>
        </HStack>
      </Table.Cell>
      <Table.Cell>
        <Badge variant="outline">{entry.runtime}</Badge>
      </Table.Cell>
      <Table.Cell minWidth="220px">
        <HStack gap={1.5} flexWrap="wrap">
          {entry.deploymentEnvironment && (
            <Badge variant="subtle">{entry.deploymentEnvironment}</Badge>
          )}
          {entry.serviceVersion && (
            <Text color="fg.muted" textStyle="xs" fontFamily="mono">
              v{entry.serviceVersion}
            </Text>
          )}
        </HStack>
        {entry.serviceInstanceId && (
          <Text
            color="fg.subtle"
            textStyle="xs"
            fontFamily="mono"
            marginTop={1}
          >
            {entry.serviceInstanceId}
          </Text>
        )}
      </Table.Cell>
      <Table.Cell minWidth="190px">
        <HStack gap={2}>
          <Text textStyle="sm">
            {t("{count} traces", { count: entry.traceCount })}
          </Text>
          {entry.errorCount > 0 && (
            <Badge colorPalette="red" variant="subtle">
              {entry.errorCount} {t("Errors")}
            </Badge>
          )}
        </HStack>
        <Text color="fg.muted" textStyle="xs" marginTop={1}>
          {t("Last seen {time}", {
            time: formatRelativeTime(entry.lastSeenAt, locale),
          })}
        </Text>
      </Table.Cell>
      <Table.Cell textAlign="end">
        <Button asChild size="xs" variant="ghost">
          <Link href={traceHref} textDecoration="none">
            {t("View traces")} <LuExternalLink />
          </Link>
        </Button>
      </Table.Cell>
    </Table.Row>
  );
}

function EmptyRegistry({ projectSlug }: { projectSlug: string }) {
  const { t } = useBarenaI18n();
  return (
    <VStack
      gap={3}
      padding={{ base: 10, md: 14 }}
      borderWidth="1px"
      borderStyle="dashed"
      borderColor="border"
      borderRadius="xl"
      textAlign="center"
    >
      <Box
        padding={3}
        borderRadius="full"
        background="orange.subtle"
        color="orange.fg"
      >
        <LuBot size={22} />
      </Box>
      <Box>
        <Text fontWeight="semibold">{t("No Agents discovered")}</Text>
        <Text color="fg.muted" textStyle="sm" marginTop={1}>
          {t(
            "Send one OTLP Trace and Barena will discover the Agent automatically.",
          )}
        </Text>
      </Box>
      <Button asChild size="sm" colorPalette="orange">
        <Link href="/me/configure" textDecoration="none">
          {t("Open connection setup")}
        </Link>
      </Button>
      <Link
        href={`/${projectSlug}/traces`}
        color="fg.muted"
        textStyle="xs"
        textDecoration="none"
      >
        {t("Open Trace Explorer")}
      </Link>
    </VStack>
  );
}

function formatRelativeTime(timestamp: number, locale: "en" | "zh-CN") {
  const elapsedSeconds = Math.max(
    0,
    Math.round((Date.now() - timestamp) / 1_000),
  );
  const formatter = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  if (elapsedSeconds < 60) return formatter.format(-elapsedSeconds, "second");
  const minutes = Math.round(elapsedSeconds / 60);
  if (minutes < 60) return formatter.format(-minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (hours < 24) return formatter.format(-hours, "hour");
  return formatter.format(-Math.round(hours / 24), "day");
}
