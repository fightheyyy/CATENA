import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { backfillCodexHistory } from "../codex-history-backfill";

const line = (value: unknown) => JSON.stringify(value);

function completedTurn(turnId: string, prompt: string, output: string): string {
  return [
    {
      type: "event_msg",
      payload: {
        type: "task_started",
        turn_id: turnId,
        started_at: 1_780_000_000,
      },
    },
    {
      type: "response_item",
      payload: {
        type: "message",
        role: "user",
        content: [{ type: "input_text", text: prompt }],
      },
    },
    {
      type: "event_msg",
      payload: { type: "agent_message", message: output, phase: "final_answer" },
    },
    {
      type: "event_msg",
      payload: {
        type: "task_complete",
        turn_id: turnId,
        completed_at: 1_780_000_010,
      },
    },
  ]
    .map(line)
    .join("\n");
}

describe("backfillCodexHistory", () => {
  let codexHome: string;

  beforeEach(async () => {
    codexHome = await mkdtemp(join(tmpdir(), "codex-history-"));
    await mkdir(join(codexHome, "sessions", "2026", "08", "03"), {
      recursive: true,
    });
    await mkdir(join(codexHome, "archived_sessions"), { recursive: true });
  });

  afterEach(async () => {
    await rm(codexHome, { recursive: true, force: true });
  });

  /** @scenario "Current and archived Codex history is deduplicated and uploaded in bounded OTLP batches" */
  it("imports current-format turns with stable ids and original timestamps", async () => {
    const duplicateTurn = "550e8400-e29b-41d4-a716-446655440000";
    const secondTurn = "550e8400-e29b-41d4-a716-446655440001";
    await writeFile(
      join(codexHome, "archived_sessions", "rollout-old.jsonl"),
      completedTurn(duplicateTurn, "first", "one"),
    );
    await writeFile(
      join(codexHome, "sessions", "2026", "08", "03", "rollout-new.jsonl"),
      [
        completedTurn(duplicateTurn, "first", "one"),
        completedTurn(secondTurn, "second", "two"),
      ].join("\n"),
    );

    const requests: any[] = [];
    const fetchImpl = vi.fn(async (_url: string, init?: RequestInit) => {
      if (typeof init?.body !== "string") throw new Error("missing JSON body");
      requests.push(JSON.parse(init.body));
      return new Response(null, { status: 200 });
    }) as unknown as typeof fetch;

    const report = await backfillCodexHistory({
      codexHome,
      endpoint: "http://collector.test/v1/traces",
      token: "trace-write-key",
      batchSize: 1,
      fetchImpl,
    });

    expect(report).toMatchObject({
      scannedFiles: 2,
      parsedTurns: 3,
      duplicateTurns: 1,
      uniqueTurns: 2,
      uploadedTurns: 2,
      batches: 2,
      readErrors: 0,
    });
    expect(fetchImpl).toHaveBeenCalledTimes(2);
    const firstResource = requests[0].resourceSpans[0];
    expect(firstResource.resource.attributes).toContainEqual({
      key: "service.name",
      value: { stringValue: "Codex Desktop" },
    });
    expect(firstResource.scopeSpans[0].scope.name).toBe(
      "catena.codex.history",
    );
    expect(firstResource.scopeSpans[0].spans[0]).toMatchObject({
      startTimeUnixNano: "1780000000000000000",
      endTimeUnixNano: "1780000010000000000",
    });
  });

  /** @scenario "A dry run inventories history without transmitting transcript content" */
  it("does not call the OTLP endpoint in dry-run mode", async () => {
    await writeFile(
      join(codexHome, "sessions", "2026", "08", "03", "rollout.jsonl"),
      completedTurn("550e8400-e29b-41d4-a716-446655440002", "hi", "hello"),
    );
    const fetchImpl = vi.fn();

    const report = await backfillCodexHistory({
      codexHome,
      endpoint: "http://collector.test/v1/traces",
      token: "unused",
      dryRun: true,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });

    expect(report.uniqueTurns).toBe(1);
    expect(report.uploadedTurns).toBe(0);
    expect(report.dryRun).toBe(true);
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  /** @scenario "History at or after live OTel activation is not synthesized a second time" */
  it("leaves overlapping live turns to Codex native telemetry", async () => {
    await writeFile(
      join(codexHome, "sessions", "2026", "08", "03", "rollout.jsonl"),
      completedTurn("550e8400-e29b-41d4-a716-446655440003", "hi", "hello"),
    );

    const report = await backfillCodexHistory({
      codexHome,
      endpoint: "http://collector.test/v1/traces",
      token: "unused",
      dryRun: true,
      beforeMs: 1_780_000_000_000,
    });

    expect(report.uniqueTurns).toBe(0);
    expect(report.skippedLiveTurns).toBe(1);
  });

  /** @scenario "Turns outside the receiver window are counted without sending doomed spans" */
  it("skips history before the configured import window", async () => {
    await writeFile(
      join(codexHome, "sessions", "2026", "08", "03", "rollout.jsonl"),
      completedTurn("550e8400-e29b-41d4-a716-446655440004", "old", "turn"),
    );
    const fetchImpl = vi.fn();

    const report = await backfillCodexHistory({
      codexHome,
      endpoint: "http://collector.test/v1/traces",
      token: "unused",
      afterMs: 1_780_000_000_001,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });

    expect(report.uniqueTurns).toBe(0);
    expect(report.skippedOutOfWindowTurns).toBe(1);
    expect(report.uploadedTurns).toBe(0);
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  /** @scenario "HTTP 200 partial-success responses never inflate uploaded history counts" */
  it("fails a batch when the OTLP receiver rejects any spans", async () => {
    await writeFile(
      join(codexHome, "sessions", "2026", "08", "03", "rollout.jsonl"),
      completedTurn("550e8400-e29b-41d4-a716-446655440005", "old", "turn"),
    );
    const fetchImpl = vi.fn(async () =>
      Response.json({
        partialSuccess: {
          rejectedSpans: 1,
          errorMessage: "span start time is more than 31 days in the past",
        },
      }),
    ) as unknown as typeof fetch;

    await expect(
      backfillCodexHistory({
        codexHome,
        endpoint: "http://collector.test/v1/traces",
        token: "trace-write-key",
        fetchImpl,
      }),
    ).rejects.toThrow("Codex OTLP export rejected 1 span");
  });
});
