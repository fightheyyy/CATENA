export type AgentRegistryStatus = "ok" | "error" | "warning";

const PLATFORM_SERVICES = new Set([
  "barena-boundary",
  "barena-control-plane",
  "barena-evaluator",
  "barena-runner",
  "catena-app",
  "catena-core",
  "catena-evolution-runtime",
  "catena-runner",
  "spiral-app",
  "spiral-core",
  "spiral-runner",
]);

/** One deployment observation produced by the trace aggregate endpoint. */
export interface AgentRegistryObservation {
  serviceName: string;
  agentName: string | null;
  agentId: string | null;
  connectorId: string | null;
  serviceVersion: string | null;
  serviceInstanceId: string | null;
  deploymentEnvironment: string | null;
  traceCount: number;
  errorCount: number;
  warningCount: number;
  lastSeenAt: number;
  latestStatus: AgentRegistryStatus;
  models: string[];
  origins: string[];
}

export interface AgentRegistryEntry extends AgentRegistryObservation {
  key: string;
  name: string;
  runtime: string;
}

/**
 * Turn the telemetry aggregate into the product read model. Platform-owned
 * services are intentionally absent: this screen answers which customer
 * Agents are deployed, while their evaluator/boundary spans remain in Trace.
 */
export function buildAgentRegistry(
  observations: AgentRegistryObservation[],
): AgentRegistryEntry[] {
  return observations
    .filter(
      (observation) =>
        observation.serviceName.length > 0 &&
        !isPlatformService(observation.serviceName),
    )
    .map((observation) => ({
      ...observation,
      key: [
        observation.serviceName,
        observation.agentId || observation.agentName || "service",
        observation.deploymentEnvironment || "environment",
        observation.serviceInstanceId || "instance",
      ].join("||"),
      name: observation.agentName || observation.serviceName,
      runtime: detectRuntime(observation),
    }))
    .sort((left, right) => {
      const bySeen = right.lastSeenAt - left.lastSeenAt;
      return bySeen || left.name.localeCompare(right.name);
    });
}

export function detectRuntime(input: {
  serviceName: string;
  connectorId?: string | null;
}): string {
  const value = `${input.connectorId ?? ""} ${input.serviceName}`.toLowerCase();
  if (value.includes("xiaoba")) return "XiaoBaOS";
  if (value.includes("openclaw")) return "OpenClaw";
  if (value.includes("claude") || value.includes("cowork")) {
    return "Claude Code";
  }
  if (value.includes("codex")) return "Codex";
  if (value.includes("hermes")) return "Hermes";
  if (value.includes("opencode")) return "opencode";
  return "OpenTelemetry";
}

export function agentTraceQuery(
  entry: Pick<AgentRegistryEntry, "serviceName" | "agentName">,
): string {
  const service = `service:${quoteQueryValue(entry.serviceName)}`;
  if (!entry.agentName) return service;
  const agent = quoteQueryValue(entry.agentName);
  return `${service} AND (attribute.gen_ai.agent.name:${agent} OR attribute.xiaoba.role.name:${agent})`;
}

export function summarizeAgentRegistry(entries: AgentRegistryEntry[]) {
  return {
    agents: entries.length,
    traces: entries.reduce((total, entry) => total + entry.traceCount, 0),
    errors: entries.reduce((total, entry) => total + entry.errorCount, 0),
    models: new Set(entries.flatMap((entry) => entry.models)).size,
  };
}

function quoteQueryValue(value: string): string {
  return `"${value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function isPlatformService(serviceName: string): boolean {
  const normalized = serviceName.toLowerCase();
  return (
    PLATFORM_SERVICES.has(normalized) ||
    normalized.startsWith("langwatch-service-")
  );
}
