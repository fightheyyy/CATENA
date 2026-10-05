# Catena Harbor 实验冻结结果

7 项手工编写的 Skill × 28 个任务 × with/without 两组 × 每组每题 3 次，共 168 个有效 rollout。
执行目标：真实 Codex CLI 0.160.0；模型：GPT-5.5；执行引擎：Harbor 0.23.0。

| 指标 | without | with |
|---|---:|---:|
| 验证通过 | 77/84（91.7%） | 80/84（95.2%） |
| 同类错误匹配次数 | 23 | 27 |
| 工具调用次数 | 660 | 796 |
| 合成秘密输出违规次数 | 1 | 0 |
| 输入 token（含缓存） | 4,384,437 | 4,772,938 |
| 输出 token | 76,494 | 93,560 |

观察到通过率差异 +3.57 个百分点。按任务聚类的 bootstrap 95% 区间为 [0.00, 9.52] 个百分点。区间含零时不能声称提升有统计显著性。

| Skill | without 通过 /12 | with 通过 /12 |
|---|---:|---:|
| git-context-isolation | 12 | 12 |
| long-command-lifecycle | 9 | 9 |
| path-discovery | 12 | 12 |
| powershell-execution | 10 | 12 |
| python-environment-selection | 12 | 12 |
| reliable-file-editing | 12 | 12 |
| secret-output-control | 10 | 11 |

实验任务为匿名重建、变体和迁移用例，全部历史 Trace 曾参与设计，不属于严格历史留出集。执行环境为 Linux Docker 与 PowerShell 7，未还原原始 Windows 环境。
原批次中 136 次有效，32 次在代理额度恢复后补跑；原批次使用并发执行，补跑使用单并发。模型标签和配置一致，但供应商后台版本未单独验证，耗时对比受执行时间、缓存和并发影响。
失败试次按冻结 verifier 判定保留。基础设施异常与供应商限流记录单列，不进入通过率分母。工具错误匹配可能包含预期的探测失败，不能直接当作故障根因。

release-manifest.json 为冻结清单，rollouts.csv 为逐次结果，verifier-evidence/ 为 168 次验证明细。完整原始会话保存在本机私有实验目录，selected-evidence-sha256.json 保存其校验值。
