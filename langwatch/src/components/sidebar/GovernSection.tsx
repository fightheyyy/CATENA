import { Eye } from "lucide-react";
import React from "react";

import { useBarenaI18n } from "~/features/barena/i18n";
import { useFeatureFlag } from "~/hooks/useFeatureFlag";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import { usePublicEnv } from "~/hooks/usePublicEnv";
import { useRouter } from "~/utils/compat/next-router";
import { featureIcons } from "~/utils/featureIcons";
import { SidebarSection } from "./SidebarSection";
import { SideMenuLink } from "./SideMenuLink";

/**
 * GOVERN section rendered identically in both the project-scope MainMenu
 * and the personal-scope PersonalSidebar. Single source of truth for
 * icons, labels, FF gating, and beta pills so the two sidebars never
 * drift apart.
 */
export const GovernSection = React.memo(function GovernSection({
  showExpanded,
}: {
  showExpanded: boolean;
}) {
  const router = useRouter();
  const { t } = useBarenaI18n();
  const publicEnv = usePublicEnv();
  const { organization, hasPermission } = useOrganizationTeamProject({
    redirectToOnboarding: false,
    redirectToProjectOnboarding: false,
  });
  const { enabled: gatewayMenuEnabled } = useFeatureFlag(
    "release_ui_ai_gateway_menu_enabled",
    {
      organizationId: organization?.id,
      enabled: !!organization?.id,
    },
  );
  const { enabled: governancePreviewEnabled } = useFeatureFlag(
    "release_ui_ai_governance_enabled",
    {
      organizationId: organization?.id,
      enabled: !!organization?.id,
    },
  );

  const showGatewayEntry =
    gatewayMenuEnabled && hasPermission("virtualKeys:view");
  const showGovernanceEntry =
    governancePreviewEnabled && hasPermission("governance:view");

  // Spiral/Barena uses project API keys for OTLP and evaluation ingress. The
  // LangWatch AI Gateway and Governance products solve a different problem
  // (model proxying and organization policy), and exposing them in Spiral's
  // primary rail sends users into a second product shell.
  if (
    publicEnv.data?.IS_BARENA_MODE ||
    (!showGatewayEntry && !showGovernanceEntry)
  ) {
    return null;
  }

  const isGatewayActive =
    router.pathname.startsWith("/settings/gateway") ||
    router.pathname === "/settings/routing-policies" ||
    router.pathname === "/settings/model-providers";
  const isGovernanceActive =
    router.pathname === "/governance" ||
    router.pathname === "/settings/governance" ||
    router.pathname.startsWith("/settings/governance/");

  return (
    <SidebarSection
      id="govern"
      label={t("Govern")}
      showExpanded={showExpanded}
      defaultExpanded={false}
    >
      {showGatewayEntry && (
        <SideMenuLink
          icon={featureIcons.gateway.icon}
          label={t("AI Gateway")}
          href="/settings/gateway/virtual-keys"
          isActive={isGatewayActive}
          showLabel={showExpanded}
        />
      )}
      {showGovernanceEntry && (
        <SideMenuLink
          icon={Eye}
          label={t("AI Governance")}
          href="/governance"
          isActive={isGovernanceActive}
          showLabel={showExpanded}
          beta
          betaLabel={t("Beta")}
        />
      )}
    </SidebarSection>
  );
});
