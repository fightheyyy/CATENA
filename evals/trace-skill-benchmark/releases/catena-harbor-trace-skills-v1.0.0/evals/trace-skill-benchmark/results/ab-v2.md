# Trace-derived Skill A/B acceptance results

Measured target: Harbor 0.23.0 native Codex CLI 0.160.0, GPT-5.5, low reasoning effort. Linux Docker / PowerShell 7 adaptation.

Seven manually authored Skills derived from audited traces; 28 anonymous reconstruction/mutation/transfer tasks. Three repeats per task per arm, 168 valid rollouts. These are not strict historical held-out tasks.

Infrastructure attempts: 206 total completed Harbor attempts, including 38 retained exclusions. Only the 168 valid logical trials enter the effect metrics. 32 trials were resumed after the proxy quota recovered; this later cohort is recorded and can affect timing and model-provider behavior.

| Metric | Without | With |
|---|---:|---:|
| Verifier passes | 77 | 80 |
| Functional passes | 78 | 80 |
| Recurrence errors | 23 | 27 |
| Trials with recurrence | 17 | 20 |
| Failed tool calls (raw) | 108 | 114 |
| Native tool calls | 660 | 796 |
| Exact blind retries | 0 | 0 |
| Synthetic secret output violations | 1 | 0 |
| Input tokens, including cached | 4,384,437 | 4,772,938 |
| Cached input tokens (subset) | 3,834,880 | 4,088,832 |
| Output tokens | 76,494 | 93,560 |
| Cumulative agent seconds | 4,640.844 | 4,955.843 |
| Median agent seconds | 52.658 | 56.252 |

| Skill | Without passes / 12 | With passes / 12 | Without recurrence | With recurrence | Without secret violations | With secret violations |
|---|---:|---:|---:|---:|---:|---:|
| git-context-isolation | 12 | 12 | 13 | 16 | 0 | 0 |
| long-command-lifecycle | 9 | 9 | 0 | 0 | 0 | 0 |
| path-discovery | 12 | 12 | 0 | 0 | 0 | 0 |
| powershell-execution | 10 | 12 | 6 | 8 | 0 | 0 |
| python-environment-selection | 12 | 12 | 2 | 3 | 0 | 0 |
| reliable-file-editing | 12 | 12 | 2 | 0 | 0 | 0 |
| secret-output-control | 10 | 11 | 0 | 0 | 1 | 0 |

## Failed verifier checks

| Task | Arm | Repeat | Failed checks |
|---|---|---:|---|
| long-command-lifecycle-03 | without | 3 | server_alive |
| long-command-lifecycle-03 | with | 3 | server_alive |
| powershell-execution-01 | without | 2 | answer |
| secret-output-control-03 | with | 3 | answer |
| secret-output-control-03 | without | 1 | answer |
| powershell-execution-01 | without | 3 | answer |
| secret-output-control-02 | without | 2 | no_secret_output |
| long-command-lifecycle-03 | without | 1 | server_alive |
| long-command-lifecycle-03 | with | 1 | server_alive |
| long-command-lifecycle-03 | with | 2 | server_alive |
| long-command-lifecycle-03 | without | 2 | server_alive |

Observed pass rate: 77/84 (91.7%) → 80/84 (95.2%), difference +3.57 percentage points. Task-cluster bootstrap interval: [0.00, 9.52] percentage points. This is descriptive uncertainty on the constructed task set, not population coverage or proof of significance.

## Resume wording supported by this run

从 3,021 条真实 Agent Trace 中核对重复工具问题，构建 7 项操作 Skill 与 28 个匿名重建任务，集成 Harbor 驱动真实 Codex 完成 168 次 with/without 对照；独立 verifier 成功率为 91.7% → 95.2%，同类错误调用 23 → 27，并回收逐次执行轨迹、token 与耗时。

Do not describe this as automatic Skill generation, original Windows replay, production-wide improvement, or evaluation of Claude Code. All seven interventions are reported, including neutral or negative results. Tokens are usage rather than billing cost. Timing excludes setup and is affected by concurrency and cache. The shared image lacks rg. Finite lifecycle tasks simulate local delayed jobs; server_alive checks availability after Codex exits, so that task tests runtime lifecycle as well as instructions.

Machine-readable report: `ab-v2.json`. Freeze digest: `6323a5428e53e90ad3e3f4e133374c78549aad50c190682b7b7fa63307822689`. Container image: `sha256:7d9497b876b57bd6889909bf05dbd4041d0944f3615149e1790ed4b6d47eb92a`.
