# Catena / Barena × Harbor / Laminar 采用决策与验证记录

日期:2026-09-25
状态:已验证(本机冒烟通过),进入采用阶段

## 1. 结论

**Laminar 与 Harbor 是 Catena/Barena 的"0→1 地基"的满分实现;Catena/Barena 的 1→100 = 建在其上、而它们没有的层。**

| | 底座(采用,不再自研) | 自研层(1→100 全部人力) |
|---|---|---|
| Catena | Laminar:OTLP 摄取、ClickHouse 存储、检索、datasets/evals、Signals | Trace Farm 跨 run 归因、GEPA 进化 Runtime、GauzMem 记忆、候选资产闭环 |
| Barena | Harbor:任务格式、40+ agent 适配、25 种环境、rewardkit、job_diff、registry | Run Bundle 确定性 Replay、release truth 门禁(cleared/held/rejected)、证据契约 |

集成走边界契约(OTLP / Run Bundle / ClickHouse 直查 / MCP / evals CLI),**不做上游 PR 式捐赠**;上游只提小修(见 §6)。

## 2. 本机已验证事实

- **Laminar 本地部署**:`E:\lmnr`(docker compose 轻量版),前端 <http://localhost:5667>,OTLP `http://127.0.0.1:8000/v1/traces`(Bearer 认证,与 catena_tap 导出器协议同构,零改动直连)。
  - 账号 `dev@catena.local`(better-auth local-email,输邮箱即登录),project `Catena` (id `b5379606-a362-4a14-907f-cd5de667c4d5`),ingest key 名 `catena-tap`。
  - 已导入 Codex 会话 284/285:2,360 traces / 102,310 spans(2025-10-10 ~ 2026-09-24)。
- **Harbor 本地部署**:包名 `harbor`(uv tool install,清华源),CLI v0.23.0(`harbor`/`hb`/`hr`)。
  - 冒烟:`examples/tasks/hello-alpine` + `oracle` agent + docker env → **reward 1.0**。
  - 产物结构(E:\tmp\harbor-jobs\<job>\):job 级 `config.json`/`lock.json`/`result.json`;trial 级 `trial.log`、`agent/`、`verifier/ctrf.json`(CTRF 报告)、`verifier/reward.txt`、`artifacts/`。
- **全环打通**:`harbor-atif2otel` 把 ATIF 轨迹转 OTLP JSON(fixture → 24 spans:agent → model call → tool call),POST 到 Laminar `/v1/traces` HTTP 200 入库。管道:**Barena(Harbor)trial → atif2otel → Laminar** 现成可用。

## 3. Barena 三功能 × Harbor 对照

| Barena 功能 | Harbor 对应 | 判定 |
|---|---|---|
| Explore | Job/Trial 执行模型 + 40+ agent 适配器 + 25 环境 | ✅ 全覆盖且更强 |
| Compare | `job_diff.py` + `TrialLock`(REUSE/REGRADE/RERUN 语义版本判定) | ✅ 原型级,补 release-truth 语义即可 |
| Replay | `harbor job resume`(中断恢复)+ `multi_step.resume_trajectory`(轨迹续跑) | ⚠️ 近亲;**从 Run Bundle 确定性重放**为自研一等功能 |
| 确定性验证 | rewardkit(criteria DSL + reward.toml 加权 → reward.json) | ✅ 全覆盖,直接采用 |

收编后 Barena 定位:**Harbor 之上的发布决策层**——所有 Harbor 用户有分数,只有 Barena 用户有"这个 agent 版本能不能发"。

## 4. 迁移顺序(已排定)

1. **Barena 收编 Harbor**:XiaoBaOS 写 Harbor adapter(照 `agents/installed/claude_code.py` 模式);任务格式迁为 Harbor task 目录;verifier 落 rewardkit;Compare 建 job_diff 之上。
2. **沙箱决策**:默认 `--env docker`;Seatbelt/Bubblewrap 保留为进程级收紧层(`sandbox_engine` 抽象加 `docker` 值);E2B 等云 provider 是远期云端多租户选项,现在不引入。
3. **Catena 证据层**:保留自有 ClickHouse 业务表(Run Bundle、候选、记忆),OTLP 入口走 Laminar;Tap/评测轨迹经 atif2otel 汇入同一 Laminar。
4. **进化引擎**:XiaoBaOS Runtime 接 GEPA(prompt/Skill 类候选),evaluator 用 Barena verifier;OpenEvolve(代码类)与 Agent Lightning(权重类)为后续路线。
5. **上游小 PR**(好感+练手):修复 frontend 镜像缺失 `lib/quickwit/indexes/`(自托管首启永不建 spans_v2 的 bug);文档 PR(HTTP_PAYLOAD_LIMIT、Codex/Claude Code 接入指南)。

## 5. 本机踩坑记录(复现时直接绕开)

- Docker Hub 被 DNS 污染 → `~/.docker/daemon.json` 加 `registry-mirrors`(docker.m.daocloud.io / docker.1ms.run);ghcr.io 直连正常。勿用本机 Clash(Docker Hub 路由半死)。
- Laminar HTTP payload 默认 5MB → `E:\lmnr\docker-compose.yml` app-server 加 `HTTP_PAYLOAD_LIMIT: 104857600`(已改)。
- Laminar 官方 frontend 镜像不含 `lib/quickwit/indexes/` → spans_v2 等索引永不创建,Total 恒 0;已手动创建 spans_v2/events/signal_events(索引随 volume 持久化)。
- 历史 spans 不在 Quickwit 索引内(spans_v2 是事后建的,spans 表 MergeTree 重导会翻倍)→ 全文搜索不覆盖历史,列表浏览正常;新数据全索引。
- 单 turn >100MB 的 rollout(2026-09-11,161MB)无法导入 → tap 层需按 turn 截断/分块(Catena 设计教训)。
- Git Bash 路径传给 Windows Python 会变 `\e\...` → YAML/参数一律用 `E:/...` 或 `C:/...`。
- 首跑 Docker 构建慢会触发 verifier 120s 超时 → `--timeout-multiplier 4`。
- uv/Python 本机原缺 → uv 装在 `~/.local/uv`,工具在 `~/.local/bin`(PATH 需包含)。

## 6. 相关产物

- 架构图:`docs/harbor-architecture-diagram.html` / `.png`
- Harbor 克隆:`E:\harbor`(浅克隆);Laminar 克隆:`E:\lmnr`
- atif2otel 样例 OTLP:`E:\tmp\atif-sample-otlp.json`
- Harbor 冒烟 job:`E:\tmp\harbor-jobs\catena-smoke-hello-2`
