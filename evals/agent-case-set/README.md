# Agent Case 集：发现版 v0

最后更新：2026-10-04。

## 这是什么

这是从 Catena 中 Codex 与 Claude Code Trace 生成的 **跨来源 Case 集**。按你的要求，合并所有账户数据，不按账户拆分。目录包含 Case 候选、私有 Trace 审核表和可执行 fixture。错误状态和工具调用信号仍需人工核对，不能直接当成任务失败。

原始对话、代码、工具参数、工具输出和可识别的 Trace ID 保留在本机 `.local/failure-study/`，不进入这个目录。确认的问题需先改写为匿名 fixture，再添加任务与独立 verifier。

## ClickHouse 全量数据概况

- 合并后有 **93,682 个唯一 Trace+Span 事件、3,021 条 Trace、302 个 Session**。数据跨 7 个 owner ID，时间范围为 2025-10-10 至 2026-09-30。统计已合并，没有按账户拆分。
- 3,015 条 Trace 含 Agent Turn。301 条 Turn Trace 的 Span 状态是 `error`，约 10.0%。这是执行状态比例，不是任务失败率。
- 301 条 error 状态 Trace 中，56 条含 Shell 工具调用；49 条含至少 2 次 Shell 调用，共 943 个 Shell Span。17 个 Shell Span 自身标记为 error，分布在 9 条 Trace 中。多次调用不等于重复了相同命令，也不等于任务失败。
- 我从 error 状态 Trace 中抽取 60 条审核样本：候选组 30 条（每个 Turn 至少 2 次 Shell 调用），对照组 30 条（0 或 1 次）。标签目前 **0/60**。Trace ID 和链接只在本机 `.local/failure-study/clickhouse-error-turn-review-20261004.csv`。
- ClickHouse 原始输入和输出列很大。全列读取超过数据库内存上限。因此，本轮从 ClickHouse 得到的是 Span 聚合统计；文本级问题线索由本机历史解析补充。

## 本机原始会话补充扫描

- 扫描 300 个本地 JSONL 源文件，总计约 2,216 MiB；没有因文件大小跳过来源。
- 解析到 298 个会话和 2,588 个 Agent Turn：Codex 2,582，Claude Code 6。
- 33 个 Turn 触发至少一条待复核信号，占扫描 Turn 的 1.27%；其余 2,555 个 Turn 没有触发当前规则。
- 33 个候选中，28 个 Turn 的 parser 状态为 `ok`，3 个为 `aborted`，1 个为 `incomplete`，1 个为 `error`。`ok` 是回合记录状态，不代表任务答对。
- 信号组按受影响 Turn 统计：25 个 shell 工具未恢复错误、9 个重复 shell 调用、1 个重复 web search 调用、1 个 Agent 错误。信号会重叠，不能相加成唯一问题数。
- 规则命中只覆盖 15 个源文件；触发次数最常见的是 shell 工具。这里的分类描述调用记录，不证明共同根因。

## 人工审核

`.local/failure-study/case-review-queue-20261004.csv` 含 60 条固定随机种子的分层样本：30 条扫描器命中、30 条未命中。每行有来源 Trace 页面链接；标签和说明目前留空。

审核标签：

- `actionable`：有具体、重复且可尝试修复的问题。
- `normal`：正常工具行为、成功恢复、合理重试，或没有可行动问题。
- `uncertain`：Trace 不完整、上下文不足，或无法判断。

不要把未完成回合、非零工具退出码或扫描器信号直接标成任务失败。完成标签后，用 `tap/catena_tap.failure_cli score` 计算混淆矩阵及分层抽样估计；若含 `uncertain`，程序会抑制误报率等估计。

## Case 清单

结构化目录见 [`cases.json`](cases.json)。当前包括 **1 个可执行 Case（3 个任务）**和 **5 组待人工核对的候选**。其中两组来自 93,677 个 ClickHouse Span 的全量聚合。

- `powershell-missing-rg`：历史扫描命中 8 个 Turn、7 个会话，共 9 次缺失命令错误调用。证据是错误输出的确定性匹配，尚待人工逐条核对；私有行级来源在 `.local/failure-study/rg-case-sources-20261004.csv`。
- 现成 fixture 有 3 个匿名任务和独立 verifier。最近一次 Harbor + Codex GPT-5.5 对照两组均 3/3 通过，缺失 `rg` 错误均为 0。说明当前适配环境下 baseline 已避开问题；这次实验没有证明 Skill 有效。
- 另有一组 Harbor 管理的三臂研究：Agents SDK 搜索 Agent 跑了 18 次有效 Trial（baseline、记忆、Skill 各 6 次）。答案通过率均为 6/6；缺失 `rg` 错误为 4→3→0；Skill 组总 token 比 baseline 多 15.2%。这是一项 Linux / PowerShell 7 适配小样本，不能外推为 Codex CLI / Claude Code 的普遍改善或成功率提升。
- 另外三组候选来自本机原始会话扫描。各候选之间可能重叠，不应将数量相加成独立问题数。

所以目前可以量化报告的是**数据覆盖与候选发现量**，不是任务失败率或 Skill 普遍收益：93,682 个唯一 Trace+Span 事件、3,021 条 Trace；301 条 Trace 带 error 状态；49 条 error 状态 Trace 含至少 2 次 Shell 调用。两份人工审核队列共 120 条，目前标签 0/120。

## 从候选变成可运行 Case

仅把人工确认的问题改写为独立用例。每个 Case 至少需要：

1. 一条或多条来源 Trace 的私有引用和去敏说明。
2. 可公开的匿名 fixture 和固定任务描述。
3. 预先冻结的期望结果和工作区外 verifier。
4. 有区分度的 baseline/candidate 条件、重复次数及原始 trial 证据。
5. 结果指标：有效试验数、答案通过数、目标错误数、调用数、token 和耗时。

当前已有的 `evals/rg-missing-search` 是唯一可执行 fixture：3 个合成只读任务。真实 Codex 实验为两组各 3/3 答对、两组都没有 `rg` 缺失错误，因此它没有证明这个 Skill 带来收益。不要把它并入这轮扫描准确率。

## 复现扫描

从仓库根目录运行：

```powershell
$env:PYTHONPATH = (Resolve-Path ./tap).Path
python -m catena_tap.failure_cli scan `
  --codex-root "$env:USERPROFILE/.codex/sessions" `
  --claude-root "$env:USERPROFILE/.claude/projects" `
  --codex-bundle ./tap/codex/plugins/tracing/dist/index.mjs `
  --max-file-mib 300 `
  --output ./.local/failure-study/complete-all-sources-20261004.json
```

为复核样本生成标签表：

```powershell
python -m catena_tap.failure_cli sample `
  ./.local/failure-study/complete-all-sources-20261004.json `
  --per-class 30 --seed 1704 `
  --output ./.local/failure-study/case-review-queue-20261004.csv
```

本机私有报告中含来源 ID。不要提交 `.local` 文件或原始 Trace 内容。
