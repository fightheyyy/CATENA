# Collector / Kafka Trace 数据面验收（2026-11-08）

工作区改造，基线提交 `7e6c8b386f780fa2c03c4792f953b9c82704123d`；未切换生产数据、未发布环境。RSI / PostgreSQL Job Queue 未修改。

## 结论与范围

官方 Collector → Kafka → 官方 ClickHouse Exporter 已通过真实容器链路、字段对照、重复投递、跨 Owner 隔离和进程故障验收。固定目标负载下数据完整；这些短基线不证明长期企业级容量。原 2026-10-27 的写入报告属于旧路径，不作为本链路容量证据。

Go 1.25.11、Collector Contrib 0.137.0、Kafka 3.9.1（三节点 KRaft，RF=3/minISR=2）、ClickHouse 25.10.2.65、PostgreSQL 17。所有组件同机，配额 4 核 / 32 GiB；Go 客户端与多个真实 HTTP Ingress 在同一测试进程，鉴权使用实际 PostgreSQL。
入口每进程并发 16、每 Owner 2，区别于生产默认并发 4。生成器上限 32 个在途请求，目标速率可能因背压延长实际发送时间。未调用真实模型、Codex 或 Harbor。

## 固定负载：每组 60 秒、目标 1000 Span/s

20 租户，10 请求/s，每请求 100 Span；短文本合成负载，包含真实身份标准化与 meta.v1 存储开销。每组预期 60000 个唯一 Span。所有 Owner/TraceId/SpanId 逐项核对，不只检查行数。

|分区|Gateway / Go Ingress|Consumer|实际 ACK Span/s|入口→Kafka P99 ms|落库 P99 ms|唯一 Span|CPU 平均核数¹|内存峰值 GiB¹|
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|6|1|1|999.4|77.1|91.1|60000|2.33|18.14|
|6|2|1|904.4|4035.5|4108.0|60000|2.46|19.32|
|6|2|2|988.2|346.7|524.7|60000|2.76|20.41|
|12|2|2|993.0|106.9|106.3|60000|2.40|20.72|

¹ 整个 cgroup，包含生成器、Go、数据库、Collector、监控和验证查询；内存包含页缓存与既有环境占用，不是应用 RSS。逐容器 Docker 样本中多个服务返回近似相同的父级用量/32 GiB 上限，与各自声明的容器限额不符，因此未据此归因或求和，也未验证独立容器资源隔离。不能把约 20 GiB 总量当作应用 RSS。
入口延迟包括鉴权/解析/转发与 Kafka 确认；落库延迟按相同宿主机时钟的 CatenaIngestedAt 减首次发送时间。P99 使用下界顺序统计量 floor((n−1)×0.99)。
每组包含扩容后的冷启动及 rebalance，未单独预热。2 Gateway / 1 Consumer 轮次尾延迟较高，不能据此推导稳定的线性扩容关系。未测试饱和极限或长时间稳定性。

## 查询、兼容性与隔离

- Codex 54 Span / 14 Trace、Claude 52 Span / 14 Trace：官方链路重复投递后，业务字段、层级、属性、事件、链接以及四类查询与旧表对照通过。新增/覆盖的可信身份属性是预期增强。
- 暂停后台合并后，3 份物理副本仍只读出 1 个逻辑 Span；另一个 Owner 使用相同 TraceId/SpanId 不被合并。
- API Key 绑定 Owner/Agent，覆盖 resource/span 中的伪造身份及内部元数据；错误 Key 拒绝，多租户结果不串租户。
- 千租户数据集：1000 Span / 1000 Owner，列表查询 P95 157.8ms、P99 186.7ms；小批量写入落库 P99 9.57s，不能忽略小批量成本。
- EXPLAIN 命中 Owner + TraceId 主键与 Owner 哈希分区；FINAL 仍可能扩展邻接读取范围。原始计划见 owner-query-plan.txt。千租户不等于百万租户或重文本场景。

## 故障验收

- ClickHouse 停机：10000 Span（100 条 Kafka 消息）继续获得 Kafka 确认，12 个分区已提交位点均保持不变；未把 Consumer 内存接收当作已落库。
- 在积压尚未落库时 SIGKILL 全部 Consumer，重启后且 ClickHouse 仍停机时，位点仍未前进。
- 恢复 ClickHouse 后约 9.43s 完成恢复和身份集合校验；该窗口包含数据库启动、测试进程和校验，并非单条消息纯恢复时延。最终 Kafka lag 为 0。
- 强杀一个 Broker 后，200 批 / 20000 Span 最终全部确认且逐项校验通过；本轮 HTTP 尝试状态为 {'200': 200, '429': 25}。有界重试上限 8，入口限制未放宽。入口确认 P99 9.15s、落库 P99 8.75s，故障下存在明显尾延迟。
- 三 Broker 同机，只验证单进程故障与副本行为，不代表机房、宿主机或磁盘故障域隔离。语义为 At-least-once + 幂等读取，不是 Exactly-once。

## 失败轮次与纠正

- 首次扩到 12 分区：旧 Consumer 未重载挂载配置，90 秒验收等待后仍只有 43800 Span。原始积压随后全部恢复，已补做 60000 个身份集合验证。脚本现显式重启 Collector 并重跑该组。
- 首次显式重启后的辅助脚本仍使用旧的随机 loopback 端口：600 请求均 503，没有确认成功；已修正为重启后重新发现端口。该轮不计入吞吐结果。
- 首次 Broker 故障生成器未重试两次 429，198 批成功、2 批未确认，不能宣称整批完成。补上有界退避后重跑；最终轮触发 25 次 429 并全部恢复。原始失败日志保留，不把拒绝请求算作落库丢失或成功。

## 代码验证与复现

- 完整 Go 回归 102 项通过（含子测试），无失败，跳过两个显式压测入口；TestOTelDataPlaneLoad 已单独执行，旧 TestTraceBackendLoadStudy 本轮未重跑。
- 补充未合并副本去重测试通过；go vet、生产 go build 通过。旧 Writer 已移至 _test.go，不编译进生产服务。
- 迁移连续两次成功；修改已应用 SQL 校验和的测试被正确拒绝。旧表按 Owner 回放 14 Trace / 54 Span，以及游标恢复成功。
- 基础/公共/评测 Compose 配置和固定版本 Collector 配置有效；新增 CI 流程已做 YAML/本地等价检查，尚未在远端 GitHub Actions 执行。
- 最终修正的脚本已用 --resume-at-12-partitions --seconds 5 完整跑通（千租户与故障段保持各自完整时长）；60 秒表格使用前述独立实测轮次，不混算短复验。

复现与迁移：[Trace 数据面](../TRACE_DATA_PLANE.md)。代码入口 scripts/benchmark_trace_pipeline.py，原始结果/资源/位点/查询计划/失败日志在 artifacts/cloud-workspace/trace-pipeline/。本报告旁的 JSON 包含具体数据、位点和源码 SHA-256。

真实业务 Trace 对照、生产迁移及读取切换尚未执行。已有配置未设置 CATENA_TRACE_BACKEND 时保留 legacy 读取；新安装示例显式选择 otel。长稳态、饱和点、大文本、跨故障域及生产权限配置仍需独立验收，不能称为已经具备企业级承载能力。

验收结束后停止专用 `catena-trace-study` 测试项目以释放端口；保留全部数据库/Kafka 卷和原始证据，不执行 `down -v`。
