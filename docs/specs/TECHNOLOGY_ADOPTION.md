# Catena 技术采用决策

## 2026-10-03 当前决定（覆盖下方 9 月 30 日的运行时选择）

用户已确认：Catena 的分析与主动 Agent 运行时统一采用 **OpenAI Agents SDK（Python）**，
不采用 Pi；Agents API 不再作为默认 Engineer 后端。现有 engine 已使用 SDK，
下一步在其上扩展受控工具循环、会话与审批恢复。持续目标、调度、预算和业务恢复仍由 Go/Postgres 负责。

云沙箱首选 **E2B**，待执行、暂停恢复、产物导出与清理实测后成为默认云执行环境；
本地开发/CI 使用 Docker。评测采用 **Harbor**，负责冻结 Case 的 baseline/candidate
执行、轨迹和 verifier 结果；Catena/Barena 保存业务证据与结论。
Harbor 已通过限定 Case 的平台 Agent 工具调用与 Docker 执行验收，
见 [首次链路实验](../HARBOR_EXPERIMENT_20261003.md)；尚未接入公开 Go API/前端，
也未验收 Codex/Claude Code adapter。E2B 云端验收仍待完成，不能宣称总体成功率提升。

后续已补充 [E2B 工具路由与 provider 接入](../E2B_INTEGRATION_20261003.md)，
依赖、配置、时长限制与清理记录已实现；缺少 API Key，云端实测仍待完成。

首个真实问题的 Case 见 [缺少 rg 的 PowerShell 搜索](../../evals/rg-missing-search/CASE.md)。
该 Case 原计划评测 Codex CLI；CLI 预跑环境无效。目前有效实验的被测对象是
独立 SDK 搜索 Agent，与平台协调 Agent 分开，不把结果归因于 Codex CLI。
以下 9 月 30 日内容保留为历史研究记录，其中 Agents API 首选与迁移计划已被本决定覆盖。

日期：2026-09-30。状态：设计决策，尚未安装新依赖或完成新服务接入。产品定位见 [总 SPEC](../../SPEC.md)。

## 1. 结论

采用“复用成熟执行、存储与观测能力，自己负责问题到验证结果的闭环”的原则。80%/20% 是投入方向，不是经过测量的代码量、成本或进度估算。

目标主线保持：Trace → Memory → Improvement → Verified Learning。Catena 自己负责问题优先级、证据关联、案例构造、业务契约、实验比较与经验有效性；不再扩张通用 Agent Runtime、Sandbox 或优化器。

但是，减少自研不等于同时引入所有依赖。每个底座必须通过一个实际 Catena 场景的接入验收，才进入默认部署。

## 2. 采用矩阵

| 层 | 技术选择 | 当前决策 | Catena 保留的责任 |
|---|---|---|---|
| Engineer Agent | OpenAI Agents API | 新路径首选；先做能力/账户验收，旧路径保留迁移期回退 | 提示、受控工具、触发与 job 状态、预算、证据 |
| Candidate 生成 | 同一 Agents API 的 Codex harness | 复用，不再另造 Patch/Planner Runtime | 输入包、版本约束、patch 与测试证据验收 |
| Candidate 沙箱 | E2B | 首选自管理云沙箱 provider；出现真实仓库执行需求时启用 | 镜像/工作区、生命周期、清理、产物导出 |
| 评测执行 | Harbor + Barena | 首选路线，复用已有采用工作 | Case、verifier、Run Bundle、比较与发布事实 |
| 评测环境 | Harbor Docker；云端可选 E2B | 本地先 Docker，按环境兼容性决定云端迁移 | 两组实验一致性与故障边界 |
| Trace | OTel/Tap + 现有 ClickHouse | 继续使用 | 原生身份、归一化、完整性与业务关联 |
| Trace 产品能力 | Laminar 的可用接口/开源能力 | 按已部署版本核验；不重做大型 Trace UI | 问题 Inbox、Case、候选 Diff、验证报告 |
| 工程记忆 | 我们自己的 GauzMem | 确定采用；复用现有接入，扩展工程记忆写入与召回 | 业务实体、事实状态、版本、租户权限、回写规则 |
| 权威业务事实 | 现有 Postgres | 继续使用 | Experiment、Decision、验证结论、审计与派生任务 |
| 个人对话记忆 | 我们自己的 GauzMem | 保留既有数据、接口与来源规则 | 用户可见对话与工程经验的隔离 |
| 自动优化 | Agent Lightning Skill | 后续评估；不默认引入 RL 训练栈 | 搜索预算、独立验证集、发布边界 |
| Reliability | Catena 契约与执行侧 verifier | 按真实案例建设 | 状态/效果/时序语义与不确定判定 |

“首选”不表示该组件已完成接入。记忆后端按用户决定采用 GauzMem，不再引入替代记忆引擎。M0 接通最小工程记忆链路；E2B 和自动优化不是 M0 首次价值的必备条件。

## 3. 原建议核验与必要修正

### 3.1 Agents API：选择成立，但三种角色要分开

官方说明 Agents API 提供托管 Codex harness、会话与上下文管理能力，适合作为 Catena Engineer 的新执行后端。[A1]

需要区分：

1. **被观察的 Agent**：用户现有 Codex CLI、Claude Code、内部 Agent，继续用 Tap/OTLP 接入。
2. **Catena Engineer**：读取证据、形成问题与候选，首选 Agents API。
3. **被评测的目标 Agent**：保持用户实际运行时与版本，由 Harbor/adapter 执行；不能为了统一技术栈换成 Codex 后再宣称验证了原系统。

所以，“Agent 层不用 CLI 自造 Runtime”不等于删除 Codex CLI 采集或 Harbor 的 Codex adapter。分析和候选生成可复用一个 managed harness；Inspector/Evolution/Reviewer 可以保留为逻辑阶段和结构化输出，不必继续作为三个自研 Runtime，但也不能删掉独立验证职责。

持续观察由 Catena 的数据水位、队列和调度触发。持久 session 本身不等于自动运行的业务监控服务。

### 3.2 E2B：官方路径存在，控制器仍需实现

官方已有 Agents API 的 E2B provider 指南，支持应用或 webhook 管理的方式；需要保存 session/sandbox 映射，并分别清理两种资源。[A2]

故“官方支持”可以省去执行协议探索，不能省去生命周期代码。Catena 或执行 worker 仍需管理 workspace 初始化、绑定、恢复、超时、失败后的产物保存与资源回收。pause/resume 与删除后重建不能混同。

分析阶段只有受控只读工具时，不必启动仓库沙箱。首次候选执行只选择一条 provider 路径；E2B 尚未验收时可使用官方托管环境完成适配验证，不同时维持两套默认链路。

候选生成环境和评测环境必须隔离。生成者可以运行公开测试，但最终 verifier/保留案例不能由它修改。候选包导出后，Harbor 分别从干净 baseline/candidate 开始，不拿被生成 Agent 修改过的工作目录当作基准。

### 3.3 Harbor：复用执行设施，不外包业务正确性

上游支持多 Agent 与环境；本机 `E:\harbor` 的 `3b287b5` 存在 Codex adapter、Docker 与 E2B provider/factory 注册及相关测试文件。本次只检查代码，没有执行云端验收。[A3]

Harbor job resume、轨迹续跑、离线证据复核和可控世界重放不是同一概念。支持某环境不自动代表支持任意故障 hook。业务效果/审批/取消时序仍需 World 或工具边界的可靠记录。

Barena 保留为 Harbor 与 Catena 之间的案例/证据/发布边界，不再实现平行的通用 Agent runner。模型评分、Harbor reward、RCP 和 Barena release verdict 分开记录。

### 3.4 GauzMem：复用自己的记忆后端

依据用户明确决定，个人记忆与工程记忆统一采用自己的 GauzMem。复用 Catena 已有 GauzMem gateway、任务记录和接入经验；先核对实际接口，再补充工程记忆所需能力，不将规划视作已实现。

工程经验定义为 Failure、Capability、Decision、Experiment、Incident、Improvement 等业务类型。Postgres 保存 Catena 已确认的业务事实及来源；GauzMem 负责工程经验的编译、关联与召回，推断需明确标记。不能让模型抽取出的关系覆盖实验计数或人工决定。

通过 MemoryPort 适配 GauzMem，M0 完成一个已确认问题的工程记忆写入、按来源召回与状态更新。命名空间不能代替鉴权；租户隔离、版本适用性、删除传播与失效语义逐项验收。GauzMem 暂时不可用时保留待处理任务和源事实，界面显示同步状态，不伪造召回结果。

保留 GauzMem 既有个人记忆数据与接口。个人记忆只接收用户可见对话；工程记忆接收有来源的失败、实验和决策，使用独立 scope、实体与 UI 标签。无需迁移到另一记忆后端。

### 3.5 Laminar：产品能力和开源可复用范围不同

公开 README 描述 Signals、自然语言行为检测、通知及 MCP/CLI 查询；因此“Trace 加聊天或告警”不是足够的差异化声明。[A5]

但本机 `E:\lmnr` 的 `5b71e45` 和公开内部说明均注明 Signals 依赖 `lmnr-private`；README 的产品列表不能直接当成 OSS 功能验收。chat-with-trace/SQL 功能与完整平台 Agent 的可用范围也须分别核对。[A6]

因此不把 Signals 作为本地 M0 必需依赖，也不声称 Laminar 只会告警、不能继续扩展到改进闭环。Catena 的价值要通过实际节省工作量验证，不能依赖对竞争产品能力的缩小描述。

### 3.6 Agent Lightning：Skill 与 RL 栈分开

v1.0.1 发布说明确有优化其他 Agent 的 Skill，适用于 editable agent + benchmark，并提到 Codex/Claude Code/Copilot。主框架 v1 同时有 RL 训练路线，二者不是同一部署需求。[A7]

先评估 Skill 是否减少候选搜索工作，不为它安装整套训练基础设施。锁定版本与实际入口，预算和 holdout 由 Catena/执行侧掌握。候选生成者不能通过改判定脚本提高分数。

官方也已有 Trace/Eval/Codex 改进闭环示例，因此组合模式不是 Catena 独有发明。[A8]

## 4. 最小集成契约

只在已有实体边界加薄 adapter，不建设一套通用插件平台。

| 接口 | 输入 | 输出与关键约束 |
|---|---|---|
| EngineerPort | Evidence Pack、scope、目标版本、预算、任务类型 | provider/session/job IDs，结构化 Finding/CaseDraft/Candidate；不拥有发布权 |
| MemoryPort | 已鉴权的查询或带版本的工程记录 | 带来源、有效期、推断状态的结果；支持删除/失效和投影重建 |
| EvaluationPort | 冻结 Case、baseline/candidate、环境与 verifier | 不可变 Trial/Evidence manifest、verdict、成本与覆盖 |
| TraceReadPort | owner、固定 IDs、范围与分页 | 有缺口标注的证据；不暴露跨租户任意 SQL |

Catena Job 以本地持久记录为准，provider 的 session 只是外部执行资源。提交前保存 operation key，超时先对账再决定重试；不能因客户端断线重复启动收费的候选任务。

最少关联字段：owner、agent、job、evidence_pack_hash、provider_session、environment、candidate_hash、case_version、experiment_id。工具重试和读取结果需保留 evidence IDs，防止模型凭空引用。

会话按 owner/project/job scope 隔离，不用一个长期 session 混合多个用户或整个团队历史。必要记忆显式检索注入，任务结束仍可独立保存证据，不依赖 provider 永久保留会话。

## 5. Memory 回写和迁移

实验完成后：先在 Postgres 提交结果、来源、版本和 outbox，再异步将工程经验同步到 GauzMem。两套存储不做不可靠的同步双写。

幂等键绑定 experiment/result_version；索引失败显示记忆更新待完成，不能回滚或伪造实验结果。人工 Decision、测量 Capability 与模型 Hypothesis 分开，模型不得把建议写成已批准决定。

删除/撤销以业务事实层为准，投影查询必须屏蔽失效来源直到图同步完成。用户隔离、幂等回写、版本冲突、删除传播与投影重建都属于采用验收。

旧 Runtime 迁移采用同一小批 Evidence Pack 的并行对比：结构化输出、来源准确率、成本、取消/恢复、产物可读性。只切换新 Job 的 backend；在途 Job 使用原后端完成，不搬迁半途会话。历史候选仍可查看，旧实现待无在途依赖且验收完成再退役。

## 6. 环境与数据责任

E2B 用作隔离工作区，Harbor 管评测生命周期，Catena/Barena 管 job 和业务协议。一个 sandbox 只有一个生命周期所有者，避免两个控制器同时创建、暂停或删除它。

保留现有 BYOK 与 provider 能力边界：Agents API 是新增特定后端，不能把用户任意 OpenAI-compatible Base URL 当作支持该 API。配置需单独验证，不静默改路由。

官方当前说明 Agents API 不支持 ZDR 且数据驻留有范围限制；选择 E2B 不等于全部数据只在自己的沙箱内。[A1] 接入前检查实际部署要求，不把全部原始 Trace/凭证打包发给模型。

每次环境都锁定 repo commit、依赖/镜像、候选摘要与允许修改范围。超时/取消后先导出可得证据，之后清理；资源泄漏和清理失败留在 job 状态中可追踪。

## 7. 按产品里程碑接入

| 顺序 | 交付 | 必须证明什么 |
|---|---|---|
| A：M0 | 现有 Trace → 有用问题卡 → GauzMem 最小工程记忆；EngineerPort 接入验证 | 用户确认问题，记忆可写入和召回；Agents API 的账户、输出与证据访问可用，不依赖新 sandbox |
| B：M1 | 一个真实问题 → Harbor 可执行案例 | baseline 失败可重现，报告能回到 Catena 原证据 |
| C：M2/M3 | Agents API 候选 → 独立环境验证 | E2B 完成初始化、导出、重连/失败清理；独立 verifier 不可被候选篡改 |
| D：记忆增强 | 扩展 M0 已接通的 GauzMem 工程记忆 | 跨版本经验召回、隔离、删除、失效与成本合格 |
| E：M4 后 | Agent Lightning Skill | 在固定预算和保留案例上优于简单候选流程 |

首期端到端 demo 必须是一个真实维护者的问题与修复对比，不以同时点亮多个 vendor 图标作为完成标准。尚未通过某 provider 验收时保留已有工作路径，不让依赖迁移阻塞问题发现和案例构建。

## 8. 与现有文档的关系

- 产品目标和用户验证门槛继续以 [总 SPEC](../../SPEC.md) 为准。
- [Reliability Engine](STATEFUL_RELIABILITY.md) 不自建通用 runner/sandbox，保留专用 World 与 verifier。
- 本文补充 2026-09-25 的采用记录：E2B 从远期备选提高为云端首选；自己的 GauzMem 承担个人与工程记忆；Agents API 为 Engineer 新路径首选；Agent Lightning 增加 Skill 评估路线。
- 旧采用记录中已经执行的冒烟结果保留，但不因此声称本轮新方案已运行。旧文档未修改的部署事实仍按原文理解。
- 本轮只改设计文档，不安装服务、不启动付费实验、不冻结或删除旧项目。

## 9. 来源与本次检查范围

官方页面核验日期为 2026-09-30；采用结论是基于来源的工程选择，不是上游对 Catena 的兼容保证。实现前锁定具体 SDK、镜像和版本。

- [A1 Agents API overview](https://developers.openai.com/api/docs/guides/agents-api/overview)
- [A2 Agents API E2B guide](https://developers.openai.com/api/docs/guides/agents-api/environments/providers/e2b)
- [A3 Harbor](https://github.com/harbor-framework/harbor)：另检查本机 3b287b5 的 README、Codex adapter 文件、environment factory 与 E2B 文件存在性。
- A4：记忆选型依据用户明确决定，采用自己的 GauzMem；接口现状参考 [原工作空间 spec](CATENA_WORKSPACE_SPEC_20260916.md)，工程记忆扩展需另行验收。
- [A5 Laminar README](https://github.com/lmnr-ai/lmnr)
- [A6 Laminar AI feature notes](https://github.com/lmnr-ai/lmnr/blob/main/docs/internal/ai-features.md)：本机 5b71e45 同时核验；不能将产品版 Signals 视为 OSS 必然可用。
- [A7 Agent Lightning releases](https://github.com/microsoft/agent-lightning/releases)：v1.0.1 Skill 与 v1.0.0 RL 路线。
- [A8 Build an Agent Improvement Loop with Traces, Evals, and Codex](https://developers.openai.com/cookbook/examples/agents_sdk/agent_improvement_loop)

未进行账户权限、云端可用性、实时计费或生产适配验证；本文不作“开箱即用”的承诺。
