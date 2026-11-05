<p align="center"><img src="frontend/public/catena-mark.svg" width="72" alt="CATENA" /></p>

# CATENA

**Trace-driven Agent Engineering Control Plane**

它从 Agent 的运行证据中发现可改进的问题，利用 Harness² 生成候选，通过 Harbor 独立验证，并保留可追溯的改进决策。

```text
Agent Trace → Evidence → Case → Harness² → Candidate → Harbor → Compare → Human Review
```

| 组件 | 负责什么 |
|---|---|
| Tap + ClickHouse | 真实 Agent Trace 的采集、持久化和查询 |
| Go + PostgreSQL | 认证、Owner 隔离、Evidence、持久 Job Queue、Lease / Heartbeat / Fencing、重试 / 取消 / 超时、Artifact 与审查记录 |
| Harness² | 候选 Harness 优化；直接调用固定上游版本的原生算法 |
| Harbor | Agent、Environment、Job、Trial、Verifier 和原始评测产物 |
| React | Agent / Trace、Case / Baseline 配置、Job、Compare、Artifact 和人工决定 |

CATENA 不执行自己的 Agent Runner、Verifier 或 Sandbox，不维护多算法 Registry 或 Composition。Memory、OpenViking、Embedding 服务、Memory Graph 和社区入口已退役；未来 Context 只保留显式授权 Markdown / AGENTS.md 的文本输入边界。

候选冻结后，Harbor 分别执行新的 Baseline 和 Candidate。Verifier、Token、耗时、命令退出与原始轨迹支持审查；LLM 自评不算效果证据。复用优化 Case 时明确标记，缺失费用不记为零。人工采用只记录决定，不自动安装候选。

## 启动

需要 Docker Engine / Docker Desktop、Compose v2。基础部署包含 Go + React、Python Engine、PostgreSQL 和 ClickHouse 四个服务。

```sh
cp deploy/catena-mvp1/.env.example deploy/catena-mvp1/.env
# 编辑 secret、数据库密码及匹配的 ClickHouse DSN
./deploy/catena-mvp1/demo.sh up
```

打开 [本地工作台](http://localhost:5570)，在「连接」注册 Agent，安装 [Tap](tap/README.md) 接入真实 Trace，并配置自己的 Responses-compatible 模型。

启用优化前，部署者必须注册审核过的 Harbor Case、固定完整文件指纹、记录确认来源，并显式授予 Engine Docker 访问权限。见 [部署指南](deploy/catena-mvp1/README.md) 和 [Harness² / Harbor 接口](docs/RSI_ADAPTERS.md)。原始 Trace 不能自动重建一个可复现环境。

## 验证与当前边界

[Operation Rebirth 阶段报告与原始证据](docs/refactors/REBIRTH.md) 区分单元测试、跨进程协议、真实 Docker Verifier 和真实模型实验。Fixture 与参考解不计入 Agent 质量对照；一次真实对照也不能证明普遍提升。历史 [168 次 Skill 对照](evals/trace-skill-benchmark/results/ab-v2.md) 使用手工 Skill，不是新 Harness² 闭环的效果证明。

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
.venv/bin/python -m pip install -e 'engine[dev,rsi]'
(cd engine && ../.venv/bin/ruff check catena_engine tests && ../.venv/bin/python -m pytest)
(cd frontend && pnpm install --frozen-lockfile --ignore-workspace && pnpm test && pnpm build && pnpm test:browser)
```

数据库 Race、迁移与复跑命令见 [Rebirth 验收](docs/refactors/REBIRTH.md)。固定上游：Harness² `1cfe254e8d25b15f37048c3730134686abe60335`、Harbor `0.23.0`、Codex CLI `0.160.0`。

```text
backend/   Go 控制面与持久队列
frontend/  React 工作流
engine/    Harness² Integration、Harbor 接入、隔离模型传输与执行回执
tap/       端侧采集与导入
evals/     冻结 Case、独立 Verifier 和历史实验
deploy/    四服务部署及显式 Harbor overlay
docs/      当前规格与历史研究；当前职责以 SPEC.md 和 Rebirth 报告为准
```

[当前规格](SPEC.md) · [架构](docs/catena/architecture/README.md) · [Engine 可靠性](docs/CODEX_ENGINE_RUNTIME.md)

Apache-2.0。[Harness²](https://github.com/google-research/harness2) 与 [Harbor](https://github.com/harbor-framework/harbor) 提供优化和评测基础；Tap 来源见 [第三方声明](tap/THIRD_PARTY_NOTICES.md)。
