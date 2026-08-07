<p align="center">
  <img src="catena-web/public/catena-mark.svg" alt="Catena" width="88" />
</p>

<h1 align="center">Catena</h1>

<p align="center"><strong>以 Trace 为燃料的 Agent 持续进化平台</strong></p>

<p align="center">
  Observe what Agents actually did. Turn repeated evidence into better agent.md, Skills, Roles, and Harnesses.
</p>

Catena 汇聚 XiaoBaOS、Codex、Claude Code 和各类 Claw 的真实运行证据，从一段时间的 Trace 与对话中发现重复问题，生成可追溯、可人工采用的 Agent 进化候选。

```text
OTLP Trace ─┐
Conversation ├─→ Evidence ─→ XiaoBaOS Evolution Runtime ─→ Agent assets
Barena Run ──┘
```

## 核心能力

- **Agent**：把同一 Runtime 的多个 telemetry source 聚合为一个可理解的 Agent。
- **对话**：保存 XiaoBaOS 用户真正看到的消息，用于提炼记忆与角色知识。
- **Trace**：用极简瀑布查看模型、Tool、Artifact、耗时和错误。
- **Trace Farm**：按 Agent 与时间窗口分析多条 Trace，展示 Inspector、Evolution、Reviewer 三个阶段。
- **进化产物**：输出标准 `agent.md`、Skill、Role，以及 XiaoBaOS 专属 Harness 建议。
- **记忆**：从对话生成可追溯记忆，并支持语义、关系与时间召回。

Catena 不托管或冒充用户的目标 Agent。平台内置的 XiaoBaOS Runtime 只消费 Evidence，不执行被测 Agent。

## 产品边界

```mermaid
flowchart LR
    GitHub["GitHub<br/>OAuth · 开发者身份"] -->|"登录"| Core

    subgraph Edge["用户环境 / CI"]
        Agent["目标 Agent<br/>Codex · Claude Code · Claw"]
        XiaoBa["XiaoBaOS<br/>拟人化 Agent"]
        Barena["Barena<br/>Explore · Replay · Compare · Verifier"]
        Trace["运行证据<br/>OTLP Trace · Artifact"]
        Conversation["用户可见对话<br/>user · delivered assistant"]

        Barena <--> Agent
        Barena <--> XiaoBa
        Agent --> Trace
        XiaoBa --> Trace
        XiaoBa --> Conversation
        Barena --> Trace
    end

    subgraph Cloud["Catena"]
        Core["Go + React 控制面<br/>身份 · Agent · API Key"]
        Facts["Evidence Store<br/>Trace · Conversation · Run"]
        Farm["Trace Farm<br/>跨 Run 问题发现"]
        Runtime["XiaoBaOS Evolution Runtime<br/>Inspector · Evolution · Reviewer"]
        Assets["候选产物<br/>agent.md · Skill · Role · Harness"]
        GauzMem["GauzMem<br/>记忆编译 · 召回 · 图谱"]
        Memories["长期记忆<br/>语义 · 关系 · 时间"]

        Core --> Facts
        Facts --> Farm --> Runtime --> Assets
        Facts -->|"用户可见对话"| GauzMem --> Memories
        Assets --> Facts
    end

    Trace -->|"OTLP / Run Bundle"| Core
    Conversation -->|"Conversation API"| Core
```

[Barena](https://github.com/fightheyyy/barena) 是端侧 Agent E2E 与发布 CI 引擎，负责 Explore、Replay、Compare 和确定性验证；Catena 负责长期证据、跨 Run 分析与进化候选。

## 架构

| 服务 | 职责 |
| --- | --- |
| `catena-core` | Go 控制面、React Web、GitHub OAuth、API Key、OTLP、Conversation 与产品 API |
| `catena-runner` | 内置 XiaoBaOS Evolution Runtime，不运行目标 Agent |
| `postgres` | 用户、Agent、Run、Job、Candidate 与审计事实 |
| `clickhouse` | Trace 与 Span 时序存储 |
| `caddy` | 公开部署的 HTTPS 与安全响应头 |

可选 `memory` profile 会增加 GauzMem、MySQL 与 Neo4j。

## 本地运行

要求 Docker Desktop、Docker Compose 与 BuildKit：

```bash
git clone https://github.com/fightheyyy/CATENA.git
cd CATENA
./deploy/catena-mvp1/demo.sh up
```

打开 <http://127.0.0.1:5570>。

```bash
./deploy/catena-mvp1/demo.sh smoke
./deploy/catena-mvp1/demo.sh logs
./deploy/catena-mvp1/demo.sh down
```

## 接入任意 OTel Agent

在设置页创建 API Key，然后配置 OTLP/HTTP exporter：

```bash
export CATENA_URL='http://127.0.0.1:5570'
export CATENA_API_KEY='barena_pat_...'
export OTEL_SERVICE_NAME='my-agent'
export OTEL_TRACES_EXPORTER='otlp'
export OTEL_EXPORTER_OTLP_PROTOCOL='http/protobuf'
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="${CATENA_URL}/v1/otlp/v1/traces"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${CATENA_API_KEY}"
```

普通 OTLP Trace 足以进行观测与跨 Run 分析；Barena Run Bundle 可补充 Artifact、Verifier 与发布结论。

## 接入 XiaoBaOS 对话

XiaoBaOS 使用同一个 API Key 增量同步用户可见的 Conversation Journal：

```bash
export CATENA_BASE_URL="${CATENA_URL}"
export CATENA_API_KEY="${CATENA_API_KEY}"
export XIAOBA_CONVERSATION_AGENT_ID='my-xiaoba'
xiaoba chat
```

Conversation 是记忆与角色知识的燃料；Trace 是 Tool、Runtime 与 Harness 分析的燃料。

### 为什么 XiaoBaOS 单独同步用户可见对话

XiaoBaOS 走的是“拟人化工作同事”路线：用户关心的不只是一次任务是否完成，还包括它是否记得长期偏好、人物关系、共同经历和沟通习惯。因此，形成记忆的事实源应该是用户真正参与并看到的对话——用户发出的消息，以及已经成功送达的 Agent 文本或文件回复。

系统 Prompt、隐藏推理、Tool 调用和失败重试仍然进入 Trace，用来诊断 Runtime 与 Harness；它们不应被当成用户经历直接写入长期记忆。Catena 因而保留两条独立的数据路径：

```text
OTLP Trace          → Trace Farm → agent.md / Skill / Role / Harness
XiaoBaOS Conversation → GauzMem   → semantic / graph / temporal memory
```

MVP1 先为 XiaoBaOS 提供第一方 Conversation 协议，因为它具备稳定的用户可见消息日志；其他 Agent 若能提供同等语义的对话事件，后续也可以通过 Conversation Adapter 接入。

## 公开单机 Beta

```bash
cp deploy/catena-mvp1/.env.public.example deploy/catena-mvp1/.env
# 配置域名、GitHub OAuth、随机密钥和模型 Provider
./deploy/catena-mvp1/public.sh config
./deploy/catena-mvp1/public.sh up
```

公开模式只暴露 Caddy 的 80/443；控制面、数据库和 Runtime 留在私有 Docker 网络。详见 [部署文档](./deploy/catena-mvp1/README.md)。

## 状态与开发

MVP1 已覆盖 GitHub 登录、个人 API Key、OTLP 导入、Agent 聚合、Span 瀑布、XiaoBaOS Conversation、Trace Farm、进化候选与中英文 UI。当前定位是 single-node Beta；多 Worker lease、备份恢复、配额与完整 RBAC 尚未完成。

```bash
cd catena-web && pnpm install --frozen-lockfile --ignore-workspace && pnpm test && pnpm typecheck && pnpm build
cd ../control-plane && go test ./... && go vet ./...
docker compose -f deploy/catena-mvp1/compose.yml config >/dev/null
```

架构契约见 [SPEC.md](./SPEC.md)，剩余工作见 [PLAN.md](./PLAN.md)，演示证据见 [MVP1 验收记录](./docs/acceptance/CATENA_MVP1_DEMO.md)。

## License

Apache License 2.0。第三方归属见 [NOTICE](./NOTICE)。
