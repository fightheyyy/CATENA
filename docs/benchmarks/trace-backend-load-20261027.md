# Trace 后端合成负载记录

日期：2026-10-27。性质：本机工程验收；不是线上 SLO 或目标服务器容量测试。

## 配置与负载

- Windows 主机：16 个逻辑 CPU，16,312,721,408 字节物理内存；Go 1.25.0，GOMAXPROCS=16。
- ClickHouse 25.10.2.65 official build，在 WSL Linux 文件系统运行；WSL 可见内存约 5.8 GiB。服务内存上限 2 GiB、单查询 1 GiB、max_threads=2，后台 merge pool=2。
- Go HTTP 服务和负载客户端在同一个测试进程，ClickHouse 在本机 WSL。鉴权使用 MemoryStore；未压测 PostgreSQL token 查询。
- 每种配置各 3 轮；每轮独立临时 database，50,000 条合成 Span、10,000 条 Trace；500 Span/请求，共 100 个成功请求。每 Span 输入 1 KiB、输出 2 KiB，内容为重复字符，不能代表私人真实文本的压缩率。
- 每轮 8 个闭环客户端。遇到 429 使用 25–55ms 测试退避，故意施加高于 Retry-After: 1s 建议的重试压力；不是推荐的生产客户端策略。每轮所有 100 个批次最终均成功。
- 轮次顺序：宽松→有界、有界→宽松、宽松→有界。最终冻结轮次独立运行，未同时运行 Go 测试。准备 payload、建库、查询和清理不计入写入耗时。
- 宽松配置为当前代码的 64/64 名额，在 8 客户端下不会触发上限，近似此前无准入限制的路径；没有把它冒称为另一个旧版本二进制。

数据 SHA256：`a848985d4c4e2bb53ddaf2774d6381247f93339be655d2acb68b0dcd7ae10736`。每轮 protobuf 请求体合计 168,359,600 字节；六轮哈希相同。

## 实测结果

下表为三个轮次的中位数；“P95 中位数”指各轮 P95 的中位数，没有把小样本合并为线上分位数。

| 指标 | 宽松 64/64 | 有界：总 4 / owner 2 |
|---|---:|---:|
| 确认写入 Span/s | 40,665.5 | 14,115.8 |
| 成功请求 P95，ms | 132.3 | 99.3 |
| 测试进程峰值 Go Heap，MiB | 505.5 | 366.3 |
| 测试进程峰值 goroutine | 60.0 | 107.0 |
| 429 响应次数 / 轮 | 0.0 | 487.0 |
| 存储/传输错误 / 轮 | 0.0 | 0.0 |
| 列表 100 条，warm P95，ms | 21.3 | 20.0 |
| 详情 5 Span，warm P95，ms | 10.5 | 10.2 |

六轮各确认 50,000 条，`SELECT count() ... FINAL` 与物理计数均为 50,000；共 300,000 条成功写入并读回。没有把重试拒绝的请求算作已接收数据。查询执行一次预热，再采 10 次；保留 FINAL 和完整详情字段。

## 结论与限制

有界配置降低了本次测试的峰值 Go 堆内存和成功请求 P95，同时降低了单 owner 吞吐，增加了重试与连接活动。goroutine 峰值反而增加；本轮没有出现持续增长或任务遗漏，但这不足以证明长期稳定。不能写成“吞吐、内存和延迟全面提升”。

成功请求 P95 不包含此前 429 的等待；Span/s 的分母包含整个重试期间。两者不能混用来宣称用户端总耗时下降。Go 堆包含负载客户端及预生成 payload，不是独立服务 RSS；数据库 RSS/CPU、连接池等待、查询 read_rows/read_bytes 尚未系统采样。

真实私人语料、本机冷缓存、持续开环负载、8 GiB / 2 核部署资源和突发恢复验收仍待测。因此本轮没有新增查询优化比例，也不据此发布生产吞吐承诺。

### 保留的失败试跑

先用 ClickHouse 1 GiB 服务内存上限试跑宽松路径，100 个批次中 96 个成功、4 个返回 503，48,000 条最终可读。ClickHouse 报 MEMORY_LIMIT_EXCEEDED，按 owner 的 DELETE mutation 清理也受内存限制失败。这次试跑没有进入上表，且没有被改写成成功；之后统一使用 2 GiB 上限、每轮独立 database 并 DROP 清理，再完整重跑两组。

WSL 挂载的 Windows 目录还曾导致 ClickHouse part rename Permission denied，数据库测试数据随后迁移至 Linux 路径。没有改动原 Docker 数据卷。

## 复跑

使用独立 ClickHouse 测试实例。Linux 数据目录、资源设置和负载参数应保持一致。以下命令从 control-plane 目录运行：

```powershell
$env:CATENA_CLICKHOUSE_TEST_DSN = 'http://default:test_password@127.0.0.1:8123/catena_test'
$env:CATENA_RUN_TRACE_LOAD_STUDY = '1'
$env:CATENA_TRACE_LOAD_REPORT = 'E:\CATENA\docs\benchmarks\trace-backend-load-recheck.json'
go test ./internal/control -run '^TestTraceBackendLoadStudy$' -count=1 -timeout=12m -v
```

脚本会为每轮创建和删除 `catena_backend_load_<timestamp>` 数据库，不读取生产 CATENA_CLICKHOUSE_DSN，不调用模型。

[原始六轮 JSON](trace-backend-load-20261027.json)。生成入口：`control-plane/internal/control/trace_load_study_test.go`。
