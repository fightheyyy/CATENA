import { useState } from "react";
import { useOrganizationTeamProject } from "~/hooks/useOrganizationTeamProject";
import { api } from "~/utils/api";
import { buildAgentRegistry } from "./agentRegistry";

const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1_000;

export function useAgentRegistry() {
  const { project } = useOrganizationTeamProject();
  const [timeRange] = useState(() => ({
    from: Date.now() - THIRTY_DAYS_MS,
    to: Date.now(),
    live: true,
  }));
  const query = api.tracesV2.agentRegistry.useQuery(
    { projectId: project?.id ?? "", timeRange },
    {
      enabled: !!project?.id,
      refetchInterval: 30_000,
      refetchOnWindowFocus: false,
    },
  );

  return {
    ...query,
    project,
    entries: buildAgentRegistry(query.data ?? []),
  };
}
