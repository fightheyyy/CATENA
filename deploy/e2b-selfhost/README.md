# Catena 自托管 E2B

状态：部署文件已固定版本并通过 Compose 解析；尚未部署，尚未验收 Harbor。

采用官方 E2B Embed 单机评估包。`compose.yaml` 与 `upstream.env.example`
原样来自 `UPSTREAM_REVISION` 指定的 e2b-dev/runtime 提交。
镜像和运行时另有各自版本，由上游环境文件锁定。
上游称其为单机评估包；生产可用性需要另行验证。

## 目标机器

- 专用 Linux x86-64 主机或启用嵌套虚拟化的 VM，推荐 Ubuntu 24.04。
- 可用的 `/dev/kvm`、`/dev/net/tun`，4 KiB 内核页、cgroup v2。
- Docker Engine >=27、Compose >=2.24。
- 上游推荐 12 GiB 内存、20 GiB 空闲磁盘；这还不包含 Catena 本身的容量。
- 默认 hugepages 为沙箱预留 4 GiB；并发增加需重新规划容量。
- 首次部署需要访问 GitHub、Google 镜像和存储端点。

当前 Windows Docker Desktop 的 WSL 环境检查结果为 KVM_ABSENT，不能直接运行。
官方部署会修改主机内核模块、sysctl、网络规则与 hugepages，应放在专用节点。

用户现有服务器为 2 核 / 8 GB，低于上游推荐内存。SSH 地址、KVM 和现有负载尚未知。
不能按默认配置将此包与 Catena、ClickHouse、Postgres 一起直接启动。
上游允许调低 HUGEPAGES，但这只能降低沙箱内存预留，不能消除控制平面的资源开销。
后续先只读检查目标节点，再评估单任务、低内存的试验配置；不承诺生产容量。

## 安装

将本目录放到目标 Linux 节点后执行：

```sh
cp -n upstream.env.example .env
docker compose up -d --wait
```

使用独立节点，与 Catena 的 Compose 项目分开。不要复用 Catena 的数据库。
本包使用 host networking，内部服务存在主机端口，不能直接暴露到公网。
初次操作按上游指南通过 SSH 隧道连接；防火墙只允许必要的可信访问。

```sh
ssh -N -L 3000:127.0.0.1:3000 -L 3001:127.0.0.1:3001 -L 3002:127.0.0.1:3002 -L 5008:127.0.0.1:5008 user@host
```

5008 隧道用于 Harbor 模板 COPY 上传。Dashboard 位于 http://127.0.0.1:3001。
模板上传返回的地址也必须能从 Harbor worker 到达。

## Harbor 连接

自建实例会生成自己的 team API Key，不需要 E2B 云账户或许可证。
在服务器本地将 SDK 配置保存到私有文件，避免输出到共享日志：

```sh
umask 077
docker compose exec -T ready cat /run/e2b/sdk.env > sdk.private.env
```

该文件包含 E2B_API_KEY、E2B_API_URL、E2B_SANDBOX_URL 等安装配置。
Harbor worker 启动前加载这三个 SDK 变量；本机安装的 E2B SDK 2.52.0
支持 URL 环境变量，模板构建与沙箱操作使用同一配置。
通过隧道连接时按实际地址调整 URL 的 hostname，保留端口与协议。
模型不能接触配置文件。不要仅设置 Key，否则 SDK 会使用默认云端地址。

## 验收

1. ready 服务就绪，base 模板可用。
2. 在设置好三个 SDK 变量的环境运行 `smoke.py`：真实创建、写文件、执行、暂停、恢复、销毁。
3. Harbor 使用 environment=e2b 跑三题两组的已有 Case，并确认六次沙箱清理。
4. 保存结果、资源消耗和失败日志后，才将自建 E2B 设为服务器默认。

`smoke.py` 需要 Catena 的 evaluation-e2b Python 依赖。
尚未运行第 1–4 步；Compose 解析不证明镜像可拉取或沙箱可执行。

## 来源

- https://github.com/e2b-dev/runtime/tree/main/embed
- https://github.com/e2b-dev/runtime/blob/main/embed/compose/README.md
- https://e2b.dev/resources/introducing-e2b-embed
