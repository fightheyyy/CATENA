# 历史记忆与 Skill 的三组对照

## 实验设计

本实验沿用 [冻结 Case](../evals/rg-missing-search/CASE.md) 的三个匿名化搜索任务。
源问题来自五个独立 Codex 历史会话：PowerShell 搜索调用缺少 `rg`，随后可见原生搜索恢复。
这说明重复工具错误，不说明整个任务失败。

| 组别 | 唯一干预 |
| --- | --- |
| baseline | 不加载历史记忆，也不加载 Skill |
| memory | 加载 OpenViking 召回并冻结的历史观察 |
| candidate | 显式加载冻结的 PowerShell 搜索 Skill，不加载记忆 |

每组每题运行两次，共 18 个 Trial。三组使用同一个 GPT-5.5 代理配置、相同工具、任务和独立 verifier。
第一轮按 baseline → memory → candidate 运行，第二轮反转组别顺序。Harbor 管理每个独立 Docker 环境；Trial 串行执行。
每次 setup 实际核对环境缺少 `rg`。fixture 只读，目标 Agent 不能通过工具读取 verifier。

被测对象是 SDK 搜索 Agent。环境是 Linux / PowerShell 7 对历史问题的适配，不能将结果解释为原 Windows 会话重放，
也不能解释为 Codex CLI 或 Claude Code 的直接评测。

## 记忆来源与冻结

为了单独测量召回上下文的作用，本次使用**人工核对的历史观察**。这条观察通过 Catena 的添加记忆接口写入 OpenViking，
使用本地 BGE-small-zh 索引。提交实验时，Go 用当前账户身份执行语义检索，只取分数不低于 0.35 的首条记录，
对内容做长度限制和敏感信息遮盖，保存快照。三题都使用同一快照，第二轮不重新检索。

观察记录缺失命令、原生 PowerShell 搜索恢复和来源证据，不含任务答案，也不包含 Skill 的明确操作步骤。
模型收到的是历史参考信息，并被要求判断它是否适用于当前环境。这不是一次自动批量 Trace 提炼实验。

检索查询：`PowerShell 搜索缺少 rg 历史错误恢复`。
本次首条命中分数约 0.900。记忆内容与哈希保存在机器可读报告中。

## 执行与查看

实验从 Catena 的「实验」页按钮发起。Go 保存账户隔离的实验记录，私有 engine 向 Harbor worker 提交执行。
平台 Agent 调用 Harbor 工具读取同一个任务，并等待结果、生成补充总结。模型总结不能改变 verifier 判定。

- 实验 ID：`experiment-1791050477860-262d7dc8cf9d5236`
- Harbor Job：`9a0c98cf08774647b932ba40d3216be7`
- 页面：`http://127.0.0.1:5570/cases?experiment=experiment-1791050477860-262d7dc8cf9d5236`

页面支持三组指标、冻结记忆、逐次奖励与耗时、工具命令和最终答案查看。
来源 Trace 入口仅在当前账户能读取该 Trace 时出现。执行日志接口先核对账户归属，再读取私有 worker 产物。
浏览器日志做长度限制和敏感信息遮盖。完整 Harbor 产物保存在 `.local/harbor-study/runs/<job_id>/`。

## 指标解释

- 答案成功由独立 verifier 精确判定。
- 缺失 `rg` 错误由保存的实际工具输出重新计数，同时检查 stdout 和 stderr。
- 运行异常、策略拒绝或缺失 verifier 奖励使 Trial 无效，不计为成功。
- 完成状态要求 18 个有效且无重复的任务 / 组别 / 重复次数组合，以及任务文件哈希不变。
- 执行耗时包括目标模型调用和搜索工具，不包括容器构建、记忆检索或协调 Agent。
- 输入和输出 token 为目标 Agent 用量。它们不是账单金额，也不包含协调 Agent 的用量。
- 三题、每组每题两次，只能支持环境专项小样本观察，不证明统计显著性或总体成功率改善。

导出器会再次核对任务哈希、实际 reward 文件和工具调用数量，然后生成
[机器可读报告](../evals/rg-missing-search/results/memory-skill-gpt55-20261004.json)：

```powershell
cd engine
./.venv/Scripts/python.exe -m catena_engine.publish_memory_study 9a0c98cf08774647b932ba40d3216be7
```

## 结果

18 个 Trial 全部有效，任务文件哈希保持不变，实际 verifier reward 和工具调用数量核对通过。

| 指标 | baseline | 仅记忆 | 仅 Skill |
| --- | ---: | ---: | ---: |
| 有效 Trial | 6/6 | 6/6 | 6/6 |
| 答案通过 | 6/6 | 6/6 | 6/6 |
| 缺失 rg 错误调用 | 4 | 3 | 0 |
| 工具调用 | 22 | 25 | 21 |
| 输入 token | 17,432 | 21,644 | 20,272 |
| 输出 token | 1,309 | 1,469 | 1,316 |
| 总 token | 18,741 | 23,113 | 21,588 |
| 累计执行秒数 | 91.686 | 96.079 | 94.703 |

本小样本中，Skill 避免了四次缺失工具错误，总 token 增加约 15.2%，累计执行时间略长。
历史记忆组仍出现三次错误，总 token 增加约 23.3%。一次错误的差异不足以证明记忆改善稳定。
三组答案全部通过，没有成功率改善；耗时也没有改善。不将错误减少包装为费用节省或普遍能力提升。

据此继续保留环境特定 Skill，不默认给所有 Agent 注入。下一轮应扩大任务难度、环境差异和重复次数，
同时加入 `rg` 可用的负对照，核对检查工具可用性的额外成本。当前结果不足以支持批量提炼所有历史 Trace。

## 接入验收

- 平台 Agent 经 Harbor 工具取回结果，已生成与 verifier 一致的三组总结。
- core、engine 和 worker 更新重启后，18 个 Trial、完成状态和总结仍能读取，没有重跑 Trial。
- 页面实际点击发起实验、读取工具日志和最终答案通过。浏览器控制台无错误或警告；390px 宽度无页面横向溢出。
- Go 实验测试、Python bridge / 实验运行时测试、前端实验指标测试、类型检查及构建通过。
- 测试覆盖无效/重复 Trial 拒绝、跨账户日志拒绝、未知 Trial 和产物路径越界拒绝。
