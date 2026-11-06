# CATENA 文档

当前流程：Trace → 分析报告 → 人工决定是否优化 → Candidate → Harbor Compare → Human Review。

## 当前文档

| 需要了解什么 | 入口 |
|---|---|
| 产品定位、启动、仓库布局 | [根 README](../README.md) |
| 已实现范围与正确性边界 | [当前规格](../SPEC.md) |
| 模块职责、执行链路与源码 | [架构](ARCHITECTURE.md) |
| 分析、人工启动改进、Case、候选和评测契约 | [Workflow](IMPROVEMENT_WORKFLOW.md) |
| Codex 隔离、权限、超时与执行回执 | [Runtime](CODEX_ENGINE_RUNTIME.md) |
| 页面职责与 URL | [Web 导航](WORKSPACE_NAVIGATION.md) |
| 本地、公网部署及 Harbor Docker 授权 | [部署指南](../deploy/catena-mvp1/README.md) |
| 升级、旧 Memory 数据导出与回滚边界 | [迁移说明](OPENVIKING_INTEGRATION.md) |
| 开发、验证、提交和分支维护 | [贡献指南](../CONTRIBUTING.md) |

## 验证记录

- [分析与人工优化门](refactors/ANALYSIS_GATE.md)：真实 Trace 分析、五项问题、零自动评测。
- [统一 Runtime](refactors/UNIFIED_RUNTIME.md)：真实候选和 Harbor 对比；该候选已由人工拒绝。
- [Rebirth](refactors/REBIRTH.md)：删减、兼容与分阶段证据；描述其提交时的架构。
- [后端负载实验](benchmarks/trace-backend-load-20261027.md)：保留测试口径与原始结果。
- [冻结评测集](../evals/trace-skill-benchmark/README.md)：历史手工 Skill 实验，不代表当前改进策略的效果。

真实模型实验、协议 Fixture、参考解和 UI 测试分别记录，不能互相替代。

## 历史资料

[archive/](archive/README.md) 保存旧产品设计、Memory、RSI、E2B 探索、页面规格和早期验收。
[refactors/](refactors/) 保存迁移过程及原始证据。历史文档中“当前”“已完成”等表述只适用于当时版本。

修改行为时更新对应的当前文档；新增历史记录放入归档或验收记录，不再向 `docs/` 根目录堆叠日期方案。
