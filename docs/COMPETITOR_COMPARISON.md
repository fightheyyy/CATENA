# Catena 与 Langfuse、LangSmith：面试用能力对照

核对日期：2026-09-30。此表比较公开文档所述能力与 Catena **当前代码**，不声称独占某类功能，也不把规划写成现状。

| 维度 | Langfuse | LangSmith | Catena 当前 |
| --- | --- | --- | --- |
| Trace 和评测 | 提供 Trace、数据集、在线/离线评测与实验对比；可将 Prompt 版本链接到 Trace。[[1]](https://langfuse.com/docs/evaluation/core-concepts) [[2]](https://langfuse.com/docs/evaluation/experiments/compare-experiments) | 提供 Trace、在线/离线评测、历史数据回测和实验对比。[[3]](https://docs.langchain.com/langsmith/evaluation-types) | Go/ClickHouse 保存 OTLP Trace，Barena Run Bundle 可关联执行证据；尚无新 SPEC 的同条件 Skill 前后实验报告。 |
| 从运行中发现问题 | 有面向 Agent 工作流的观测与评测能力，不能称其只做日志展示。[[1]](https://langfuse.com/docs/evaluation/core-concepts) | 文档描述在线异常发现、生产反馈回流；Issues Agent API 处于 Beta。[[3]](https://docs.langchain.com/langsmith/evaluation-types) [[4]](https://docs.langchain.com/langsmith/smith-api/issues-agent/%5Bbeta%5D-create-the-issues-agent-for-a-session) | Tap 能还原 Codex/Claude Code Turn；离线扫描生成待人工核对的问题信号，尚无产品内持久问题 Inbox。 |
| 候选与回归 | 支持 Prompt 版本、数据集实验与同版本评测；可用自定义代码评测。[[2]](https://langfuse.com/docs/evaluation/experiments/compare-experiments) [[5]](https://langfuse.com/docs/evaluation/experiments/experiments-via-ui) | 支持历史数据回测、代码/模型评测及版本比较。[[3]](https://docs.langchain.com/langsmith/evaluation-types) | Inspector/Evolution/Reviewer 能生成带来源的 Skill 草案；本地配对实验入口已完成，草案到真实 Case 的产品链路与效果数据尚未完成。 |
| 当前可展示的差异 | 通用 AI 应用观测与实验平台 | 通用 Agent 观测、评测与部署生态 | 聚焦 Codex/Claude Code 原生会话解析及本地运行证据；拟把可审查的 Skill 文件与独立回归验证连成更短路径。效果差异须由实验验证。 |

面试时先承认两者都覆盖 Trace→评测的相邻闭环。Catena 的待验证假设是：对**维护编程 Agent 配置**的人，原生会话还原、重复问题审查和可直接带走的 Skill 文件能减少排查和验证成本。当前证据只支持解析、导入和候选生成；尚不能声称更高发现准确率或修复成功率。
