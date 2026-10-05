# Trace → Skill → Harbor A/B benchmark

Seven scoped Skills, 28 anonymous tasks, native Harbor Codex adapter, three repeats per arm: 168 planned model rollouts.

The task families come from the [Trace audit](../../docs/TRACE_SKILL_AUDIT_20261004.md). Skills are manually written from those observations. The tasks are anonymous reconstructions and synthetic variants; they are **not** strict historical held-out tasks. All audited traces were available during design. The four task types per family are two reconstructions, one mutation, and one transfer fixture with a changed project layout or workflow.

| Skill | Tasks | Observable target |
|---|---:|---|
| powershell-execution | 4 | Correct data processing with PowerShell; parser failures |
| path-discovery | 4 | Active configuration discovery; missing-path calls |
| reliable-file-editing | 4 | Correct scoped edits, permissions and unrelated content preserved |
| secret-output-control | 4 | Useful audit result with no synthetic secret in tool output or answer |
| python-environment-selection | 4 | Existing or offline project runtime selected; actual dependency failures |
| git-context-isolation | 4 | Correct repository, dirty-work preservation and isolated worktree |
| long-command-lifecycle | 4 | Final result, single launch, or live verified service |

## Execution and intervention

- Target: Harbor 0.23.0 built-in Codex adapter / Codex CLI 0.160.0 / proxy GPT-5.5, low reasoning effort, web search disabled.
- Environment: isolated Linux Docker containers, PowerShell 7, Python venv, Git and Node. This adapts the Windows failure mechanisms; it is not an original Windows trace replay.
- The reused base image removes `rg`, including Codex's bundled copy. Both arms have this same limitation. Treat tool-error and efficiency results as specific to this environment rather than a standard fully provisioned coding environment.
- Without: no benchmark Skill installed.
- With: only the corresponding Skill installed, explicitly invoked by name. Task, image, model and verifier are identical.
- Fresh container and Codex home per trial. Pair order alternates by task and repeat; task pairs are shuffled with seed 431558. Parallel execution reduces runtime but can affect timing measurements.
- No external packages or task services are required. Credential fixtures use synthetic canaries; actual model credentials stay in local runner state and are not copied into task fixtures.
- `frozen-sha256.json` checks all materialized task and Skill files before and after execution. Do not edit cases after observing A/B outcomes; version a revised suite instead.

Harbor uploads `tests/` after the agent phase. The verifier checks exact structured answers, changed files, preservation hashes, modes, Git state, execution evidence, service readiness, and synthetic secret output as appropriate. Verifier success determines pass rate. A nonzero command exit alone does not determine task failure.

First run the reference solutions through Harbor. Their rewards validate task solvability and verifier acceptance; these are not model rollouts and do not enter A/B metrics.

```powershell
docker build -t catena/trace-skill-eval:1 -f evals/trace-skill-benchmark/Dockerfile.base evals/trace-skill-benchmark
$env:PYTHONUTF8='1'
$env:PYTHONIOENCODING='utf-8'
engine/.venv/Scripts/python.exe evals/trace-skill-benchmark/run_study.py --mode oracle --run oracle-v2 --repeats 1 --concurrency 4
engine/.venv/Scripts/python.exe evals/trace-skill-benchmark/run_study.py --mode ab --run ab-v1 --repeats 3 --concurrency 4
```

The default local proxy endpoint is port 8317. The runner privately reads the existing CLIProxyAPI client-key file. `CATENA_EVAL_KEY_FILE` can override its path. Resume with exactly the same run name and selection; completed trials are retained. Invalid trials are recorded, never silently discarded or converted to passes.

## Metrics and limits

Metrics include verifier pass rate, functional pass rate, error recurrence, failed tool calls, exact blind retries, total native tool calls, synthetic secret output, input/output/cached token counts, and agent execution duration. Input tokens already include cached tokens; do not add cache counts again. Tokens are usage, not money. Setup/build time is outside agent execution duration.

Error recurrence is family-specific output matching. It is an operational signal, not a root-cause classifier. Python diagnostic calls with observed exit code zero are excluded. Failed tool calls are a raw count and can include expected probes or negative searches. Blind retries compare consecutive tool arguments exactly; retries that change arguments are not counted. Skill-read observations only measure visible file reads, not every possible loading channel.

Three repeats characterize variability on these tasks. Four tasks per family do not support broad claims about all coding agents or statistical significance. The finite lifecycle fixtures simulate delayed local jobs; they are not real network package installations or training runs. Report neutral or negative results as well as improvements. A resume is artifact reuse, not a fresh independent replication.

Public sanitized results go in `results/`; complete native sessions, model connection setup and Harbor artifacts remain in `.local/trace-skill-benchmark/`.
