# Managed Infrastructure 实际验收

2026-11-10，macOS + Docker Desktop。此记录区分真实执行、数据库故障契约测试和未验收能力；没有将 SDK probe 或 Mock Provider 当成完整平台验收。

## 环境与固定版本

Apple M5 / Mac17,3，10 CPU，16 GiB 宿主内存；Docker Engine 29.9.2，Docker VM 10 CPU / 8,319,504,384 bytes。OpenSandbox SDK / Server 1.2.0，execd / egress release-1.2.0，Codex 0.160.0，Harbor 0.23.0。真实模型为用户已配置的 Responses-compatible `gpt-5.6-luna`。

使用独立 PostgreSQL 测试容器和随机测试 schema；真实演示使用该容器的专用默认 schema。没有修改原部署的 PostgreSQL / ClickHouse 数据。Kafka / Collector / ClickHouse 使用本轮独立 Compose 项目，复用原 Collector 配置和迁移。

## 真实执行：A / B / F / G

[完整 Run / Artifact 指纹](managed/execution.json) · [Trace 与原始计时](managed/metrics.json)

1. 注册 Agent → 不可变托管定义 → Session → 幂等提交 Run。Go 持久调度并分配官方 OpenSandbox，在其中启动真实 Codex。
2. 第一 Run 创建 `hello.py`，实际执行 Python，将 `hello catena` 写入 `run-output.txt`；下载 checkpoint 并校验 SHA-256 和文件内容。
3. 第二 Run 复用同一 Sandbox 和 Codex thread，追加第二行代码，实际输出 `hello catena\nsecond run`。重复提交同幂等键返回相同 Run。
4. Provider pause / resume 均完成，并等待实际状态收敛。
5. 使用私有 adapter 强制销毁该 Provider 实例；Reconciler 发现丢失并释放容量。第三 Run 建立新 Sandbox，generation 2，从 checkpoint 恢复代码与 Codex conversation，再次执行，得到相同两行输出。
6. 关闭 Session，确认所有相关 Sandbox TERMINATED、会话 DELETED。
7. 三次 Trace 均经过真实 Collector → Kafka → 官方 ClickHouse Exporter，使用 Go `GET /v1/traces/{id}` 读回，分别为 5 / 6 / 5 个 Span，包含 Run / LLM / Shell。

三次 Run 均为 attempt 1 / COMPLETED；这是限定验收样本 3/3，不是平台普遍成功率。每次有 2 次真实模型请求；用量、时间和文件哈希见 JSON。接入调试期间曾出现组件版本、权限编码、Bearer 解析和工具 PATH 问题，已修复并保留本机失败记录；没有将这些失败计入这组成功样本后声称整体 100% 成功。

[真实 UI 运行证据](managed/ui.json)：另通过 ego-browser 在 React 页面实际创建 Session、填写任务、点击排队，实时 SSE 显示完成；下载 Artifact 确认 `ui-proof.txt` 内容为 `managed UI verified`。随后经 UI 关闭 Session，Sandbox Monitor 观察到 TERMINATED。桌面与 390px 移动页面检查完成，移动页面无水平溢出。

## 真实 Worker Crash / Lease / Fencing：D / E / G

[实测记录](managed/recovery.json)，可用 `scripts/managed_recovery_smoke.py` 重复。

在真实 Codex 的长 Shell 工具调用开始后，对本轮启动的 Go PID 执行 SIGKILL。重新启动相同控制面，数据库读取旧 Run / Session / Sandbox，等待旧租约失效。观察到：

- Run fence 1 → 2，attempt 保持 1。
- Run 为 NEEDS_REVIEW，没有伪造 result / checkpoint，也没有重放任务。
- 旧 Provider Sandbox 确认销毁。
- 本次从 SIGKILL 到隔离完成 **24.533 秒**；不是恢复时间 P95，也不是继续原任务成功。

旧 Worker 结果覆盖、旧心跳、旧事件写入和过期授权回调由真实 PostgreSQL 契约测试进一步验证拒绝。Provider 不能在执行侧原子检查数据库 fence，因此不宣称外部工具在 Lease 过期瞬间停止；通过隔离旧实例、撤销模型能力、阻止新执行接管保证保守安全边界。

## 并发 / 配额 / 故障契约：C / H

8 个并发 claimants、全局 capacity 4，在真实 PostgreSQL 运行 10 / 50 / 100 个队列任务。执行体是确定性测试结果，**没有同时运行 100 个 Codex / Sandbox**。

| 排队任务数 | 调度与测试结果持久化耗时 |
|---|---:|
| 10 | 140.255 ms |
| 50 | 389.454 ms |
| 100 | 683.139 ms |

这些数值只衡量测试环境中的调度控制逻辑，不能当作模型吞吐量、容器启动性能或生产 SLA。原日志在 [metrics.json](managed/metrics.json)。

同一数据库契约测试覆盖：20 个并发重复提交仅创建一个 Run；同 key 不同输入冲突；跨租户提交 / 读取 / 活动会话操作拒绝；Session 执行互斥；租户轮转；租户及全局容量；未知创建不释放预留；清理失败保留容量并重试；控制面重开连接后恢复；模型预算、撤销、真实 HTTP credential 隔离与 usage 去重；SSE 游标和幂等关闭。

真实三 Run 的 queue wait nearest-rank P50 **121.155 ms**、P95 **262.427 ms**。两次冷分配（含 controller / SDK 初始化）为 **1991 / 1765 ms**；同 Session 热分配记录为 **1 ms**。样本很小，不据此推断通用性能。资源利用率、100 个真实模型任务成功率、清理成功率分布、恢复 P95 均未测。

## 容器实测约束

[Docker inspect 证据](managed/container-security.json)：privileged=false，无 CapAdd，危险 capabilities 已 drop，no-new-privileges=true，memory=1 GiB、NanoCpus=1e9、PIDs=128。挂载只有 Provider 自己的 execd runtime named volume，没有宿主敏感路径或 Docker Socket。

出站采用官方 dns+nft deny policy，仅允许实测宿主网关 IP 的模型端口。Docker 后端不支持 secureAccess；因此只作为可信本地开发边界，非生产恶意多租户安全承诺。

## 自动化回归：I

- `go test -race ./...`：通过，配置真实 PostgreSQL；覆盖原 Analysis Queue / Worker / receipt / Review 回归及新 Managed 契约。
- `go vet ./...`：通过。
- Python Engine：**52 passed**，包含原 Analysis / Improvement / Harbor 协议回归和 checkpoint 安全测试；真实 Harbor 质量对比未重跑。
- Frontend：**38 单元测试通过**，类型检查与构建通过。
- 浏览器原核心业务：**13 测试通过**，包括原 Trace / Analysis / Compare / Human Review。
- Ruff、Compose 配置检查、前端嵌入构建一致性和 diff whitespace 检查通过。

现有 ClickHouse / 跨进程测试按原环境变量决定是否启用；没有把跳过项记为单独实测通过。新的真实 Trace 查询记录已在上面单独列出。

## 未完成 / 未验证

1. 新 Run → Investigation → Candidate → Harbor → Review 的全套真实质量实验未重跑；仅 Trace 接入实测和旧业务回归通过。
2. Provider 长期断网 / PostgreSQL 断网、真正跨机器 Worker 接管、恶意容器逃逸和跨租户网络渗透未进行真实破坏实验；现有数据库失败路径采用 fail-closed / UNKNOWN。
3. NEEDS_REVIEW 的人工批准继续 UI、任意中间工具恢复、对象存储 / GC / 总磁盘配额、费用计价、实时资源利用率、完整多节点部署尚未交付。
4. 初期 SDK 版本调试留下 1 条 UNKNOWN 创建预留（没有仍运行的用户 Sandbox），控制器保留到硬 TTL + grace，不通过强行释放预留伪造清理成功。
5. 本机脚本 setup、独立 Compose 和等价进程配置已验证；完整 default `dev.py up/down` 对新目录的端到端启动 / 停止未另做一轮，避免与保留的演示端口冲突。实际模型执行与清理使用同样 SDK / Server / Go / 镜像及私有配置。

没有推送 GitHub，没有修改生产配置或删除原业务数据。初始本地改动备份仍在原 stash，未与本轮新架构强行合并。
