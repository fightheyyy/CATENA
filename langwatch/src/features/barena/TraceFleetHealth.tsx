import { Box, Grid, HStack, Skeleton, Text, VStack } from "@chakra-ui/react";
import { Link } from "~/components/ui/link";
import { summarizeAgentRegistry } from "./agentRegistry";
import { useBarenaI18n } from "./i18n";
import { useAgentRegistry } from "./useAgentRegistry";

export function TraceFleetHealth({
  variant = "home",
}: {
  variant?: "home" | "compact";
}) {
  const { t } = useBarenaI18n();
  const registry = useAgentRegistry();
  if (!registry.project) return null;

  if (registry.isLoading) {
    return variant === "home" ? (
      <Skeleton height="78px" borderRadius="xl" />
    ) : null;
  }
  if (registry.isError || registry.entries.length === 0) return null;

  const metrics = summarizeAgentRegistry(registry.entries);
  const values = [
    { label: t("Agents observed"), value: metrics.agents },
    { label: t("Trace volume"), value: compactNumber(metrics.traces) },
    { label: t("Errors"), value: compactNumber(metrics.errors) },
    { label: t("Models"), value: metrics.models },
  ];

  if (variant === "compact") {
    return (
      <HStack
        width="full"
        minHeight="38px"
        paddingX={4}
        paddingY={1.5}
        gap={5}
        borderBottomWidth="1px"
        borderColor="border.muted"
        overflowX="auto"
        flexShrink={0}
      >
        <Link
          href={`/${registry.project.slug}/agent-registry`}
          fontSize="xs"
          fontWeight="semibold"
          color="fg"
          textDecoration="none"
          whiteSpace="nowrap"
        >
          {t("Runtime health")}
        </Link>
        {values.map((item) => (
          <HStack key={item.label} gap={1.5} whiteSpace="nowrap">
            <Text color="fg.subtle" textStyle="xs">
              {item.label}
            </Text>
            <Text fontFamily="mono" fontWeight="semibold" textStyle="xs">
              {item.value}
            </Text>
          </HStack>
        ))}
      </HStack>
    );
  }

  return (
    <Box
      borderWidth="1px"
      borderColor="border.muted"
      borderRadius="xl"
      padding={4}
    >
      <HStack justify="space-between" gap={3} marginBottom={3}>
        <VStack align="start" gap={0}>
          <Text fontWeight="semibold" textStyle="sm">
            {t("Runtime health")}
          </Text>
          <Text color="fg.muted" textStyle="xs">
            {t("Last 30 days")}
          </Text>
        </VStack>
        <Link
          href={`/${registry.project.slug}/agent-registry`}
          color="orange.fg"
          textStyle="xs"
          fontWeight="medium"
          textDecoration="none"
        >
          {t("Agent Registry")} →
        </Link>
      </HStack>
      <Grid
        templateColumns={{ base: "repeat(2, 1fr)", md: "repeat(4, 1fr)" }}
        gap={3}
      >
        {values.map((item) => (
          <Box key={item.label}>
            <Text fontFamily="mono" fontSize="lg" fontWeight="semibold">
              {item.value}
            </Text>
            <Text color="fg.muted" textStyle="xs">
              {item.label}
            </Text>
          </Box>
        ))}
      </Grid>
    </Box>
  );
}

function compactNumber(value: number): string {
  return new Intl.NumberFormat("en", {
    notation: value >= 1_000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(value);
}
