# Catena V2 架构收敛记录

基线：`8ac6708ca10bab0b87913bd2d62d5dcb7fbf5bea`。本报告按六个 Phase 更新。

## Phase 1：依赖审计

| 旧职责 | Go 调用方/存储 | Engine | UI/部署 | 处理 |
|---|---|---|---|---|
| 三 Role | evolution_job.go 的执行与输出修复；evolution_runtime.go 的 Role RPC；EvolutionJob Queue | roles.py、runtime.py、/v1/turn | EvolutionWorkspace、Launcher、协议类型 | 迁移 Evidence/Queue/Candidate，删除固定 Role 执行与专属测试 |
| GauzMem | memory.go 中 Client、通用数据、Handler、Evidence 混合 | 无 | memory profile、MySQL/Neo4j/Qdrant、Dockerfile.gauzmem | 通用能力保留，OpenViking 迁移后删除提供方专属实现 |
| OpenViking | openviking_memory.go、owner header/URI 约束、memory task records | 待迁移为 Memory Adapter | compose.openviking.yml、CPU embedding | 迁移到 Engine，保留 owner 隔离和异步任务来源 |
| Barena | Runner、Run/Issue/Case/Replay/HarnessVersion/Release、Run Bundle、Scenario adoption、Store/Postgres 方法 | 固定 rg-missing-search Worker/Bridge/Coordinator | runner-client、Dockerfile.runner、ExperimentWorkspace、构建上下文 | 正式路径删除；实验集与有效报告保留 |
| 通用基础 | Trace/Agent/Auth、MemoryStore、Postgres、Lease/Heartbeat/Fencing、Retry、取消、脱敏 | 私有服务鉴权、隔离 Codex transport、RSI/Harbor adapter | Trace/Agent/Memory/Outputs | 保留复用 |

发现：旧 Runner 默认缺 SPIRAL_RUNNER_URL；该链被正式退役，不为其继续补部署。旧 Conversation 写 Handler 没有路由，历史读取不与写 Handler 的去留混同。认证表 `barena_users`/sessions/api_tokens 不删除。GauzMem 不等于 Go MemoryStore。

### 基线验证

- `/tmp/go/bin/go test ./...`：通过（未配置数据库的集成测试仍按原条件跳过）。
- 独立 Harbor Engine 环境 `pytest`：104 passed / 7 Windows-only skipped。
- 前端 pnpm test、typecheck、build：均通过，日志 /tmp/catena-v2-baseline-web.log。
- 核心源码计数：103 个文件 / 30,658 行。口径为 control-plane、engine、catena-web/src、tap/catena_tap 中 .go/.py/.ts/.tsx，排除 *_test.go、tests、Go 内嵌 Web。最终使用同一口径，不将冻结实验快照计入核心。

## 迁移与验收边界

目标：Trace → Evidence → Analysis Job → Artifact → Evaluation / Review。RSI 与 Memory 是平级 Pipeline，共用原持久队列和审计身份，不互相作为启动前提。Harbor 只负责 Evaluation。

数据库采用增量、可版本化迁移；历史认证和业务表保留，不执行 DROP/清空。旧执行请求不得被 V2 当作新模型任务重放。Engine 的执行 receipt 只消除重复外部启动，不提供第二套轮询队列或租约。

本任务不自动调用收费模型、不发布公网服务。真实模型、Docker Harbor、OpenViking 提炼缺少实际条件时记录 BLOCKED，不用 fixture 通过率冒充真实评测。

## Phase 2：基础契约

增加 Artifact Envelope 与 Job pipeline/adapter/options/checkpoint 字段，暂时允许旧编排共存。Artifact 存在原 Job JSON 文档中，通过原 SaveLeasedJob 原子落库，不新增第二套队列或产物数据库。写入验证 owner/job/evidence，读取按 owner 过滤；认证和历史表不变。Go 全套单元测试及新增 owner/fenced Artifact 回归通过。
