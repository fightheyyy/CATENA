import type { AgentSummary } from "./types";

export type Locale = "zh" | "en";

export function agentSources(agent: AgentSummary) {
  const seen = new Set<string>();
  return (agent.sources ?? []).filter((source) => {
    const serviceName = source.service_name.trim();
    if (!serviceName || seen.has(serviceName)) return false;
    seen.add(serviceName);
    return true;
  });
}

export function agentSourceKindLabel(
  kind: NonNullable<AgentSummary["sources"]>[number]["kind"],
  locale: Locale,
) {
  if (kind === "native_live") return locale === "zh" ? "实时" : "Live";
  if (kind === "history_backfill") return locale === "zh" ? "历史" : "History";
  return "OTel";
}

export function agentSourceSummary(agent: AgentSummary, locale: Locale) {
  const labels = [...new Set(agentSources(agent).map((source) => agentSourceKindLabel(source.kind, locale)))];
  return labels.join(" + ");
}

export function agentIdentitySourceLabel(source: string, locale: Locale) {
  if (source === "api_key") return locale === "zh" ? "Agent 接入密钥" : "Agent connection key";
  if (source === "catena.alias") return locale === "zh" ? "Catena Agent 归并" : "Catena Agent mapping";
  if (source === "service.name") return "OTel service.name";
  return source;
}
