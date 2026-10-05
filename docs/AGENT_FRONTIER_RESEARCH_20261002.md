# Catena：Agent 前沿调研与缺口

调研日期：2026-10-02。依据官方文档、官方仓库、发布说明与研究论文；没有对远端产品进行账户、付费或部署验收。本文是研究建议，不代表已经采用新组件。

## 结论

建议把 Catena 定位为：**从真实 Agent 经历中积累经验，用独立实验选择改进，并把验证过的能力交回原运行时的 Agent 实验平台。**

前沿能力可以持续接入，但共享目标、任务、证据、记忆、案例、候选和实验实体。每个接入都要产生一个可观察的用户行为和可比较的结果。

一个重要竞争变化：2026-09-24 发布的 [LangSmith Engine v2](https://www.langchain.com/blog/langsmith-engine-v2-redteam) 已公开描述扫描生产 Trace、聚合问题、提出修复、复现并自动验证修复。自动验证和红队能力当时面向 Deployment 用户私测，自托管 BYOK 是后续计划。不能再以“Trace → 修复闭环”本身宣称 Catena 独有。

Catena 的差异化假设应集中在：本地历史和运行时原生证据；Codex/Claude Code 等异构目标；BYOK；可带走的 Skill/插件；可复查的独立回归实验。实际优势仍需效果、成本和用户操作时间证明。

## 当前代码核对

- OTLP/Tap、ClickHouse、Go/React、账户与 Agent 凭证、BYOK、多租户边界、候选包和来源关联已存在。
- `engine/catena_engine/runtime.py` 已使用 **OpenAI Agents SDK**，通过 `OpenAIChatCompletionsModel` 支持兼容提供方。每个角色 `max_turns=1`，没有给角色注册工具；当前是单轮角色分析，尚无长任务探索执行。
- **托管 Agents API** 与 E2B 是采用决策，未完成接入；它们与现有 Agents SDK 是不同路径。保留 BYOK 时需显式声明各 backend 的能力，不把兼容 Base URL 当作支持托管 API。
- `ProactiveWorkspace.tsx` 的目标存在 localStorage，页面可见时每分钟检查 Trace；关闭页面后的持久调度、对话、通知、审批尚未实现。
- 当前跨 Trace 分析先扫描最多 100 条，再冻结最多 12 条 Trace；Evidence 还有限制。需要分层统计和检索才能对整个语料库作结论。
- 离线问题扫描和配对实验可复跑；现有文档记录的 3 题合成试跑是 baseline 3/3、candidate 3/3，没有证明真实修复收益。失败研究截至 9 月 30 日的范围是 282 会话、1,828 Turn，不能把所有入库 Span 数当作已分析数据量。
- GauzMem 已有个人记忆接入与可选部署；工程经验的已验证结果回写、失效与发布生命周期尚需完成。

核对入口：[README](../README.md)、[技术采用决策](specs/TECHNOLOGY_ADOPTION.md)、[主动助手](PROACTIVE_ASSISTANT.md)、[失败研究](FAILURE_STUDY.md)。

## 前沿能力与适合 Catena 的接法

| 方向 | 当前官方例子 | Catena 缺口 | 可验收接入 |
| --- | --- | --- | --- |
| 长任务 harness 与恢复 | [OpenAI Agents API](https://developers.openai.com/api/docs/guides/agents-api/architecture)、[Claude Managed Agents](https://www.anthropic.com/engineering/managed-agents)、[Deep Agents runtime](https://www.langchain.com/blog/runtime-behind-production-deep-agents) | 没有可检索证据、运行案例、持续修订的工程 Agent 工具循环 | 一个 provider adapter + 只读 Evidence 工具；断线、重启后能对账恢复；保存成本与取消状态 |
| 主动性与跨渠道 | [Today](https://today.ai/articles/blog/meet-today)、[OpenClaw Heartbeat](https://docs.openclaw.ai/gateway/heartbeat) | 持久目标、增量游标、事件触发、预算、静默与收件箱 | 新 Trace 触发一个去重问题卡；页面关闭后继续；用户能暂停并看来源 |
| 记忆整理与经验复用 | [Claude Dreaming](https://claude.com/blog/new-in-claude-managed-agents)、[Hermes](https://github.com/NousResearch/hermes-agent) | 去重、冲突、过期、版本适用范围，以及确认结果到 GauzMem 的回写 | 每轮实验形成带来源的经验；新任务召回适用经验；失效后不再使用。Dreaming 是研究预览，不能当作普遍可用 API |
| Skill/插件/运行时扩展 | [Agent Skills 标准](https://agentskills.io/home)、[Claude Code mods](https://claude.com/blog/claude-code-mods)（10 月 1 日） | 草案后的版本、安装、灰度、撤销与效果监测 | 同一候选在 Codex/Claude 上按兼容范围安装；记录版本与 hash；可回退。mods 属于 Claude 特定执行扩展，需独立适配 |
| 自动优化 | [Agent Lightning v1.0.1 Skill](https://github.com/microsoft/agent-lightning/releases)、[DSPy GEPA](https://github.com/stanfordnlp/dspy/blob/main/docs/docs/diving-deeper/gepa-in-depth.md)、[CASD 论文](https://arxiv.org/abs/2609.26261) | 冻结案例、优化预算、候选搜索、保留验证集、统一比较 | 简单人工候选 vs Lightning Skill vs GEPA；记录成功率、成本、延迟。CASD 是从离线语料归纳规则的研究路线，不能替代上线前验证 |
| 多 Agent 专家协作 | [DeerFlow](https://github.com/bytedance/deer-flow)、[Claude multiagent](https://claude.com/blog/new-in-claude-managed-agents) | 当前三个角色缺少并行探索、独立工作区和任务依赖 | 解析/环境/工具专家并行调查同一问题，产物统一回到 Evidence；与单 Agent 基线比效果与成本 |
| 工具和 Agent 互操作 | [MCP Apps](https://blog.modelcontextprotocol.io/posts/2026-01-26-mcp-apps/)、[MCP Tasks](https://tasks.extensions.modelcontextprotocol.io/)、[A2A](https://a2a-protocol.org/latest/) | Catena 尚未作为外部 Agent 可用的工具服务/任务服务 | 先提供按租户读取证据、查询问题、启动实验的 MCP；再把 Diff/实验报告做成 MCP App；有远端委托场景再接 A2A |
| 浏览器/计算机执行 | [Browser Use](https://browser-use.com/developers)、E2B/Harbor 既定路线 | Trace 中的浏览器操作缺少截图/录像、语义动作及效果验证 | 复现一个真实 Web 工作流；保存动作、截图、产物与 verifier；受控工作区执行 |
| 评测与主动找问题 | [LangSmith Engine v2](https://www.langchain.com/blog/langsmith-engine-v2-redteam)、[Anthropic evals](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents) | 真实 Case 产品链、重复 trial、回归门槛、故障注入 | 固定任务集与环境，分别评估结果和过程；覆盖恢复、取消、重复执行和环境故障；生成同条件对照报告 |
| 任务级模型路由 | [LangChain router](https://www.langchain.com/blog/how-to-build-a-model-router-in-the-harness)（10 月 1 日） | 固定单模型配置，没有任务级成本/效果比较 | 简单信号分类用低成本模型，复杂复现/候选用强模型；在真实任务上比单模型与路由，记录退化和升级条件 |

注意：OTLP 是观察数据入口；MCP 是工具入口；A2A 是远端 Agent 委托；用户目标、审批、GitHub webhook 是控制输入。它们可以共存，不能把“统一 Trace 输入”解释成平台所有交互都必须通过 OTLP。

## 建议优先顺序

### P0：先形成一个后台工作到验证报告的完整体验

1. **持久主动助手**：Go/Postgres 保存 Goal、Cursor、Task、Inbox、预算；支持新 Trace 和定时触发、去重、重启恢复、暂停。页面有对话、证据卡和任务进度。
2. **真正的 Engineer 工具循环**：沿用现有 SDK 也可以；注册 `query_traces`、`read_evidence`、`search_experience` 等受控工具，按需检索和分页，避免一次塞入全部原始历史。持久 job 由 Catena 管理。
3. **实验工作台与 Skill 生命周期**：从人工确认的真实失败冻结 10–20 个案例；Harbor/Barena 执行 baseline/candidate；展示通过率、重复次数、成本、回归、Skill diff、来源和回退。

这三项共用现有控制面，先选一条默认执行路径。托管 Agents API 能力验收可以并行进行，但不必为了托管迁移阻塞首次真实验证。

### P1：把前沿方法接成可比较的实验

4. GauzMem 工程经验回写与定期整理：自动归纳生成待核对修改，验证结论作为事实保存，保留历史和失效版本。
5. Optimizer adapter：先对比简单候选与 Agent Lightning Skill，再增加 GEPA；两者使用同一案例、预算和保留集。
6. Catena MCP + MCP App：让用户在已有 Agent 对话里看证据、审查 Skill diff、查看实验报告。
7. Browser Use 作为一个执行 adapter：先证明一个真实 Web 案例，而后扩展计算机操作。

### P2：有基线后增加架构宽度

8. 第二种 harness（如 DeerFlow/Deep Agents/Claude Managed Agents）、A2A 远端委托、多专家协作、模型路由、红队/故障注入。
9. 若确有训练资源与数据，再评估 Agent Lightning 的 RL 路线；Skill 优化与模型权重训练是两个投入级别。

## 推荐的展示故事

用户给 Catena 一个持续目标 → 新运行触发调查 → 多条证据归并成一个可核对问题 → 检索既有经验 → 生成 Skill 候选 → 干净环境复现与对照 → 人工选择发布 → 后续运行验证是否复发。

每接一种新技术，保存 provider/版本、输入 hash、输出、效果、成本、耗时、失败/不确定状态。用户可以切换方法，并回答“这一种为什么更好”。这是建议的产品设计，不是当前实现声明。
