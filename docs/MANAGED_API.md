# Managed API

复用现有 Cookie / 用户 PAT / 签名项目鉴权与 Owner 隔离。本地模式只绑定 `127.0.0.1`；Sandbox 只能访问独立的 capability 模型端口。未启用 `CATENA_MANAGED_RUNNER_URL` 时，管理接口返回 503，不伪造状态。

先使用原 `POST /v1/agents` 注册 Agent，并通过原 `PUT /v1/me/llm-config` 配置 Responses-compatible 模型。原 Agent 注册 / 查询响应不变。

| Method / path | 行为 |
|---|---|
| `POST /v1/managed-agents` | 给已有注册 Agent 创建不可变托管定义 |
| `GET /v1/managed-agents` | 当前 Owner 的定义，最多 100 |
| `POST /v1/agent-sessions` | 创建独立逻辑会话 |
| `GET /v1/agent-sessions` | 最近 100 个未删除会话；关闭的记录仍可按 ID 查询 |
| `GET /v1/agent-sessions/{id}` | 会话及最近 100 个 Run |
| `POST /v1/agent-sessions/{id}/runs` | 持久排队，首次 202，幂等重试 200 |
| `GET /v1/agent-runs/{id}` | 状态、attempt / fence、结果、用量、Trace、Artifact 引用；不返回 capability hash |
| `GET /v1/agent-runs/{id}/events` | SSE；支持 `Last-Event-ID` 或 `?after=N` |
| `POST /v1/agent-runs/{id}/cancel` | 幂等撤销；运行中任务由 Reconciler 隔离回收 |
| `GET /v1/agent-runs/{id}/artifact` | gzip tar checkpoint，带 `X-Content-SHA256`，仅 Owner 可下载 |
| `POST /v1/agent-sessions/{id}/pause` | 空闲会话进入 PAUSING，再调用 Provider pause |
| `POST /v1/agent-sessions/{id}/resume` | 逻辑 READY，必要时 Provider resume；没有 Sandbox 时在下一 Run 重建 |
| `DELETE /v1/agent-sessions/{id}` | 空闲会话进入 DELETING，取消排队任务，确认清理后 DELETED；保留 Run / Artifact |
| `GET /v1/sandboxes` | 当前 Owner 的 Sandbox；活动资源优先，最多 100 |
| `GET /v1/me/resources` | 配额、预留、排队 / 运行数、真实数据库状态与不支持的 metrics 标记 |

示例：

```sh
# 配置好用户鉴权后；本机开发模式无需额外 Cookie。
API=http://127.0.0.1:18789
curl -fsS "$API/v1/managed-agents" -H 'Content-Type: application/json' \
  -d '{"agent_id":"你的注册 Agent ID","instructions":"Work in /workspace and verify changes.","timeout_seconds":600}'
curl -fsS "$API/v1/agent-sessions" -H 'Content-Type: application/json' \
  -d '{"agent_id":"你的注册 Agent ID"}'
curl -fsS "$API/v1/agent-sessions/SESSION_ID/runs" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: my-task-1' \
  -d '{"input":"Create hello.py and run it with python3."}'
curl -N "$API/v1/agent-runs/RUN_ID/events"
```

定义可传 `runtime_type`（仅 codex）、`runtime_version`（仅 0.160.0）、`tool_policy`（仅 sandbox-write）。默认超时 600 秒，允许 10–1800 秒，instructions ≤32 KiB。Agent ID 全局唯一，不同 Owner 无法替已有 Agent 增加定义。

Run input 1–65536 bytes；必须传 1–200 bytes `Idempotency-Key`。同租户 / Session / key / input 返回同一 Run；同 key 不同 input 为 409。资源不足时保持 QUEUED；排队超租户上限为 429。一个 Session 只能有一个 RUNNING Run。运行中 pause / resume / close 为 409，先 cancel 并等待隔离；NEEDS_REVIEW 只能关闭，不会直接重新执行。

SSE 事件按 event_id 递增，单次每批 200 条；连接 25 秒轮换，标准 EventSource 会自动带最后游标重连。每 Run 最多 10000 个持久事件，单事件 ≤64 KiB，浏览器保留最近 500 条显示。历史游标按 Run 过滤，不泄露其他租户数据。POST 的 HTTP 生命周期不等待模型执行完成。

私有接口 `/internal/managed/authorize` 仅接受 SDK adapter secret；`/internal/managed/model/responses` 仅接受活动 Run capability。独立模型 listener 不提供任何其他路径。不要发布 SDK adapter 或 OpenSandbox Server 到公网。
