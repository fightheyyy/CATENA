# Catena Docker 部署

默认启动 catena-core（Go+嵌入 React）、catena-engine、PostgreSQL 17、ClickHouse 25.10，以及 Collector Contrib 0.137.0 Gateway/Consumer 和三节点 Kafka 3.9.1。需要 Compose 2.20+。不引入 Redis。

Trace 经 Go 鉴权后转发 Collector → Kafka → Collector → 官方 ClickHouse Exporter；版本化迁移创建 `otel_traces`，保留 `catena_spans`。已有部署必须先按 [Trace 迁移与回滚流程](../../docs/TRACE_DATA_PLANE.md) 完成历史数据和真实 Trace 对照，不能直接依赖新部署默认读取切换。

```sh
cp deploy/catena-mvp1/.env.example deploy/catena-mvp1/.env
# 编辑 .env，密码/ClickHouse DSN 保持一致；模型在用户 Connections 配置
./deploy/catena-mvp1/demo.sh up
./deploy/catena-mvp1/smoke.sh
```

默认 Web 仅监听 127.0.0.1:5570，Engine 不发布端口。远程服务使用现有 compose.public.yml、GitHub OAuth、独立强随机 secret；本任务不自动发布公网。`public.sh` 保留现有 Caddy 部署入口。`Dockerfile.core.prebuilt` 用于已生成 Web dist 的离线构建。

## Codex / Harbor 显式配置

1. 创建 operator-owned 绝对目录，如 `/srv/catena-evaluation`，里面放已审核 Harbor task、cases.json、runs/。目录让 Engine UID 1001 可访问，receipt volume 仅 Engine 使用。
2. cases.json 的 key 为公开 Case ID；value 为 path（相同绝对路径）、sha256（`catena_engine.evaluation.harbor.task_digest`）。任务文件包括 verifier 均参与指纹，禁止 symlink。另需 confirmed_by，引用实际人工确认记录；通过原生 task.toml artifacts 或 manifest artifacts 声明实际产物文件，例如 ["/workspace/case/result.json"]。
3. 设置 `CATENA_EVALUATION_WORKSPACE` 和宿主 Docker socket 的 `CATENA_DOCKER_GID`，合并 compose.evaluation.yml。任务和输出采用相同绝对路径 bind mount，否则宿主 Docker 找不到 Engine 中的路径。

```sh
docker compose -f deploy/catena-mvp1/compose.yml -f deploy/catena-mvp1/compose.evaluation.yml --env-file deploy/catena-mvp1/.env config
docker compose -f deploy/catena-mvp1/compose.yml -f deploy/catena-mvp1/compose.evaluation.yml --env-file deploy/catena-mvp1/.env up -d --build
```

Docker socket 具有宿主控制能力，不是安全的多租户沙箱；只允许受信 operator 注册的任务。默认配置不挂载 socket。Engine 单分析槽、1 CPU/1GiB；Harbor 每 Trial 1 CPU/1024MiB、并发 1，这些容器资源在 Engine cgroup 外，部署需额外预算。不得把此配置当成已证明 2 核 8GB 承载任意并发评测。模型调用、网络和镜像拉取需另行授权/配置。

## 持久化、迁移和恢复

- postgres-data / clickhouse-data 保留原数据；catena-engine-data 保存外部 execution receipt 与默认分析结果，不能随意删除，否则会丢失去重依据。
- Go 自动执行内嵌 migrations/001_core.sql 与版本 2、3 增量迁移。保留原认证/队列表名和已有历史表，绝不清空生产表；只把旧 schema 未完成 Job 标为 retired/failed，原 document 的历史输出保留。旧 completed 记录不改写，界面不会把它转成新 Artifact。
- 旧执行 API 返回 404；历史研究 Case/结果仍在 evals/ 与 docs/，专用 Engine 代码为 research archive。
- 升级前自行备份数据库、ClickHouse 和 Engine receipt/结果；本轮未修改线上数据。不使用 down -v。
- Engine 重启留下 running receipt 时拒绝自动重放，需要核查实际外部操作后人工决定。OpenViking 已退役；历史提供方目录/数据需先按导出说明保留。

`CATENA_JOB_WORKERS=1`、Job/request timeout 默认 20m；lease 30s / heartbeat 5s。请求限流与队列边界沿用既有 Go 实现，lease loss 不能覆盖后继 Worker 的结果。

[当前验收与限制](../../docs/refactors/REBIRTH.md)。隔离 Docker 数据库测试不等于目标云生产部署验收；真实模型效果必须单独评测，不能用协议 Fixture 代替。

升级前必须停止旧 Worker。Memory API 和社区入口返回 404，pipeline=memory 返回 400。迁移 003 只退役活动 Memory Job，不删除表或历史文档。见 [导出/回滚边界](../../docs/OPENVIKING_INTEGRATION.md)。已有 Review 的 Job 不允许普通 API 删除。

Apple Silicon 复跑现有冻结 amd64 Benchmark 时，构建和 Engine 均需 Docker 原生 `DOCKER_DEFAULT_PLATFORM=linux/amd64`；这会使用仿真，耗时不代表 Linux amd64 生产机器。

## Trace 调查回调

分析模式通过只读 `list_traces` / `get_trace` 自主取证。Compose 已为 Engine 设置
`CATENA_CONTROL_URL=http://catena-core:8787`；分离部署时改为 Engine 可达的 Go 内网地址。
每次执行使用短期任务能力凭证，不能用它查询其他 Job / Owner，也不会进入模型上下文。
查询工具不授予 Docker 或 Harbor 权限；人工启动优化和最终 Review 仍是独立步骤。
