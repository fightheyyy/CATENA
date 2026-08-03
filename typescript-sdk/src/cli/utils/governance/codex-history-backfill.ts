import { createReadStream } from "node:fs";
import { readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";

import {
  createCodexRolloutParser,
  type CodexTurnIO,
} from "./codex-rollout";
import { postCodexTurns } from "./codex-rollout-otlp";

const DEFAULT_BATCH_SIZE = 25;
const DEFAULT_MAX_BATCH_BYTES = 2_000_000;

export interface CodexHistoryBackfillProgress {
  scannedFiles: number;
  totalFiles: number;
  uniqueTurns: number;
  uploadedTurns: number;
}

export interface CodexHistoryBackfillReport {
  scannedFiles: number;
  filesWithTurns: number;
  readErrors: number;
  parsedTurns: number;
  duplicateTurns: number;
  skippedOutOfWindowTurns: number;
  skippedLiveTurns: number;
  uniqueTurns: number;
  uploadedTurns: number;
  batches: number;
  dryRun: boolean;
}

export interface BackfillCodexHistoryOptions {
  endpoint: string;
  token: string;
  codexHome?: string;
  includeArchived?: boolean;
  dryRun?: boolean;
  batchSize?: number;
  maxBatchBytes?: number;
  /** Exclude turns before the receiver's accepted historical window. */
  afterMs?: number;
  /** Exclude turns at/after live OTel activation to avoid synthetic duplicates. */
  beforeMs?: number;
  fetchImpl?: typeof fetch;
  onProgress?: (progress: CodexHistoryBackfillProgress) => void;
}

async function walkJsonl(dir: string): Promise<string[]> {
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return [];
  }
  const files: string[] = [];
  for (const entry of entries) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) files.push(...(await walkJsonl(full)));
    else if (entry.isFile() && entry.name.endsWith(".jsonl")) files.push(full);
  }
  return files;
}

export async function findCodexHistoryFiles(
  codexHome = join(homedir(), ".codex"),
  includeArchived = true,
): Promise<string[]> {
  const files = await walkJsonl(join(codexHome, "sessions"));
  if (includeArchived) {
    files.push(...(await walkJsonl(join(codexHome, "archived_sessions"))));
  }
  return files.sort();
}

function estimatedBytes(turn: CodexTurnIO): number {
  return Buffer.byteLength(JSON.stringify(turn), "utf8");
}

async function parseHistoryFile(file: string): Promise<CodexTurnIO[]> {
  const parser = createCodexRolloutParser({
    synthesizeMissingTraceIds: true,
    includePriorHistory: false,
  });
  const input = createReadStream(file, { encoding: "utf8" });
  const lines = createInterface({ input, crlfDelay: Infinity });
  try {
    for await (const line of lines) parser.pushLine(line);
    return parser.finish();
  } finally {
    lines.close();
    input.destroy();
  }
}

/**
 * Convert persisted Codex rollouts into idempotent OTLP traces.
 *
 * The importer intentionally works one file at a time, deduplicates forked
 * history by deterministic trace id, and caps both turn count and serialized
 * bytes per request. A failed request stops the run; rerunning is safe because
 * both trace and span ids are stable.
 */
export async function backfillCodexHistory(
  options: BackfillCodexHistoryOptions,
): Promise<CodexHistoryBackfillReport> {
  const codexHome = options.codexHome ?? join(homedir(), ".codex");
  const files = await findCodexHistoryFiles(
    codexHome,
    options.includeArchived ?? true,
  );
  const batchSize = Math.max(1, options.batchSize ?? DEFAULT_BATCH_SIZE);
  const maxBatchBytes = Math.max(
    128_000,
    options.maxBatchBytes ?? DEFAULT_MAX_BATCH_BYTES,
  );
  const dryRun = options.dryRun ?? false;
  const seen = new Set<string>();
  let batch: CodexTurnIO[] = [];
  let batchBytes = 0;
  const report: CodexHistoryBackfillReport = {
    scannedFiles: 0,
    filesWithTurns: 0,
    readErrors: 0,
    parsedTurns: 0,
    duplicateTurns: 0,
    skippedOutOfWindowTurns: 0,
    skippedLiveTurns: 0,
    uniqueTurns: 0,
    uploadedTurns: 0,
    batches: 0,
    dryRun,
  };

  const flush = async (): Promise<void> => {
    if (batch.length === 0 || dryRun) return;
    await postCodexTurns({
      turns: batch,
      nowMs: Date.now(),
      endpoint: options.endpoint,
      token: options.token,
      fetchImpl: options.fetchImpl,
      timeoutMs: 30_000,
      exportOptions: {
        serviceName: "Codex Desktop",
        scopeName: "catena.codex.history",
      },
    });
    report.uploadedTurns += batch.length;
    report.batches++;
    batch = [];
    batchBytes = 0;
  };

  for (const file of files) {
    report.scannedFiles++;
    let turns: CodexTurnIO[];
    try {
      turns = await parseHistoryFile(file);
    } catch {
      report.readErrors++;
      options.onProgress?.({
        scannedFiles: report.scannedFiles,
        totalFiles: files.length,
        uniqueTurns: report.uniqueTurns,
        uploadedTurns: report.uploadedTurns,
      });
      continue;
    }
    if (turns.length > 0) report.filesWithTurns++;
    report.parsedTurns += turns.length;

    for (const turn of turns) {
      if (seen.has(turn.traceId)) {
        report.duplicateTurns++;
        continue;
      }
      seen.add(turn.traceId);
      if (
        options.afterMs !== undefined &&
        turn.startedAtMs !== null &&
        turn.startedAtMs < options.afterMs
      ) {
        report.skippedOutOfWindowTurns++;
        continue;
      }
      if (
        options.beforeMs !== undefined &&
        turn.startedAtMs !== null &&
        turn.startedAtMs >= options.beforeMs
      ) {
        report.skippedLiveTurns++;
        continue;
      }
      report.uniqueTurns++;
      if (dryRun) continue;

      const turnBytes = estimatedBytes(turn);
      if (
        batch.length > 0 &&
        (batch.length >= batchSize || batchBytes + turnBytes > maxBatchBytes)
      ) {
        await flush();
      }
      batch.push(turn);
      batchBytes += turnBytes;
    }

    options.onProgress?.({
      scannedFiles: report.scannedFiles,
      totalFiles: files.length,
      uniqueTurns: report.uniqueTurns,
      uploadedTurns: report.uploadedTurns,
    });
  }

  await flush();
  return report;
}
