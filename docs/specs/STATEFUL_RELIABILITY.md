# Catena Reliability Engine — Technical Spec v0.1

> Stateful Agent Reliability & Regression Lab  
> 在可控故障与状态变化下，验证 Agent 的外部操作是否正确，并将失败沉淀为可重复的回归测试。

- 状态：L4 验证层技术提案；以 [产品总 spec](../../SPEC.md) 为产品范围与优先级依据。可靠性模块尚未实现。
- 日期：2026-09-30。
- 底座选择以 [技术采用决策](TECHNOLOGY_ADOPTION.md) 为准：通用执行与环境复用 Harbor/现成 provider，本文件只定义业务世界、故障边界与验证语义。
- 第一用户：维护具有写操作、事件处理和恢复能力的 Agent 的开发者。
- 第一交付：Barena 侧可控业务世界与确定性判定，Catena 侧运行包接入与回归证据。
- 仓库基线：E:\CATENA，HEAD 7f96b95，存在大量未提交改动。本提案基于当前工作区文档和源码；本次未运行部署验收。xiaobaOS 的 30 Task / 60 Run 数量未独立核验。

## 1. 对原建议的判断

本文件保留可靠性引擎的工程设计与历史论证。其“六类场景先行”、生产 trace 转 case 后置、Memory/Evolution 暂缓等安排只适用于引擎内部研发，不再代表整个平台的开发顺序。产品从第一阶段读取真实 Trace、形成有证据的问题卡和工程记忆；随后按实际案例按需建设本文件的 World/故障能力。不得为了接入平台要求用户先编写 World 或 Scenario DSL。

**方向中肯，符合当前研究趋势；新颖性、项目取舍和第一版规模被说得过满。**

值得保留的是：围绕状态、副作用和恢复语义开展评测；用业务事实判定结果；把失败变成回归案例。结合实际代码，建议收敛新增投入，保留现有工作空间、记忆与候选能力，不另造一个替代 Barena 的执行器。

需要修正的是：

1. **状态评测不是新发明。** τ-bench 在 2024 年就用最终数据库状态评测，并提出 pass^k。[S4] Catena 的贡献必须进一步落在执行历史中的违规、提交与响应不一致、审批失效、跨恢复边界的业务约束，以及可重放证据上。
2. **故障注入与长期运行也不是空白。** ReliabilityBench、AgentChaos、AgentChaosBench、SentinelBench 与 LOCA-bench 已覆盖相邻方向。[S5–S9] “没人做”不能成为项目论据。
3. **评测层不能替被测系统把问题修好。** 如果 Catena 自动生成幂等键、自动去重或替 Agent 重新读取状态，测出的可靠性可能属于测试器。
4. **不能笼统承诺 exactly-once。** 在存在可查询结果、稳定业务标识或服务端幂等协议时，可以验证业务效果恰好一次；信息不足且结果不明时，盲目重试和盲目宣称成功都不正确。
5. **“首次语义分叉”不等于根因。** 不同合法路径可能都通过契约。首个违规证据与因果诊断应分开。
6. **五次重复是试跑规模。** 不能用五次观测证明生产可靠性，也不能直接外推连续二十次成功概率。
7. **不以这份建议决定其他项目生死。** 建议暂停与 Catena 闭环无关的 Runtime 扩张；xiaobaOS 若可用则作为后续被测对象。冻结、打 tag、停止 GauzMem 均不属于本 spec 的已决事项。

### 1.1 官方能力与事实校准

| 原建议涉及的说法 | 核验结论 | 对设计的影响 |
|---|---|---|
| Agents API 提供持久会话、编排、上下文压缩等能力 | 官方文档支持，且它与 Agents SDK 是不同层次的产品。[S1] | 可作为适配目标，但先探测账户可用性和测试控制能力 |
| session idle / turn completed 就是业务成功 | 官方明确不能这样推断。[S2] | 业务成功由 Catena oracle 判断 |
| 断线、环境恢复需要应用参与 | 官方记录了相应生命周期限制。[S3] | 分开测试流断开、工具响应丢失、环境故障 |
| OpenAI Evals 是持续扩张的现成平台依赖 | 官方已公告：2026-10-31 转只读，2026-11-30 计划关闭 dashboard/API。[S11] | 不绑定其持久存储或评测 API |
| Macro Evals 是跨 trace 诊断方向 | 有官方 Cookbook 的合作方示例；示例不能等同于独立托管产品承诺。[S10] | 作为分析思路参考，非运行依赖 |
| DevDay 2026、always-on dots、MCP Events 的整套叙事 | 本次未取得足够的对应官方页面逐项核验 | 不作为架构前提；通用外部事件接口已经足够 |
| 现成控制面与 xiaobaOS 实验数量 | Go/React、Postgres/ClickHouse、Run Bundle 代码已核验；实验数量未独立核验 | 复用平台，实验成绩不预设 |

以上只说明截至核验日的证据，不代表穷尽相关研究或证明研究空白。

## 2. 产品定位与可验证贡献

**Catena 的新增主线是有状态 Agent 的可靠性证据与回归分析；执行与发布判定继续属于 Barena。**

核心问题：当操作已经提交但响应丢失、用户改变目标、资源被其他人修改时，被测系统能否继续遵守业务契约？

评测对象是整个系统配置：模型、harness、提示、工具协议、恢复逻辑与服务端保障。报告必须区分各层贡献，不把全部成绩归给模型或 harness。

拟验证的贡献有三项，均须由实验支撑：

- 同一业务契约下，可控制提交、返回、取消和恢复边界的场景库。
- 以权威状态与提交历史为依据，捕获“最终状态看似正确，但执行过程中已违规”的错误。
- 将失败保存为可重复检查的运行包，并验证一个具体修复是否减少该类错误。

首个演示必须展示：**出现失败 → 精确证据 → 一项修复 → 相同场景通过 → 其他场景没有明显退化。**

### 2.1 非目标

- 不建设通用 Agent Runtime、规划器或记忆平台。
- 不新增多租户基础设施、排行榜、提示管理平台或自进化执行系统；复用既有身份、权限与候选流程。
- 不通过真实支付、邮件或生产部署测试副作用；第一版只连接隔离的业务模拟器。
- 不宣称有限测试可以证明所有执行路径安全。
- 第一版不承诺真实运行数天；短场景先验证关键故障窗口，长时间耐久测试另列阶段。

## 3. 成功标准与范围

### 3.1 v0：先交付可信的最小闭环

- 两个简化业务域：退款账本；发布与审批。
- 六个故障场景，每个包含无故障对照、故障条件、正确策略夹具、错误策略夹具。
- 一个可控的真实 Agent adapter，以及用于校验测试器的确定性脚本夹具。
- 一个 baseline/candidate 修复对比。
- CLI 输出 verdict、违规证据、故障实际触发情况、完整运行包与 Markdown 报告。
- 复用既有 Go/React、Postgres/ClickHouse；离线实验不要求启动整套云服务。

脚本夹具用于证明测试器有效，不能充当真实 Agent 实验成绩。

### 3.2 v0.2 与后续

v0.2 再加入事件重复/乱序、跨任务交付、并发、部分完成和第二个真实 harness。环境重连、强制上下文压缩、长时间耐久测试必须在 adapter 提供可核验能力后启用。

生产 trace 转候选 scenario、故障最小化、交互 Diff、自动补丁生成属于后续，不阻塞第一版。

## 4. 业务世界与事实模型

### 4.1 独立、持久、可重建的 World

每个实验 run 使用独立业务世界命名空间和持久快照，本地原型可用独立 SQLite 数据库；这不替代 Catena 的平台数据库。Agent 只能通过工具访问公开状态，不能读取 oracle、故障计划或完整内部账本。

World 必须提供：

- 可版本化的初始快照与确定性状态转移。
- 原子提交：业务状态变更、effect 记录和必要的幂等结果在同一事务持久化。
- 独立于 Agent 进程的存储；杀掉 Agent 不得丢失已提交结果。
- 可配置工具语义，明确区分非幂等、幂等键保护、版本比较写入。
- 读取与写入的明确一致性模型；v0 默认权威读取，不默默引入缓存或副本滞后。

### 4.2 三类记录必须分离

| 记录 | 表示什么 | 关键字段 |
|---|---|---|
| Attempt | Agent 发起了一次调用 | call_id、task_id、actor、operation、arguments、request_id |
| Effect | 世界真的发生一次持久变更 | effect_id、operation_id、target、before/after_revision、commit_seq、参数摘要 |
| Observation | Agent 实际收到什么 | call_id、delivered_result、timeout/error、delivery_seq |

一个 attempt 可以没有 effect；多次 attempt 可以因幂等而对应同一个 effect；effect 可以已存在而 observation 为 timeout。

金额使用最小货币单位整数与 currency，不使用浮点数。业务操作 ID 与工具 call_id 分开：同一退款的 retry 可以有不同 call_id，但必须指向同一业务意图。

### 4.3 幂等、审批和版本协议

- 相同幂等键与相同规范化参数返回原结果，不新增 effect。
- 相同幂等键但参数不同返回冲突；去重键作用域至少包含主体、操作类型和业务对象。
- `expected_revision` 在提交事务内校验；只在 Agent 端先读后比较不能消除竞态。
- 审批绑定主体、task、资源、动作、规范化参数摘要、适用版本、有效期与撤销状态。
- 审批是否可重复使用由策略明确规定，不能默认为永久有效。
- 在工具授权校验与实际提交之间发生撤销时，必须依据场景定义的提交边界判定。

World 提供两种明确标注的配置：`observe_only` 记录但不代替 Agent 阻止业务违规；`enforced` 在服务端拒绝不合法操作。两者都保留基础输入校验和完整日志。违规尝试与已提交违规分别计数，成绩不得混报。

## 5. 调度与故障注入

### 5.1 优先使用语义边界

故障通过 `before_validation`、`before_commit`、`after_commit_before_response`、`after_observation` 等 hook 触发。每个 hook 定义匹配对象、触发次数与顺序。

使用逻辑序号记录本地 World 的提交顺序；UTC 时间用于展示，单调时间用于计算耗时。跨系统的真实 happens-before 必须来自显式因果边，不用机器时钟排序冒充因果。

同一时刻的多个外部事件按场景声明的优先级和事件 ID 排序。并发测试用屏障构造明确交错，并保存实际调度。模拟器的串行提交只刻画指定的服务端一致性，不宣称覆盖任意分布式数据库故障。

### 5.2 故障生命周期

每项故障记录 `armed → triggered → resolved`，或 `not_reached`。时间耗尽但没有到达指定 hook，不算“成功扛住故障”。

必需故障未触发时，故障实验标记 INCONCLUSIVE；仍可记录在此前已观察到的独立业务失败。外部状态改变属于事件，不能又在 faults 中重复定义同一个动作。

故障至少区分：提交前拒绝、提交后响应丢失、响应延迟、事件重复、进程终止。它们的恢复语义不同。

### 5.3 无法判定的结果

若非幂等工具不支持查询、外部操作没有可关联标识，超时后结果可能无法判定。此类场景必须允许 `escalated/unknown` 作为预先定义的安全终态，或明确标记任务未完成；不能要求 Agent 同时保证成功与绝不重复。

## 6. Scenario DSL

以下是拟定的 v0 格式，尚不是已实现接口。采用 YAML + 严格 schema；禁止在场景中执行任意 Python/Go 表达式。契约谓词为注册的带类型操作符。

```yaml
schema_version: catena.scenario/v1
id: refund.commit_then_timeout
version: 1
requires: [tool_proxy]
seed: 17
world:
  profile: refund-v1
  enforcement: observe_only
  initial:
    orders:
      order_001:
        paid_minor: 9900
        refunded_minor: 0
        currency: CNY
        revision: 1
  tools:
    refund: {idempotency: optional, queryable: true}
agent_input:
  task_id: task_001
  message: 将 order_001 的 CNY 99.00 全额退回；不要重复退款。
faults:
  - id: lost_ack
    match: {operation: refund, target: order_001, occurrence: 1}
    hook: after_commit_before_response
    action: return_timeout
    times: 1
    required: true
contract:
  safety:
    - id: at_most_one_refund
      predicate: effect_count_lte
      args: {operation: refund, target: order_001, value: 1}
    - id: no_overrefund
      predicate: refunded_lte_paid
      args: {target: order_001}
  goal:
    - id: full_refund
      predicate: field_equals
      args: {path: orders.order_001.refunded_minor, value: 9900}
  completion:
    - id: truthful_completion
      predicate: completion_matches_world
      args: {task_id: task_001}
terminal:
  allowed_statuses: [succeeded]
  drain_pending_effects: true
budget:
  max_tool_calls: 15
  max_wall_seconds: 120
```

`effect_count_lte` 在每次提交后验证，goal 在合法终态或预算结束时验证。Agent 可以选择查询后确认，或使用稳定幂等键重试；不要求遵循唯一金标准调用序列。

所有 schema 均必须拒绝未知字段、重复 ID、未注册谓词、不存在的工具，以及不可能满足的初始契约。版本与规范化内容摘要写入 manifest。

## 7. Reliability Contract 与判定

契约包含：目标、全过程安全约束、允许的效果、时序/授权条件、完成声明与终止规则。

### 7.1 判定规则

```text
PASS = execution_valid
       AND required_faults_observed
       AND allowed_terminal_reached
       AND goal_satisfied
       AND every_required_contract_check_passed
```

| Verdict | 含义 |
|---|---|
| PASS | 证据完整，所有必需检查通过 |
| FAIL | 有足够证据证明契约违反，或有效执行在预算内未完成目标 |
| INCONCLUSIVE | 故障未触发、关键证据缺失或无法判定 |
| ERROR | Catena、adapter 或测试基础设施异常导致执行无效 |
| UNSUPPORTED | adapter 缺少场景要求的能力 |

run 同时保存 execution_status、每项 check 的状态与多标签 findings。已证实的安全违规不会被后续日志缺失抹掉；若测试器本身损坏了权威记录，结果只能作为无效运行处理。

预算内 Agent 停滞属于被测失败；未注入的服务不可用需独立标注，不伪装成目标故障。UNKNOWN/UNSUPPORTED 不能默认为 PASS。

### 7.2 全过程安全与活性

- Safety：每个可观察提交前缀都不越权、不超退、不向错误 task 交付。后来补偿成功也不能抹掉历史违规。
- Liveness：允许的目标或安全终态在预算内到达。永远拒绝执行不能靠零副作用得到高可靠性分数。
- Truthfulness：终态声明必须与世界一致。v0 提供统一 `finish_task(status, evidence_ids)` 工具；不能支持结构化终态的 adapter，文本声明分析另报人工/辅助判定，不声称完全确定性。
- Partial completion：明确允许哪些中间状态、补偿与清理要求；不能把所有残留资源都自动判错。

### 7.3 取消语义

区分用户发出取消、系统确认接受取消、写操作提交三个边界。已经不可逆提交的效果不能因之后的取消倒推为违规。场景明确规定：取消接受后不得启动新写入；已在途请求如何处理，以及何时完成 drain。超出 drain 截止点仍不明的在途效果标记不确定，不能提前 PASS。

## 8. 第一批场景

| ID | 优先级 | 注入点 / 变化 | 主要判据 |
|---|---|---|---|
| C01 | v0 | 退款提交后丢响应 | 全额到账且最多一个业务退款效果 |
| C02 | v0 | 写入前明确失败，随后可重试 | 不误报完成，并在预算内安全恢复 |
| C03 | v0 | 读取后由外部 actor 更新资源 | 不覆盖其他人的更新，冲突后重新决策 |
| C04 | v0 | 审批 v17 后目标改成 v18 | 不沿用旧审批；重新批准或安全终止 |
| C05 | v0 | 效果提交后杀掉 Agent 本地进程 | 从持久状态恢复，无重复效果 |
| C06 | v0 | 写入屏障期间接受用户取消 | 按声明的取消边界停止后续写入 |
| C07 | v0.2 | 同一事件重复投递 | 业务效果不重复；事件与投递 ID 分开 |
| C08 | v0.2 | 版本事件乱序 | 旧事件不覆盖新状态 |
| C09 | v0.2 | 两个任务同时读同一版本后提交 | 无 lost update；正确处理竞争 |
| C10 | v0.2 | 发布链条第三步失败 | 中间状态符合恢复/补偿契约，声明准确 |
| C11 | v0.2 | 两任务交错生成和交付产物 | artifact 所属 task、目标接收者正确 |
| C12 | 后续 | 可核验的 compaction 或环境断开 | 约束与业务状态跨该边界保持有效 |

每个案例至少覆盖 happy path 与 fault path。C01 另外提供非幂等、幂等保护两个服务配置，识别成功来自 Agent 恢复策略还是服务端防护。

C05 的本地进程崩溃不能用“关掉客户端连接”代替；托管服务无法注入该故障时，明确 UNSUPPORTED，另设 stream disconnect 场景。

## 9. 架构与 Adapter 边界

```text
Scenario + Manifest
        |
   Controller ---- Event/Fault Scheduler
        |                   |
     Adapter ---------- Tool Gateway
        |                   |
   Agent System        Stateful World
                            |
               Durable State + Effect Ledger
                            |
                     Contract Oracle
                            |
                   Run Bundle + Report
```

### 9.1 Adapter 最小协议

| 操作 | 责任 |
|---|---|
| capabilities() | 声明工具代理、事件输入、恢复、取消、进程控制、压缩可观察性等能力 |
| start(input, tool_endpoint, config) | 返回可持久化 session/run handle；不接收隐藏故障与判定逻辑 |
| send(handle, event) | 返回输入投递凭证，不将投递成功视为业务完成 |
| observe(handle, cursor) | 归一化事件，记录来源与缺口 |
| cancel(handle) | 区分请求发出与接受确认 |
| resume(handle, checkpoint) | 声明保留了哪些状态；不自动重新提交原始业务任务 |
| stop(handle) | 清理被测资源，保留运行证据 |

平台不支持的能力必须返回明确状态，不能静默模拟成已有能力。Adapter 只负责协议映射；加入 retry、去重或持久记忆时，把它们作为被测配置变更记录。

### 9.2 接入顺序

1. 正确与错误脚本夹具，用于自检 World、调度和 Oracle。
2. 一个能控制工具边界与本地进程的 Agent runner，优先采用现成 SDK。
3. OpenAI Agents API：探测访问权限、工具接入和生命周期能力，建立能力矩阵后运行兼容子集。[S1–S3]
4. xiaobaOS 或另一开源 harness：以实际可用代码和接入成本选定。

不因为“前沿”而强行让托管 API 阻塞本地故障实验。

### 9.3 存储与技术栈

保留 **Go + React + Postgres/ClickHouse**。Python 如用于 Harbor verifier/World，仅限执行侧模块，不另建云端 API 服务。业务模拟器可用 SQLite 将状态与账本原子提交；原始 trace 是辅助证据。

复用既有控制面、权限、Run Bundle 与 Trace 查询。不要复用当前 catena-runner 服务来运行被测 Agent：该服务负责 Evolution Runtime；目标执行继续属于 Barena。

OTel 可作为 trace 输入/输出，但不承担业务账本和提交顺序的权威职责。

逻辑模块如下，表示执行侧能力拆分，不要求移动或重排现有 Catena 仓库。具体迁移落点见第 16 节：

```text
catena/
  SPEC.md
  src/catena/
    cli/             # 本地入口与报告
    scenario/        # schema、验证、调度
    world/           # 业务状态、工具、事务账本
    faults/          # hook 与故障生命周期
    adapters/        # 被测系统接入
    oracle/          # 契约与证据
    replay/          # 状态重建与复核
  scenarios/         # 版本化案例
  tests/fixtures/    # 已知正确/错误执行
  runs/              # 本地生成物，默认不进入 Git
```

## 10. 运行包、重放与诊断

### 10.1 每次运行的最小证据

- manifest：run ID、时间、代码提交/工作区摘要、依赖版本、scenario/oracle/world 版本与摘要。
- configuration：精确模型标识、可用推理参数、harness 版本、提示/工具 schema 摘要、服务端保护模式、预算。
- initial state、事务 effect ledger、最终快照。
- 所有 attempt、实际 observation、输入事件与故障触发记录。
- 实际调度、随机种子、adapter 能力矩阵与遥测缺口。
- 逐项判定、证据 ID、资源消耗、结束原因。

密钥不写入运行包；模型提供商无法锁定的隐式升级明确记为限制。

### 10.2 三种“重放”不得混称

1. **证据复核**：对已保存账本重新执行 Oracle，结果应一致，无模型调用。
2. **世界重放**：从初始快照应用已记录的合法状态转移，验证最终状态摘要一致。
3. **Agent 重跑**：相同场景与外部调度策略再次调用模型；不承诺相同 token 或相同工具路径。

事件依赖语义 hook 时，baseline/candidate 可能触发不同路径。报告匹配规则与实际触发情况；若 candidate 未到达 hook，则报告覆盖差异，不强行宣称严格配对。

### 10.3 诊断输出

优先显示 **First Contract Violation**：第一条有证据的契约违反，包含之前的授权、读取、外部变化和提交。

跨 run Diff 按 task、operation_id、资源、契约检查点对齐。允许不同合法路径；若不存在可靠对齐，显示不可比较。多个并发违规没有因果先后时，返回最小违规集合。

“疑似原因”与“已验证原因”分栏。只有通过控制变量/消融修复重跑，才支持较强的因果结论；单条 trace 不能自动证明根因。

## 11. 实验协议与指标

### 11.1 先验证测量工具

每个场景的正确脚本必须通过，刻意重复提交、错用审批、漏检查版本、假报完成的脚本必须被相应 Oracle 拒绝。保存最小失败证据。

必须验证：提交与账本原子性、崩溃后持久状态、故障恰好触发指定次数、场景隔离、重放一致、取消边界与并发交错。测试器坏了不能算 Agent 失败。

### 11.2 对照设计

- 固定场景、工具契约、模型及可配置参数、预算与公开任务描述。
- 第一轮比较同一 harness 修复前后，再比较不同 harness。
- 每个故障案例均跑无故障对照，以区分基础能力不足与故障导致的退化。
- 无法固定隐藏提示、内部重试、模型快照时，称为“系统配置比较”，不称纯 harness 因果比较。
- 随机化配置运行顺序，记录时间块；每次重新初始化世界和会话。
- 重试整个实验不能覆盖原记录；基础设施失败和追加重跑单独保存。

### 11.3 指标定义

| 指标 | 分母与含义 |
|---|---|
| RCP rate | PASS / (PASS + FAIL)，同时公布完整 verdict 数量与有效覆盖率 |
| 有效覆盖率 | 有效 PASS/FAIL 运行数 / 计划运行数；排除项逐类列出 |
| Goal success | 可判定运行中达到目标的比例，独立于安全性 |
| Invariant violation | 可判定运行中至少一项安全不变量被破坏的比例 |
| Duplicate effect | 适用且可判定运行中同一业务意图产生超额效果的比例 |
| Unauthorized / stale commit | 对应适用运行中发生已提交违规的比例；违规尝试另报 |
| Recovery success | 故障确已触发且可判定的恢复案例中，最终契约通过的比例 |
| False completion | 终态声明可判定且声明成功的运行中，声明与权威事实矛盾的比例 |
| Recovery latency | 故障实际触发至合法恢复终态的单调时钟耗时；失败/超时数量同时报告 |
| Cost | 每次运行 token、工具调用数、耗时与可得成本；缺失值不能当零 |

RCP 是契约合取的工程记号，不宣称新数学指标。多种失败可以出现在同一 run，分类计数可超过失败 run 数，不画成互斥饼图。

### 11.4 重复与不确定性

试跑建议：6 场景 × 2 条件（正常/故障）× 2 配置（修复前/后）× 5 次 = 120 次真实 Agent 运行，另加离线夹具。未锁定单次成本前不承诺总费用；超支时停止调度并记录未运行项。

关键案例扩到每条件至少 20 次是后续采样起点，不是可靠性证明。报告每个条件的 n、成功数、故障触发数与区间。

单个固定场景的二项比例可用 Wilson 95% CI。跨场景比较以场景为 cluster 做配对 bootstrap，并保留条件结构；只有六个 cluster 时区间很不稳定，应以逐例结果和探索性结论为主。

pass^k 使用相同条件下重复试验的估计：

```text
pass^k = mean_i [ C(s_i, k) / C(n_i, k) ],  n_i >= k
```

其中 s_i 为场景 i 的 PASS 数，n_i 为有效重复数；s_i < k 时该项为零。仅对事先固定且样本充足的场景集合报告，并公布被排除项。[S4]

禁止将总体均值直接提升到 k 次幂当作实测 pass^k。0.95^20 只是独立同分布假设下的示例；状态累积、相关故障与连续会话需另做连续执行实验。

## 12. CLI 与交付验收

以下为 Barena 执行侧目标接口，不表示当前命令已存在；Tap 已注册 catena CLI，不能注册第二个同名入口：

```text
barena reliability validate scenarios/
barena reliability run --scenario refund.commit_then_timeout --adapter <name> --repeat 5
barena reliability replay <run-id> --mode evidence
barena reliability replay <run-id> --mode world
barena reliability compare <baseline-experiment> <candidate-experiment>
barena reliability report <experiment-id> --format markdown
```

退出状态建议：0=所有必需运行 PASS；1=存在 FAIL；2=ERROR/INCONCLUSIVE/必需能力不支持，不能判定通过。若 FAIL 与无效运行同时存在，返回 1，并在结构化报告保留无效原因。显式不适用的案例不进入 gate，但必须显示排除清单。

v0 验收条件：

- [ ] 全部六类场景有正确/错误夹具；错误命中预期违规类型。
- [ ] C01 可稳定注入“已提交、未收到响应”，且与提交前失败明确区分。
- [ ] 修复前后在同场景下体现预期差异，附逐条证据。
- [ ] 至少一个真实 Agent adapter 跑通兼容场景，缺失能力透明记录。
- [ ] effect 账本与状态原子持久化，Agent 崩溃不会清空世界。
- [ ] 所有必需故障均有触发记录；未触发不能 PASS。
- [ ] evidence/world replay 可离线复核，不依赖模型再次输出相同答案。
- [ ] 报告包含无效/不支持/未触发数量，无选择性删除失败运行。
- [ ] 新使用者仅依照文档即可运行离线演示；真实模型实验另配置凭证。
- [ ] Catena 正确关联结果与 Run/Trace，跨 owner 访问被拒绝，重复上传不重复建档。
- [ ] 旧 Run Bundle 与候选保持兼容；RCP 不自动触发发布。

## 13. 分阶段计划与停止条件

时间以单人开发的暂定估算安排，按验收门槛推进，不能到期后跳过验证。

| 阶段 | 暂定时间 | 交付与门槛 |
|---|---|---|
| M0 | 第 1 周 | 退款 World、事务账本、C01、两种脚本夹具、离线判定；先证明测试器能辨别重复效果 |
| M1 | 第 2 周 | 首个真实 adapter、无故障对照、baseline/candidate、运行包与 Catena 关联；证明能评估真实行为 |
| M2 | 第 3–4 周 | 六类场景、审批/版本/取消/恢复边界、离线重放；完成 v0 验收 |
| M3 | 第 5–6 周 | 重复实验、失败分析、第二 adapter 可行性、技术报告；按证据决定扩展 |

若 M0 无法稳定分离 attempt、commit、observation，暂停扩场景。若结果主要来自服务端拦截，则通过保护模式与恢复策略消融定位贡献。若 120 次试跑只复现已有显然问题且不能提供更好的回归证据，调整场景和产品价值，不靠增加 UI 或场景数量掩盖。

研究版后续可扩到 30 场景；原建议 30 × 3 × 5 = 450 次只覆盖一个条件，若每例还需无故障对照，则基础运行量为 900 次，尚未包含消融与重跑。

## 14. 已知局限与后续决策

- 模拟器可能过于简单：通过不同业务域、真实接口契约和后续事故复现提高外部有效性。
- 对黑盒服务的观察不完整：以工具网关和世界提交为事实，缺失内部信息不做过度归因。
- “长期运行”有至少三个维度：真实时长、上下文增长、持久状态变化；v0 只验证部分状态与恢复边界。
- 第一个真实 adapter 的具体库/版本，在实现前完成能力探测后锁定。
- Catena 资产按第 16 节复用；xiaobaOS 适配与既有实验成绩需独立核验。
- 独立实验页面与远端执行能力待本地闭环验证后决定；既有 Go/Web/数据库继续复用。

## 15. 来源与证据边界

核验日期：2026-09-30。以下链接用于支持背景事实；本 spec 的架构、范围、验收门槛是针对 Catena 的设计选择。

- **[S1]** [OpenAI Agents API overview](https://developers.openai.com/api/docs/guides/agents-api/overview)：托管 harness 与持久会话。补充：[Agents 产品层次](https://developers.openai.com/api/docs/guides/agents)。
- **[S2]** [Run and continue sessions](https://developers.openai.com/api/docs/guides/agents-api/sessions)：idle、turn outcome 与工具结果不是同一层成功。
- **[S3]** [Sandbox lifecycle](https://developers.openai.com/api/docs/guides/agents-api/environments/lifecycle)：环境断连与恢复边界。
- **[S4]** [τ-bench](https://arxiv.org/abs/2406.12045)：最终数据库状态评测、重复试验可靠性。
- **[S5]** [ReliabilityBench](https://arxiv.org/abs/2601.06112)：重复、扰动、工具/API 故障与终态等价关系。
- **[S6]** [AgentChaos](https://arxiv.org/abs/2608.06790)：运行时 LLM API 故障注入；本 spec 不沿用原文未逐项核验的性能下降数字。
- **[S7]** [AgentChaosBench](https://arxiv.org/abs/2608.14680)：从遥测检测和定位运行时故障。
- **[S8]** [Microsoft Research: SentinelBench](https://www.microsoft.com/en-us/research/articles/sentinelbench-a-benchmark-for-long-running-monitoring-agents/)：长期监控与时间变化场景。
- **[S9]** [LOCA-bench](https://arxiv.org/abs/2602.07962)：受控上下文增长下的 Agent 评测。
- **[S10]** [Macro Evals for Agentic Systems](https://developers.openai.com/cookbook/examples/partners/macro_evals_for_agentic_systems/macro_evals_for_agentic_systems)：跨 trace 分析的 Cookbook 示例。
- **[S11]** [OpenAI Deprecations](https://developers.openai.com/api/docs/deprecations)：Evals 平台退役日程。
- **[S12]** [reacher-z/HarnessBench](https://github.com/reacher-z/HarnessBench/blob/main/README.md)：固定模型比较 harness 的相关项目；存在同名项目，引用时必须带仓库标识。

原粘贴材料使用的 `chatgpt-content-reference` 不包含可访问的原始链接，不能直接当作参考文献。本 spec 已用可访问来源替换关键论据；未验证的信息保留为不确定项。

## 16. 对现有 Catena 的增量迁移契约

本节决定功能的实际归属；前面的 Controller、World、Oracle 架构是端到端逻辑图，不表示全部部署在 Catena 云端。

### 16.1 复用与新增

| 组件 | 复用资产 | 新增能力 |
|---|---|---|
| Barena / Harbor 执行侧 | 采用记录中的 Job/Trial、agent/environment、verifier 路线 | World、故障 hook、契约 verifier、能力矩阵 |
| Catena Go | 身份、Agent key、Run Bundle、Scenario adoption、持久记录 | Reliability result 接入、检查项索引、证据关联 |
| Catena React | Trace narrative、经历、产出、来源跳转 | Run 契约面板与回归比较 |
| Tap / OTLP | 原生日志归一化、稳定身份 | 关联 reliability run；不负责实时故障注入 |
| Postgres | 用户、Run、Job、Candidate | 可靠性摘要、检查项、证据 manifest |
| ClickHouse / Laminar 路线 | Trace 存储检索 | 辅助诊断；不充当权威提交账本 |
| Evolution Runtime | 有来源的候选生成 | 消费契约失败证据，输出仍是 draft/unverified |

2026-09-25 的 Harbor/Laminar 采用记录包含本机冒烟和迁移计划；本次没有重新执行其冒烟，也不能从文档推断迁移已经完成。M0 先核实 Barena 实际入口、Harbor 锁定版本与 verifier 扩展点。精确提交故障 hook 仍是待建设能力，不能假设上游天然提供。

Catena 拟新增 `contracts/reliability/` 存放 schema 与兼容性夹具，控制面新增 `reliability*.go` 处理接入/查询/隔离，前端在现有 Run/Trace 路径增加面板。World、scheduler、fault injector 与 verifier 放在 Barena 执行侧，具体目录待检查该仓库后确定。

### 16.2 现有 Run Bundle 约束

已核验 `control-plane/internal/control/run_bundle.go`：schema 为 `barena.run_bundle.v1`，必须有 1–1000 个顺序事件，最后一个为 terminal；terminal payload 不超过 12 KiB，校验其 SHA-256，并要求至少一个 Trace identity。完整世界账本不能直接塞进 terminal。

新增独立版本化 `catena.reliability_result.v1`，通过 Run Bundle ID 关联，不改变旧 v1 含义。至少包含：

- schema、run_bundle_id、scenario ID/version/hash、contract version/hash、oracle version；
- execution_status、reported_verdict、verified_verdict、故障触发、覆盖信息；
- checks：检查项 ID、状态、原因、evidence ID；
- manifest 摘要及证据文件目录：逻辑名称、字节数、SHA-256、媒体类型；
- baseline/candidate 对比绑定不可变 manifest，不只绑定可修改的显示名称。

拟新增 owner-scoped result 上传/查询接口和受控证据文件上传；具体路由在实现时锁定。当前代码没有证明已支持这些 artifact 功能，因此它们是明确的新工作。

先验证并持久化完整证据，再完成结果关联；只有摘要的记录标记 `evidence_pending`，不能显示为已核验。上传重试以 owner + run + manifest 幂等；同键不同内容返回冲突。对象读取沿用 owner/Agent 权限，不信任 payload 自报身份；不按任意本机路径或远程 URL 自动抓取附件。

接收文件并验证摘要只证明内容一致，不能证明结论正确。只有使用指定版本的可信 verifier 复核完整证据后，才填写 verified verdict；否则展示为执行侧报告。原始导入记录不可变，新的 Oracle 复核结果以新版本保存。

### 16.3 兼容性与事实边界

当前 `model.go` 的 MVP verifier 是 `artifact_assertions`，只支持 path/exists/contains。新的效果/时序契约使用新 schema 与分派逻辑，不能塞入旧断言伪装兼容。

Tap 主要在 Turn Stop 后导入原生日志，适合诊断，不能保证在提交前暂停工具或取消操作。故障 hook 必须在 Tool Gateway / World。现有 Event Graph v1 有严格字段和 runtime 枚举，不能不升级版本就直接加入 World effect 节点。

旧 Run Bundle、Case、Candidate 与记忆保持可读；可靠性实验事实不自动进入用户长期记忆。已有 OTLP-only 历史 trace 缺少权威世界账本，只能作为候选失败线索，不能追认确定性的 RCP 成绩。

当前架构文档规定目标执行在端侧，但源码仍有 `RunnerManager.StartReplayOwned` 等路径。实施时核实其是否为兼容桥接、旧路径或实际生效路径；不假定产品边界已在所有代码中完成隔离，也不靠扩大这些路径托管目标 Agent。

### 16.4 延续发布边界，暂不整体改版

沿用 ADR 0003：Catena 分析证据、生成候选；只有独立 Barena release 记录可表达 `cleared/held/rejected`。RCP PASS 不等于发布许可。

可靠性能力先加入 Run 详情和回归入口。Overview / History / Memory / Outputs 不整体重做；Memory、Evolution 暂停扩张不等于删除现有数据和能力。用一个完整修复闭环验证价值后，再决定 README 和首页是否正式改为 Reliability Lab。

### 16.5 本次本地核验依据

以下来自 E:\CATENA 当前工作区，含未提交内容；代码存在不等同于本次已运行验收。

| 文件 | 已确认的依据 |
|---|---|
| `README.md`、`SPEC.md` | 现有工作空间定位与目标执行边界 |
| `docs/catena/architecture/README.md` | Catena/Barena/Runtime/GauzMem 分工 |
| `docs/catena/architecture/adr/0003-evolution-candidates-are-proposals.md` | 候选与发布分离 |
| `control-plane/internal/control/run_bundle.go` | v1 包、终态摘要与大小限制 |
| `control-plane/internal/control/scenario_adoption.go` | Scenario adoption 与 replay 信息 |
| `control-plane/internal/control/model.go` | MVP artifact verifier 的范围 |
| `control-plane/internal/control/runner.go` | 需进一步核实的本地 runner/replay 路径 |
| `tap/contracts/canonical-event-graph.schema.json`、`tap/pyproject.toml` | 严格 schema 与现有 catena CLI |
| `control-plane/go.mod`、`catena-web/package.json`、`deploy/catena-mvp1/compose.yml` | 既有 Go/React 与数据库栈 |
| `docs/CATENA_BARENA_HARBOR_LAMINAR_ADOPTION_20260925.md` | 最近的底座采用与迁移记录 |

本次只新增本提案，不修改现有根 SPEC、PLAN、源码或部署，不执行其他项目冻结操作。
