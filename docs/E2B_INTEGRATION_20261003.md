# Harbor + E2B 接入

状态：已实现配置、工具路由与清理记录，尚未完成云端实测；本机缺少 E2B API Key。

## 已实现

- 平台 Agent 的 `submit_harbor_experiment` 新增 `environment=docker|e2b`，默认 Docker。
- worker 选择 Harbor Docker provider 或继承官方 E2B provider 的薄适配器。
- 安装本机 E2B SDK 2.52.0、dockerfile-parse 2.0.1；新增可选 `evaluation-e2b` 依赖。
- 使用原三个 Linux/PowerShell 7 fixture、答案、Skill；每组每题一次，总计六次。
- 搜索工具改用已创建的普通用户 `catena-eval`，跨 provider 不依赖数字 UID 查找。
- E2B sandbox 运行时长限定 1200 秒，替代上游默认的 24 小时请求。
- 模型看不到 E2B Key；worker 从环境变量或受控本地文件读取。
- 缺少凭证时立即返回 `blocked/e2b_credentials_missing`，不创建模板或沙箱，不回退到 Docker。
- 每个 Trial 保存 sandbox ID、时长及 kill 调用确认；清理失败不能计入有效 Trial。
- 上传文件使用 root 身份执行基础设施操作；模型的搜索命令使用普通用户及只读包装器。
- 已完成结果可重复读取；未知 resume ID 被拒绝，不因此另开云端评测。

三个生成的 Dockerfile 已由 E2B SDK 本地解析通过。这仅是格式验收，
不证明云端模板构建、权限、文件上传、命令执行和 verifier 已通过。
相关 13 项回归检查已通过，包括无凭证阻塞、provider 幂等约束、未知 resume
不创建新任务、沙箱时长与清理失败可见性。
原有引擎的 16 项测试也通过。

实际部署的 engine 已调用 E2B 提交工具，返回
`blocked/e2b_credentials_missing`，provider 为 `e2b`、Trial 数为 0，
没有回退到 Docker。阻塞记录 ID 为 `141b3e958a5e406b941779de447946b0`。
响应保存在 `.local/harbor-study/platform-response-e2b.json`；这验证了工具路由和
缺失凭证处理，不构成云端执行通过。

## 凭证与运行

将 E2B API Key 保存在 Git 忽略文件：

`E:/CATENA/.local/harbor-study/e2b-api-key.txt`

也支持启动 worker 时设置 `E2B_API_KEY` 或 `CATENA_E2B_KEY_FILE`。
无需把凭证加入 engine 容器、Case、模型提示或公开报告。
worker 每次提交 E2B 任务时可加载新建的 key 文件。

更新并启动 worker、重建 engine 的方式沿用
[Harbor 本地实验说明](HARBOR_EXPERIMENT_20261003.md)。然后在 engine 目录执行：

```powershell
.venv/Scripts/python.exe -m catena_engine.harbor_smoke --environment e2b
```

结果保存到 `.local/harbor-study/platform-response-e2b.json`；不会覆盖此前 Docker 响应。
读取已完成任务时须保留 provider：

```powershell
.venv/Scripts/python.exe -m catena_engine.harbor_smoke --environment e2b --resume-job <job_id>
```

## 云端验收条件

1. 官方 E2B provider 构建并复用 PowerShell 模板。
2. 实际 microVM 中缺少 rg，普通用户能执行只读搜索。
3. 三题 × 两组返回精确答案 verifier 的结果，并保存逐次日志。
4. Task 摘要未变化；错误计数同时读取 stdout/stderr。
5. 六个沙箱均记录成功 kill，报告中的试验有效性检查包含清理证据。
6. 平台 Agent 取回实际结果，不伪造奖励或在失败时暗中切换后端。

模板属于复用构建产物；销毁沙箱与删除模板是不同操作。
此路径尚未接入前端 Case 页面或 Postgres Experiment；E2B 不支持原 Windows Case 的等价重放。
