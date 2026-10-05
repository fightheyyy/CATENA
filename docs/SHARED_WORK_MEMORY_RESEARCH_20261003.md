# 共享工作记忆调研

调研日期：2026-10-03。依据官方文档、官方仓库与作者论文；未做接入实测。

## 产品判断

Catena 的目标是跨 coding agent、会话和团队成员共享项目上下文。
在此定义下，建议独立「工作记忆」页面。此前浏览器内关注目标不能代表此能力。
主导航可为：工作台、经历、工作记忆、实验、产出。

## 已核对的做法

| 来源 | 事实 | 对 Catena 的启发 |
| --- | --- | --- |
| [Letta Context Repositories](https://www.letta.com/blog/context-repositories/) | 2026-02-12 发布；本地文件、Git 版本、渐进加载，子 Agent 使用 worktree 合并记忆；可从 Claude Code/Codex 历史初始化 | 项目知识可版本化；变更、冲突与回滚是重要产品能力 |
| [Letta memory blocks](https://docs.letta.com/v1-sdk/memory/memory-blocks) | 多 Agent 可挂载同一个 block，也支持只读 block | 共享记忆需要明确的读写角色；小型固定规则与按需检索可以并存 |
| [Supermemory coding plugin](https://github.com/supermemoryai/opencode-supermemory/blob/main/README.md) | 归一化 Git origin 参与仓库容器标识；多个 coding 工具共享；区分个人与项目；有自动 capture 和 recall | 项目身份应独立于 Agent 名称；同仓库认定不等于成员授权 |
| [Mem0](https://docs.mem0.ai/core-concepts/how-it-works) | add 提取可复用事实，search 按查询返回上下文；user/agent/run/metadata 限定范围；当前自动提取是追加，需要显式 update/delete 纠正旧记忆 | 写入与召回是两条独立管线；不能假设抽取自动解决事实冲突 |
| [Zep](https://help.getzep.com/what-is-context-engineering) | 时间知识图谱记录实体与事实，旧事实可失效并保留历史，检索提供选定上下文 | 记忆要记录适用时间与替代关系；不必首版即引入图数据库 |
| [Claude Code memory](https://code.claude.com/docs/en/memory) | 项目指令与自动记忆分离；auto memory 在本机，同仓库 worktree 共享，不自动跨机器共享 | Catena 的团队/跨工具共享需要额外适配，文件导出是一种途径 |
| [ShareMem 论文](https://arxiv.org/abs/2609.32511) | 2026-09-26 提交的预印本；共享可复用经验，同时从接收用户的记忆获取偏好；本地整理后再合入共享池 | 个人偏好与项目经验分开，共享内容需要整理与接受步骤；论文结果不代表 Catena 效果 |

官方材料证明这是多个产品与研究的明确方向，不足以证明市场普及程度或哪种实现最优。

## 页面边界

- 经历：原始事件与 Trace。
- 工作记忆：当前可用的项目事实、约定、决策、经验和短期交接。
- 实验：对某项方法或记忆进行对照验证。
- 产出：可安装、复制、下载的 Skill 等资产。

工作记忆建议围绕项目导航，而非把各 Agent 的私人记忆混在一起。
页面包含：当前上下文、待确认/冲突更新、版本记录、调用记录。
每条记忆附带：作用域、项目、可读写成员、类型、内容、来源 Trace/证据、版本、状态、适用时间。
短期交接状态与长期项目知识使用不同过期规则。

## Catena 第一阶段建议

继续使用现有 Go/PostgreSQL/ClickHouse 与平台分析引擎。
PostgreSQL 存当前记忆、版本、访问授权、来源与召回日志；ClickHouse 存原始 Trace。
从全文与结构化过滤开始，召回验收后按需补向量；不以增加数据库为验收目标。

OTLP 仅是经历输入。需要另一条上下文输出链路：
1. 平台 Agent 的受控 search/read 工具。
2. 外部 Agent 的 MCP / hooks / 本地同步适配，按客户端能力接入。
3. Markdown / AGENTS.md / CLAUDE.md 导出或同步，优先简短索引和渐进检索。

服务端先做项目权限过滤，再检索，并记录每次返回的记忆版本。
多人写入使用版本检查，冲突进入待处理状态，避免最后一次写入覆盖有效知识。
Trace 中模型推测和工具输出不能自动升级成高优先级指令；候选与已确认内容分开。

首个验收场景：Agent A 的经历形成一条可回查项目记忆；Agent B 新会话检索并使用该版本；
修改约定后旧版本失效；跨项目查询不能返回该内容；页面可以查到写入与使用证据。
Harbor 后续对比有/无记忆，衡量重复错误、完成率、调用次数和 token，而不是用条目数量证明价值。

## 当前代码边界

现有 `/memory` 为 XiaoBaOS 对话记忆兼容功能，不能直接宣称具备上述项目共享权限、跨工具注入与调用审计。
本次完成调研和建议，没有创建新页面或替换旧记忆实现。
