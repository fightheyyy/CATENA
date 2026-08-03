import { useBarenaI18n } from "~/features/barena/i18n";
import { featureIcons } from "~/utils/featureIcons";
import { projectRoutes } from "~/utils/routes";
import { projectScopedDestination } from "./projectScopedNav";
import { SideMenuLink } from "./SideMenuLink";

export const BARENA_PRIMARY_NAVIGATION = [
  {
    id: "home",
    label: "Home",
    path: projectRoutes.home.path,
    icon: featureIcons.home.icon,
    isActive: (pathname: string) => pathname === "/[project]",
  },
  {
    id: "agents",
    label: "Agents",
    path: projectRoutes.agent_registry.path,
    icon: featureIcons.agents.icon,
    isActive: (pathname: string) => pathname.includes("/agent-registry"),
  },
  {
    id: "trace",
    label: "Trace",
    path: projectRoutes.traces_v2.path,
    icon: featureIcons.traces_v2.icon,
    isActive: (pathname: string) => pathname.includes("/traces"),
  },
  {
    id: "explore",
    label: "Explore",
    path: projectRoutes.scenarios.path,
    icon: featureIcons.simulations.icon,
    isActive: (pathname: string) => pathname.includes("/simulations"),
  },
  {
    id: "evolution",
    label: "Evolution",
    path: projectRoutes.evolution.path,
    icon: featureIcons.evolution.icon,
    isActive: (pathname: string) => pathname.includes("/evolution"),
  },
] as const;

export function BarenaProductNavigation({
  project,
  pathname,
  showLabel,
}: {
  project?: { slug: string };
  pathname: string;
  showLabel: boolean;
}) {
  const { t } = useBarenaI18n();

  return BARENA_PRIMARY_NAVIGATION.map((item) => {
    const destination = projectScopedDestination({
      path: item.path,
      label: item.label,
      project,
    });
    return (
      <SideMenuLink
        key={item.id}
        icon={item.icon}
        label={t(item.label)}
        href={destination.href}
        unavailableReason={destination.unavailableReason}
        isActive={item.isActive(pathname)}
        showLabel={showLabel}
      />
    );
  });
}
