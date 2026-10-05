# Case：Windows PowerShell 缺少 rg 时的仓库搜索

2026-10-04 更新：完成 [OpenViking 记忆 / Skill 的 Harbor 三组对照](../../docs/MEMORY_SKILL_EXPERIMENT_20261004.md)。
每题每组两次，共 18 个有效 Trial。三组答案均 6/6，缺失 rg 错误为 baseline 4、记忆 3、Skill 0。
Skill 总 token 增加约 15.2%，没有成功率或耗时改善。此 Linux / PowerShell 7 适配结果与下文 Windows 实验分别记录。

状态：真实重复工具错误已确认；匿名化 Case 已冻结；确定性重放通过；
SDK + 代理 GPT-5.5 的 12 次有效对照完成。Skill 在本小样本中减少错误调用，
没有提高答案成功率或令牌效率。Codex CLI 试跑仍保留为无效环境记录。

## 来源

在本机 Codex 会话中发现 5 个独立会话、5 次 `rg` 调用返回
`ObjectNotFound: (rg:String)` / `CommandNotFoundException`。
日期：2026-04-10、2026-04-16（3 次）、2026-04-21（UTC）。
其中已入库的一条来源 Trace：`32224e4d48f38a77ef965b9ae585aa80`，
Span：`a358eb5604e7b275`。

逐条核对原始 function call/output：首次搜索错误后能看到 PowerShell
原生文件枚举或文本查找。结论限于“多会话重复支付一次失败搜索成本”，
不声称任务最终失败，也不声称每个会话都进行了无意义的重复重试。
原始调用、会话文件名和私人路径仅保存在
`.local/failure-study/rg-missing-evidence-20261003.json`，不公开原始项目内容。
这次定位扫描跳过超过 32 MiB 的文件，只匹配原生 function_call_output；
不是全部数据的发生率统计。

## 可复现环境

Windows PowerShell，工具 PATH 仅包含 Windows 和 PowerShell 路径，未安装 rg。
不修改机器 PATH、不卸载工具；配置仅作用于评测子进程。
`check_environment.py` 在相同 PATH 下实际执行原始 rg 搜索形式和 PowerShell
替代搜索，验证失败机制和无匹配语义。这是确定性工具重放，不是模型效果评测。

## 冻结任务

三个 fixture 是从历史中的源码实现查找、角色配置查找任务抽象出的匿名化小仓库，
不是原始仓库重放。第三题为无匹配负对照。实现、归档、禁用角色和文档混合存在。
答案必须精确匹配独立 verifier，不能用字符串包含式判定。

主模型实验：Codex CLI，模型 gpt-5.5，每题每组 2 次，交替 baseline/candidate。
双方使用干净工作目录、隔离 CODEX_HOME、同一 shell PATH、提示和输出 schema。
candidate 唯一实验干预是发现得到的 Skill。记录 Skill 是否被读取，但该读命令
计数仅为线索，不保证完整衡量所有 Skill 加载渠道。

## 判定

首先要求实际工具执行可用、无策略拒绝干扰、同一缺失 rg 环境成立。
之后比较独立答案通过率、rg 缺失错误次数、失败调用、总调用、耗时和令牌。
小样本仅支持试跑结论，不能外推真实任务总体成功率。
如果答案通过率相同，只能报告是否减少工具错误及成本，不能写成功率提升。
如果工具策略或模型权限阻断，则记录实验无效，不能将其当作 Skill 无效。

复跑：

```powershell
$env:PYTHONPATH = (Resolve-Path ./tap).Path
python evals/rg-missing-search/check_environment.py .local/skill-study/rg-missing-environment.json
python -m catena_tap.skill_experiment evals/rg-missing-search/manifest.json --output .local/skill-study/rg-missing-run
```

Catena 的平台运行时确定为 OpenAI Agents SDK；这里的 Codex CLI 是被评测的目标
Agent，二者职责不同。本 Case 验收不宣称已接入 E2B 或 Harbor。

## 本次结果（2026-10-03）

环境重放：3/3 复现缺失 rg，替代搜索 3/3 通过。
CLI pilot：一个 Case 的 baseline/candidate 各运行一次。
baseline 有 1 次策略拒绝、4 次成功 rg 调用；candidate 有 2 次策略拒绝、
3 次成功 rg 调用，且超时。两次尝试有效性均为 false，不比较通过率。
candidate 轨迹有 2 次读取 Skill 的命令，但这不能证明 Skill 在缺失 rg 环境有效。
完整报告和独立有效性复核保留在
`.local/skill-study/rg-missing-pilot-20261003/report.json` 与 `assessment.json`。

预跑还揭示 Windows 的 `.cmd` 超时只杀父进程会让 Node/Codex 保持输出管道，
已修复为超时终止该评测进程树，并用真实父子进程超时测试验证。
相关实验入口测试 8 项通过，Skill 结构校验通过。

## SDK + CLIProxyAPI GPT-5.5 有效对照

代理从独立本地配置启动，复用用户已有认证，去掉旧请求 payload 改写规则；
Catena 的分析模型已配置并通过现有 engine 的一次真实模型调用。

SDK 评测使用与模型完全相同的 PowerShell 工具做缺失 rg 预检。
工具仅接受只读 literal 命令，禁止外部路径、变量表达式、写入、重定向和
非白名单程序。没有 E2B/Harbor 接入声明，也不是通用沙箱。
工具边界测试 7 项通过；开发阶段未通过边界测试的 SDK v1 试跑不计入结果。

candidate 显式加载完整 Skill 到 instructions；baseline 不加载。
这测量 Skill 被启用后的效果，不测量 Codex/Claude Code 的自动 Skill 发现。
任务、模型、工具、答案和环境固定，顺序交替，每题每组各 2 次。

| 指标 | Baseline | Candidate |
| --- | --- | --- |
| 有效试验 | 6/6 | 6/6 |
| 独立答案通过 | 6/6 | 6/6 |
| 缺失 rg 错误调用 | 4 | 0 |
| 总工具调用 | 25 | 26 |
| 输入令牌 | 19,887 | 25,741 |
| 输出令牌 | 1,367 | 1,613 |
| 累计耗时（秒） | 82.172 | 85.593 |

结论：该 Skill 在明确缺失 rg 的这个小样本中避免了错误调用；
没有改善答案成功率，输入令牌约增加 29.4%。保留为环境专项候选，
不默认给所有 Agent 注入，不宣称降低费用或广泛改善 Coding Agent。
尚无原始账单或统计显著性估计。

匿名化结果见 [公开报告](results/sdk-gpt55-20261003.json)。
完整消息/工具证据在 `.local/skill-study/rg-missing-sdk-20261003-v2/`。
SDK 复跑入口：

```powershell
$env:PYTHONPATH = (Resolve-Path ./engine).Path
./engine/.venv/Scripts/python.exe -m catena_engine.skill_eval evals/rg-missing-search/manifest.json --output .local/skill-study/rg-missing-sdk-run --api-key-file '<本地代理客户端密钥文件>'
```

模型只看到匿名化 fixtures 与 Skill，不提交原始私人会话。12 次对照与 3 题
规模不足以估计真实项目总体效果，不能将 4→0 外推为总体失败率下降。
