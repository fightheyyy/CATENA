import { DashboardLayout } from "~/components/DashboardLayout";
import { withPermissionGuard } from "~/components/WithPermissionGuard";
import { useBarenaI18n } from "~/features/barena/i18n";
import { ReleaseWorkbench } from "~/features/barena/ReleaseWorkbench";
import Head from "~/utils/compat/next-head";

function EvolutionPage() {
  const { t } = useBarenaI18n();
  return (
    <DashboardLayout>
      <Head>
        <title>{t("Evolution Station")} · Catena</title>
      </Head>
      <ReleaseWorkbench />
    </DashboardLayout>
  );
}

export default withPermissionGuard("evaluations:view", {
  layoutComponent: DashboardLayout,
})(EvolutionPage);
