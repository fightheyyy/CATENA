# 第一个真实问题：PowerShell 搜索缺少 rg

## 结论

重复问题真实存在，Case 可复现。通过本地 CLIProxyAPI 的 GPT-5.5，
OpenAI Agents SDK 已完成 12 次有效对照。显式加载 Skill 后，缺失 rg 的
错误调用从 4 次降到 0 次；两组答案均为 6/6，未观察到成功率提升。

| 阶段 | 结果 |
| --- | --- |
| 原始证据 | 5 个独立 Codex 会话，5 次 CommandNotFoundException |
| 历史上下文核对 | 随后可见原生 PowerShell 搜索；不能认定任务整体失败 |
| 冻结 Case | 源码查找、配置查找、无匹配负对照，共 3 题 |
| 受控工具重放 | 原始搜索形式 3/3 复现错误，替代搜索 3/3 符合预期 |
| Codex 模型 pilot | baseline/candidate 各 1 次，有效试验数为 0 |
| 无效原因 | CLI 仍可运行 rg、PowerShell 策略拒绝；candidate 超时 |
| SDK 有效对照 | 3 题 × 2 次重复 × baseline/candidate，共 12 次有效试验 |
| 答案通过 | baseline 6/6；candidate 6/6 |
| 缺失 rg 错误调用 | baseline 4 次；candidate 0 次 |
| 工具调用 | baseline 25 次；candidate 26 次 |
| 输入令牌 | baseline 19,887；candidate 25,741（增加 29.4%） |
| 发布决定 | 保留环境特定候选，不自动安装或宣传总体成功率提升 |

fixture 是从真实失败模式抽象的匿名化任务，不是原始仓库完整重放。
确定性工具重放不能替代模型对照。报告中的 answer pass 和实验有效性分开记录。

## 本次落地

同日另完成 [平台 Agent 调用 Harbor 的容器实验](HARBOR_EXPERIMENT_20261003.md)：
这是 PowerShell 7/Linux 适配，6 次有效 Trial，两组均 3/3，缺失 rg 错误 2→0。
与上面的 Windows 12 次 SDK 对照分别记录；均没有证明总体任务成功率提升。

- [Case 与复跑方式](../evals/rg-missing-search/CASE.md)
- [冻结 manifest](../evals/rg-missing-search/manifest.json)
- [候选 Skill](../evals/rg-missing-search/skill/SKILL.md)
- [精确答案 verifier](../evals/rg-missing-search/verify.py)
- [环境机制检查](../evals/rg-missing-search/check_environment.py)

实验入口补充同一工具 PATH、输出 schema、verifier 摘要、失败调用、耗时、令牌、
策略拒绝与 rg 环境矛盾检查；修复 Windows 超时后子进程仍存活的问题。
8 项相关测试通过，Skill 结构校验通过。原始私人会话和模型轨迹仅保存在 `.local/`。

## 已完成的模型接入与实验

本地 CLIProxyAPI 已启动，GPT-5.5 可用；Catena 当前账户的模型配置已保存，
`configured=true`，现有分析引擎实际调用通过。密钥没有写入公开结果。

新增 SDK 实验入口与受控只读 PowerShell 工具，确认模型实际调用环境缺少 rg，
冻结题目、Skill、fixture 和 verifier；交替运行两组，独立 verifier 判定答案。
SDK 工具边界相关 7 项测试通过。首轮因 fixture 被边界测试意外写入而作废，
修复后在干净 fixture 上重新完成全部 12 次试验。

[公开结果](../evals/rg-missing-search/results/sdk-gpt55-20261003.json)
保留逐次指标和摘要；原始模型轨迹只保存在 `.local/`。
这里的 Skill 是直接加入 SDK 指令的显式加载，不是自动发现或 Codex CLI
接入实验。三题的小型重构实验只支持“减少此环境的错误调用”，不能推断
真实历史任务成功率、总体误报率或统计显著性；没有观察到工具调用或令牌节省。

运行时采用决定已同步：OpenAI Agents SDK 为 Catena 默认运行时，
E2B/Harbor 仍是待接入验收的执行与评测方案。
