import { createHash } from "node:crypto";
import * as fs from "node:fs/promises";
import * as http from "node:http";
import * as os from "node:os";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

import { afterEach, describe, expect, it } from "vitest";

import { parseCodexRollout } from "../src/codex-graph.js";
import { traceToOTLP } from "../src/otlp.js";
import { runCodexHook, settleCodexHook, writePluginCredentials } from "../src/runtime.js";
import { ledgerPath } from "../src/state.js";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureDirectory = path.join(root, "fixtures/codex/sessions/2026/08/14");
const fixture = path.join(fixtureDirectory, "rollout-redacted-main.jsonl");
const golden = path.join(root, "fixtures/golden/codex.canonical.json");
const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map((directory) => fs.rm(directory, { recursive: true, force: true })));
});

function trace(graph: Awaited<ReturnType<typeof parseCodexRollout>>, turnId: string) {
  return graph.traces.find((value) => value.turn_id === turnId)!;
}

function correlationTraceId(sessionId: string, turnId: string) {
  return createHash("sha256")
    .update(`catena:codex:${sessionId}:${turnId}`)
    .digest("hex")
    .slice(0, 32);
}

describe("Codex canonical event graph", () => {
  it("matches the redacted real fixture golden and accounts every runtime row", async () => {
    const graph = await parseCodexRollout(fixture);
    expect(graph).toEqual(JSON.parse(await fs.readFile(golden, "utf-8")));
    expect(graph.runtime).toBe("codex");
    expect(graph.session_id).toBe("22222222-2222-4222-8222-222222222222");
    expect(graph.traces).toHaveLength(14);
    expect(graph.traces.flatMap((value) => value.accounting)).toHaveLength(131);
  });

  it("uses native correlation ids and strictly pairs serial, parallel, failed and unknown tool evidence", async () => {
    const graph = await parseCodexRollout(fixture);
    expect(graph.traces.map((value) => value.trace_id)).toEqual(
      graph.traces.map((value) => correlationTraceId(graph.session_id, value.turn_id)),
    );
    expect(graph.traces.map((value) => value.nodes[0].attributes["catena.trace.correlation"])).toEqual(
      graph.traces.map((value) => `${graph.session_id}:${value.turn_id}`),
    );

    const serial = trace(graph, "turn_serial");
    expect(serial.nodes.filter((node) => node.kind === "tool").map((node) => node.runtime_id)).toEqual([
      "call_serial_1",
      "call_serial_2",
    ]);
    expect(serial.nodes.filter((node) => node.kind === "model")[1]?.input).toEqual([
      { call_id: "call_serial_1", name: "exec_command", output: "[redacted] first result", error: undefined },
    ]);

    const parallelTools = trace(graph, "turn_parallel").nodes.filter((node) => node.kind === "tool");
    expect(Object.fromEntries(parallelTools.map((node) => [node.runtime_id, node.output]))).toEqual({
      call_parallel_1: "[redacted] A",
      call_parallel_2: "[redacted] B",
    });

    expect(trace(graph, "turn_failure").state).toBe("error");
    expect(trace(graph, "turn_abort").state).toBe("aborted");
    expect(trace(graph, "turn_retry").nodes.some((node) => node.kind === "retry")).toBe(true);
    expect(trace(graph, "turn_compact").nodes.some((node) => node.kind === "context_compact")).toBe(true);

    const unknown = trace(graph, "turn_unknown");
    expect(unknown.state).toBe("error");
    expect(unknown.nodes.filter((node) => node.kind === "unmatched_tool_result").map((node) => node.runtime_id)).toEqual([
      "call_unknown_nonempty",
    ]);
    expect(unknown.nodes.some((node) => node.kind === "tool" && node.runtime_id === "call_unknown_nonempty")).toBe(false);

    const mcp = trace(graph, "turn_mcp").nodes.find((node) => node.kind === "tool")!;
    expect(mcp.runtime_id).toBe("call_mcp");
    expect(mcp.attributes).toMatchObject({
      "gen_ai.tool.type": "mcp",
      "mcp.server": "fixture_server",
      "mcp.tool": "fixture_tool",
    });
    expect(trace(graph, "turn_mcp").nodes.filter((node) => node.kind === "model")).toHaveLength(2);
  });

  it("preserves the Computer built-in action, result and exact call_id", async () => {
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "catena-codex-computer-"));
    temporaryDirectories.push(temporary);
    const computerFixture = path.join(temporary, "computer.jsonl");
    const rows = [
      { timestamp: "2026-08-14T05:00:00.000Z", type: "session_meta", payload: { id: "33333333-3333-4333-8333-333333333333" } },
      { timestamp: "2026-08-14T05:00:01.000Z", type: "event_msg", payload: { type: "task_started", turn_id: "turn_computer" } },
      { timestamp: "2026-08-14T05:00:01.100Z", type: "turn_context", payload: { model: "gpt-5.4" } },
      { timestamp: "2026-08-14T05:00:01.200Z", type: "event_msg", payload: { type: "user_message", message: "[redacted] computer action" } },
      {
        timestamp: "2026-08-14T05:00:02.000Z",
        type: "response_item",
        payload: {
          type: "computer_call",
          id: "item_computer",
          call_id: "call_computer",
          status: "completed",
          action: { type: "screenshot" },
          results: { image: "[redacted]" },
        },
      },
      { timestamp: "2026-08-14T05:00:02.100Z", type: "event_msg", payload: { type: "task_complete", turn_id: "turn_computer" } },
    ];
    await fs.writeFile(computerFixture, `${rows.map((row) => JSON.stringify(row)).join("\n")}\n`);

    const graph = await parseCodexRollout(computerFixture);
    const computer = trace(graph, "turn_computer").nodes.find((node) => node.kind === "tool")!;
    expect(computer.runtime_id).toBe("call_computer");
    expect(computer.input).toEqual({ type: "screenshot" });
    expect(computer.output).toEqual({ image: "[redacted]" });
    expect(computer.attributes["gen_ai.tool.type"]).toBe("computer");
  });

  it("parents subagent evidence through the exact spawn tool and emits stable error spans", async () => {
    const graph = await parseCodexRollout(fixture);
    expect(graph.traces.map((value) => traceToOTLP(graph, value))).toEqual(
      JSON.parse(await fs.readFile(path.join(root, "fixtures/golden/codex.otlp.json"), "utf-8")),
    );
    const delegated = trace(graph, "turn_subagent");
    const launch = delegated.nodes.find((node) => node.kind === "tool" && node.runtime_id === "call_spawn")!;
    const thread = delegated.nodes.find((node) => node.name === "agent.subagent.thread")!;
    const child = delegated.nodes.find((node) => node.name === "agent.subagent.turn")!;
    expect(thread.parent_key).toBe(launch.key);
    expect(child.parent_key).toBe(thread.key);

    const aborted = trace(graph, "turn_abort");
    const first = traceToOTLP(graph, aborted);
    expect(traceToOTLP(graph, aborted)).toEqual(first);
    const spans = (first.resourceSpans[0].scopeSpans as Array<{ spans: Array<Record<string, unknown>> }>)[0].spans;
    expect((spans[0].status as { code: number }).code).toBe(2);
  });
});

describe("Codex live hook", () => {
  it("loads the Agent key and Catena URL from private plugin data", async () => {
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "catena-codex-plugin-data-"));
    temporaryDirectories.push(temporary);
    const sessions = path.join(temporary, "sessions/2026/08/14");
    const pluginData = path.join(temporary, "plugin-data");
    await fs.mkdir(sessions, { recursive: true });
    await fs.mkdir(pluginData, { recursive: true });
    await fs.cp(fixtureDirectory, sessions, { recursive: true });
    const copied = path.join(sessions, "rollout-redacted-main.jsonl");
    const authorization: Array<string | undefined> = [];
    const server = http.createServer(async (request, response) => {
      authorization.push(request.headers.authorization);
      for await (const _chunk of request) {
        // Drain the request so the client can reuse the connection.
      }
      response.writeHead(200).end("{}");
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    try {
      const address = server.address() as { port: number };
      const credentials = await writePluginCredentials(pluginData, {
        url: `http://127.0.0.1:${address.port}`,
        apiKey: "plugin-data-key",
      });
      expect((await fs.stat(credentials)).mode & 0o777).toBe(0o600);
      expect((await fs.readdir(pluginData)).filter((name) => name.endsWith(".tmp"))).toEqual([]);

      expect(await runCodexHook(
        {
          session_id: "22222222-2222-4222-8222-222222222222",
          transcript_path: copied,
          hook_event_name: "Stop",
        },
        { PLUGIN_DATA: pluginData },
      )).toEqual({ parsed: 14, uploaded: 14, skipped: 0, failed: 0 });
      expect(authorization).toHaveLength(14);
      expect(new Set(authorization)).toEqual(new Set(["Bearer plugin-data-key"]));
    } finally {
      server.close();
    }
  });

  it("refuses plugin credentials readable by another user", async () => {
    if (process.platform === "win32") return;
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "catena-codex-unsafe-key-"));
    temporaryDirectories.push(temporary);
    const pluginData = path.join(temporary, "plugin-data");
    await fs.mkdir(pluginData, { recursive: true });
    const credentials = path.join(pluginData, "credentials.json");
    await fs.writeFile(credentials, JSON.stringify({ api_key: "unsafe-key" }));
    await fs.chmod(credentials, 0o644);

    await expect(runCodexHook(
      { transcript_path: fixture, hook_event_name: "Stop" },
      { PLUGIN_DATA: pluginData },
    )).rejects.toThrow("chmod 600");
  });

  it("uses the historical parser and does not upload a completed turn twice", async () => {
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "catena-codex-test-"));
    temporaryDirectories.push(temporary);
    const sessions = path.join(temporary, "sessions/2026/08/14");
    await fs.mkdir(sessions, { recursive: true });
    await fs.cp(fixtureDirectory, sessions, { recursive: true });
    const copied = path.join(sessions, "rollout-redacted-main.jsonl");
    const requests: unknown[] = [];
    const server = http.createServer(async (request, response) => {
      let body = "";
      for await (const chunk of request) body += chunk;
      requests.push(JSON.parse(body));
      response.writeHead(200).end("{}");
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    try {
      const address = server.address() as { port: number };
      const environment = {
        CATENA_API_KEY: "test-key",
        CATENA_OTLP_ENDPOINT: `http://127.0.0.1:${address.port}/v1/traces`,
      };
      const input = {
        session_id: "22222222-2222-4222-8222-222222222222",
        transcript_path: copied,
        hook_event_name: "Stop",
      };
      expect(await runCodexHook(input, environment)).toEqual({ parsed: 14, uploaded: 14, skipped: 0, failed: 0 });
      expect(await runCodexHook(input, environment)).toEqual({ parsed: 14, uploaded: 0, skipped: 14, failed: 0 });
      expect(requests).toHaveLength(14);
      expect(await parseCodexRollout(copied)).toEqual(await parseCodexRollout(fixture));
    } finally {
      server.close();
    }
  });

  it("does not advance the ledger after a rejected upload", async () => {
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "catena-codex-fail-"));
    temporaryDirectories.push(temporary);
    const sessions = path.join(temporary, "sessions/2026/08/14");
    await fs.mkdir(sessions, { recursive: true });
    await fs.cp(fixtureDirectory, sessions, { recursive: true });
    const copied = path.join(sessions, "rollout-redacted-main.jsonl");
    const server = http.createServer((_request, response) => response.writeHead(503).end());
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    try {
      const address = server.address() as { port: number };
      const result = await runCodexHook(
        { session_id: "22222222-2222-4222-8222-222222222222", transcript_path: copied },
        { CATENA_API_KEY: "test-key", CATENA_OTLP_ENDPOINT: `http://127.0.0.1:${address.port}` },
      );
      expect(result.failed).toBe(14);
      await expect(fs.stat(ledgerPath(copied))).rejects.toMatchObject({ code: "ENOENT" });
    } finally {
      server.close();
    }
  });

  it("settles the Stop-hook snapshot to the exact historical result after task_complete is appended", async () => {
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "catena-codex-settle-"));
    temporaryDirectories.push(temporary);
    const sessions = path.join(temporary, "sessions/2026/08/14");
    await fs.mkdir(sessions, { recursive: true });
    await fs.cp(fixtureDirectory, sessions, { recursive: true });
    const copied = path.join(sessions, "rollout-redacted-main.jsonl");
    const lines = (await fs.readFile(copied, "utf-8")).trimEnd().split("\n");
    const taskComplete = lines.pop()!;
    expect(taskComplete).toContain('"type":"task_complete"');
    await fs.writeFile(copied, `${lines.join("\n")}\n`);

    const requests: unknown[] = [];
    const server = http.createServer(async (request, response) => {
      let body = "";
      for await (const chunk of request) body += chunk;
      requests.push(JSON.parse(body));
      response.writeHead(200).end("{}");
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    try {
      const address = server.address() as { port: number };
      const environment = {
        CATENA_API_KEY: "test-key",
        CATENA_OTLP_ENDPOINT: `http://127.0.0.1:${address.port}/v1/traces`,
      };
      const input = {
        session_id: "22222222-2222-4222-8222-222222222222",
        turn_id: "turn_mcp",
        transcript_path: copied,
        hook_event_name: "Stop",
      };
      expect(await runCodexHook(input, environment)).toEqual({
        parsed: 14,
        uploaded: 14,
        skipped: 0,
        failed: 0,
      });
      const append = new Promise<void>((resolve, reject) => {
        setTimeout(() => {
          fs.appendFile(copied, `${taskComplete}\n`).then(resolve, reject);
        }, 25);
      });
      await settleCodexHook(input, environment, 20, 10);
      await append;

      const historical = await parseCodexRollout(fixture);
      expect(requests.at(-1)).toEqual(traceToOTLP(historical, trace(historical, "turn_mcp")));
      expect(await parseCodexRollout(copied)).toEqual(historical);
    } finally {
      server.close();
    }
  });
});
