# 平台 Agent 调用 Harbor：首次链路实验

## 结果

真实部署的平台引擎通过工具提交 Harbor、等待完成并取回结果，链路已通过。
Job ID：`5c5fe8a179e8460f9f2577ef6be7e4aa`。6 次有效 Trial，Task 摘要未变化。

| 指标 | baseline | candidate |
| --- | --- | --- |
| 精确答案通过 | 3/3 | 3/3 |
| 缺失 rg 错误调用 | 2 | 0 |
| 工具调用 | 12 | 10 |
| 输入令牌 | 8,903 | 10,029 |
| 输出令牌 | 824 | 595 |
| 策略拒绝 / 运行异常 | 0 / 0 | 0 / 0 |

输入令牌增加约 12.6%。这里观察到错误调用减少，答案成功率持平；三题各一次
只能验证链路与此环境中的行为，不能推断总体成功率改善或稳定的效率增益。
[机器可读结果](../evals/rg-missing-search/results/harbor-gpt55-20261003.json)。

## 实际路径

```text
现有 Docker catena-engine 的 /v1/experiment（私有、需要引擎鉴权）
  → GPT-5.5 / OpenAI Agents SDK 协调 Agent
  → submit_harbor_experiment、wait_harbor_result 工具
  → 本机鉴权 Harbor worker
  → Harbor Job / Trial
  → 每次试验独立 Docker 环境中的 PowerShell 7 工具
  → Harbor verifier
  → 工具返回结果 → 平台 Agent 总结
```

协调 Agent 与被测 SDK 搜索 Agent 分开。协调 Agent 在部署中的 engine 容器里
选择并调用工具；被测 Agent 的模型循环由 worker 执行，文件和搜索工具在
Harbor 管理的容器中执行。没有在容器中安装 Codex CLI 或 Claude Code。

## Case 与环境

沿用 `evals/rg-missing-search` 的三个 fixture、目标答案和原 Skill，转换为
Harbor Task。任务提示把 Windows PowerShell 改为 Linux 上的 PowerShell 7。
这是环境适配实验，不是原 Windows 会话重放，不与此前 Windows 的 12 次结果合并。

- 镜像：`mcr.microsoft.com/powershell@sha256:62300a213a9293916333df2b014cd3a8f22fb0b0b65f2bb446aaf436bcf8c868`。
- Harbor：本机 `E:/harbor`，版本 0.23.0，提交 `3b287b5cc2f0f30745eec73cc1ace578cbfae4c5`。
- 目标模型：代理 GPT-5.5；SDK 0.22.3；worker 的 OpenAI Python 客户端 2.54.0。
- 三题 × baseline/candidate，每题每组一次，串行；目标 Agent 最多 10 轮、180 秒。
- setup 在实际容器中检查 `Get-Command rg`，环境必须确实缺少 rg。
- 搜索使用原有只读命令包装器，工具以 UID 1000 执行；fixture 移除写权限。
- 两组使用同一冻结 Task；仅 candidate 增加 Skill 指令，属于显式加载。
- verifier 由 Harbor 在 Agent 执行结束后上传并调用；Agent 搜索工具不能访问
  verifier 的绝对路径或通过 `..` 越界。verifier 为 shared 环境模式，并非单独容器。
- 试验后由 Harbor 删除容器；报告保留任务摘要、逐次奖励、令牌和错误计数。

## 当前接口边界

这是限定一个已批准 Case 的实验入口。worker 接受 Case ID 和幂等 request ID，
不接受任意主机命令、任意任务目录、模型密钥或动态镜像。
模型密钥由 worker 私下读取，协调模型看不到代理密钥或 worker token。

后续已接到公开 Go API、前端 Case 页面及 Postgres Experiment 记录，见
[`HARBOR_WEB_EXPERIMENTS.md`](HARBOR_WEB_EXPERIMENTS.md)。本节其余内容记录初次 CLI 验收的状态。
现有 `/v1/turn` 的三个分析角色仍保持原有单次回答；工具循环在新的
`/v1/experiment` 中。worker 会重新加载已完成结果，允许通过
`--resume-job <job_id>` 重新读取；这不会执行新 Trial，也不是恢复中断的执行。
运行中的任务索引在内存中，重启后不能恢复正在执行的 Trial。
E2B 和真实 Codex/Claude Code adapter 尚未通过此实验验收。

## 本地复跑

本地 worker 需要可调用 Docker 的 Windows 主机和 Harbor 依赖。当前采用
`uv pip install --python engine/.venv/Scripts/python.exe E:/harbor` 安装已核对的本机版本。
普通 engine 部署不需要安装 Harbor；可选 `evaluation` extra 声明其依赖。

在 `engine` 目录运行 `python -m catena_engine.harbor_local_setup`，然后在仓库根目录运行：

```powershell
docker compose -p catena -f deploy/catena-mvp1/compose.yml `
  -f .local/harbor-study/compose.override.yml up -d --build --force-recreate --no-deps catena-engine
```

最后在 `engine` 目录运行 `python -m catena_engine.harbor_smoke`，它会向实际 engine
容器提交请求，保存完整响应到 `.local/harbor-study/platform-response.json`。
启动脚本使用本机已有 CLIProxyAPI 的 client-key 文件；凭证和 override 在 Git 忽略目录。
worker 监听 8792，要求独立随机 token；部署中 engine 经 `host.docker.internal` 连接。
重跑前应先停止已记录 PID 对应的 worker，避免重复监听。

## 被排除的基础设施失败

第一次 HTTP 调用因 engine 缺少 `httpx` 失败，补充显式依赖。
第一份 Harbor Job 在执行 Trial 前因 Windows GBK 无法输出 Rich 进度字符而失败，
有效 Trial 为零；协调 Agent 取回失败状态，并没有报告 Skill 改善。
worker 改为 UTF-8 后重新执行。基础设施失败不纳入 Skill 对照。

Docker 将命令 stderr 合并到 stdout，初次统计漏算了 2 次缺失 rg 错误。
已从保存的工具日志重新统计两个流，保持原模型执行、答案及 verifier 结果不变。
保留旧响应供审计，平台 Agent 随后重新读取校正结果。新增回归检查覆盖合并输出，
避免将“答案通过”和“没有工具错误”混为一谈。
