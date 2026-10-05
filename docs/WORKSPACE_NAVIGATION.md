# 页面职责

Catena 的主要导航收敛为四项：

| 页面 | 职责 |
| --- | --- |
| 工作台 `/` | 近期动态、关注目标、统一分析表单、分析记录与详情 |
| 经历 `/traces` | 原始 Trace 检索、会话和证据查看 |
| 实验 `/cases` | Case、Harbor 对照执行、实验记录和验证证据 |
| 产出 `/evolution` | 资产阅读、复制、下载、删除与来源回查 |

工作台统一使用 `AgentEvolutionLauncher`，支持 24h / 7d / 30d。
关注目标继承原主动助手的浏览器存储；监控新经历仍只在页面打开时运行。
分析记录在工作台查看，详情 URL 为 `/?agent=...&job=...`，刷新可恢复。
产出的来源分析深链接 `/evolution?job=...` 保留；资产页不再另设分析列表或分析表单。
`/assistant` 和旧 `/evolution?agent=...` 分析入口解析为工作台。

这是页面职责整理，不代表已经完成任意 Skill 到 Harbor Case 的自动转换与验证关联。
实验页目前继续使用已冻结的 `rg-missing-search` Case。
