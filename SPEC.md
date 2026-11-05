# CATENA 当前规格

CATENA = Trace-driven Agent Engineering Control Plane。

Agent Trace → Evidence → Case → Harness² → Candidate → Harbor → Compare → Human Review。

CATENA 管理 Trace、脱敏且固定来源的 Evidence、已审核 Case 的注册、PostgreSQL Job 调度、Artifact / 版本 / 实验与人工决定。Harness² 提供优化算法，Harbor 提供唯一正式 Agent 执行与评测底座。模型自己的选择或评分不构成改进证据。

| 边界 | 实现职责 |
|---|---|
| Tap / Go / ClickHouse | OTLP 持久化、背压、身份隔离、可追溯的原始经历 |
| Go / PostgreSQL | Auth、Agent、加密模型配置、Evidence、Lease / Heartbeat / Fencing、幂等 / Retry / Timeout / Cancel、Artifact、Review |
| Harness² Integration | 固定 commit，直接调用原生 scheduler；模型与文件边界保持受限 |
| Harbor | 原生 Agent / Environment / Job / Trial / Verifier，固定 Case 指纹前后校验 |
| React | Trace、实验配置、状态、可比较指标、候选文件和人工审查 |

唯一任务创建入口为 `POST /v1/analysis-jobs`，pipeline 仅 `rsi`，算法仅 Harness²。删除 CATENA 多算法 Registry 和 Composition；Harness² 上游原生搜索模式保留。旧算法/组合选项显式拒绝。外部请求不能选择 import path、执行器、任意文件路径或 Owner。

Candidate 只允许 AGENTS.md / Skill 文本，保留父版本与内容哈希。冻结候选后，Harbor 新跑 Baseline / Candidate 对照；可添加独立 held-out Case。复用优化 Case 不伪称 held-out。结果持久化原始 Verifier、用量、耗时、Artifact 路径/指纹和已知费用；未知值不补零。单次对照只支持有限工程结论。

Review 必须基于已完成候选和有效独立对照，记录认证用户、时间、adopt/reject 和理由。同一决定重试幂等；决定不可覆盖，普通 API 不能删除已有 Review 的 Job。采用不触发自动安装。

Go 只有一套持久队列。Engine receipt 是付费操作去重记录；Engine 重启后的不确定执行拒绝自动重放。重试、取消、租约丢失和 fencing 仍是必要正确性边界。

Memory / OpenViking、Memory API/UI、Embedding 和社区功能退役。历史数据不清空，旧终态 Job / Artifact 和会话读取兼容保留。未来 Context 仅预留经过授权的 Markdown / AGENTS.md 文本输入，不实现检索、Embedding、知识图谱或长期记忆引擎。

部署四个服务，无 Redis、Kafka、Kubernetes。Harbor Docker socket 由可信部署者显式授权；不承诺安全的恶意多租户 Docker 沙箱。Owner 隔离保留，组织成员与共享权限未交付。

[阶段报告、测试、迁移与实验边界](docs/refactors/REBIRTH.md)。历史研究文档不代表当前执行架构。
