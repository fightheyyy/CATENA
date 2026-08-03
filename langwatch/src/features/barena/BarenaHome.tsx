import {
  Badge,
  Box,
  Card,
  Grid,
  Heading,
  HStack,
  Icon,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  LuActivity,
  LuArrowRight,
  LuFlaskConical,
  LuGitPullRequestArrow,
  LuRadio,
} from "react-icons/lu";
import { Link } from "~/components/ui/link";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import { EvolutionRuntimeStatus } from "./EvolutionRuntimeStatus";
import { useBarenaI18n } from "./i18n";
import { TraceFleetHealth } from "./TraceFleetHealth";

export function BarenaHome() {
  const { project } = useOrganizationTeamProject();
  const { t } = useBarenaI18n();
  if (!project) return null;

  const actions = [
    {
      icon: LuActivity,
      eyebrow: t("Observe"),
      title: t("See what every Agent actually did"),
      description: t(
        "Collect runtime-native OpenTelemetry traces in one project, regardless of the Agent framework.",
      ),
      action: t("Open Trace Explorer"),
      href: `/${project.slug}/traces`,
    },
    {
      icon: LuFlaskConical,
      eyebrow: t("Explore"),
      title: t("Test behavior with simulated users"),
      description: t(
        "Drive an HTTP Agent through multi-turn scenarios and retain the Judge result with its execution evidence.",
      ),
      action: t("Run an Explore"),
      href: `/${project.slug}/simulations/scenarios`,
    },
    {
      icon: LuGitPullRequestArrow,
      eyebrow: t("Evolution"),
      title: t("Turn failures into release evidence"),
      description: t(
        "Review an Issue, freeze it as a deterministic Case, and Replay it before the next Agent release.",
      ),
      action: t("Open Evolution"),
      href: `/${project.slug}/evolution`,
    },
  ];

  return (
    <Box width="full" padding={{ base: 5, lg: 8 }}>
      <VStack align="stretch" gap={6} maxWidth="1180px" margin="0 auto">
        <Box
          position="relative"
          overflow="hidden"
          borderWidth="1px"
          borderColor="border.muted"
          borderRadius="2xl"
          padding={{ base: 6, md: 8 }}
          background="bg.surface"
          boxShadow="0 20px 60px rgba(245, 107, 26, 0.08)"
          _before={{
            content: '""',
            position: "absolute",
            width: "360px",
            height: "360px",
            right: "-150px",
            top: "-210px",
            borderRadius: "full",
            background:
              "radial-gradient(circle, rgba(245,107,26,0.22), rgba(245,107,26,0))",
            pointerEvents: "none",
          }}
        >
          <VStack align="start" gap={4} position="relative">
            <HStack gap={2}>
              <Badge colorPalette="orange" variant="subtle">
                Catena Platform
              </Badge>
              <HStack gap={1.5} color="green.fg" fontSize="xs">
                <LuRadio size={12} />
                <Text>{t("OTLP-native")}</Text>
              </HStack>
            </HStack>
            <Box maxWidth="760px">
              <Heading
                as="h1"
                fontSize={{ base: "3xl", md: "4xl" }}
                letterSpacing="-0.035em"
                lineHeight="1.08"
              >
                {t("Your Agent evolution workspace")}
              </Heading>
              <Text color="fg.muted" marginTop={3} fontSize="md">
                {t(
                  "Observe deployed runtimes, explore unknown behavior, and turn concrete failures into auditable Replay and release decisions.",
                )}
              </Text>
            </Box>
            <HStack
              gap={2}
              flexWrap="wrap"
              color="fg.muted"
              fontFamily="mono"
              fontSize="xs"
            >
              {[
                t("OTLP Trace"),
                t("Explore Run"),
                t("Issue"),
                t("Immutable Case"),
                t("Replay"),
                t("Release Gate"),
              ].map((step, index, values) => (
                <HStack key={step} gap={2}>
                  <Text>{step}</Text>
                  {index < values.length - 1 && (
                    <Text color="orange.fg">→</Text>
                  )}
                </HStack>
              ))}
            </HStack>
          </VStack>
        </Box>

        <EvolutionRuntimeStatus />

        <TraceFleetHealth />

        <Grid templateColumns={{ base: "1fr", md: "repeat(3, 1fr)" }} gap={4}>
          {actions.map((action) => (
            <Card.Root
              key={action.href}
              variant="outline"
              borderColor="border.muted"
              transition="transform 150ms ease, border-color 150ms ease, box-shadow 150ms ease"
              _hover={{
                transform: "translateY(-2px)",
                borderColor: "orange.emphasized",
                boxShadow: "md",
              }}
            >
              <Card.Body gap={4}>
                <HStack justify="space-between">
                  <Box
                    padding={2.5}
                    borderRadius="xl"
                    background="orange.subtle"
                    color="orange.fg"
                  >
                    <Icon as={action.icon} boxSize={5} />
                  </Box>
                  <Text
                    textTransform="uppercase"
                    letterSpacing="0.12em"
                    fontSize="2xs"
                    fontWeight="semibold"
                    color="fg.subtle"
                  >
                    {action.eyebrow}
                  </Text>
                </HStack>
                <Box>
                  <Text fontWeight="semibold" fontSize="lg">
                    {action.title}
                  </Text>
                  <Text color="fg.muted" textStyle="sm" marginTop={1.5}>
                    {action.description}
                  </Text>
                </Box>
                <Link
                  href={action.href}
                  color="orange.fg"
                  fontSize="sm"
                  fontWeight="medium"
                  textDecoration="none"
                  marginTop="auto"
                >
                  {action.action} <LuArrowRight size={14} />
                </Link>
              </Card.Body>
            </Card.Root>
          ))}
        </Grid>
      </VStack>
    </Box>
  );
}
