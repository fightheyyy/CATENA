# Web 工作流

React 只调用同源 Go API；认证、Job、Artifact 和 Review 的事实来源在后端。

## 页面与源码

| 页面 | 源码 | 职责 |
|---|---|---|
| 工作台 `/` | `src/WorkbenchWorkspace.tsx` | Agent、近期 Trace、分析入口和分析记录 |
| 经历 `/traces` | `src/TraceExplorer.tsx`、`src/TraceNarrative.tsx` | Agent / Session / Trace 检索、执行叙事与原始 Span |
| 产出 `/evolution` | `src/AnalysisWorkspace.tsx` | 报告、人工启动改进、Compare、候选文件和人工审阅 |
| Agent `/agents` | `src/AgentWorkspace.tsx` | 接入状态与观测信息 |
| 连接 `/api-keys` | `src/ApiManagementPage.tsx` | Agent 凭证和 Owner 模型配置 |
| 设置 `/settings` | `src/App.tsx` | 语言、主题和账户 |

`src/navigation.ts` 管理 URL；`src/useWorkspace.ts` 管理按页面加载与刷新；`src/api.ts` 是 API Client。
`src/AgentAnalysisLauncher.tsx` 创建分析，只有完成分析后的显式操作才能创建改进任务。

## 必须保留的行为

- 报告先于历史任务；窄屏历史默认收起。问题分开呈现证据、假设、不确定性，原始 JSON 和引用可展开。
- 分析完成不自动优化。人工选择已确认 Case 与 Baseline，才提交带 `analysis_job_id` 的新改进任务。
- Compare 保留缺失值；不把一次通过写成普遍提升。Review 只记录决定，不安装文件。
- 候选文件可展开、复制和下载；已有 Review 的任务不能删除。删除仍需确认。
- 深链接保持精确的 Trace / Job，刷新和历史导航不能静默切换到其他记录。
- 异步请求支持取消和错误恢复；跨页面 API 失败不能阻断设置等独立入口。
- 密钥不写入页面或日志；凭证复制沿用已有受限流程。
- 中英文、深浅色及 390px 窄屏保持可用；表格在自身容器内横向滚动。

没有 Memory Graph、独立实验页面、三角色分析进度或通用插件工作台。

开发与验证见 [贡献指南](../CONTRIBUTING.md)，后端契约见 [Workflow](../docs/IMPROVEMENT_WORKFLOW.md)。
