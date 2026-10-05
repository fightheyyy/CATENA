# 主动助手：当前实现与技术路线

更新于 2026-10-03。当前运行时为 OpenAI Agents SDK；Harbor 限定 Case 链路通过，E2B 待验收。

同日新增限定 Case 的私有 `/v1/experiment` 工具循环，已完成
[平台 Agent → Harbor → Docker → verifier 实验](HARBOR_EXPERIMENT_20261003.md)。
它尚未接到此页面或公开 Go API；原有三个分析角色仍为单次回答。E2B 待验收。

## 产品方向

Catena 的主动助手从已接入的 Agent Trace 发现值得核对的线索，让用户指定一个持续关注的目标，并把分析产出连回原始证据。它与 Catena 的“Trace → 可验证改进”主线共用数据和产出，不引入第二套输入协议。Trace 入口仍是 OTLP。

参考产品的启发：Meta [Muse](https://about.fb.com/news/2026/09/introducing-muse-personal-ai-agent/) 和 OpenAI [Dots](https://openai.com/index/introducing-dots/) 把长期目标与持续工作做成用户可见的产品能力；[Today](https://today.ai/articles/blog/what-is-today) 强调记忆、主动提醒和执行前确认。Catena 不把这些产品的功能描述等同于已经实现的能力。

## 之前选定的技术栈

详细决策在 [TECHNOLOGY_ADOPTION.md](specs/TECHNOLOGY_ADOPTION.md)。这里沿用同一架构，不另造通用 Agent 运行时：

| 能力 | 选型 | 在主动助手中的作用 |
| --- | --- | --- |
| 观察输入 | OTLP/Tap → ClickHouse | 收集 Codex、Claude Code 等 Agent 的运行证据 |
| 目标、游标、任务、预算 | Catena Go 服务 → Postgres | 保存持续目标和通知状态，保证幂等与租户隔离 |
| 分析与候选生成 | OpenAI Agents SDK（Python），沿现有 engine 扩展 | 读取证据、形成线索和 Skill 候选；SDK 会话/恢复能力不能代替 Catena 的后台调度器 |
| 记忆 | GauzMem | 按来源召回确认过的个人/工程经验 |
| 沙箱与评测 | E2B 按需；Harbor + Barena | 验证候选改进，不把模型建议直接当作成功结果 |
| 展示 | React | 展示目标、信号、证据、分析结果及审批入口 |

## 当前页面的真实能力

- `/assistant` 汇总近 7 天 Trace、含错误状态的线索及已有分析任务。错误状态仅用于提醒核对，不能直接认定为任务失败。
- 用户可输入关注目标、选择已接入的 Agent，并用现有 Evolution Job API 发起一次分析。分析结果在“产出”中查看。
- 目标和选择暂存于当前浏览器、按账户区分。选择“页面打开时关注新经历”后，页面可见期间每分钟检查一次新 Trace 并刷新列表。
- 这还不是后台持续运行的私人 Agent：关闭页面后不会监测、发通知或自行执行外部动作。面向 Trace 查证的 SDK 工具、持久目标、调度、GauzMem 主动召回及审批工作流仍需分阶段接入并验收。

## 下一阶段验收

1. 在 Postgres 为每个租户保存持续目标、Trace 水位、检查间隔和预算；仅在用户明确开启后调度。
2. 后台任务按水位只处理新增 Trace，建立可复查的线索收件箱；重复线索合并，低置信度或证据不足时明确保留“不确定”。
3. Agents SDK 用受控只读工具分析证据；外部写入、消息、Skill 发布均需显式确认。后台 worker 上报异步状态，Catena 负责幂等、重试与业务状态。
4. 用人工标注集测误报，用保留回归案例测 Skill 上线前后的真实效果，再决定是否增加自然语言对话和更广泛的主动能力。
