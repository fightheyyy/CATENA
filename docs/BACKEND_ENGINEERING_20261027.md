# Go 后端改造与验收

日期：2026-10-27。范围是 Evolution 分析任务和 OTLP 接入。Harbor Experiment、旧 Barena Run、记忆服务的调度没有迁移到本队列。

## 1. 可靠任务执行

```mermaid
flowchart LR
    API[创建分析任务] -->|幂等键 + 请求指纹| PG[(PostgreSQL)]
    PG -->|短事务 SKIP LOCKED| W[有界 Go Worker]
    W -->|事务外 HTTP 调用| E[现有分析引擎]
    W -->|心跳 + 阶段检查点 + fencing token| PG
    PG -->|过期接管 / 有界重试| W
    UI[分析页面] -->|取消意图 / 查询状态| PG
```

- 创建请求先持久化，HTTP 请求结束不影响任务生命周期。启动时 Worker 会领取数据库中待执行或租约已过期的任务。
- 每次领取增加 `attempt` 和 fencing token；续租、阶段保存、终态保存、释放都核对租约。旧 Worker 不能覆盖新 Worker 的提交。
- 网络错误、HTTP 429/5xx、阶段持久化失败和超时允许有界重试。确定性输入错误与其他 HTTP 4xx 不盲目重试。
- 已提交的阶段输出会复用；阶段 request ID、候选 ID 保持稳定。尚未提交的外部调用可能重做并重复计费，调度保证是 **at-least-once**。
- 取消是持久化意图。运行中任务通过心跳确认并停止；排队或租约已过期的任务可以直接取消。页面区分“正在取消”与“已取消”。
- 队列最多 1,000 个活动任务、每 owner 最多 100 个。已存在的同指纹幂等请求不受满队列影响；新的超额请求返回 429。终态任务不占活动容量。
- PostgreSQL 连接池最多 16 个连接、空闲最多 8 个；等待受调用 context 控制。领取事务不持有外部模型调用。

### 默认配置

| 配置 | 默认值 |
|---|---:|
| `CATENA_JOB_WORKERS` | 2 |
| `CATENA_JOB_LEASE_TTL` | 30s |
| `CATENA_JOB_HEARTBEAT` | 5s |
| `CATENA_JOB_POLL_INTERVAL` | 500ms |
| `CATENA_JOB_TIMEOUT` | 10m |
| `CATENA_JOB_MAX_ATTEMPTS` | 3，含首次执行 |
| `CATENA_JOB_RETRY_BASE` / `CATENA_JOB_RETRY_MAX` | 1s / 30s |

退避使用半幅到全幅抖动。数据库中断时，新建任务通知不会跳过退避。各副本的 Worker 上限会累加；队列容量由数据库事务共同约束。

### API 与运行证据

- `POST /v1/evolution-jobs/{id}/cancel`：要求当前用户拥有任务；本地无登录模式延续现有可查看本地任务的语义。
- `GET /v1/evolution-jobs/queue`：登录模式仅返回当前 owner 的排队、运行、过期租约、取消、终态计数和最早排队时间；本地模式显示本地可见总队列。
- Worker 记录领取、attempt、fence、任务年龄、执行时长和恢复事件。Go 原子计数目前用于进程内验收，尚未导出 Prometheus 指标。

### 已通过的故障测试

| 场景 | 验收内容 |
|---|---|
| 并发领取 | 8 个竞争者处理 24 个任务，无重复有效领取；完成后无遗漏 |
| 有界执行 | 2 个 Worker 排空 12 个已持久任务，执行峰值不超过 2 |
| OS 进程强杀 | 领取后、外部调用完成但未提交、检查点提交后三个窗口分别强杀；租约过期后接管 |
| 旧 Worker 恢复 | 旧 token 的心跳、保存、释放及旧更新入口均不能覆盖接管结果 |
| 数据库中断 | TCP 代理切断真实 PostgreSQL 连接 400ms；执行停止、领取退避，恢复后 attempt 2 完成 |
| 阶段重试 | 分别在 Evolution、Reviewer 首次返回 503；之前已提交的阶段不重复调用 |
| 取消与耗尽 | 排队取消、执行中取消、完成/取消竞态、最后一次尝试崩溃均进入终态 |
| 旧表升级 | 保留旧 queued/running 行，移除调度列并恢复旧 CHECK 后，连续迁移两次；两种旧任务均可领取完成 |
| 幂等与隔离 | 满队列仍返回已有幂等请求；不同请求冲突；队列统计与取消校验 owner |

测试使用 PostgreSQL 17.6 和可控执行器，不调用私人模型、不消耗模型 token。进程恢复的超时设置是加速测试配置，不是生产恢复 SLA。

## 2. Trace 接入与查询

鉴权后、读取请求体前申请进程级和 owner 级并发名额。没有等待队列；超额请求返回 `429 + Retry-After: 1`。客户端应退避并重试。

| 配置 | 默认值 |
|---|---:|
| `CATENA_INGEST_CONCURRENCY` | 每进程 4 |
| `CATENA_INGEST_PER_OWNER` | 每 owner 2 |
| `CATENA_INGEST_MAX_SPANS` | 每请求 4,096 |
| `CATENA_INGEST_TIMEOUT` | 15s |
| 请求体字节上限 | 16 MiB |

超出 Span/字节上限返回 413，应拆批。请求体读取有截止时间，数据库写入使用请求 context。成功响应仍在 ClickHouse `Send()` 成功后返回；没有提前确认的内存队列。

Span 数上限在解码后检查。解码内存首先由字节上限与并发上限约束，尚未加入 protobuf wire 预扫描或按实际字节加权的内存预算。鉴权发生在接入名额之前，本名额不等于覆盖全站鉴权的速率限制。

过载不读请求体、写入失败不返回成功、取消传递、名额释放及 owner 限额已测试。ClickHouse 集成验证覆盖重复版本、租户隔离、稳定排序、时间窗和完整详情读取。没有删除 `FINAL` 或丢弃宽字段以换取指标。

本轮保留现有查询实现。三轮合成负载、查询测量及限制见 [负载报告](benchmarks/trace-backend-load-20261027.md)；这些数字不是 2 核 8 GiB 服务器的容量结论。

## 3. 本地验收与 CI

在 `control-plane` 目录执行，两个连接必须指向**独立测试数据库**：测试会创建表、写合成行、创建/删除临时 schema 或 database。

```powershell
$env:BARENA_TEST_DATABASE_URL = 'postgres://test_user:test_password@127.0.0.1:5432/catena_test?sslmode=disable'
$env:CATENA_CLICKHOUSE_TEST_DSN = 'http://default:test_password@127.0.0.1:8123/catena_test'
go test ./...
go vet ./...
go test -race ./...
```

Windows 的 race 检查需要 GCC/CGO。当前本地使用 Go 1.25.0、w64devkit 2.10.1；CI 在 Ubuntu 执行。CI 已增加 PostgreSQL 17 和固定版本 ClickHouse 服务，使数据库集成测试不再因缺少连接而跳过。

本地 Web 验收：57 条单元测试、17 条浏览器回归、TypeScript 检查和构建通过。浏览器测试使用可控 HTTP fixture；它验证交互状态，不替代真实数据库故障测试。构建产物已同步到 Go embedded Web，并按文件哈希核对。

本地数据库集成的全量 `go test ./...`、`go vet ./...`、`go test -race ./...` 已通过。远端 CI 本轮尚未运行，不能据此称 GitHub 构建已绿。

## 4. 升级与回滚

1. 保存 PostgreSQL 备份、ClickHouse 数据和当前镜像；数据库备份需在受控环境验证恢复。
2. **停止所有旧 core 副本**，再启动包含新迁移与 Worker 的版本。旧二进制不带 fencing 条件，不能混跑。
3. 新版本自动增加调度列、更新取消状态 CHECK、创建领取索引。旧 running 行没有有效租约，会被新 Worker 接管；可能重做未提交的模型调用。
4. 核对队列状态、已完成任务、OTLP 小批写入和读取，再开放流量。保留相同加密主密钥和引擎连接配置。
5. 本版未提供向旧调度语义的自动降级。不要在新队列活动时直接切回旧镜像；停止新 Worker、排空/核对活动任务并使用经过验证的备份恢复方案。

当前 Docker Desktop 仍有旧启动故障，本轮使用独立原生 PostgreSQL 和 WSL Linux 文件系统中的 ClickHouse 完成数据库验收，没有修改原有 Docker 数据卷。Compose 配置静态校验通过；尚未完成新镜像构建、空环境全栈启动、真实旧数据备份恢复和目标服务器压测，因此 **尚未发布 v0.1**。
