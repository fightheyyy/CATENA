# V2 Engine 与 Codex CLI

固定 Inspector/Evolution/Reviewer 与 OpenAI Agents SDK 回退已移除。当前 RSI Adapter 使用 `engine/catena_engine/runtime/codex.py` 的通用 JSON transport，固定 Codex CLI 0.160.0，Responses 协议，默认 low effort。独立临时 HOME/CODEX_HOME，禁用 shell、MCP/扩展、宿主 Skill 发现；输出和流大小受限，超时/取消终止进程组并清理临时目录。

Go 创建持久化 Analysis Job，经私有 Bearer token 和 X-Catena-Owner 调用 `/v2/rsi` 或 `/v2/memory/extract`。收到运行 receipt 后轮询 `/v2/executions/{id}`；取消发 DELETE。Engine 一次只允许一个新的分析启动，health/状态/Memory 检索保持可读。provider 提炼接受后不具备 Catena 事务回滚能力。

`CATENA_ENGINE_REQUEST_TIMEOUT` 默认 20m；外层 `CATENA_JOB_TIMEOUT` 默认 20m，实际较早到期者生效。RSI budget.seconds 默认 900，单次 CLI/Harbor 遵守 host 剩余预算。Worker 关闭、lease loss 和超时通过 context 传递；进程组清理由 transport 测试覆盖。

模型由用户 Connections 配置，Go 解密后临时传入；Engine receipt fingerprint 排除 API key，不存请求密钥。重启后的不确定执行不得自动再花费。Engine 状态目录必须持久化、单实例使用；不支持多个 Engine 共享同一 receipt root 的并发保证。

[RSI](RSI_ADAPTERS.md) · [部署](../deploy/catena-mvp1/README.md) · [V2 验收](refactors/CATENA_V2.md)。
