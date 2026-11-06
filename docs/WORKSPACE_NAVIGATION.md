# 页面职责

主导航只有工作台、经历、产出。Agent、连接、设置为辅助入口。

| 页面 | 路径 | 职责 |
|---|---|---|
| 工作台 | `/` | 选择 Agent / 时间窗、创建分析、近期 Trace 与任务 |
| 经历 | `/traces?agent=…&trace=…` | 精确 Trace、会话层次、证据与 Span |
| 产出 | `/evolution?job=…` | 分析报告、显式启动优化、候选、Compare、人工审阅 |
| Agent | `/agents?agent=…` | 接入状态与历史信息 |
| 连接 | `/api-keys` | Agent 凭证与模型配置 |
| 设置 | `/settings` | 语言、主题、账户 |

路由实现见 [`navigation.ts`](../frontend/src/navigation.ts)。`/assistant` 和只有 Agent 参数的旧 `/evolution?agent=…` 入口归到工作台；不存在独立 `/cases` 页面。

分析与改进共用 Job 展示，但分两次显式提交。分析完成后可以仅保留报告；没有自动优化或自动采用。
详情与页面边界见 [Web 规格](../frontend/SPEC.md)。
