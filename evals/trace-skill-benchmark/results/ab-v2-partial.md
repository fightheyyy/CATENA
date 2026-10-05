# Trace-derived Skill A/B pilot results (incomplete)

This is a provisional report. The planned 168-rollout acceptance run was interrupted by the GPT-5.5 proxy usage limit. Only rows with complete Harbor, Codex, verifier, trajectory, and token evidence are included in the effect summaries.

| Item | Value |
|---|---:|
| Planned rollouts | 168 |
| Valid rollouts | 136 |
| Missing / invalid rollouts | 32 |
| Exact paired valid rollouts | 66 |

## Available-row A/B result

| Metric | Without | With |
|---|---:|---:|
| Verifier passes | 63/69 (91.3%) | 64/67 (95.5%) |
| Functional passes | 64/69 | 64/67 |
| Recurrence error calls | 17 | 21 |
| Failed tool calls | 87 | 90 |
| Native tool calls | 541 | 624 |
| Synthetic secret violations | 1 | 0 |
| Input tokens | 3,532,131 | 3,779,906 |
| Output tokens | 60,357 | 74,589 |
| Agent seconds | 3818.8 | 4020.9 |

On available rows, verifier pass rate is 63/69 (91.3%) → 64/67 (95.5%), a descriptive difference of +4.22 percentage points. Exact paired rows are 60/66 → 63/66.

The result is not a complete acceptance result. It must not be described as a 168-rollout benchmark until the missing rows are rerun after the proxy quota resets. The missing rows are excluded, not counted as failures.

## Resume wording supported by this pilot

从 3,021 条真实 Agent Trace 中核对重复操作故障，构建 7 项操作 Skill 与 28 个 Harbor 评测任务；使用真实 Codex 完成 136 个有效 with/without rollout，独立 verifier 通过率在有效样本上由 91.3% 提升至 95.5%（+4.22 个百分点），同时记录逐次工具调用、token、耗时和安全检查结果。

Scope: Harbor 0.23.0 native Codex CLI 0.160.0, GPT-5.5, Linux Docker / PowerShell 7 adaptation. The tasks are anonymous reconstructions, mutations, and transfer cases; they are not strict historical held-out tasks or original Windows replays. The current 32 missing rows were blocked by the local proxy's provider usage limit, so the effect estimate is provisional.

Machine-readable report: `ab-v2-partial.json`. Oracle gate: 28/28 reference trials passed. Frozen suite digest: `6323a5428e53e90ad3e3f4e133374c78549aad50c190682b7b7fa63307822689`.
