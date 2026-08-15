# Catena Runtime Capture

Catena reads the authoritative local evidence written by Codex CLI and Claude
Code, assembles it with parsers derived from Langfuse's MIT-licensed open-source
plugins, converts it to `catena.coding_agent.event_graph.v1`, and sends
deterministic OTLP spans to Catena. It does not proxy model traffic and does not
connect to Langfuse.

Supported and acceptance-tested runtimes:

- Codex CLI `0.147.0` rollout JSONL
- Claude Code `2.1.112` transcript JSONL

Codex App, Hermes and OpenClaw are not supported. A runtime is added only after
it has a dedicated parser, redacted real fixture, golden graph and real E2E.

## Configuration

```bash
export CATENA_URL="https://your-catena.example"
export CATENA_API_KEY="catena_agent_..."
```

`CATENA_OTLP_ENDPOINT` can override the complete trace endpoint and
`CATENA_TRACE_DEBUG=true` enables fail-open diagnostics. No Langfuse API key or
environment variable is used.

For the Codex plugin, environment variables take precedence. When Codex is
started outside that shell (for example from a desktop launcher), the plugin
falls back to `credentials.json` in Codex's per-plugin `PLUGIN_DATA` directory:

```json
{
  "url": "http://127.0.0.1:5570",
  "api_key": "catena_agent_..."
}
```

The plugin data directory must be owner-only (`0700`) and the credential file
must be `0600`; a more permissive file is rejected fail-open. The Agent key is
never written to the repository, plugin bundle or `~/.codex/config.toml`.

## Live hooks

The committed plugins are:

- `codex/plugins/tracing` — Codex `Stop` hook
- `claude-plugin` — Claude Code `Stop` and `SessionEnd` hooks

The Claude plugin runs the local `catena_tap` package. From this checkout:

```bash
cd tap
python3.12 -m pip install -e .
cd codex
pnpm install
pnpm build
codex plugin marketplace add "$PWD"
codex plugin add tracing@catena-runtime
```

Both hook paths are fail-open. Parser offsets and upload ledgers advance only
after Catena accepts every candidate trace. Span IDs are stable across retries,
so a crash after upload but before the local state rename is also idempotent.
Codex CLI writes `task_complete` immediately after the synchronous Stop hook;
the plugin therefore runs one bounded detached settle pass through the same
parser, replacing the provisional incomplete spans by stable ID. Claude uses
Stop for incremental evidence and SessionEnd for the corresponding final pass.

“Live” here means turn-end incremental synchronization, not token streaming:
Codex invokes `Stop` as soon as each completed, aborted or interrupted Turn
stops, and that invocation uploads the newly changed Turn graph. Tool and model
events are therefore visible after the Turn stops, not while tokens are still
being generated. If the process dies before `Stop` can run, the ledger remains
unchanged and the next Stop or an explicit historical import recovers the same
stable spans.

Runtime identity is authoritative. Codex CLI `0.147.0` supplies native
`session_id` and `turn_id` but no W3C Trace ID, so Catena deterministically
encodes that exact pair as the transport Trace ID. Claude does the same with
its native `sessionId` and user-row UUID. Neither path creates a Capture Session
or fingerprints Prompt text.

## Historical import

Historical import invokes the same parser used by the live hook:

```bash
catena trace import claude /path/to/transcript.jsonl
catena trace import claude /path/to/transcript.jsonl --otlp
catena trace import claude /path/to/transcript.jsonl --upload

catena trace import codex /path/to/rollout.jsonl
catena trace import codex /path/to/rollout.jsonl --otlp --upload
```

Canonical output includes source accounting for every input JSONL row. A
non-empty unknown `call_id`/`tool_use_id` becomes explicit error evidence and
is never rebound to another tool by ordering or prompt similarity.

## Verification

```bash
cd tap
python3.12 -m pip install -e '.[dev]'
pytest
python3.12 -m compileall -q catena_tap

cd codex
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test
pnpm build
```

See `fixtures/README.md`, `THIRD_PARTY_NOTICES.md`, and the two pinned source
records under `third_party/`.
