import type { ScenarioRunData } from "~/server/scenarios/scenario-event.types";

const TERMINAL_STATUSES = new Set([
  "SUCCESS",
  "FAILED",
  "ERROR",
  "CANCELLED",
  "STALLED",
]);
const OUTPUT_PATHS = new Set([
  "",
  "$.response",
  "$.message",
  "$.content",
  "$.choices[0].message.content",
]);
const STANDARD_HTTP_BODY_TEMPLATE = `{
  "thread_id": "{{threadId}}",
  "messages": {{messages}}
}`;

export interface ScenarioAdoptionScenario {
  id: string;
  name: string;
  situation: string;
  criteria: string[];
}

export interface ScenarioAdoptionAgent {
  id: string;
  name: string;
  type: string;
  config: unknown;
}

export interface ScenarioRunAdoptionBody {
  schema: "barena.scenario_run_adoption.v1";
  source_project_id: string;
  scenario_run_id: string;
  scenario_id: string;
  source_status: string;
  started_at: string;
  completed_at: string;
  duration_in_ms: number;
  scenario: { name: string; objective: string; criteria: string[] };
  target: { type: "http"; reference_id: string; name: string };
  trace_ids: string[];
  primary_trace_id: string;
  judge?: {
    verdict: string;
    reasoning?: string;
    met_criteria: string[];
    unmet_criteria: string[];
    error?: string;
  };
  replay:
    | { supported: false; reason: string }
    | {
        supported: true;
        url: string;
        method: "POST";
        output_path?: string;
        timeout_ms: number;
      };
}

export function buildScenarioRunAdoption(input: {
  projectId: string;
  run: ScenarioRunData;
  scenario: ScenarioAdoptionScenario;
  agent: ScenarioAdoptionAgent;
}): ScenarioRunAdoptionBody {
  const status = String(input.run.status).toUpperCase();
  if (!TERMINAL_STATUSES.has(status)) {
    throw new Error("Only a terminal Explore run can become Barena evidence.");
  }
  const metadata = asRecord(input.run.metadata);
  const langwatch = asRecord(metadata?.langwatch);
  if (
    langwatch?.targetType !== "http" ||
    langwatch.targetReferenceId !== input.agent.id ||
    input.agent.type !== "http"
  ) {
    throw new Error(
      "This Explore run did not execute the selected registered HTTP Agent.",
    );
  }
  if (
    input.run.scenarioId !== input.scenario.id ||
    !input.scenario.situation.trim()
  ) {
    throw new Error(
      "The retained Scenario definition is missing or inconsistent.",
    );
  }
  const runTraceIds = (input.run.traceIds ?? []).filter(validTraceId);
  const messageTraceIds = input.run.messages
    .map((message) =>
      "trace_id" in message && typeof message.trace_id === "string"
        ? message.trace_id
        : undefined,
    )
    .filter(
      (value): value is string => value !== undefined && validTraceId(value),
    );
  const traceIds = unique([...runTraceIds, ...messageTraceIds]);
  const primaryTraceId = runTraceIds.at(-1) ?? messageTraceIds.at(-1);
  if (!primaryTraceId || traceIds.length === 0) {
    throw new Error(
      "The HTTP Agent did not retain a correlated W3C Trace; adoption fails closed.",
    );
  }
  const startedAt = finiteTimestamp(input.run.timestamp, "Scenario started_at");
  const duration = Math.max(0, Math.round(input.run.durationInMs));
  const completedAt = Math.max(
    startedAt,
    input.run.updatedAt && Number.isFinite(input.run.updatedAt)
      ? input.run.updatedAt
      : startedAt + duration,
  );
  const results = input.run.results;
  const judge = results
    ? {
        verdict: String(results.verdict).toLowerCase(),
        ...(results.reasoning && {
          reasoning: bounded(results.reasoning, 12_000),
        }),
        met_criteria: results.metCriteria.map((value) => bounded(value, 4_000)),
        unmet_criteria: results.unmetCriteria.map((value) =>
          bounded(value, 4_000),
        ),
        ...(results.error && { error: bounded(results.error, 2_000) }),
      }
    : undefined;

  return {
    schema: "barena.scenario_run_adoption.v1",
    source_project_id: input.projectId,
    scenario_run_id: input.run.scenarioRunId,
    scenario_id: input.scenario.id,
    source_status: status,
    started_at: new Date(startedAt).toISOString(),
    completed_at: new Date(completedAt).toISOString(),
    duration_in_ms: duration,
    scenario: {
      name: input.scenario.name,
      objective: input.scenario.situation,
      criteria: [...input.scenario.criteria],
    },
    target: {
      type: "http",
      reference_id: input.agent.id,
      name: input.agent.name,
    },
    trace_ids: traceIds,
    primary_trace_id: primaryTraceId,
    ...(judge && { judge }),
    replay: classifyReplay(input.agent.config),
  };
}

export function classifyReplay(
  value: unknown,
): ScenarioRunAdoptionBody["replay"] {
  const config = asRecord(value);
  if (!config) return unsupported("The HTTP Agent configuration is invalid.");
  const method =
    typeof config.method === "string" ? config.method.toUpperCase() : "POST";
  if (method !== "POST")
    return unsupported("Replay supports POST Agents only.");

  const auth = asRecord(config.auth);
  if (auth && auth.type !== undefined && auth.type !== "none") {
    return unsupported(
      "Authenticated HTTP Agents can Explore, but credentials are never frozen into a Replay Case.",
    );
  }
  const headers = Array.isArray(config.headers) ? config.headers : [];
  for (const rawHeader of headers) {
    const header = asRecord(rawHeader);
    const key =
      typeof header?.key === "string" ? header.key.trim().toLowerCase() : "";
    const headerValue =
      typeof header?.value === "string"
        ? header.value.trim().toLowerCase()
        : "";
    if (
      key &&
      !(key === "content-type" && headerValue === "application/json")
    ) {
      return unsupported(
        "Custom HTTP headers can contain secrets and are not frozen into Replay Cases.",
      );
    }
  }
  if (
    typeof config.bodyTemplate === "string" &&
    config.bodyTemplate.trim() &&
    normalizeTemplate(config.bodyTemplate) !==
      normalizeTemplate(STANDARD_HTTP_BODY_TEMPLATE)
  ) {
    return unsupported(
      "Custom HTTP body templates are not supported by deterministic Replay.",
    );
  }
  if (typeof config.url !== "string") {
    return unsupported("The HTTP Agent URL is missing.");
  }
  let parsed: URL;
  try {
    parsed = new URL(config.url);
  } catch {
    return unsupported("The HTTP Agent URL is invalid.");
  }
  if (
    !["http:", "https:"].includes(parsed.protocol) ||
    !parsed.hostname ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    config.url.includes("{") ||
    config.url.includes("}")
  ) {
    return unsupported(
      "Replay requires an absolute HTTP(S) URL without credentials, query values, fragments, or templates.",
    );
  }
  const outputPath =
    typeof config.outputPath === "string" ? config.outputPath.trim() : "";
  if (!OUTPUT_PATHS.has(outputPath)) {
    return unsupported(
      "The configured HTTP output path is not supported by Replay.",
    );
  }
  const timeoutMs = config.timeoutMs === undefined ? 30_000 : config.timeoutMs;
  if (
    typeof timeoutMs !== "number" ||
    !Number.isInteger(timeoutMs) ||
    timeoutMs < 1_000 ||
    timeoutMs > 120_000
  ) {
    return unsupported("HTTP Replay timeout must be from 1000 to 120000 ms.");
  }
  return {
    supported: true,
    url: parsed.toString(),
    method: "POST",
    ...(outputPath && { output_path: outputPath }),
    timeout_ms: timeoutMs,
  };
}

function normalizeTemplate(value: string): string {
  return value.replace(/\s+/g, "").trim();
}

function unsupported(reason: string): { supported: false; reason: string } {
  return { supported: false, reason };
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function validTraceId(value: string): boolean {
  return /^[a-f0-9]{32}$/i.test(value);
}

function unique(values: string[]): string[] {
  return [...new Set(values.map((value) => value.toLowerCase()))];
}

function finiteTimestamp(value: number, label: string): number {
  if (!Number.isFinite(value) || value <= 0)
    throw new Error(`${label} is invalid.`);
  return value;
}

function bounded(value: string, limit: number): string {
  return value.length <= limit ? value : value.slice(0, limit);
}
