# Managed Agent Infra：一致性修复与真实并发验收

本轮只验收托管、调度、恢复、回收和 Trace 可追踪性。不执行新 Run → Analysis → Candidate → Harbor → Review 的真实质量实验，不构造 Candidate 或评测结果。原分析、改进、Harbor、Review 的代码、API 和数据保留。

## 一致性改动

- 生命周期对账改为 **短事务登记 intent → 事务外 Provider I/O → 按操作 owner / fence / lease 和 Sandbox revision 提交**。慢 list / pause / resume / terminate 不持有 PostgreSQL 事务、连接锁或全局调度锁。并发 Reconciler 不会重复领取活动 intent；进程丢失后可从持久 intent 接管，迟到响应无法提交新代际状态。
- 操作租约、空闲期、TTL 和未知创建等待期在 PostgreSQL 内判定。Provider 超时不释放容量；暂停或恢复结果不确定时，禁止复用旧实例，先确认销毁。终止按不可变 Provider ID 重试；不同动作之间不假设外部 Exactly-once。
- Adapter 缓存 stdout `result`，等官方 SDK 的命令结束与 `exit_code=0` 后才转发退出凭证。Go 还要等真实 EOF 并验证 Run lease / fence，记录 `command_exited_at` 后才提交终态和 checkpoint。取消、超时、租约丢失先进入非终态 `STOPPING`；隔离确认与终态在同一事务提交。同 Session 不能领取下一 Run。
- Trace 按 event_id 每批 100 条导出全部事件。每页使用稳定 Span ID；跨页工具起点与游标一起在 Collector ACK 后持久化，重试不会跳过未确认页。Run 根、模型和工具 Span 与分页事件使用同一 trace_id，复用原 Collector → Kafka → ClickHouse。
- 增量迁移 005 保留旧数据。旧终态不会补造退出证明；原先超过 200 个事件的已导出记录重新入队补发。升级先排空旧 Worker，再升级 Go 与私有 adapter。

## 可复现故障验证

[协议测试结果](managed-consistency/contracts.json)使用真实 PostgreSQL 与可控 HTTP Provider 测试端点；不是虚构的业务 Trace，也不是实际模型性能证据。验证慢 Provider 不阻塞 admission / claim、超时保留容量、并发控制器不重复领取、租约接管后拒绝旧响应、延迟 EOF 不允许重叠执行、Adapter 崩溃不提交结果、过期 Run 拒绝写入，以及取消必须等 Provider 确认。

分页协议测试写入 253 个 Runtime 事件，连同排队和终态共 255 个；工具开始与结束跨页。模拟接收端已存储但 ACK 丢失，重启导出器后从持久游标恢复，逐个核对 event ID、稳定 Span ID 和工具时间。测试 Sink 不连接真实业务 Collector。

[真实 Collector 重试证据](managed-consistency/collector-retry.json)：运行真实任务期间停掉本机专用 Gateway 约 35 秒，4 个真实终态 Run 暂留持久导出队列。重启后通过实际 Kafka / ClickHouse / Go Trace API 读回，逐一匹配 PostgreSQL 中所有 event ID，无缺失、无重复。

[真实功能复测](managed-consistency/session-smoke.json)：三次 Coding Run、同会话复用、pause/resume、Provider 丢失后从 checkpoint 重建以及资源清理。此项和吞吐档位分开。

[真实 Worker 崩溃](managed-consistency/worker-crash.json)：观察到 Codex 正在执行指定的 90 秒前台命令后，对本机测试 Worker 发送 SIGKILL，再启动同一二进制。28.474 秒后确认旧 Sandbox 已终止，Run 在同一事务转为 NEEDS_REVIEW。执行仍是第 1 次，fence 由 1 升到 2，未重放、未发布结果。

## 分级方法

Apple M5 / Mac17,3、10 CPU、16 GiB；Docker Desktop Engine 29.9.2，VM 内存约 7.75 GiB。OpenSandbox SDK / Server 1.2.0、execd / egress release-1.2.0、Codex 0.160.0；模型为已有 Responses 兼容 `gpt-5.6-luna`。

每档同时提交 10 / 20 / 50 个 Run，分到两个独立 Session，允许同 Session 串行复用 Sandbox。**最多两个 Sandbox 和两个活跃 Run，50 是并发提交数。** 每个实例 1 CPU / 1 GiB / 128 PIDs；执行超时 120 秒。Run 要编写含断言的 Python 实现并实际执行，校验 Workspace 文件、JSON 产物、命令退出证据、快照哈希。采用每 Run 至多 4 次模型请求、每次至多 1024 output tokens、租户每分钟 30 次的临时预算；结束恢复原配额。

队列等待为 PostgreSQL `claimed_at - created_at`；执行时长为 `finished_at - execution_started_at`；冷分配时长取真实 `sandbox.ready` 事件，包含 controller / SDK 开销，不含模型响应。P50 / P95 使用 nearest-rank。每档通常只有两次冷分配样本，不能把该 P95 当作通用 SLA。Docker stats 为采样峰值，排队等待与主机调度共同影响耗时。

## 首次完整分级结果

[原始 Run、分布和 Docker 采样](managed-consistency/reuse-graded-runs.json)。三档结束均确认资源预留归零。

| 并发提交 | 进入真实模型执行 | 成功 | 排队 P50 / P95 | 执行 P50 / P95 | 冷分配 P50 / P95 | 模型请求 | 观察到的 input / output tokens |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 10 | 10 | 10/10 (100%) | 30.50 / 67.82 s | 15.45 / 18.55 s | 1.98 / 1.99 s | 20 | 183,509 / 4,036 |
| 20 | 20 | 20/20 (100%) | 76.37 / 157.81 s | 17.41 / 21.29 s | 2.01 / 2.01 s | 40 | 426,147 / 7,892 |
| 50 | 46 | 44/50 (88%) | 195.39 / 383.86 s | 16.90 / 22.91 s | 1.84 / 1.84 s | 90 | 1,258,156 / 17,483 |

50 档两个 Adapter 命令流失去有效退出确认，先 STOPPING，确认销毁后收敛；其会话其余四个排队任务取消，没有被计作实际执行或成功。首批统计为 44/50，未用后续试验覆盖。记录当时出现宿主日志与 Docker VM 内 PostgreSQL 时间偏差；这是现场观察，不是已确认的 Provider 断流根因。随后将所有租约和资源时限判断收敛到 PostgreSQL，并在阻止本机空闲休眠的条件下单独复测 50 档。

另保留两次前置试跑：[3 请求预算 9/10](managed-consistency/preflight-10.json)、[命令流中断 6 成功 / 2 待审 / 2 取消](managed-consistency/transport-failure-10.json)。前者有一个已执行代码的 Run 在补救工具错误后触发请求上限；它仍是失败。后者没有把丢失的退出确认当成成功。两次失败均按既定安全边界隔离并回收。

## 最终 50 档复测

[50 档复测原始结果](managed-consistency/graded-50-awake.json)：50 个请求在 0.156 s 内提交，**50 个 Run 全部进入真实模型执行并通过代码、工具输出、产物和退出凭证验证**。最高 2 个 Sandbox / 2 个活跃 Run，整个队列 468.52 s 结束，预留归零。

| 成功 | 排队 P50 / P95 | 执行 P50 / P95 | 冷分配 P50 / P95 | 模型请求 | 观察到的 input / output tokens |
|---:|---:|---:|---:|---:|---:|
| 50/50 (100%) | 214.06 / 432.28 s | 17.30 / 25.85 s | 2.07 / 2.12 s | 100 | 1,526,757 / 20,078 |

四个档位的 Docker 单实例采样峰值（首轮 10 / 20 / 50、复测 50）分别为 CPU 16.89% / 17.64% / 23.28% / 22.21%，内存 54.36 / 62.21 / 81.00 / 101.50 MiB，PIDs 86 / 72 / 81 / 86。复测期间宿主与数据库时钟偏差采样最大为 4 ms。该次复测未重现首轮断流，无法据此认定断流根因或生产成功率。

[完整实测核对](managed-consistency/trace-coverage.json)：首轮 80 个 Run 与复测 50 个 Run 的 PostgreSQL event ID 和 Trace 查询完全一致，共 1,512 个事件，外加独立 Worker 崩溃 Run 的事件。成功 Run 均有模型 Span、工具 Span 和命令退出凭证；核对 118 组同 Session 的相邻执行，无重叠。最终无活跃、排队或预留资源，也没有残留 Sandbox 或出站 sidecar 容器。真实批次每 Run 最多 12 个事件；255 个事件及跨页 ACK 丢失由独立协议测试覆盖。

## 兼容性回归

- 后端：独立 PostgreSQL 上 `go -C backend test -race -count=1 ./...` 通过，含 Analysis 持久队列、worker、Review 和 Managed 协议；`go -C backend vet ./...` 通过。
- Engine：Python pytest **54 项全过**，包括 Analysis、Improvement、Harbor evidence / RSI Harbor、Managed adapter；Ruff 通过。Harbor 使用既有自动化测试，不运行新的真实对照实验。
- 前端：**38 项**单测与 **13 项**浏览器流程通过，覆盖 Analysis、Compare 和 Human Review；生产构建与 Go 内嵌静态文件一致。Docker Compose 配置校验通过。

## 尚存边界

- 安全：Docker/OpenSandbox 当前仅用于可信任务；Docker 后端没有 secureAccess，也未做生产逃逸审计。短期凭证和终止能阻止继续执行，但不会撤销命令已产生的外部副作用。已执行但结果不明的 Run 进入人工处理，不自动重放。
- 资源：CPU、内存、PID 与模型请求受限；未支持每 Workspace 磁盘总配额、对象存储 GC 或 API 实时利用率。Provider 无法确认资源已隔离时保留 reservation，宁可等待也不超额。模型 token 统计为上游返回的观察值，异常中断请求可能没有 usage。
- 部署：真实实测在单机 Docker 完成。跨节点路由、HA 控制面和网络分区未验收；发布前需排空旧 Worker 并成对升级 Go 与 adapter。
