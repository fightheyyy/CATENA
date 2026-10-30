# M0 runtime infrastructure acceptance

Original base: `ec31cab6951dcb0fdf60b90e70eb3c3bc229dcb6`; integrated with
`2164ec4` (leased Workers / ingestion backpressure). Scope is dependency isolation,
timeout/admission bounds, existing runtime contracts and documentation. No Agent
runtime replacement, Memory/OpenViking, MCP tools, frontend, database schema or
Harbor evaluation-logic changes.

## Dependency/CI repair

- Analysis Engine + explicit SDK rollback: Agents SDK 0.22.3 / OpenAI 3.x.
- Isolated Harbor environment: Harbor 0.23.0 (including E2B), Agents SDK 0.12.0,
  OpenAI 2.26.0. Both environments pass dependency checks. OpenAI is pinned here
  because 2.54.0 makes `cache_write_tokens` mandatory and breaks the older SDK's
  default token-detail construction.
- SDK is an optional extra, loaded lazily; the Engine image explicitly installs
  `.[sdk]`. Harbor extras install the compatible worker dependencies in a separate
  venv. Installing modern SDK and Harbor extras together is intentionally unsupported.
- CI runs the modern SDK contracts and the **full** suite in the Harbor venv.
  This retains task conversion, native Codex artifacts, credentials/idempotency,
  evidence bounds, E2B lifecycle adapter and experiment coordinator coverage.
  Seven existing Windows-only PowerShell tests remain skipped on Linux.
- First repaired full Actions run: https://github.com/fightheyyy/CATENA/actions/runs/40373702065
  (all five jobs passed). Final PR check links are available on the PR.

## Real model acceptance

Historical run on the original PR revision, before the leased Worker integration.
These measurements are retained as historical evidence, not a fresh model run
on the integrated revision. Current CI separately runs the real CLI against a
synthetic Responses provider and disposable PostgreSQL.

The first integrated CI run exposed shared test-queue contamination: the Worker
also drained pending jobs left by preceding integration tests. The CLI acceptance
now reuses `isolatedQueuePostgres` for a dedicated schema, including its fresh
connection read-back and cleanup. The exact three-Role assertion is retained.

A real `gpt-5.6-luna` / **low** turn used CLIProxyAPI 8.0.31, pinned Codex CLI
0.160.0 and the existing Go acceptance test. The test PUTs owner model configuration
through the Go API, POSTs an Evolution Job, executes the three Roles through the
Engine, checks the existing output validators, and opens a **fresh PostgreSQL
connection** to read back Finding, Candidate and Review. PostgreSQL 17 ran in a
separate disposable test database, not the existing application database.

| Role | Engine duration | Requests | Input tokens | Output tokens |
| --- | ---: | ---: | ---: | ---: |
| Inspector | 40.394 s | 1 | 6,970 | 223 |
| Evolution | 7.908 s | 1 | 6,775 | 540 |
| Reviewer | 5.994 s | 1 | 7,582 | 183 |
| Total | 54.296 s | 3 | 21,327 | 946 |

The complete acceptance test took 54.432 s. Job:
`evolution-job-1793037649534-fa5c6b4942fda8a5`. Candidate `agent_md`, review `pass`,
all stages `completed`. The fixture uses retained incomplete-input evidence;
this proves infrastructure/contract compatibility, not candidate effectiveness.
Machine-readable, credential-free measurements: [M0_REAL_MODEL_RESULT.json](M0_REAL_MODEL_RESULT.json).

An earlier run encountered a local proxy process exiting, with a delayed
Inspector and failed Evolution. It is excluded from the successful measurements.
The proxy was restarted in a detached process group before the successful run.
Neither those external setup files nor auth material are part of this PR.
No provider key, raw Trace, prompt or generated asset is included in this report.

## Timeout, cleanup and capacity

- Previous hardcoded budget was 120 s. It is now `CATENA_ENGINE_TURN_TIMEOUT`
  (default 5m, maximum 15m). The successful sample's longest stage was 40.394 s:
  it alone does not prove 120 s is insufficient. The larger configurable default
  provides operational allowance for CLI startup/provider stalls; operators can
  retain `2m` when they prefer earlier failure. It is not a throughput claim.
- Go forwards the exact run budget and retains a 10 s transport/cleanup allowance.
  Go tests verify a delayed Engine timeout is preserved through leased Worker
  failure persistence, later stages remain queued, and stage HTTP contexts inherit
  Worker cancellation and the earlier job deadline.
- The real CLI + synthetic Responses acceptance also stalls an SSE stream:
  Engine returns its own `timeout` before the Go transport deadline, and the
  provider observes connection cancellation. The local acceptance passed in
  13.497 s, including three completed turns, unsupported endpoint and stalled SSE.
- Python process tests force a SIGTERM-resistant parent, spawn a child, exercise
  coroutine cancellation and excessive output, and check process-group death and
  temporary-directory removal. HTTP timeout also verifies capacity is released
  only after cleanup. No `catena-turn-*` directories remained after local acceptance.
- Engine admission defaults to one turn, immediately rejecting excess turns with
  429. Tests keep a slow analysis active while checking health and Harbor bridge
  queries, then verify slot reuse. This prevents parallel analysis CLI launches;
  it does not add a queue or change Harbor logic.
- Compose Engine limits: 1 CPU, 1GiB RAM, 128 PIDs, default concurrency 1, intended
  for a 2-core / 8GB shared host. Compose configuration was validated. The live
  local model test used the host Engine; CI builds and tests the pinned image.
  These are configured limits, not measured worst-case resource guarantees.

## Compatibility and residual risks

- The three `ROLES` instructions and JSON transport schemas are unchanged. Codex
  tests cover exact developer-instruction mapping, final-message envelopes,
  isolated homes/credentials and the default HTTP path. SDK rollback tests cover
  all three instructions/envelopes. `CATENA_ENGINE_BACKEND=sdk` remains explicit;
  a failed Codex call never falls back to another harness.
- Client disconnect cancellation is still unsupported (`cancellation=false`).
  A disconnected call can occupy its slot until the run deadline. Three stages
  can consume up to about 15 minutes at the default budget.
- Engine admission is per process. The existing PostgreSQL queue now supplies
  leased Worker recovery and bounded retry of HTTP 429/5xx, preserving committed
  stage checkpoints and fencing writes. Tests cover both 429 and 503 recovery.
  Default Go Worker count is 1, matching the default Engine slot; the per-attempt
  deadline is 20m, covering the three 5m stages and cleanup/persistence. A custom
  shorter job deadline takes precedence; increase it when increasing stage limits.
- Go cancels stage HTTP requests on Worker cancellation, shutdown and lease loss.
  The synchronous Engine still does not cancel a CLI on client disconnect: it can
  run until its own budget and cause temporary 429 responses during recovery.
  Interrupted uncommitted stages can execute again, incurring duplicate model
  costs. Fencing guarantees apply to job writes, not exactly-once model execution.
- Engine CPU/memory/PID limits do not constrain separate Harbor workers, Postgres
  or ClickHouse. Host-wide overload can still affect HTTP latency; an OOM can
  become a failed turn/transport error. No full-host production load test was run.
- Owner-supplied Base URLs remain an outbound-network/SSRF concern. Read-only CLI
  configuration is not a complete network isolation boundary. Keep the Engine
  private, use real service tokens and enforce deployment egress policy.
- Keys are still present in process environments for provider access. Same-user
  host processes/admins remain inside the trust boundary. CI uses a synthetic
  provider; live-provider availability/cost is not tested on every PR.
- SDK worker version is intentionally older. Keep the two environments separate
  and revalidate both dependency sets when upgrading Harbor/LiteLLM/SDK.

The PR description lists current changes and check results. Merge requires
passing the integrated revision checks; historical green runs are not sufficient.
