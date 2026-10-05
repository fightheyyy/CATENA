# 编程 Agent 失败线索研究

状态：全量本机扫描完成；首个真实问题已有匿名 fixture 与 Harbor/Codex 评测，但最新适配环境中未观察到 Skill 收益；总体人工标签仍未完成。最后运行：2026-10-04。

## 2026-10-04：全量会话扫描与 Case 集

按不区分账户的口径扫描本机 Codex 与 Claude Code 历史：300 个源文件、约 2,216 MiB，298 个会话、2,588 个 Turn，0 个跳过来源。33 个 Turn 命中待核对信号（1.27%）；这是候选线索率，不是失败率。对这些 Turn 的信号节点完成分类后，发现 8 个 Turn、7 个会话含有 `rg` 不可用错误输出匹配，共 9 次调用。匹配是自动化结果，逐条人工确认仍待完成。

另随机抽取 30 个命中 Turn 和 30 个未命中 Turn 做审核队列；当前标签 **0/60**，因此不能报告准确率、误报率或召回率。行级来源与 Trace ID 只保存在 `.local/failure-study/`。

Case 集目录见 [agent-case-set](../evals/agent-case-set/README.md) 和 [结构化清单](../evals/agent-case-set/cases.json)。现有 `rg` fixture 有 3 个独立验证任务。最新 Harbor + Codex GPT-5.5 实验 baseline 与 Skill 均 3/3 通过，缺失 `rg` 错误均为 0；实验没有显示处理效应。该结果可用于说明实验链路和当前无收益观察，不能宣称 Skill 提升成功率。

## 2026-10-03：真实重复问题与环境验收

从本机原生 Codex function_call_output 中确认了 5 个独立会话、5 次缺失 rg
错误，后续调用可见 PowerShell 原生查找。它们是重复的工具环境问题，不能据此
断定整项任务失败。扫描跳过大于 32 MiB 的文件，不报告总体发生率。

已冻结 [rg-missing-search Case](../evals/rg-missing-search/CASE.md)：三个匿名化
任务分别检查源码定位、角色配置和无匹配负对照；精确 JSON 答案由工作区外
verifier 判定。Case 来自真实错误模式，但 fixture 不是原始仓库重放。

在受控子进程 PATH 中，确定性重放 3/3 复现 `CommandNotFoundException`，
PowerShell 替代搜索 3/3 通过机制检查。这仅证明故障与替代搜索可复现，
不证明模型使用 Skill 后成功率提升。

Codex CLI 环境预跑显示其工具仍能执行 rg，且有 PowerShell 命令被策略拒绝，
与缺失 rg 的目标环境不一致，不能计入有效对照。运行器新增策略拒绝、rg 成功
调用、环境无效原因与 `valid_for_comparison`，防止把答对与实验有效性混为一谈。

本次还复核了下方 9 月 30 日的 6 个试跑轨迹：6/6 都含工具策略拒绝，1/6
实际成功调用 rg。原记录 baseline 3/3、candidate 3/3 保留为历史试跑，
不作为 Skill 有效或无效的证据。

随后接入本地 CLIProxyAPI 的 GPT-5.5，当前账户模型配置为 `configured=true`，
现有分析引擎实测通过。受控 Agents SDK 对照完成 12 次有效试验：两组均
6/6 答对，显式加载 Skill 后缺失 rg 错误调用从 4 次降到 0 次，工具调用
25→26，输入令牌 19,887→25,741（+29.4%）。只证明此重构环境减少错误调用，
不证明成功率提升或效率改善，也未验证 Codex 自动加载 Skill。
详见 [Case 结论](CASE_RG_MISSING_20261003.md) 与
[逐次结果](../evals/rg-missing-search/results/sdk-gpt55-20261003.json)。

## 问题和口径

平台 Agent 同日经私有实验入口调用 Harbor，在独立 Docker 环境完成三个 Case 的
无 Skill/有 Skill 对照。6 次有效 Trial、两组均 3/3，缺失 rg 错误 2→0；
这是 Linux/PowerShell 7 适配，不与 Windows 实验合并。
详见 [链路实验](HARBOR_EXPERIMENT_20261003.md)。

目标是从 Codex/Claude Code 的真实会话找出**值得人核对**的重复工具问题。一个 Turn 是分母；Span 和工具调用次数都不能代替 Turn。检测结果是问题假设，不代表任务失败、根因或 Skill 已修复。

当前规则只读取 Tap 的 `catena.coding_agent.event_graph.v1`：

| 信号 | 规则 | 主要局限 |
| --- | --- | --- |
| `unrecovered_tool_error` | 工具状态为 error，或 Codex 结果明确写出非零 `Exit code`；随后未观察到同一工具成功 | 其他工具可能已完成恢复；没有业务终态 |
| `repeated_failed_call` | 同一 Turn 中同一工具、相同参数摘要连续失败至少两次，中间没有其他工具步骤 | 重试仍可能合法；参数摘要不说明根因 |
| `repeated_identical_call` | 同一 Turn 中同一工具、相同参数摘要连续调用至少三次 | 某些轮询合法；常见等待工具已排除 |

输出仅保留来源 ID、节点 ID、信号与计数，不复制原始提示、参数或工具结果。输出的 `status=hypothesis` 不可作为公开故障数。

## 2026-10-03 旧版扫描（已由全量扫描取代）

旧版扫描读到 **282 个会话、1,828 个 Turn**，其中 **16 个 Turn 有至少一个待核对信号**；**15 个源文件因 32 MiB 单文件上限跳过**。这些数已被 2026-10-04 扫描取代，仅保留作规则演进记录，不应作为当前覆盖率指标。

复跑时使用已构建的 `tap/codex/plugins/tracing/dist/index.mjs`，在仓库根目录执行：

```powershell
$env:PYTHONPATH = (Resolve-Path ./tap).Path
python -m catena_tap.failure_cli scan `
  --codex-root '<本机 Codex sessions 目录>' `
  --claude-root '<本机 Claude projects 目录>' `
  --codex-bundle ./tap/codex/plugins/tracing/dist/index.mjs `
  --output ./.local/failure-study/report.json
python -m catena_tap.failure_cli sample ./.local/failure-study/report.json `
  --per-class 30 --seed 17 --output ./.local/failure-study/labels.csv
```

`sample` 从有信号、无信号两组分别随机抽取最多 30 个 Turn，固定 seed 可复现；当前抽样是全部 16 个有信号 Turn 加 30 个无信号 Turn，共 46 个。审查者从 Catena Trace 页面或原始会话核对完整上下文，逐行填 `actionable`、`normal` 或 `uncertain`。`actionable` 指有具体、可尝试修复的重复问题；单次命令非零、正常轮询、故意测试失败及上下文不足均不能自动标为 actionable。分歧应由第二人复核，先记录原标签，不覆盖历史。

完成全部抽样标签后运行：

```powershell
python -m catena_tap.failure_cli score ./.local/failure-study/report.json `
  ./.local/failure-study/labels.csv --output ./.local/failure-study/metrics.json
```

评分保留 TP/FP/TN/FN，并按两组在完整扫描中的比例加权估计误报率和召回率。存在 `uncertain` 时不输出率，避免把未知当作正常。未标注时不得报告误报率；抽样外、超限文件和解析失败的覆盖范围必须一起披露。

## 后续验证门槛

从人工确认的问题中选一个，冻结真实来源、目标 Agent/Skill 版本和可执行 Case。分别运行无 Skill 的 baseline 与仅增加该 Skill 的 candidate，使用独立 verifier、相同任务集和相同时间上限；记录每个 trial 的结果、成本、未覆盖项与回归。没有这组实验前，只能展示 Skill **候选**，不能宣传成功率提升。

### 可执行的配对实验

`tap/catena_tap/skill_experiment.py` 提供实验入口。先把人工确认的问题改写成 10–20 个可以独立验证的匿名化小任务；每个任务有干净的 `fixture` 目录、相同的任务提示和**预先写好**的验证命令。候选 Skill 是一个带 `SKILL.md` 的目录。以下清单是格式示例，不是已运行的实验：

```json
{
  "schema_version": "catena.skill_experiment.v1",
  "model": "<固定的 Codex 模型>",
  "skill_name": "example-fix",
  "skill": "skill",
  "repeats": 2,
  "agent_timeout_seconds": 900,
  "verify_timeout_seconds": 120,
  "cases": [
    {
      "id": "case-01",
      "fixture": "fixtures/case-01",
      "prompt": "修复此目录中的任务，完成后运行现有测试。",
      "verify": ["python", "-m", "unittest", "discover", "-s", "tests"]
    }
  ]
}
```

清单中的路径相对清单文件所在目录。运行器将 fixture 复制到隔离工作目录，仅在 candidate 副本内放入 `.codex/skills/example-fix/SKILL.md`，每次运行使用只复制登录凭证的临时 `CODEX_HOME`，随后用同一模型运行 `codex exec --json --ephemeral --full-auto`。基线与候选交替运行；只有 Agent 正常结束且独立验证命令返回 0 才算成功。报告记录 fixture、提示、Skill、清单的摘要，以及每次的 Agent 退出码、Turn 完成状态、验证结果、命令次数和令牌用量；每次的原始 JSONL 保留在本地目录。若要判断 Skill 是否被实际调用，还需人工检查原始 trace，不能仅凭候选成功推断。

```powershell
$env:PYTHONPATH = (Resolve-Path ./tap).Path
python -m catena_tap.skill_experiment ./.local/skill-study/manifest.json `
  --output ./.local/skill-study/run-01
```

运行前检查清单、fixture、Skill 和验证命令。原始 JSONL 和 Agent 工作目录可能含私人内容，放在 Git 忽略的 `.local/` 下；不要公开。当前尚未完成全部人工标签，因此还没有针对已确认真实失败的有效前后对照，也没有可信的 a%→b% 提升结果。实验设计参考 [OpenAI 官方的 Skill 评测方法](https://developers.openai.com/blog/eval-skills)。

### 首个 Skill 候选与小样本试跑

抽查原始工具结果时，在不同历史会话里看到 PowerShell 搜索调用因 `rg` 不可用而出错；这是一项**工具环境问题**的证据，不等于整项任务失败。基于这一点写了候选 [`powershell-search-fallback`](../evals/skills/powershell-search-fallback/SKILL.md)。它要求区分“命令不存在”和“无匹配”，并在需要时用 PowerShell 原生搜索。该候选已通过 Skill 结构校验。

为先检查实验链路，用 [`powershell-search.json`](../evals/powershell-search.json) 冻结了 3 个公开的合成只读查找任务，固定模型 `gpt-5.5`，每题每组各跑一次。结果是 **baseline 3/3、candidate 3/3**；候选每题都读取了 `SKILL.md`，总工具调用 **16→19**，累计输入令牌 **1,156,735→1,349,936**。原始运行保存在本机 `.local/skill-study/powershell-search-readonly-01/`。这是一个**没有显示收益**的小样本结果，不应写成成功率提升。

试跑中，当前 Codex CLI 的自动执行策略拒绝写文件，故先前写 `answer.txt` 的版本对两组都无法有效评分；改为只读任务后才得到上面的结果。此前 `gpt-6-luna` 被该 CLI/账号拒绝，也没有计入试跑。3 个合成用例不足以估计真实失败修复率，且实际 CLI 环境可运行 `rg`，没有复现历史中的缺失命令环境。要验证这个 Skill，需在确认的缺失 `rg` 环境和真实任务回归集重跑；若依旧不改善，应修改或淘汰候选。
