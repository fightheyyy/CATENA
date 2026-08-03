/**
 * Emit codex turn input/output (recovered from the rollout transcript) as OTLP
 * spans on codex's native per-turn trace_ids when available. Historical
 * backfill uses deterministic synthetic trace ids because current Codex
 * Desktop rollouts persist turn_id but no longer persist the OTel join key.
 * See codex-rollout.ts for why the transcript is the only content source codex
 * offers.
 */
import { createHash } from "node:crypto";
import { readFile, readdir, stat } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { type CodexTurnIO, parseCodexRollout } from "./codex-rollout";

/** Deterministic 16-hex span id derived from the turn's trace_id. */
function ioSpanId(traceId: string): string {
  return createHash("sha256").update(`${traceId}:langwatch.io`).digest("hex").slice(0, 16);
}

function attr(key: string, value: string) {
  return { key, value: { stringValue: value } };
}

interface OtlpExportRequest {
  resourceSpans: unknown[];
}

export interface CodexIOExportOptions {
  serviceName?: string;
  environment?: string;
  scopeName?: string;
}

/**
 * Build an OTLP/JSON ExportTraceServiceRequest with one span per turn. Each
 * span rides codex's real trace_id and carries `langwatch.input` /
 * `langwatch.output` (read directly by the trace-summary IO accumulation) plus
 * `langwatch.span.type=llm` so the drawer renders it as the model response.
 *
 * `langwatch.input` is the full request body as the LangWatch structured
 * `chat_messages` envelope (system prompt + accumulated conversation + tool
 * calls). The receiver's `parseJsonStringValues` step parses the JSON string
 * into the `{ type, value }` object, and the LangWatch extractor canonicalises
 * it to `gen_ai.input.messages` + `gen_ai.system_instructions`, so the drawer
 * renders the same full conversation a claude trace does.
 */
export function buildCodexIOExportRequest(
  turns: CodexTurnIO[],
  nowMs: number,
  options: CodexIOExportOptions = {},
): OtlpExportRequest {
  const resourceAttributes = [
    attr("service.name", options.serviceName ?? "codex"),
  ];
  if (options.environment) {
    resourceAttributes.push(
      attr("deployment.environment.name", options.environment),
    );
  }
  const spans = turns.map((turn) => {
    const endCandidateMs = turn.completedAtMs ?? nowMs;
    const startMs = turn.startedAtMs ?? endCandidateMs;
    const endMs = Math.max(startMs, endCandidateMs);
    const attributes = [
      attr("langwatch.span.type", "llm"),
      attr(
        "langwatch.input",
        JSON.stringify({ type: "chat_messages", value: turn.inputMessages }),
      ),
      attr("langwatch.output", turn.output),
    ];
    if (turn.turnId) attributes.push(attr("codex.turn.id", turn.turnId));
    if (turn.sessionId) {
      attributes.push(attr("gen_ai.conversation.id", turn.sessionId));
    }
    attributes.push(
      attr("codex.trace_id.source", turn.traceIdSource ?? "native"),
    );
    if (turn.model) {
      attributes.push(attr("gen_ai.request.model", turn.model));
      attributes.push(attr("gen_ai.response.model", turn.model));
    }
    return {
      traceId: turn.traceId,
      spanId: ioSpanId(turn.traceId),
      name: "codex.turn.response",
      kind: 1,
      startTimeUnixNano: `${startMs}000000`,
      endTimeUnixNano: `${endMs}000000`,
      attributes,
      status: {},
    };
  });

  return {
    resourceSpans: [
      {
        resource: { attributes: resourceAttributes },
        scopeSpans: [
          {
            // A langwatch.* scope (NOT codex_cli_rs) so the ingestion
            // infra-span filter leaves these content spans alone.
            scope: { name: options.scopeName ?? "langwatch.codex.rollout" },
            spans,
          },
        ],
      },
    ],
  };
}

/**
 * Find rollout files codex wrote at or after `sinceMs`. Codex lays them out as
 * ~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<sessionid>.jsonl; we walk the
 * date subdirs and keep files whose mtime is within the session window.
 */
export async function findRecentRollouts(
  sinceMs: number,
  sessionsRoot = join(homedir(), ".codex", "sessions"),
): Promise<string[]> {
  const out: string[] = [];
  async function walk(dir: string, depth: number): Promise<void> {
    let entries;
    try {
      entries = await readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of entries) {
      const full = join(dir, e.name);
      if (e.isDirectory()) {
        // Year/month/day nesting is 3 deep; don't descend forever.
        if (depth < 3) await walk(full, depth + 1);
      } else if (
        e.isFile() &&
        e.name.startsWith("rollout-") &&
        e.name.endsWith(".jsonl")
      ) {
        try {
          const s = await stat(full);
          if (s.mtimeMs >= sinceMs) out.push(full);
        } catch {
          /* skip unreadable */
        }
      }
    }
  }
  await walk(sessionsRoot, 0);
  return out;
}

/** Read + parse every in-window rollout into one flat turn list. */
async function readRolloutTurns(
  sinceMs: number,
  sessionsRoot: string,
): Promise<CodexTurnIO[]> {
  const files = await findRecentRollouts(sinceMs, sessionsRoot);
  const turns: CodexTurnIO[] = [];
  for (const file of files) {
    try {
      turns.push(...parseCodexRollout(await readFile(file, "utf8")));
    } catch {
      /* skip unreadable rollout */
    }
  }
  return turns;
}

/**
 * POST a batch of turns as OTLP IO spans. Capped at 5s so a slow or unreachable
 * endpoint can't wedge the user's shell; the caller swallows failures (content
 * recovery must never break a coding session).
 */
export async function postCodexTurns(args: {
  turns: CodexTurnIO[];
  nowMs: number;
  endpoint: string;
  token: string;
  fetchImpl?: typeof fetch;
  timeoutMs?: number;
  exportOptions?: CodexIOExportOptions;
}): Promise<void> {
  const {
    turns,
    nowMs,
    endpoint,
    token,
    fetchImpl,
    timeoutMs = 5_000,
    exportOptions,
  } = args;
  const body = buildCodexIOExportRequest(turns, nowMs, exportOptions);
  const doFetch = fetchImpl ?? fetch;
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await doFetch(endpoint, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    if (!response.ok) {
      throw new Error(`Codex OTLP export failed with HTTP ${response.status}`);
    }

    // OTLP may acknowledge a request with HTTP 200 while rejecting individual
    // spans in `partialSuccess`. Treat that as a failed batch: callers can
    // safely retry because trace/span ids are deterministic.
    const responseText =
      typeof (response as { text?: unknown }).text === "function"
        ? await response.text()
        : "";
    if (responseText.trim()) {
      try {
        const payload = JSON.parse(responseText) as {
          partialSuccess?: {
            rejectedSpans?: number | string;
            errorMessage?: string;
          };
        };
        const rejected = Number(payload.partialSuccess?.rejectedSpans ?? 0);
        if (Number.isFinite(rejected) && rejected > 0) {
          const detail = payload.partialSuccess?.errorMessage?.trim();
          throw new Error(
            `Codex OTLP export rejected ${rejected} span(s)${detail ? `: ${detail}` : ""}`,
          );
        }
      } catch (error) {
        // A non-JSON success body is legal for some compatible collectors. A
        // parsed partial-success rejection, however, must reach the caller.
        if (
          error instanceof Error &&
          error.message.startsWith("Codex OTLP export rejected ")
        ) {
          throw error;
        }
      }
    }
  } finally {
    clearTimeout(timeout);
  }
}

/**
 * Recover codex turn I/O from rollouts written during this session and POST it
 * as OTLP spans. Best-effort and fully swallowed: a coding session must never
 * fail because the post-hoc content harvest hit a snag. Returns the number of
 * turns emitted (0 when nothing was found).
 */
export async function harvestAndEmitCodexIO(args: {
  sinceMs: number;
  nowMs: number;
  endpoint: string;
  token: string;
  sessionsRoot?: string;
  fetchImpl?: typeof fetch;
}): Promise<number> {
  const { sinceMs, nowMs, endpoint, token, sessionsRoot, fetchImpl } = args;
  const turns = await readRolloutTurns(
    sinceMs,
    sessionsRoot ?? join(homedir(), ".codex", "sessions"),
  );
  if (turns.length === 0) return 0;
  await postCodexTurns({ turns, nowMs, endpoint, token, fetchImpl });
  return turns.length;
}

/**
 * Streaming harvester: emits each turn the moment it completes instead of
 * dumping the whole session in one POST on exit. The wrapper polls `harvest()`
 * on an interval while codex runs (plus one final sweep on exit). The rollout
 * is append-only and `parseCodexRollout` only yields turns that have a reply,
 * so an in-flight turn simply isn't in the parse yet; we additionally dedup by
 * trace_id so a turn is POSTed exactly once across ticks. Re-emitting the same
 * turn would be idempotent server-side anyway (the span id is derived from the
 * trace_id), so a failed POST is safely retried on the next tick.
 */
export function createCodexIOStreamer(args: {
  sinceMs: number;
  endpoint: string;
  token: string;
  sessionsRoot?: string;
  fetchImpl?: typeof fetch;
}): { harvest: (nowMs: number) => Promise<number> } {
  const root = args.sessionsRoot ?? join(homedir(), ".codex", "sessions");
  const emitted = new Set<string>();
  return {
    async harvest(nowMs: number): Promise<number> {
      const turns = await readRolloutTurns(args.sinceMs, root);
      const fresh = turns.filter((t) => t.traceId && !emitted.has(t.traceId));
      if (fresh.length === 0) return 0;
      await postCodexTurns({
        turns: fresh,
        nowMs,
        endpoint: args.endpoint,
        token: args.token,
        fetchImpl: args.fetchImpl,
      });
      // Mark emitted only after a successful POST so a transient failure
      // retries the same turns next tick (dedup keeps the retry idempotent).
      for (const t of fresh) emitted.add(t.traceId);
      return fresh.length;
    },
  };
}
