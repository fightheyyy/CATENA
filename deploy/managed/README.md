# macOS + Docker Desktop 托管开发环境

这个独立本机配置保留原部署，使用自己的数据库容器 / 卷与端口，不修改已有生产配置。固定 OpenSandbox SDK / Server `1.2.0`、execd / egress `release-1.2.0`、Codex `0.160.0`。完整 Python 依赖固定在 `requirements.txt`；镜像版本和 Node 基础镜像摘要固定。

需要 macOS、运行中的 Docker Desktop、Python 3.12+、Go 1.25+、Node 24 / pnpm 10.24。Docker 建议至少 8 GiB；默认最多 2 个用户 Sandbox，每个 1 CPU / 1 GiB，另有 Provider sidecar 和数据面开销。不要同时启动 100 个容器；10 / 50 / 100 指排队测试档位。

```sh
python3.12 deploy/managed/dev.py setup
python3.12 deploy/managed/dev.py up
python3.12 deploy/managed/dev.py status
# 打开 http://127.0.0.1:18789/agent-sessions
```

setup 构建前端、同步 Go 嵌入资源、构建 Go / Codex 镜像，生成 `.local/managed-dev/` 下权限受限的随机密钥、配置和环境。OpenSandbox Server / SDK adapter 在宿主机独立运行；用户 Agent 代码只在 Sandbox 内执行。Server 自身需要 Docker 权限，用户实例不挂载 Docker Socket、宿主目录或数据库密钥。

up 启动隔离 PostgreSQL、原 Collector → Kafka → ClickHouse 数据面、官方 OpenSandbox Server、私有 adapter 和 Go Worker。Kafka 在本机配置只有一个 Broker，别名用于原 Collector 配置兼容，**没有 HA 承诺**。完整旧生产部署的 3 Broker 配置不变。

| 端口 | 用途 |
|---|---|
| 127.0.0.1:18789 | React / 用户 API |
| 宿主网关 IP:18788 | 仅 Run capability 模型网关；其他路径 404 |
| 127.0.0.1:18792 | 私有 SDK adapter |
| 127.0.0.1:18080 | 官方 OpenSandbox Server |
| 127.0.0.1:42000–42100 | Provider 发布的内部端口 |
| 127.0.0.1:55441 | 独立开发 PostgreSQL |
| 127.0.0.1:18125 | 独立开发 ClickHouse |
| 127.0.0.1:14318 | 原 Collector OTLP 接收端 |

在「连接」注册 Agent、配置你已有的 Responses-compatible 模型，再在「托管会话」创建定义与 Session。Sandbox 不需要购买云服务，真实模型执行需要可用模型凭证。模型 Key 不注入 Sandbox。

```sh
# 真实模型测试，产生文件并验证复用 / pause / resume / rebuild / cleanup。
python3.12 scripts/managed_smoke.py \
  --environment .local/managed-dev/env.json --output .local/managed-dev/acceptance.json

# 真实 Worker 强杀与安全隔离实验；只对本脚本启动的 PID 操作。
python3.12 scripts/managed_recovery_smoke.py \
  --environment .local/managed-dev/env.json --output .local/managed-dev/recovery.json

python3.12 deploy/managed/dev.py cleanup
python3.12 deploy/managed/dev.py down
```

down 先请求回收并等待预留清零，然后停止服务；保留数据库 / Trace 卷及 checkpoint，不执行 `down -v`。若外部创建结果 UNKNOWN 或 Provider 无法确认销毁，会报错并保留服务运行以继续对账；不要把未确认清理当成已删除。OpenSandbox Server 停止时不能依赖其 TTL 后台任务主动清理，因此不能先停 Provider 再假定资源会自动消失。无需资源时先 cleanup。

模型网关出站 IP 由 Docker 内 `getent host.docker.internal` 实测，每次 up 重新校准；Sandbox 只允许这个 IP / 模型端口，默认不允许 DNS、互联网和外部包下载。不要把 `CATENA_MANAGED_CONTROL_URL` 配成无法在该策略下解析的主机名。

旧 Analysis / Improvement / Harbor 仍使用原 Engine 配置。需要在此 Go 实例运行分析时，按原部署指南配置 `CATENA_ENGINE_URL` / `CATENA_ENGINE_TOKEN` 和 Engine 的 Control URL；脚本不会给分析 Engine 自动授予 Docker 权限，也不会将 Harbor Trial 复用为 Session Sandbox。

验证：

```sh
BARENA_TEST_DATABASE_URL='postgres://postgres:local-managed-dev@127.0.0.1:55441/postgres?sslmode=disable' \
  go -C backend test -race ./...
go -C backend vet ./...
PYTHONPATH=engine .local/managed-dev/venv/bin/python -m pytest engine/tests/test_managed_checkpoint.py
pnpm --dir frontend test
pnpm --dir frontend build
pnpm --dir frontend test:browser
```

数据库测试在随机独立 schema 内运行并清理该测试 schema。全 Engine 回归需要原 `engine[dev,evaluation]` 的 Harbor 依赖。Docker 不可用时，仍可使用本机 PostgreSQL 跑数据库测试；真实执行必须单独标为未验收。

[API](../../docs/MANAGED_API.md) · [架构 / 恢复 / 安全边界](../../docs/MANAGED_INFRASTRUCTURE.md) · [真实验收](../../docs/refactors/MANAGED_ACCEPTANCE.md)
