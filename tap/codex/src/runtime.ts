import { chmod, mkdir, readFile, rename, stat, unlink, writeFile } from "node:fs/promises";
import { join } from "node:path";

import type { CanonicalEventGraph, CanonicalTrace } from "./canonical.js";
import { canonicalGraphJSON, parseCodexRollout } from "./codex-graph.js";
import { endpointFromEnvironment, exportGraph, traceToOTLP, type ExportOptions } from "./otlp.js";
import {
  loadLedger,
  recordUploadedTraces,
  tracesNeedingUpload,
  withLedgerLock,
} from "./state.js";

export type CodexHookInput = {
  session_id?: string;
  turn_id?: string | null;
  transcript_path?: string;
  hook_event_name?: string;
};

export type RuntimeEnvironment = NodeJS.ProcessEnv;

type PluginCredentials = {
  api_key?: unknown;
  url?: unknown;
};

export async function writePluginCredentials(
  pluginData: string,
  credentials: { apiKey: string; url: string },
): Promise<string> {
  const apiKey = credentials.apiKey.trim();
  const url = credentials.url.trim().replace(/\/$/, "");
  if (!apiKey) throw new Error("Catena Agent API key is required");
  if (!url || !["http:", "https:"].includes(new URL(url).protocol)) {
    throw new Error("Catena URL must be an absolute HTTP(S) URL");
  }
  await mkdir(pluginData, { recursive: true, mode: 0o700 });
  if (process.platform !== "win32") await chmod(pluginData, 0o700);
  const destination = join(pluginData, "credentials.json");
  const temporary = join(pluginData, `.credentials.${process.pid}.${Date.now()}.tmp`);
  try {
    await writeFile(
      temporary,
      `${JSON.stringify({ url, api_key: apiKey }, null, 2)}\n`,
      { encoding: "utf-8", flag: "wx", mode: 0o600 },
    );
    if (process.platform !== "win32") await chmod(temporary, 0o600);
    await rename(temporary, destination);
  } catch (error) {
    await unlink(temporary).catch(() => undefined);
    throw error;
  }
  return destination;
}

function credentialsPath(environment: RuntimeEnvironment): string | undefined {
  const explicit = environment.CATENA_CREDENTIALS_FILE?.trim();
  if (explicit) return explicit;
  const pluginData = environment.PLUGIN_DATA?.trim();
  return pluginData ? join(pluginData, "credentials.json") : undefined;
}

async function environmentWithCredentials(
  environment: RuntimeEnvironment,
): Promise<RuntimeEnvironment> {
  if (environment.CATENA_API_KEY?.trim()) return environment;
  const file = credentialsPath(environment);
  if (!file) return environment;

  let metadata;
  try {
    metadata = await stat(file);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return environment;
    throw error;
  }
  if (!metadata.isFile()) throw new Error(`Catena credentials path is not a file: ${file}`);
  if (process.platform !== "win32" && (metadata.mode & 0o077) !== 0) {
    throw new Error(`Catena credentials must be readable only by the current user (chmod 600): ${file}`);
  }

  const credentials = JSON.parse(await readFile(file, "utf-8")) as PluginCredentials;
  const apiKey = typeof credentials.api_key === "string" ? credentials.api_key.trim() : "";
  if (!apiKey) throw new Error(`Catena credentials do not contain api_key: ${file}`);
  const url = typeof credentials.url === "string" ? credentials.url.trim() : "";
  return {
    ...environment,
    CATENA_API_KEY: apiKey,
    ...(!environment.CATENA_URL?.trim() && !environment.CATENA_OTLP_ENDPOINT?.trim() && url
      ? { CATENA_URL: url }
      : {}),
  };
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function debugEnabled(environment: RuntimeEnvironment): boolean {
  return ["1", "true", "yes", "on"].includes((environment.CATENA_TRACE_DEBUG ?? "").toLowerCase());
}

function exportOptions(environment: RuntimeEnvironment): ExportOptions | undefined {
  const apiKey = environment.CATENA_API_KEY?.trim();
  if (!apiKey) return undefined;
  return {
    endpoint: endpointFromEnvironment(environment),
    apiKey,
    debug: debugEnabled(environment),
  };
}

export async function runCodexHook(
  input: CodexHookInput,
  environment: RuntimeEnvironment = process.env,
): Promise<{ parsed: number; uploaded: number; skipped: number; failed: number }> {
  if (!input.transcript_path) return { parsed: 0, uploaded: 0, skipped: 0, failed: 0 };
  const options = exportOptions(await environmentWithCredentials(environment));
  if (!options) return { parsed: 0, uploaded: 0, skipped: 0, failed: 0 };
  return withLedgerLock(input.transcript_path, async () => {
    const graph = await parseCodexRollout(input.transcript_path!);
    if (input.session_id && input.session_id !== graph.session_id) {
      throw new Error(
        `Codex hook session_id ${input.session_id} does not match rollout ${graph.session_id}`,
      );
    }
    const ledger = await loadLedger(input.transcript_path!);
    const candidates = tracesNeedingUpload(graph.traces, ledger);
    const result = await exportGraph(graph, options, candidates);
    const uploaded = candidates.filter((trace) => result.uploaded.includes(trace.turn_id));
    if (uploaded.length > 0) await recordUploadedTraces(input.transcript_path!, ledger, uploaded);
    return {
      parsed: graph.traces.length,
      uploaded: uploaded.length,
      skipped: graph.traces.length - candidates.length,
      failed: result.failed.length,
    };
  });
}

export async function settleCodexHook(
  input: CodexHookInput,
  environment: RuntimeEnvironment = process.env,
  attempts = 20,
  delayMs = 100,
): Promise<void> {
  if (!input.transcript_path || !input.turn_id) return;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const graph = await parseCodexRollout(input.transcript_path);
    const target = graph.traces.find((trace) => trace.turn_id === input.turn_id);
    if (target && target.state !== "incomplete") {
      await runCodexHook(input, environment);
      return;
    }
    await wait(delayMs);
  }
  // Preserve any new incomplete evidence after the bounded settle window.
  await runCodexHook(input, environment);
}

export async function importCodexRollout(
  rolloutFile: string,
  options: {
    output?: "canonical" | "otlp";
    traceId?: string;
    environment?: RuntimeEnvironment;
    upload?: boolean;
  } = {},
): Promise<{ graph: CanonicalEventGraph; output: string; uploaded: string[]; failed: string[] }> {
  const graph = await parseCodexRollout(rolloutFile);
  const traces: CanonicalTrace[] = options.traceId
    ? graph.traces.filter((trace) => trace.trace_id === options.traceId)
    : graph.traces;
  const output =
    options.output === "otlp"
      ? `${JSON.stringify(traces.map((trace) => traceToOTLP(graph, trace)), null, 2)}\n`
      : canonicalGraphJSON({ ...graph, traces });
  if (!options.upload) return { graph: { ...graph, traces }, output, uploaded: [], failed: [] };
  const exporter = exportOptions(await environmentWithCredentials(options.environment ?? process.env));
  if (!exporter) throw new Error("CATENA_API_KEY is required for historical upload");
  const result = await exportGraph(graph, exporter, traces);
  return { graph: { ...graph, traces }, output, ...result };
}
