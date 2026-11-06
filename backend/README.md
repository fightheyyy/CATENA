# Go 控制面

负责认证、Owner 隔离、Agent 凭证、OTLP / ClickHouse、Evidence、PostgreSQL 持久队列、Artifact、Compare 和不可变 Review。

| 源码 | 职责 |
|---|---|
| `cmd/catena-server/` | 服务入口 |
| `internal/control/analysis_job.go` | 分析 / 改进准入、冻结证据、调用私有 Engine、保存结果 |
| `internal/control/analysis_worker.go`、`postgres_analysis_queue.go` | Lease、Heartbeat、Fencing、取消、重试与超时 |
| `internal/control/review.go`、`artifact.go` | 对比结果检查、人工决定和历史产物 |
| `internal/control/migrations/` | 追加式数据库迁移；保留历史数据 |
| `internal/control/web/` | 从 `frontend/dist` 同步的嵌入式 Web 构建产物 |

不执行目标 Agent、不实现 Verifier 或第二套 Runner。内部分析与改进交给 Engine，目标执行与评测交给 Harbor。

[架构](../docs/ARCHITECTURE.md) · [Workflow](../docs/IMPROVEMENT_WORKFLOW.md) · [部署](../deploy/catena-mvp1/README.md) · [开发与验证](../CONTRIBUTING.md)
