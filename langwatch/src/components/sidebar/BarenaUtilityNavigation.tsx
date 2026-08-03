import { KeyRound } from "lucide-react";
import { useBarenaI18n } from "~/features/barena/i18n";
import { featureIcons } from "~/utils/featureIcons";
import { SideMenuLink } from "./SideMenuLink";

export function BarenaUtilityNavigation({
  pathname,
  showLabel,
  visible,
}: {
  pathname: string;
  showLabel: boolean;
  visible: boolean;
}) {
  const { t } = useBarenaI18n();
  if (!visible) return null;

  const apiKeysActive = pathname.startsWith("/settings/api-keys");
  const settingsActive =
    (!apiKeysActive && pathname.startsWith("/settings")) ||
    pathname.startsWith("/me/configure");

  return (
    <>
      <SideMenuLink
        icon={KeyRound}
        label={t("API Keys")}
        href="/settings/api-keys"
        isActive={apiKeysActive}
        showLabel={showLabel}
      />
      <SideMenuLink
        icon={featureIcons.settings.icon}
        label={t("Settings")}
        href="/settings"
        isActive={settingsActive}
        showLabel={showLabel}
      />
    </>
  );
}
