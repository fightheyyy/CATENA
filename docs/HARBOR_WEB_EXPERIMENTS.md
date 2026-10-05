# Harbor 网页实验

2026-10-04 新增：页面可勾选历史记忆组，发起三个任务 × 三组 × 两次的 18 Trial 对照。
记忆由当前账户的 OpenViking 召回并冻结，只加入 memory 组；candidate 只加载 Skill。
旧的六次实验仍可查看。页面显示来源证据、冻结记忆、耗时和逐次工具日志。
结果与范围见 [三组实验报告](MEMORY_SKILL_EXPERIMENT_20261004.md)。下文六次描述为原先的两组模式。

## 当前范围

`/cases` 展示已冻结的 `rg-missing-search` Case。用户发起后，Go 先创建
owner-scoped PostgreSQL 记录，再通过私有 engine 向 Harbor worker 提交。
Docker 为当前唯一网页执行后端。一次实验为三个任务 × baseline / Skill 两组，
每题每组一次，最多一个实验运行；每个 Trial 串行执行、1 CPU、1024 MiB。

被测对象是 SDK 搜索 Agent，不是 Codex CLI / Claude Code；环境为 Linux
PowerShell 7 的匿名 fixture，不是原 Windows 会话重放。

## 用户操作

1. 在「连接」保存自己的模型配置。
2. 打开「Case 实验」，选择「发起对照实验」。
3. 页面每五秒同步运行状态，可离开后返回或刷新。
4. 查看两组有效试验、答案通过、工具错误、调用数与令牌数。
5. 展开逐次验证证据查看每题奖励和任务冻结检查。

小样本结果不能包装为总体成功率提升。未收集的指标显示空值；无效试验不计入
成功或有效试验的使用量。完成状态要求六个有效 Trial 和冻结 Task 检查。

## 接口与存储

- `GET /v1/experiment-cases`：可执行 Case 白名单。
- `POST /v1/experiments`：`case_id` 与客户端 `request_id`，返回 202。
- `GET /v1/experiments`：当前账户最近 50 个实验。
- `GET /v1/experiments/{experiment_id}`：当前账户实验与最新状态。
- `catena_experiments`：owner、请求幂等键、状态、结构化结果与时间。

跨账户查询返回 404；全局单任务约束由数据库唯一索引和 worker 双重限制。
客户端重试同一 request_id 不创建第二个实验。同步读取不会重新提交。
Go 后台同步结果，服务重启后从数据库找回活动记录并继续查询 worker。
Harbor worker 保存 request_id；若 worker 本身重启，未完成执行标记为 interrupted
失败，不冒充恢复了 Trial。完成结果继续可读。

平台 SDK 协调 Agent 在另一个后台请求中读取同一幂等任务并总结结果。
模型总结是补充信息，不能覆盖 verifier 的成功与有效性判定。
Go 重启时这个总结请求可能中断，但 Harbor 执行结果仍可同步。

## 凭据

Go 从当前 owner 的加密模型配置恢复临时凭据，传给私有 engine / worker。
被测 SDK 使用相同模型；凭据只留在 worker 内存，通过随机任务引用交给适配器，
不会写入 Harbor Job config、实验表或浏览器。
本地 Windows worker 的 `CATENA_EVAL_HOST_ALIAS=127.0.0.1` 把 Docker 中的
`host.docker.internal` 模型地址转换成本机地址；其他部署不设置此别名。

## 部署

沿用 `HARBOR_EXPERIMENT_20261003.md` 的 worker 启动与 engine override。
新的 Go API 和 React 页面随 core 镜像部署，数据库启动迁移自动创建实验表。
当前 worker 仍运行在 Windows 主机，不是自动安装到你的 Linux 服务器。
配置完成前不宣称服务器部署或 Codex / Claude Code 评测已就绪。

## 网页端实际验收（2026-10-03）

从 `/cases` 浏览器按钮提交实验 `experiment-1791002891249-e8dda1fc2fd1be98`，
对应 Harbor Job `7362cfd4036447228af195eb182b6298`，使用当前账户 GPT-5.5。
六个 Trial 全部有效，冻结任务检查通过；记录已写入 PostgreSQL。

| 指标 | Baseline | Skill |
| --- | ---: | ---: |
| 答案通过 | 3/3 | 3/3 |
| 缺失 rg 错误 | 2 | 0 |
| 工具调用 | 9 | 10 |
| 输入 token | 7,591 | 10,731 |
| 输出 token | 637 | 701 |

本次成功率持平，Skill 避免了缺失工具错误，但增加了 token 使用量；不能据此声称总体效果提升。

core / engine 重建重启后，同一实验 ID、六个 Trial 与完成状态仍可读取；
浏览器刷新和逐次证据展开通过，390px 移动布局可读，浏览器控制台无错误。
前端 55 项测试、构建及 Go 实验测试通过；Python bridge 三项测试通过。
