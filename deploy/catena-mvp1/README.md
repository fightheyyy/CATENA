# Catena V2 Docker 部署

默认只启动 catena-core（Go+嵌入 React）、catena-engine、PostgreSQL 17、ClickHouse 25.10。无旧 Runner/XiaoBaOS/GauzMem/MySQL/Neo4j/Qdrant 运行依赖。

```sh
cp deploy/catena-mvp1/.env.example deploy/catena-mvp1/.env
# 编辑 .env，密码/ClickHouse DSN 保持一致；模型在用户 Connections 配置
./deploy/catena-mvp1/demo.sh up
./deploy/catena-mvp1/smoke.sh
```

默认 Web 仅监听 127.0.0.1:5570，Engine 不发布端口。远程服务使用现有 compose.public.yml、GitHub OAuth、独立强随机 secret；本任务不自动发布公网。`public.sh` 保留现有 Caddy 部署入口。`Dockerfile.core.prebuilt` 用于已生成 Web dist 的离线构建。

## RSI / Harbor 显式配置

1. 创建 operator-owned 绝对目录，如 `/srv/catena-rsi`，里面放已审核 Harbor task、cases.json、runs/。目录让 Engine UID 1001 可访问，receipt volume 仅 Engine 使用。
2. cases.json 的 key 为公开 Case ID；value 为 path（相同绝对路径）和 task_digest（`catena_engine.evaluation.harbor.task_digest`）。任务文件包括 verifier 均参与指纹，禁止 symlink。
3. 设置 `CATENA_RSI_WORKSPACE` 和宿主 Docker socket 的 `CATENA_DOCKER_GID`，合并 compose.rsi.yml。任务和输出采用相同绝对路径 bind mount，否则宿主 Docker 找不到 Engine 中的路径。

```sh
docker compose -f deploy/catena-mvp1/compose.yml -f deploy/catena-mvp1/compose.rsi.yml --env-file deploy/catena-mvp1/.env config
docker compose -f deploy/catena-mvp1/compose.yml -f deploy/catena-mvp1/compose.rsi.yml --env-file deploy/catena-mvp1/.env up -d --build
```

Docker socket 具有宿主控制能力，不是安全的多租户沙箱；只允许受信 operator 注册的任务。默认配置不挂载 socket。Engine 单分析槽、1 CPU/1GiB；Harbor 每 Trial 1 CPU/1024MiB、并发 1，这些容器资源在 Engine cgroup 外，部署需额外预算。不得把此配置当成已证明 2 核 8GB 承载任意并发评测。模型调用、网络和镜像拉取需另行授权/配置。

## Memory 显式配置

使用原 setup_openviking.py 准备提供方配置，审核模型与 embedding；设置 `CATENA_OPENVIKING_API_KEY` 和配置目录，再叠加 compose.openviking.yml。Engine 访问 private OpenViking，Go 没有提供方密钥。两条 Pipeline 无需同时启用。

## 持久化、迁移和恢复

- postgres-data / clickhouse-data 保留原数据；catena-engine-data 保存外部 execution receipt 与默认 RSI 结果，不能随意删除，否则会丢失去重依据。
- Go 自动执行内嵌 migrations/001_core.sql 与版本 2 增量迁移。保留原认证/队列表名和已有历史表，绝不清空生产表；只把旧 schema 未完成 Job 标为 retired/failed，原 document 的历史输出保留。旧 completed 记录不改写，界面不会把它转成新 Artifact。
- 旧执行 API 返回 404；历史研究 Case/结果仍在 evals/ 与 docs/，专用 Engine 代码为 research archive。
- 升级前自行备份数据库、ClickHouse 和 Engine receipt/结果；本轮未修改线上数据。不使用 down -v。
- Engine 重启留下 running receipt 时拒绝自动重放，需要核查实际外部操作后人工决定。Memory provider 已接受的提炼可能在 Catena 取消后继续，但不发布 Artifact。

`CATENA_JOB_WORKERS=1`、Job/request timeout 默认 20m；lease 30s / heartbeat 5s。请求限流与队列边界沿用既有 Go 实现，lease loss 不能覆盖后继 Worker 的结果。

[当前验收与限制](../../docs/refactors/CATENA_V2.md)。隔离 Docker 数据库测试不等于目标云生产部署验收；真实模型/OpenViking 仍需独立验证。
