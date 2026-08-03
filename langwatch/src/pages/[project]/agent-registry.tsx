import { DashboardLayout } from "~/components/DashboardLayout";
import { withPermissionGuard } from "~/components/WithPermissionGuard";
import { AgentRegistry } from "~/features/barena/AgentRegistryView";
import { useBarenaI18n } from "~/features/barena/i18n";
import Head from "~/utils/compat/next-head";

function AgentRegistryPage() {
  const { t } = useBarenaI18n();
  return (
    <DashboardLayout>
      <Head>
        <title>{t("Agent Registry")} · Catena</title>
      </Head>
      <AgentRegistry />
    </DashboardLayout>
  );
}

export default withPermissionGuard("traces:view", {
  layoutComponent: DashboardLayout,
})(AgentRegistryPage);
