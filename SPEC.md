# CATENA 当前规格

CATENA = Trace-driven Agent Engineering Control Plane。

Agent Trace → Codex 工具调查 → Problem / Evidence → 人工决定是否优化 → 冻结 Candidate → Harbor Compare → Human Review。

CATENA 管理 Trace、脱敏且固定来源的 Evidence、已审核 Case 的注册、PostgreSQL Job 调度、Artifact / 版本 / 实验与人工决定。Codex 提供统一的内部分析与改进 Runtime，Harbor 提供独立目标执行与评测底座。模型自己的选择或评分不构成改进证据。

| 边界 | 实现职责 |
|---|---|
| Tap / Go / ClickHouse | OTLP 持久化、背压、身份隔离、可追溯的原始经历 |
| Go / PostgreSQL | Auth、Agent、加密模型配置、Evidence、Lease / Heartbeat / Fencing、幂等 / Retry / Timeout / Cancel、Artifact、Review |
| Codex App Server + Skill | 归因、候选提出与整理；原生 Skill 输入和受控工具调用 |
| Improvement Workflow | 固定 Probe / Freeze / Compare，保留预算、取消与失败记录 |
| Harbor | 原生 Agent / Environment / Job / Trial / Verifier，固定 Case 指纹前后校验 |
| React | Trace、实验配置、状态、可比较指标、候选文件和人工审查 |

唯一任务创建入口为 `POST /v1/analysis-jobs`，pipeline 为 `analyze` 或 `improve`，内部执行为 `codex-skill`。分析调用 `/v3/analyze`，通过 `list_traces` / `get_trace` 自主多轮取证，不依赖 Harbor；优化调用 `/v3/improve`，必须由用户显式提交已完成分析的 analysis_job_id，复制其报告和 Evidence 快照。分析不会自动进入优化。旧 rsi/Harness² 创建请求显式拒绝，已有终态记录和 Review 继续读取。旧非终态任务不得转成新策略自动执行；升级需先停旧 Worker。Skill 借鉴 Propose / Compose，运行依赖和原生算法循环已删除，不宣称原算法等价。

Candidate 只允许 AGENTS.md / Skill 文本，保留父版本与内容哈希。冻结候选后，Harbor 新跑 Baseline / Candidate 对照；可添加独立 held-out Case。复用优化 Case 不伪称 held-out。结果持久化原始 Verifier、用量、耗时、Artifact 路径/指纹和已知费用；未知值不补零。单次对照只支持有限工程结论。

Review 必须基于已完成候选和有效独立对照，记录认证用户、时间、adopt/reject 和理由。同一决定重试幂等；决定不可覆盖，普通 API 不能删除已有 Review 的 Job。采用不触发自动安装。

Go 只有一套持久队列。Engine receipt 是付费操作去重记录；Engine 重启后的不确定执行拒绝自动重放。重试、取消、租约丢失和 fencing 仍是必要正确性边界。

分析模型只接收目标与固定查询范围，不接收整包 Trace 上下文。只读工具复用 Go TraceStore 和现有脱敏逻辑；Owner / Agent / 时间窗由服务端固定，任务能力凭证受活动 Lease 约束、结束即撤销。报告只能引用实际 get_trace 返回的 Trace / Span，查询记录与快照随报告持久化。

工程记忆复用 Evidence、候选版本、实验、Artifact 和 Review 的持久记录。Memory / OpenViking、Memory API/UI、Embedding 和社区功能退役。历史数据不清空，旧终态 Job / Artifact 和会话读取兼容保留。未来 Context 仅预留经过授权的 Markdown / AGENTS.md 文本输入，不实现检索、Embedding、知识图谱或长期记忆引擎。

部署四个服务，无 Redis、Kafka、Kubernetes。Harbor Docker socket 由可信部署者显式授权；不承诺安全的恶意多租户 Docker 沙箱。Owner 隔离保留，组织成员与共享权限未交付。

[当前 Workflow 与边界](docs/IMPROVEMENT_WORKFLOW.md) · [分析门验收](docs/refactors/ANALYSIS_GATE.md) · [改进实测](docs/refactors/UNIFIED_RUNTIME.md) · [历史重构报告](docs/refactors/REBIRTH.md)。历史研究文档不代表当前执行架构。

## 收敛边界与下一验收

当前冻结 Go / PostgreSQL / ClickHouse、统一 Codex Runtime 和 Harbor 的职责边界。新增策略优先复用现有 Skill / Tool / Workflow；不为尚不存在的第二实现增加框架。

近期工作仅限核心接入、报告可读性、证据可信度、实验操作和真实案例验证。组织共享、通用 Code Patch、经验检索、插件市场与自动采用不属于本轮交付；版本化工程记录继续保留。

下一验收目标是：外部开发者不依赖作者陪同，独立完成 Trace → 调查 → 人工选择优化 → Harbor 评测 → Review。需记录操作阻碍、问题是否被用户认可及决策依据；工程链路通过和优化效果成立分别评价。该验收尚未完成，现有独立实验不能替代它。
