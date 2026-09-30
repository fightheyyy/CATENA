import type { AgentSummary } from "./types";

export function registeredAgentSummaries(agents: AgentSummary[]) {
  return agents.filter((agent) => agent.registered);
}

export function canAnalyzeAgent(agent: AgentSummary) {
  return agent.connected && agent.trace_count >= 2;
}

export type ConnectionShell = "sh" | "powershell";

export function agentConnectionConfig(origin: string, name: string, shell: ConnectionShell, token = "<CATENA_API_KEY>") {
  const values = {
    CATENA_URL: origin.replace(/\/+$/, ""), CATENA_API_KEY: token, OTEL_SERVICE_NAME: name,
    OTEL_TRACES_EXPORTER: "otlp", OTEL_EXPORTER_OTLP_PROTOCOL: "http/protobuf",
  };
  if (shell === "powershell") {
    return [...Object.entries(values).map(([key, value]) => `$env:${key} = '${value.replace(/'/g, "''")}'`),
      '$env:OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = "$env:CATENA_URL/v1/otlp/v1/traces"',
      '$env:OTEL_EXPORTER_OTLP_HEADERS = "Authorization=Bearer $env:CATENA_API_KEY"'].join("\n");
  }
  return [...Object.entries(values).map(([key, value]) => `export ${key}='${value.replace(/'/g, "'\\''")}'`),
    'export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="${CATENA_URL}/v1/otlp/v1/traces"',
    'export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${CATENA_API_KEY}"'].join("\n");
}
