# Catena V2 当前规格

定位保持 **Agent 长期记忆、团队工作记忆与工作进化平台**。当前实现 owner 级隔离；组织成员、共享权限和自动采用仍不属于已交付能力。

主链路：Trace → Evidence → Analysis Job → Artifact → Evaluation / Review。

| 边界 | 当前职责 |
|---|---|
| Tap / Go OTLP / ClickHouse | 采集、限流背压、身份隔离、持久化和查询 |
| Go Control Plane / PostgreSQL | Auth、Agent、加密模型配置、任务调度、Lease/Heartbeat/Fencing、Retry/Timeout/Cancel、Artifact 审计 |
| Engine RSI | 复用 Harness² Adapter/registry/host，经隔离 Codex CLI 生成优化候选；Harbor 验证 |
| Engine Memory | 独立 OpenViking Adapter 提炼与检索，沿用 owner 哈希和文件 URI 边界 |
| Artifact | 最小共享 envelope；类型 rsi_candidate / engineering_memory，具体 payload 各自保留语义 |
| React | Trace、任务状态、产物、来源和验证结果；保留历史会话读取 |

RSI 与 Memory 彼此不是启动前提。Harbor 不是第三种 Pipeline，也不接管 Go 的任务队列。唯一正式任务创建入口 `POST /v1/analysis-jobs`；查询、取消、删除位于相同资源下。`GET /v1/artifacts` 按 owner/type 读取现有 Job JSON，不新建产物数据库。

RSI 输入必须引用 operator 注册且 fingerprint 固定的 Harbor Case；原始 Trace 不足以自动复现。Candidate 保留 parents/content hash/files；Harbor Trial 及 held-out 情况供人审查，不把模型评价等同于 verifier，通过率需明确分母和样本限制。Memory Artifact 保存原始证据关联、provider task/result，extracted 不等于 confirmed。

Go 提供单一持久队列，Engine receipt 只记录外部操作身份与结果，不是第二套队列。重试可连接仍在运行或读取已完成的执行；Engine 重启留下不确定 running receipt 时拒绝自动重放，防止重复模型费用。Memory 接收后先持久化 native task ID，再轮询；取消后远程 provider task 可能继续，Catena 不发布它的 Artifact。

部署默认 Go + Engine + PostgreSQL + ClickHouse；Memory/RSI 的外部依赖按指南显式配置。无 Redis/Kafka/Kubernetes。旧三 Role、SDK 回退、GauzMem、内嵌 Barena/XiaoBaOS Runner 不再运行。历史 DB 表和实验材料保留，V2 增量迁移不清空生产数据。

[实现、兼容性、验证与限制](docs/refactors/CATENA_V2.md)。历史路线图仅供研究参考，以本规格与当前源码为准。
