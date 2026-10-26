# Codex CLI role runtime

Catena still owns Inspector → Evolution → Reviewer, their instructions, task
prompts, evidence, validation and persistence. Only `/v1/turn` changes harness.
`/v1/manifest` and the Go ↔ Engine v1 schemas remain unchanged. `/v1/experiment`
and `/v1/harbor/*` keep their existing SDK/Harbor implementation.

## Configuration

The Engine image installs `@openai/codex@0.160.0`. The default backend is
`CATENA_ENGINE_BACKEND=codex`; set `CATENA_ENGINE_BACKEND=sdk` and recreate the
Engine for explicit rollback. Failed Codex turns never silently retry with SDK.
`CATENA_CODEX_BIN` can select an installed CLI executable for local development.
`CATENA_CODEX_REASONING_EFFORT` defaults to `low`; it is an Engine setting,
not a change to the existing model request schema.

The existing owner configuration supplies the exact model ID, Base URL and API
key. Codex uses a private provider with `wire_api="responses"`. The actual turn
checks compatibility with `/responses`, SSE streaming and JSON structured
outputs. Chat Completions support alone is insufficient. Unsupported endpoints
fail explicitly; authentication or transient failures return `model_error`.
Use the SDK backend for providers that only support Chat Completions.

Role instructions map verbatim to `developer_instructions`. No system prompt
file, Codex source modification, Role framework or new dispatcher is involved.
The output schema constrains transport shape; existing Go validators retain
business validation, including `agent_md`, Skill and DSH Plugin contracts.

Each turn runs `codex exec --json --ephemeral` in a fresh temporary workspace,
HOME and CODEX_HOME. It does not inherit host credentials or personal config.
Host Skill discovery and bundled Skills, plugins, hooks, apps, MCP discovery,
web search and shell tools are disabled. Project instructions are disabled.
The sandbox is read-only with approvals disabled. The provider key is passed
through a dedicated environment variable, never argv or the config file;
model tool subprocesses inherit no environment. Temporary files are removed
on success, failure, timeout and coroutine cancellation.

The adapter reads the final-message file, validates a JSON Role envelope, and
maps `turn.completed` Usage to the existing requests/input_tokens/output_tokens
fields. Cached input tokens are already included in input_tokens. CLI stdout
and stderr are bounded at 4 MiB each and final output at 128 KiB. A failed turn
or nonzero exit cannot become a successful result. Timeout/cancellation kills
the entire process group, escalating to SIGKILL. Diagnostics do not return raw
CLI stderr, provider errors or credentials. Client disconnect cancellation is
still unsupported by the existing synchronous HTTP protocol and the manifest
continues to advertise `cancellation=false`; the turn's own timeout still applies.

## Verification

```sh
cd engine
python -m pip install -e '.[dev,evaluation-e2b]'
pytest
python ../evals/trace-skill-benchmark/validate_verifiers.py
cd ../control-plane
go test ./...
go vet ./...
go test -race ./internal/control
```

`TestCodexEngineEvolutionAcceptance` is opt-in. Point it at a private Engine
running the pinned real CLI. Its default provider is a deterministic Responses
SSE stub; neither the Engine nor CLI is mocked. For an Engine in Docker, set
`CATENA_CODEX_ACCEPT_CALLBACK_HOST` to a hostname through which that container
can reach the test process's HTTP listener. CI uses `host.docker.internal` and
an explicit host-gateway mapping.

```sh
export CATENA_CODEX_ACCEPT_ENGINE_URL=http://127.0.0.1:18790
# Optional private Engine bearer token:
export CATENA_CODEX_ACCEPT_ENGINE_TOKEN=...
# Use a dedicated disposable database; the test creates its own local principal.
export BARENA_TEST_DATABASE_URL=postgresql://.../codex_acceptance
# Optional, when Engine runs in Docker:
export CATENA_CODEX_ACCEPT_CALLBACK_HOST=host.docker.internal
go test -run TestCodexEngineEvolutionAcceptance -v -timeout 10m ./internal/control
```

The test starts an Evolution Job through the existing Catena API, verifies
all three stages and their Usage, and reopens PostgreSQL to check that Finding,
Candidate and Review are committed. It also checks that the real Responses
request retains Codex's system instructions without a Skills/MCP catalog.
CI runs this real CLI + stub-provider + PostgreSQL test without external keys.

For a real model provider, additionally supply these environment variables
through a secret store or a protected local environment file:

- `CATENA_CODEX_ACCEPT_MODEL_URL`: Responses-compatible Base URL
- `CATENA_CODEX_ACCEPT_MODEL_NAME`: exact owner model ID
- `CATENA_CODEX_ACCEPT_MODEL_KEY`: provider key

A stub-provider pass verifies the harness and persistence integration, not model
availability or quality. A real-provider pass must be recorded separately.
