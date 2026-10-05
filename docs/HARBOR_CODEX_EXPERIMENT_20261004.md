# Catena → Harbor → 真实 Codex 的首次完整实验

日期：2026-10-04。

## 实际运行的对象

- Catena 使用已有的 Agents SDK 协调器调用 Harbor 工具，读取并总结实验结果。
- Harbor 0.23.0 使用内置 `codex` adapter，安装并运行 Codex CLI 0.160.0。
- 被测 Codex 使用用户的 CLIProxyAPI GPT-5.5，reasoning effort 为 low，关闭 web search。
- Docker 中运行 Linux / PowerShell 7，Agent 使用非 root 用户，仓库 fixture 只读。
- `rg` 被有意移除，包括 Codex 自带的副本。六个 verifier 环境探针均确认 `rg_absent=true` 与 CLI 版本。
- 本次没有评测 Claude Code，也没有把此前自建 SDK 搜索 Agent 的结果计入。

实验页面：<http://127.0.0.1:5570/cases?experiment=experiment-1791086601354-b6a071d2048594fe>。

Harbor job：`24563438f90e4959bd2a5dc7f1fb1928`。

## 对照方式

三个匿名重建任务：查找命令实现、确认启用角色配置、确认不存在的符号。每题每组运行一次，共六次。

- baseline：不加载搜索 Skill。
- candidate：由 Harbor 安装 `powershell-search-fallback` Skill，并显式要求使用。

各组使用同一任务、模型配置和独立 verifier。正确答案只在 verifier 中；Agent 完成后才从 CLI 输出提取答案。任务文件的 SHA256 在运行前后保持一致。

原生 session、命令输出、最终答案和独立 reward 已交叉核对。报告导出程序也核对了 Codex CLI 的 `turn.completed` usage 与 Harbor token 数。

## 真实结果

| 指标 | baseline | Skill |
| --- | ---: | ---: |
| 有效试验 | 3/3 | 3/3 |
| 答案通过 | 3/3 | 3/3 |
| 缺失 rg 错误 | 0 | 0 |
| 原生命令调用 | 14 | 14 |
| 输入 token | 125,117 | 128,764 |
| 其中缓存输入 token | 89,088 | 95,744 |
| 输出 token | 2,730 | 2,483 |
| 累计 Agent 执行秒数 | 148.085 | 127.013 |

**本次没有证明 Skill 改善成功率或减少 rg 缺失错误。** 新版 Codex 的 baseline 已自行使用 PowerShell 搜索，没有触发缺失 rg 的错误。

Skill 组观察到累计执行时间少 21.071 秒，输入与输出合计 token 多 3,400。每组仅三次，缓存情况也不同，不能把这个时间差解释为稳定的性能收益或成本节省。

两组均存在 PowerShell 引号或变量展开问题，并最终恢复。非零退出码也可能来自 `Get-Command rg` 的可用性探测，不能把所有非零退出码都计作错误。

## 无效的先前尝试

Codex CLI 0.118.0 的尝试被模型服务拒绝，明确要求升级 CLI。该轮在 Catena 保留为无效记录：`experiment-1791086094721-130b14d16cc12127`。它属于版本兼容故障，没有计入这张结果表。

## 已经完成和仍未证明的部分

已经跑通：Catena 页面发起实验、Harbor 驱动真实 Codex、安装 Skill、独立验证、回收命令与 token 数据、平台 Agent 总结、页面查看原始日志和结果。

这三个 Linux fixture 不是原 Windows 会话重放。显式加载 Skill 的实验也没有证明自动召回的价值。本次未比较历史记忆组。

下一步应从真实 Trace 找到当前 Codex 仍会重复犯的问题，再构造有区分度的 Case。不要为取得漂亮数字而修改 baseline 或混入此前 SDK Agent 的结果。

## 可复查材料

- 公开匿名报告：`evals/rg-missing-search/results/codex-gpt55-20261004.json`。
- 完整 Harbor 产物：`.local/harbor-study/runs/24563438f90e4959bd2a5dc7f1fb1928/`。
- 导出：在 engine 的 Python 环境中运行 `python -m catena_engine.publish_codex_study 24563438f90e4959bd2a5dc7f1fb1928`。
- 新环境先构建 `deploy/catena-mvp1/Dockerfile.codex-eval`，镜像名为 `catena/codex-eval:0.160.0`，再启动本地 Harbor worker。

Token 数为被测 Codex 的 usage，输入包含缓存输入；没有换算金额。耗时不包括容器构建或平台协调器开销。
