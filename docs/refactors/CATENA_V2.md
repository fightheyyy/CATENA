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

## Phase 3：RSI 迁移

正式创建入口为 `POST /v1/analysis-jobs`，Worker 复用原 PostgreSQL lease/heartbeat/fencing，调用私有 `/v2/rsi`。候选、父版本、Case fingerprints、Harbor trial 证据和状态保存于 Job JSON/Artifact；无真实验证不标为 verified。界面读取新任务及 Artifact，选择管理员注册的 Harbor Case。

固定三 Role Prompt、SDK、RPC 和专属测试退役。历史固定 rg Worker/Bridge/Coordinator 等移至 `research/legacy-engine`，不再打包；复现实验应 checkout 基线。通用 Codex JSON transport 保留独立 HOME、子进程组终止、输出上限。Engine 私有 token + Go owner，执行 receipt 持久化（不存凭据），重试不重复启动已完成/仍在运行的付费操作；进程崩溃留下不确定 running 时拒绝自动重放，要求人工核查。默认单分析槽，不阻塞 health/receipt/查询。

验证：Go 全套单元测试通过；Python 55 tests 通过（含真实 pinned Harness² 的确定性适配测试、Harbor Adapter 和 Codex 子进程 timeout/cancel 测试）；React typecheck 通过。Go HTTP→原 Worker→私有 Engine stub→Artifact 持久化为协议集成测试；不冒充真实模型或 Docker Harbor 成功率。真实付费模型调用未授权、未执行。

## Phase 4：Memory 迁移

OpenViking HTTP、trusted identity、owner URI 检查、文件/关系/搜索与异步提炼迁至 Engine Memory Adapter。Go 仅通过私有 Engine token/owner 访问；不持有提供方密钥。`catena_<sha256(catena-memory:owner)[:24]>` 映射、48-bit 文件 ID 与原数据兼容。GauzMem Client、Dockerfile、MySQL/Neo4j/Qdrant 专属服务配置删除；现有数据卷不删除。

Memory 独立 `pipeline=memory`：先保存 provider task ID checkpoint，再轮询；临时失败的重试只恢复 status，不重复 commit。Artifact 保存原 Evidence 引用/哈希、提供方任务和原生结果，不把整个 owner 的现有文件冒认成本次提炼结果，状态为 extracted 而非 confirmed。旧直接提炼 API 退役，Trace 提炼按钮进入同一队列。provider 已接受的远程提炼在 Go 取消后可能继续，Catena 停止轮询、不发布其产物；未宣称具备提供方事务回滚。

验证：Go 全套通过，含持久化 checkpoint 后临时失败/重试/只提炼一次；Engine 57 tests 通过，新增原 owner 哈希、越界 URI、跨 owner task/搜索/文件、跨 Engine receipt 重启不重复 commit。均为协议 fixtures；此环境未配置 OpenViking/模型凭证，真实提炼 BLOCKED。

## Phase 5：退役 Barena

删除 Node Runner、Runner client、Run bundle、Scenario adoption、Issue→Case→Replay→Release 自动晋级及固定 Experiment HTTP/Store/内存/PG 实现。先迁移 newID/bounded/Trace ID 校验等共享函数。认证表、用户/Agent/API token、Go MemoryStore、历史会话读和队列保留。公共 Profile 保留基本资料，移除以旧 Run 推算能力评级。HTTP 构造收敛为 `NewHTTPHandler(Store, HTTPConfig)`。

前端移除专用 Experiment 导航/页面；输出页读 Artifact/来源/Harbor 结果。历史 Conversation 的记忆操作也进入同一 Analysis Job，通过 owner-scoped 历史读取生成脱敏 Evidence，不再有独立提炼任务调度器。Harbor Adapter 移至 `engine/evaluation`，固定实验 Worker 已归档；删除同步 RSI CLI，Go Queue 为唯一正式启动路径。Compose 和 core image 不再构建 Runner/XiaoBaOS。

本阶段 Go tests、React typecheck 通过。研究数据、冻结 Release、Verifier 结果及报告原位保留；过时代码在 research/legacy-engine 并标注仅历史用途。生产旧表数据不 DROP/DELETE。
