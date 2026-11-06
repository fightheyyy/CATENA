<p align="center"><img src="frontend/public/catena-mark.svg" width="72" alt="CATENA" /></p>

# CATENA

**Trace-driven Agent Engineering Control Plane**

它从 Agent 的运行证据中发现可改进的问题，通过统一 Codex Agent 分析并生成候选，通过 Harbor 独立验证，并保留可追溯的改进决策。

```text
Trace → Analysis Skill → Problem / Evidence → 人工决定是否优化 → Improvement Skill → Harbor Compare → 人工决定是否采用
```

| 组件 | 负责什么 |
|---|---|
| Tap + ClickHouse | 真实 Agent Trace 的采集、持久化和查询 |
| Go + PostgreSQL | 认证、Owner 隔离、Evidence、持久 Job Queue、Lease / Heartbeat / Fencing、重试 / 取消 / 超时、Artifact 与审查记录 |
| Codex App Server | 统一分析与改进 Agent；显式加载 Skill，调用受控 Harbor Tool |
| 确定性 Workflow | 两次 Probe、候选冻结、独立对照、预算与阶段顺序 |
| Harbor | Agent、Environment、Job、Trial、Verifier 和原始评测产物 |
| React | Agent / Trace、Case / Baseline 配置、Job、Compare、Artifact 和人工决定 |

Codex 负责推理，Skill 指导分析，Tool 提供受控能力，Workflow 控制确定性步骤。当前提供 trace-analysis 与 harness-improvement 两种受控任务模式；改进模式借鉴 Harness² 的 Propose / Compose 思路，不保留其递归搜索、选择或 Library 语义。Harness² 已从运行依赖中移除，没有算法 Registry 或插件 SDK。

Harbor 负责被评测 Agent 的原生执行和 Verifier。内部分析 Runtime 与评测目标分开；当前薄适配仅接入 Codex 目标，没有宣称已交付其他 Agent 的候选注入协议。

工程记忆使用已有 PostgreSQL 中的 Evidence、候选版本、实验和不可变人工决定。OpenViking、Embedding、检索和自动经验回注保持退役。模型选择不构成效果证据；候选冻结后执行新的 Baseline / Candidate 对照，未知费用或统计不补零，人工采用只记录决定，不自动安装。

精确阶段、权限和请求格式见 [改进 Workflow](docs/IMPROVEMENT_WORKFLOW.md)。

## 启动

需要 Docker Engine / Docker Desktop、Compose v2。基础部署包含 Go + React、Python Engine、PostgreSQL 和 ClickHouse 四个服务。

```sh
cp deploy/catena-mvp1/.env.example deploy/catena-mvp1/.env
# 编辑 secret、数据库密码及匹配的 ClickHouse DSN
./deploy/catena-mvp1/demo.sh up
```

打开 [本地工作台](http://localhost:5570)，在「连接」注册 Agent，安装 [Tap](tap/README.md) 接入真实 Trace，并配置自己的 Responses-compatible 模型。

启用优化前，部署者必须注册审核过的 Harbor Case、固定完整文件指纹、记录确认来源，并显式授予 Engine Docker 访问权限。见 [部署指南](deploy/catena-mvp1/README.md) 和 [Codex / Harbor 接口](docs/IMPROVEMENT_WORKFLOW.md)。原始 Trace 不能自动重建一个可复现环境。

## 验证与当前边界

[统一 Runtime 验收与真实证据](docs/refactors/UNIFIED_RUNTIME.md) 区分单元测试、跨进程协议、真实 Docker Verifier 和真实模型实验。Fixture 与参考解不计入 Agent 质量对照；一次真实对照也不能证明普遍提升。历史 [168 次 Skill 对照](evals/trace-skill-benchmark/results/ab-v2.md) 使用手工 Skill，不是当前改进策略的效果证明。

- Owner 隔离已保留；组织共享权限未实现。
- 不确定的 Engine receipt 在重启后拒绝自动重放，避免重复外部费用；receipt/run 卷必须保留。
- 原始 Harbor 文件存于私有运行目录，Web 显示相对位置及指纹。
- Review 决定不可覆盖，已有 Review 的 Job 不能通过普通 API 删除。
- 升级前停旧 Worker 并备份；迁移保留历史表、终态 Job、checkpoint 和 Artifact。见 [Memory 导出说明](docs/OPENVIKING_INTEGRATION.md)。

## 开发

Go 1.25、Python 3.12、Node.js 24、pnpm 10.24。

```sh
(cd backend && go test ./... && go vet ./...)
python3.12 -m venv .venv
.venv/bin/python -m pip install -e 'engine[dev,evaluation]'
(cd engine && ../.venv/bin/ruff check catena_engine tests && ../.venv/bin/python -m pytest)
(cd frontend && pnpm install --frozen-lockfile --ignore-workspace && pnpm test && pnpm build && pnpm test:browser)
```

数据库 Race、迁移与复跑命令见 [Rebirth 验收](docs/refactors/REBIRTH.md)。固定执行版本：Harbor `0.23.0`、Codex `0.160.0`；分析使用 App Server，目标执行由 Harbor 管理。

```text
backend/   Go 控制面与持久队列
frontend/  React 工作流
engine/    改进 Workflow、Skill、Codex App Server、Harbor 接入与执行回执
tap/       端侧采集与导入
evals/     冻结 Case、独立 Verifier 和历史实验
deploy/    四服务部署及显式 Harbor overlay
docs/      当前规格与历史研究；当前职责以 SPEC.md 和 Rebirth 报告为准
```

[当前规格](SPEC.md) · [架构](docs/catena/architecture/README.md) · [Engine 可靠性](docs/CODEX_ENGINE_RUNTIME.md)

Apache-2.0。[Harness²](https://github.com/google-research/harness2) 与 [Harbor](https://github.com/harbor-framework/harbor) 分别提供方法启发和评测基础；Tap 来源见 [第三方声明](tap/THIRD_PARTY_NOTICES.md)。
