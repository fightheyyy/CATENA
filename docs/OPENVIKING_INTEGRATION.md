# OpenViking：V2 Engine Memory Adapter

Go 不再直接调用 OpenViking。路径为 authenticated Go → durable Analysis Job → private Engine Memory Adapter → OpenViking。Memory 与 RSI 平级，无需配置 RSI/Harbor Case 即可从 Trace 或历史 Session 提炼。

`POST /v1/analysis-jobs` 请求：`{"pipeline":"memory","trace_id":"<retained trace>"}`，或 agent_id/window_start/window_end，或 agent_id/conversation_id（历史会话）。必须发送 Idempotency-Key。Artifact 保存在原任务 document 中，关联 Evidence refs/SHA、provider task ID 和 result，状态 extracted，不表示确认事实。

Engine 环境配置 `CATENA_OPENVIKING_URL` / `CATENA_OPENVIKING_API_KEY`；提供方 trusted mode。Go 私有请求 X-Catena-Owner 只从认证上下文生成，浏览器不能指定。Engine 沿用原 `catena_` + SHA256(`catena-memory:` + owner)[:24] 作为 user，account=catena、role=user；`viking://user/<derived>/memories` 和原 48-bit file ID 保持兼容。URI 与任务 ID 必须属于同 owner，拒绝路径穿越/编码逃逸。

提炼 commit 有持久化 execution receipt；Go 在查询前保存 native task checkpoint。重试不再提炼；Engine 崩溃后的不确定 running receipt 要求人工核查。Go 取消/超时不会撤销 OpenViking 已接受的任务；Catena 停止跟踪且不发布产物。提供方原生结果没有列出文件时只记录任务结果，不把整个记忆库归因给该任务。

检索、列表、文件关系与手写 Note 继续由 Go 认证后经 Engine 转发。文件关系只是来源/链接，不冒充实体推断图。旧 Memory task ledger 只保留历史状态查询，不是新队列。GauzMem 及专属数据库服务已删除，数据卷需运维自行备份保管，本轮不删卷。

可选部署 overlay 为 `deploy/catena-mvp1/compose.openviking.yml`。原 `setup_openviking.py` 生成配置的功能保留；其模型/embedding 选择需要操作人明确授权。真实 OpenViking 测试尚需服务与凭据，协议 fixture 不代替实际提炼验收。
