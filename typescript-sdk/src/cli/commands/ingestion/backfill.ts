import { statSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

import { readCodexTraceExporterTarget } from "@/cli/utils/codex-config-toml";
import {
  backfillCodexHistory,
  type CodexHistoryBackfillReport,
} from "@/cli/utils/governance/codex-history-backfill";

export interface BackfillOptions {
  dryRun?: boolean;
  archived?: boolean;
  confirmSensitiveHistory?: boolean;
  json?: boolean;
  /** Test-only overrides; the public command follows CODEX_HOME. */
  codexHome?: string;
  fetchImpl?: typeof fetch;
  batchSize?: number;
  historyDays?: number;
  /** Test-only clock override. */
  nowMs?: number;
}

const DAY_MS = 24 * 60 * 60 * 1_000;
const DEFAULT_HISTORY_DAYS = 30;

function bearerFromEnvironment(): string | null {
  const headers = process.env.OTEL_EXPORTER_OTLP_HEADERS ?? "";
  const headerToken = /(?:Authorization\s*=\s*)?Bearer\s+([^,\s]+)/i.exec(
    headers,
  )?.[1];
  return headerToken ?? process.env.LANGWATCH_API_KEY?.trim() ?? null;
}

function renderReport(report: CodexHistoryBackfillReport): void {
  process.stdout.write(
    [
      `Scanned ${report.scannedFiles} Codex session files.`,
      `Found ${report.uniqueTurns} importable unique completed turns (${report.duplicateTurns} fork/history duplicates removed).`,
      report.skippedOutOfWindowTurns > 0
        ? `${report.skippedOutOfWindowTurns} older turn(s) were outside the platform import window.`
        : null,
      report.skippedLiveTurns > 0
        ? `${report.skippedLiveTurns} turn(s) at or after live OTel activation were left to native telemetry.`
        : null,
      report.dryRun
        ? "Dry run only; no trace data was uploaded."
        : `Uploaded ${report.uploadedTurns} traces in ${report.batches} OTLP batches.`,
      report.readErrors > 0
        ? `${report.readErrors} unreadable file(s) were skipped.`
        : null,
    ]
      .filter((line): line is string => Boolean(line))
      .join("\n") + "\n",
  );
}

export async function backfillCommand(
  toolArg: string,
  options: BackfillOptions = {},
): Promise<CodexHistoryBackfillReport> {
  const tool = toolArg.trim().toLowerCase().replace(/-/g, "_");
  if (tool !== "codex") {
    throw new Error(
      `Historical backfill currently supports codex only; received '${toolArg}'.`,
    );
  }
  if (!options.dryRun && !options.confirmSensitiveHistory) {
    throw new Error(
      "Codex history contains prompts, replies, and tool output. Re-run with --confirm-sensitive-history after verifying the configured OTLP project.",
    );
  }

  const codexHome =
    options.codexHome ?? process.env.CODEX_HOME ?? join(homedir(), ".codex");
  const configPath = join(codexHome, "config.toml");
  const target = readCodexTraceExporterTarget(configPath);
  if (!target && !options.dryRun) {
    throw new Error(
      "Codex OTLP is not configured. Run `langwatch ingest install codex` first.",
    );
  }
  const token = target?.token ?? bearerFromEnvironment();
  if (!token && !options.dryRun) {
    throw new Error(
      "The Codex trace exporter has no Authorization token. Re-run `langwatch ingest install codex` or set OTEL_EXPORTER_OTLP_HEADERS.",
    );
  }

  const historyDays = options.historyDays ?? DEFAULT_HISTORY_DAYS;
  if (!Number.isInteger(historyDays) || historyDays < 1 || historyDays > 30) {
    throw new Error("--days must be an integer between 1 and 30.");
  }
  const nowMs = options.nowMs ?? Date.now();

  let lastProgress = 0;
  const report = await backfillCodexHistory({
    endpoint: target?.endpoint ?? "http://dry-run.invalid/v1/traces",
    token: token ?? "dry-run",
    codexHome,
    includeArchived: options.archived ?? true,
    dryRun: options.dryRun,
    batchSize: options.batchSize,
    afterMs: nowMs - historyDays * DAY_MS,
    beforeMs: target
      ? (() => {
          try {
            return statSync(configPath).mtimeMs;
          } catch {
            return undefined;
          }
        })()
      : undefined,
    fetchImpl: options.fetchImpl,
    onProgress: options.json
      ? undefined
      : (progress) => {
          if (
            progress.scannedFiles === progress.totalFiles ||
            progress.scannedFiles - lastProgress >= 100
          ) {
            lastProgress = progress.scannedFiles;
            process.stderr.write(
              `\rCodex history ${progress.scannedFiles}/${progress.totalFiles} files · ${progress.uniqueTurns} unique turns · ${progress.uploadedTurns} uploaded`,
            );
            if (progress.scannedFiles === progress.totalFiles) {
              process.stderr.write("\n");
            }
          }
        },
  });

  if (options.json) {
    process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  } else {
    renderReport(report);
  }
  return report;
}
