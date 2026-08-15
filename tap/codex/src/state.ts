import { createHash, randomUUID } from "node:crypto";
import * as fs from "node:fs/promises";

import type { CanonicalTrace } from "./canonical.js";

export type UploadLedger = {
  version: 1;
  completed: Record<string, string>;
  observed: Record<string, { trace_id: string; digest: string; state: string }>;
};

export function ledgerPath(rolloutFile: string): string {
  return `${rolloutFile}.catena`;
}

function emptyLedger(): UploadLedger {
  return { version: 1, completed: {}, observed: {} };
}

export async function loadLedger(rolloutFile: string): Promise<UploadLedger> {
  try {
    const value = JSON.parse(await fs.readFile(ledgerPath(rolloutFile), "utf-8")) as Partial<UploadLedger>;
    return {
      version: 1,
      completed: value.completed && typeof value.completed === "object" ? value.completed : {},
      observed: value.observed && typeof value.observed === "object" ? value.observed : {},
    };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return emptyLedger();
    throw error;
  }
}

async function saveLedger(rolloutFile: string, ledger: UploadLedger): Promise<void> {
  const file = ledgerPath(rolloutFile);
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  await fs.writeFile(temporary, `${JSON.stringify(ledger, null, 2)}\n`, { encoding: "utf-8", mode: 0o600 });
  await fs.rename(temporary, file);
}

export function traceDigest(trace: CanonicalTrace): string {
  return createHash("sha256").update(JSON.stringify(trace)).digest("hex");
}

export function tracesNeedingUpload(traces: CanonicalTrace[], ledger: UploadLedger): CanonicalTrace[] {
  return traces.filter((trace) => {
    if (ledger.completed[trace.turn_id] === trace.trace_id) return false;
    const observed = ledger.observed[trace.turn_id];
    return !observed || observed.trace_id !== trace.trace_id || observed.digest !== traceDigest(trace);
  });
}

export async function recordUploadedTraces(
  rolloutFile: string,
  ledger: UploadLedger,
  traces: CanonicalTrace[],
): Promise<void> {
  for (const trace of traces) {
    ledger.observed[trace.turn_id] = {
      trace_id: trace.trace_id,
      digest: traceDigest(trace),
      state: trace.state,
    };
    if (trace.state !== "incomplete") ledger.completed[trace.turn_id] = trace.trace_id;
  }
  await saveLedger(rolloutFile, ledger);
}

async function wait(milliseconds: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, milliseconds));
}

export async function withLedgerLock<T>(rolloutFile: string, callback: () => Promise<T>): Promise<T> {
  const lockFile = `${ledgerPath(rolloutFile)}.lock`;
  let handle: fs.FileHandle | undefined;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      handle = await fs.open(lockFile, "wx", 0o600);
      break;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error;
      try {
        const stat = await fs.stat(lockFile);
        if (Date.now() - stat.mtimeMs > 60_000) await fs.unlink(lockFile);
      } catch {
        // Another hook may have released the lock between stat and unlink.
      }
      await wait(50);
    }
  }
  if (!handle) throw new Error(`could not acquire Catena rollout state lock ${lockFile}`);
  try {
    return await callback();
  } finally {
    await handle.close().catch(() => undefined);
    await fs.unlink(lockFile).catch(() => undefined);
  }
}
