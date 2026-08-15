#!/usr/bin/env node
import { spawn } from "node:child_process";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

import {
  importCodexRollout,
  runCodexHook,
  settleCodexHook,
  writePluginCredentials,
  type CodexHookInput,
} from "./runtime.js";

async function readStdin<T>(): Promise<T> {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) chunks.push(Buffer.from(chunk));
  const text = Buffer.concat(chunks).toString("utf-8").trim();
  if (!text) throw new Error("hook stdin is empty");
  return JSON.parse(text) as T;
}

function usage(): never {
  console.error(
    "usage: catena-codex-hook import <rollout.jsonl> [--otlp] [--upload] [--trace-id <id>]\n" +
      "       catena-codex-hook configure --plugin-data <directory> --url <catena-url>",
  );
  process.exit(2);
}

function option(args: string[], name: string): string | undefined {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : undefined;
}

async function runConfigure(args: string[]): Promise<void> {
  const pluginData = option(args, "--plugin-data") ?? process.env.PLUGIN_DATA;
  const url = option(args, "--url");
  if (!pluginData || !url) usage();
  const input = await readStdin<{ token?: unknown; api_key?: unknown }>();
  const apiKey = typeof input.token === "string"
    ? input.token
    : typeof input.api_key === "string"
      ? input.api_key
      : "";
  const destination = await writePluginCredentials(pluginData, { apiKey, url });
  process.stdout.write(`Catena Codex credentials configured at ${destination}\n`);
}

async function runImport(args: string[]): Promise<void> {
  const file = args[0];
  if (!file) usage();
  await readFile(file, "utf-8");
  const traceIndex = args.indexOf("--trace-id");
  const traceId = traceIndex >= 0 ? args[traceIndex + 1] : undefined;
  if (traceIndex >= 0 && !traceId) usage();
  const result = await importCodexRollout(file, {
    output: args.includes("--otlp") ? "otlp" : "canonical",
    upload: args.includes("--upload"),
    traceId,
  });
  process.stdout.write(result.output);
  if (result.failed.length > 0) process.exitCode = 1;
}

function scheduleSettle(input: CodexHookInput): void {
  if (!input.turn_id || (input.hook_event_name ?? "Stop").toLowerCase() !== "stop") return;
  const encoded = Buffer.from(JSON.stringify(input)).toString("base64url");
  const child = spawn(process.execPath, [fileURLToPath(import.meta.url), "settle-hook", encoded], {
    detached: true,
    stdio: "ignore",
    env: process.env,
  });
  child.unref();
}

async function runSettle(encoded: string | undefined): Promise<void> {
  if (!encoded) return;
  const input = JSON.parse(Buffer.from(encoded, "base64url").toString("utf-8")) as CodexHookInput;
  await settleCodexHook(input);
}

async function main(): Promise<void> {
  if (process.argv[2] === "configure") {
    await runConfigure(process.argv.slice(3));
    return;
  }
  if (process.argv[2] === "import") {
    await runImport(process.argv.slice(3));
    return;
  }
  if (process.argv[2] === "settle-hook") {
    await runSettle(process.argv[3]);
    return;
  }
  let input: CodexHookInput | undefined;
  try {
    input = await readStdin<CodexHookInput>();
    await runCodexHook(input);
  } catch (error) {
    if (process.env.CATENA_TRACE_DEBUG === "true") {
      console.error("[catena-runtime] Codex hook failed open", error);
    }
    // Runtime hooks are always fail-open.
  } finally {
    if (input) scheduleSettle(input);
  }
}

main().catch((error) => {
  if (["configure", "import"].includes(process.argv[2] ?? "")) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
});
