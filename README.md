<p align="center"><img src="frontend/public/catena-mark.svg" width="72" alt="CATENA" /></p>

# CATENA

**Trace-driven Agent Engineering Control Plane**

CATENA 从真实 Agent Trace 中定位问题，由人工决定是否生成候选，再通过 Harbor 独立验证并记录改进决策。

```text
Agent Trace → Evidence → Codex Analysis → Problem / Evidence
                                               │
                                        人工决定是否优化
                                               ↓
                    Codex Improvement → 冻结 Candidate
                                               ↓
                           Harbor Baseline / Candidate
                                               ↓
                                     Compare → Human Review
```

## 产品范围

面向维护 Coding Agent 的开发者，当前只围绕三个用户动作交付：

| 用户动作 | 当前入口 | 得到什么 |
|---|---|---|
| 找到值得调查的执行记录 | 经历 `/traces` | 原始 Trace、工具调用、错误和执行上下文 |
| 调查问题，决定是否优化 | 产出 `/evolution?job=…` 的分析报告 | 问题、证据引用、根因假设和不确定性 |
| 验证候选，决定采纳或拒绝 | 同一入口的改进任务 | 候选文件、Harbor 对比、人工决定 |

调查报告和改进实验目前共用产出页，不是三个独立工作台。连接、Agent 和设置只提供必要的接入与配置。

当前固定 **Go 控制面 + Codex 内部 Agent + Harbor 评测**。Skill 是内部策略扩展手段，不建设插件市场；工程记忆先保留版本化记录。暂不扩展新的 Runtime、Memory 检索、自动采用或通用 Code Patch。

下一阶段只解决核心使用障碍和真实案例验证：让一位外部开发者在没有作者陪同的情况下，完成一次 Trace → 调查 → 人工选择优化 → 评测 → Review。这个目标尚未验收。

## 架构

| 模块 | 职责 |
|---|---|
| Tap + ClickHouse | 采集、存储和查询真实 Trace |
| Go + PostgreSQL | 认证、Owner 隔离、Evidence、持久 Job Queue、Artifact、版本与人工决定 |
| Codex App Server | 同一个 Runtime；Analysis 通过 Trace 工具自主多轮取证，Improvement 生成候选 |
| CATENA Workflow | 明确阶段、预算、候选冻结与对照顺序 |
| Harbor | 目标 Agent、Environment、Job、Trial、Verifier 与执行产物 |
| React | Trace 浏览、报告、人工启动优化、Compare 和 Review |

分析先用 `list_traces` / `get_trace` 查询证据、检验假设，再输出问题报告；不会自动进入优化。候选当前支持 `AGENTS.md` 和 Skill 文本；采用只记录决定，不自动安装文件。
Go 保留 Lease / Heartbeat / Fencing、幂等、重试、取消和超时。工程记忆目前是可追溯的 Evidence / Candidate / Compare / Review 记录，没有检索或自动回注。

Harness² 只提供 Propose / Compose 方法启发，已不是运行依赖。OpenViking 和旧 Memory API/UI 已退役。

## 启动

需要 Docker Engine / Docker Desktop 和 Compose v2。常驻服务为 Go + React、Python Engine、PostgreSQL、ClickHouse。

```sh
cp deploy/catena-mvp1/.env.example deploy/catena-mvp1/.env
# 配置 secret、数据库密码及匹配的 ClickHouse DSN
./deploy/catena-mvp1/demo.sh up
```

打开 [localhost:5570](http://localhost:5570)，在「连接」注册 Agent、配置自己的 Responses-compatible 模型，并安装 [Tap](tap/README.md) 接入 Trace。
分析不需要 Harbor Case；优化需要部署者确认并注册 Case、固定文件指纹、显式授予 Engine Docker 访问权限。完整步骤见 [部署指南](deploy/catena-mvp1/README.md)。

升级前停止旧 Worker，并备份 PostgreSQL、ClickHouse 与 Engine receipt/run 卷；不确定执行不能盲目重放。旧 Memory 数据按 [迁移说明](docs/OPENVIKING_INTEGRATION.md) 保留和导出。

## 仓库

```text
backend/   Go 控制面、持久队列和嵌入式 Web
frontend/  React 页面与浏览器检查
engine/    Codex Runtime、Skills、确定性 Workflow 和 Harbor 接入
tap/       端侧 Trace 采集与导入
evals/     冻结 Case、Verifier 和实验结果
deploy/    部署配置
scripts/   维护工具
docs/      当前文档；archive/ 保存旧方案，refactors/ 保存迁移证据
```

[文档索引](docs/README.md) · [当前规格](SPEC.md) · [架构与源码](docs/ARCHITECTURE.md) · [Workflow](docs/IMPROVEMENT_WORKFLOW.md) · [开发与维护](CONTRIBUTING.md)

## 验证状态

- [真实 Trace 调查](docs/refactors/TRACE_INVESTIGATION.md)：7 次工具调用，读取单条 Trace 的 95/145 个 Span，保存 3 个问题，不自动启动优化。
- [真实候选与 Harbor 对比](docs/refactors/UNIFIED_RUNTIME.md)：工程链路已运行，该候选已由人工拒绝。
- 调查与候选评测的证据来自不同次实验；尚未验证外部用户独立完成当前版本全流程，也未证明归因持续有效或候选普遍提升。
- Fixture、协议测试、UI 测试和参考解不属于 Agent 质量证据。一次对照也不能证明普遍提升。
- 当前目标薄适配为 Codex；组织共享权限、通用 Code Patch 和自动经验检索未交付。Docker socket 仅供可信部署者使用。

固定执行版本：Codex `0.160.0`、Harbor `0.23.0`。开发工具和验证命令见 [贡献指南](CONTRIBUTING.md)。

Apache-2.0，见 [LICENSE](LICENSE.md)。[Harness²](https://github.com/google-research/harness2) 提供方法启发，[Harbor](https://github.com/harbor-framework/harbor) 提供评测基础；Tap 来源见 [第三方声明](tap/THIRD_PARTY_NOTICES.md)。
