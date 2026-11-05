# Rebirth reproduction

Run from repository root. Secrets, raw source traces and complete Harbor native
sessions remain under `.local/rebirth/` (ignored, private). Published summaries
contain hashes and anonymous task artifacts only. No production DB is required.

## Engineering checks

```sh
python3.12 -m venv .venv
.venv/bin/python -m pip install -e 'engine[dev,rsi]'
.venv/bin/python -m pip check
(cd engine && ../.venv/bin/ruff check catena_engine tests && ../.venv/bin/python -m pytest)
(cd frontend && pnpm install --frozen-lockfile --ignore-workspace && pnpm test && pnpm build && pnpm test:browser)
# Sync frontend/dist to backend/internal/control/web before compiling Go.
(cd backend && go test -race -count=1 ./... && go vet ./...)
.venv/bin/python evals/trace-skill-benchmark/validate_verifiers.py
.venv/bin/python scripts/source_inventory.py
```

Database integration environment used here (test-only credentials, loopback ports):

```sh
docker run -d --rm --name catena-rebirth-postgres -e POSTGRES_USER=catena_test -e POSTGRES_PASSWORD=catena-rebirth-test-only -e POSTGRES_DB=catena_test -p 127.0.0.1:55439:5432 --memory=512m postgres:17-alpine
docker run -d --rm --name catena-rebirth-clickhouse -e CLICKHOUSE_PASSWORD=catena-rebirth-test-only -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 -p 127.0.0.1:18124:8123 --memory=1536m clickhouse/clickhouse-server:25.10.2.65-alpine
export BARENA_TEST_DATABASE_URL='postgres://catena_test:catena-rebirth-test-only@127.0.0.1:55439/catena_test?sslmode=disable'
export CATENA_CLICKHOUSE_TEST_DSN='http://default:catena-rebirth-test-only@127.0.0.1:18124/default?dial_timeout=5s'
export CATENA_ENGINE_TEST_PYTHON="$PWD/.venv/bin/python"
(cd backend && go test -race -count=1 -v ./... && go vet ./...)
```

These containers are disposable test services. Do not apply their credentials or
cleanup commands to existing user/production services. The normal Compose deployment
preserves volumes; [retirement/export](../../../OPENVIKING_INTEGRATION.md) applies to upgrades.

## Real source and Case

1. Use the existing Tap importer on an authorized native Codex rollout:
   `catena trace import codex /private/rollout.jsonl --otlp`. Save the original
   snapshot and its SHA256 before import. The importer emits an array of OTLP
   batches; upload each through the normal Agent-key authenticated OTLP endpoint.
2. Query the owned Trace back from CATENA. This run retained 145 actual spans.
   The original rollout, exported OTLP, retained Trace and Evidence are saved
   privately. Published source/Evidence hashes are in `real-e2e-status.json`.
3. Use the frozen task `evals/trace-skill-benchmark/tasks/path-discovery-01`.
   Operator confirmation was requested in chat; the user delegated this choice.
   It reconstructs a path-discovery failure mechanism, not the original full task.
   Manifest `confirmed_by` records that authorization, not fabricated review.
4. Register the full `task_digest` as `sha256` and
   `"artifacts": ["/workspace/case/result.json"]` in the operator manifest, with
   identical absolute task/run paths visible to Engine and Docker host. Set
   `CATENA_RSI_CASES_MANIFEST`, `CATENA_RSI_RUNS_ROOT`, persistent
   `CATENA_ENGINE_EXECUTIONS_ROOT`, and the Docker socket group. See deployment docs.

## Native Harbor environment verification

The existing frozen base is amd64. This Apple Silicon run used Docker's native
platform setting; no task or verifier was edited after seeing outcomes.

```sh
docker build --platform linux/amd64 -t catena/codex-eval:0.160.0 -f deploy/catena-mvp1/Dockerfile.codex-eval .
docker build --platform linux/amd64 -t catena/trace-skill-eval:1 -f evals/trace-skill-benchmark/Dockerfile.base evals/trace-skill-benchmark
DOCKER_DEFAULT_PLATFORM=linux/amd64 .venv/bin/harbor run --agent oracle --path evals/trace-skill-benchmark/tasks/path-discovery-01 --jobs-dir .local/rebirth/harbor --job-name oracle-path-discovery-01-amd64 --n-concurrent 1 --n-attempts 1 --quiet
```

The successful oracle reward was 1. A prior attempt without platform selection
failed before execution; it remains preserved. Oracle is a reference solution,
never an Agent model quality trial. Image IDs/architecture are in `images.json`.

## Real Harness² / Harbor experiment

Start the existing CLIProxyAPI with its existing private credentials; select
`gpt-5.6-luna` in Connections. In Docker Desktop, the Engine and target containers
used `http://host.docker.internal:8317/v1`. Both optimizer and target use the same
owner model with low effort. Never place API Keys in requests committed to Git.

The private Go endpoint was loopback `http://127.0.0.1:18787`; Engine was private
and bearer-protected. Execute the normal four-service stack or the documented
host Go + container Engine/DB equivalent. Register the Case before invoking:

```sh
.venv/bin/python evals/rebirth.py \
  --url http://127.0.0.1:18787 \
  --trace-id '<owned real Trace ID>' \
  --case-id path-discovery-01 \
  --baseline docs/refactors/evidence/rebirth/baseline-AGENTS.md \
  --output .local/rebirth/new-run
```

For authenticated deployments set `CATENA_API_TOKEN` privately. This small client
only submits/polls the public Go API, saves private request identity/results and
self-checks provenance and the frozen baseline/candidate pair. It never runs an
Agent, verifier or optimization algorithm. Its new-output-directory requirement
prevents accidental reruns over an existing experiment. On timeout inspect the
saved Job ID and Engine receipt; do not blindly submit a new paid job.

The final real execution used the same API request contract with k=1, upstream
parallel refinement, 12 optimizer-call / 8 Trial / 900 second ceilings. It made
4 optimizer calls and 6 Harbor Trials: 4 probes, then a fresh baseline/candidate
pair. No held-out Case was supplied. All six result hashes and original native
session files were checked against the persisted comparison.

Inspect `experiment-summary.json`, `candidate.json` and both candidate text files.
Record the human decision in the UI or `POST /v1/analysis-jobs/{id}/reviews`; do
not install these artifacts automatically. Restore/reopen PostgreSQL to verify
review persistence. Keep the raw Harbor run and receipt directories with the DB.

The experiment uses amd64 emulation and reuse of the optimization Case. Cost is
Harbor-reported target cost, not a provider invoice; optimizer dollar cost is
unknown. Native exec wrapper output did not preserve inner command exit codes,
so error counts remain null. Valid trials are not proof of general improvement.
