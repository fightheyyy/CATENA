import { Box, VStack } from "@chakra-ui/react";
import {
  Bot,
  ClipboardList,
  Database,
  Gauge,
  ListTree,
  Settings as SettingsIcon,
  Sliders,
  Smartphone,
  Sparkles,
} from "lucide-react";
import React, { useMemo, useState } from "react";
import { useBarenaI18n } from "~/features/barena/i18n";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import { usePublicEnv } from "~/hooks/usePublicEnv";

import { useRequiredSession } from "~/hooks/useRequiredSession";
import { api } from "~/utils/api";
import { useRouter } from "~/utils/compat/next-router";
import { findPersonalProject } from "~/utils/personalProject";

import { MENU_WIDTH_COMPACT, MENU_WIDTH_EXPANDED } from "./MainMenu";
import { BarenaProductNavigation } from "./sidebar/BarenaProductNavigation";
import { BarenaUtilityNavigation } from "./sidebar/BarenaUtilityNavigation";
import { GovernSection } from "./sidebar/GovernSection";
import { LanguageToggle } from "./sidebar/LanguageToggle";
import { isOnlineEvaluationsActivePath } from "./sidebar/navigationActiveState";
import { SideMenuLink } from "./sidebar/SideMenuLink";
import { SupportMenu } from "./sidebar/SupportMenu";
import { ThemeToggle } from "./sidebar/ThemeToggle";

/**
 * Personal-scope sidebar rendered by DashboardLayout when
 * `personalScope=true`. Mirrors MainMenu's column shape (compact-on-hover,
 * width math, top-aligned primary nav + bottom-aligned utilities) so the
 * page geometry stays identical between project and personal scopes.
 *
 * Spec: specs/ai-gateway/governance/persona-aware-chrome.feature
 *       — Persona 1 / Persona 2 (personal scope)
 */
export const PersonalSidebar = React.memo(function PersonalSidebar({
  isCompact = false,
}: {
  isCompact?: boolean;
}) {
  const router = useRouter();
  const [isHovered, setIsHovered] = useState(false);
  const { t } = useBarenaI18n();
  const publicEnv = usePublicEnv();
  const isBarenaMode = publicEnv.data?.IS_BARENA_MODE ?? false;

  const effectiveCompact = isCompact && !isBarenaMode;
  const showExpanded = !effectiveCompact || isHovered;
  const currentWidth = showExpanded ? MENU_WIDTH_EXPANDED : MENU_WIDTH_COMPACT;

  const isUsageActive = router.pathname === "/me";
  const isConfigureActive = router.pathname.startsWith("/me/configure");
  const isDevicesActive = router.pathname.startsWith("/me/devices");
  const isOrgSettingsActive =
    router.pathname === "/settings" ||
    (router.pathname.startsWith("/settings") &&
      !router.pathname.startsWith("/settings/gateway")) ||
    (isBarenaMode && router.pathname.startsWith("/me/configure"));

  const session = useRequiredSession();
  const { organizations, hasPermission } = useOrganizationTeamProject({
    redirectToOnboarding: false,
    redirectToProjectOnboarding: false,
  });
  const personalProject = useMemo(
    () =>
      findPersonalProject({
        organizations,
        userId: session.data?.user?.id,
      }),
    [organizations, session.data?.user?.id],
  );
  const personalProjectSlug = personalProject?.slug ?? null;
  const personalProjectId = personalProject?.id ?? null;
  const tracesHref = personalProjectSlug
    ? `/${personalProjectSlug}/traces`
    : null;

  // The personal library entries link to the personal project's own
  // `/[project]/<section>` routes, so highlight them off the current path
  // the same way MainMenu does for project nav.
  const isTracesActive = router.pathname.includes("/traces");
  const isOnlineEvaluationsActive = isOnlineEvaluationsActivePath(
    router.pathname,
  );
  const isDatasetsActive = router.pathname.includes("/datasets");
  const isAnnotationsActive = router.pathname.includes("/annotations");
  const isAutomationsActive = router.pathname.includes("/automations");

  // Personal-workspace advanced features unlock the library nav entries
  // (datasets, evaluations, annotations, automations). Default-empty
  // storage means existing users see Traces only; clicking the bundle
  // checkbox in /me/configure flips them on with one atomic flip + audit.
  const featuresQuery = api.personalWorkspaceFeatures.get.useQuery(
    { projectId: personalProjectId ?? "" },
    {
      enabled: !!personalProjectId && !isBarenaMode,
      refetchOnWindowFocus: false,
    },
  );
  const features = featuresQuery.data;

  return (
    <Box
      background="bg.page"
      width={effectiveCompact ? MENU_WIDTH_COMPACT : MENU_WIDTH_EXPANDED}
      minWidth={effectiveCompact ? MENU_WIDTH_COMPACT : MENU_WIDTH_EXPANDED}
      height="calc(100vh - 60px)"
      position="relative"
      onMouseEnter={() => effectiveCompact && setIsHovered(true)}
      onMouseLeave={() => effectiveCompact && setIsHovered(false)}
    >
      <Box
        position={effectiveCompact ? "absolute" : "relative"}
        zIndex={effectiveCompact ? 100 : "auto"}
        top={0}
        left={0}
        width={currentWidth}
        height="calc(100vh - 60px)"
        background="bg.page"
        transition="width 0.15s ease-in-out"
        overflow="hidden"
      >
        <VStack
          paddingX={2}
          paddingTop={2}
          paddingBottom={2}
          gap={0}
          height="100%"
          align="start"
          width={MENU_WIDTH_EXPANDED}
          justifyContent="space-between"
        >
          <VStack
            width="full"
            gap={0.5}
            align="start"
            flex={1}
            minHeight={0}
            overflowY="auto"
            overflowX="hidden"
          >
            {isBarenaMode ? (
              <BarenaProductNavigation
                project={personalProject ?? undefined}
                pathname={router.pathname}
                showLabel={showExpanded}
              />
            ) : (
              <>
                <SideMenuLink
                  icon={Gauge}
                  label={t("My Usage")}
                  href="/me"
                  isActive={isUsageActive}
                  showLabel={showExpanded}
                />
                {tracesHref && (
                  <SideMenuLink
                    icon={ListTree}
                    label={t("Traces")}
                    href={tracesHref}
                    isActive={isTracesActive}
                    showLabel={showExpanded}
                  />
                )}
                {personalProjectSlug && features?.evaluations && (
                  <SideMenuLink
                    icon={ClipboardList}
                    label={t("Online Evals")}
                    href={`/${personalProjectSlug}/online-evaluations`}
                    isActive={isOnlineEvaluationsActive}
                    showLabel={showExpanded}
                  />
                )}
                {personalProjectSlug && features?.datasets && (
                  <SideMenuLink
                    icon={Database}
                    label={t("Datasets")}
                    href={`/${personalProjectSlug}/datasets`}
                    isActive={isDatasetsActive}
                    showLabel={showExpanded}
                  />
                )}
                {personalProjectSlug && features?.annotations && (
                  <SideMenuLink
                    icon={Sparkles}
                    label={t("Annotations")}
                    href={`/${personalProjectSlug}/annotations`}
                    isActive={isAnnotationsActive}
                    showLabel={showExpanded}
                  />
                )}
                {personalProjectSlug && features?.automations && (
                  <SideMenuLink
                    icon={Bot}
                    label={t("Automations")}
                    href={`/${personalProjectSlug}/automations`}
                    isActive={isAutomationsActive}
                    showLabel={showExpanded}
                  />
                )}
                <SideMenuLink
                  icon={Smartphone}
                  label={t("Devices")}
                  href="/me/devices"
                  isActive={isDevicesActive}
                  showLabel={showExpanded}
                />
                <SideMenuLink
                  icon={Sliders}
                  label={t("Configure")}
                  href="/me/configure"
                  isActive={isConfigureActive}
                  showLabel={showExpanded}
                />
                <GovernSection showExpanded={showExpanded} />
              </>
            )}
          </VStack>

          <VStack width="full" gap={0.5} align="start">
            {isBarenaMode ? (
              <BarenaUtilityNavigation
                pathname={router.asPath || router.pathname}
                showLabel={showExpanded}
                visible={hasPermission("organization:view")}
              />
            ) : (
              <>
                {hasPermission("organization:view") && (
                  <SideMenuLink
                    icon={SettingsIcon}
                    label={t("Settings")}
                    href="/settings"
                    isActive={isOrgSettingsActive}
                    showLabel={showExpanded}
                  />
                )}
                <SupportMenu showLabel={showExpanded} />
                <LanguageToggle showLabel={showExpanded} />
                <ThemeToggle showLabel={showExpanded} />
              </>
            )}
          </VStack>
        </VStack>
      </Box>
    </Box>
  );
});
