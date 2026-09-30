# 本地 Codex 历史导入

已导入本机 Catena：`http://127.0.0.1:5570`，Agent 名称「我的 Codex」。未上传到公网服务器，也未开启未来会话的自动同步。

## 范围与结果

- 扫描 `%USERPROFILE%\.codex\sessions` 与 `archived_sessions`，共 303 个 JSONL 文件，初始快照约 2.17 GB。
- 301 个会话包含执行记录；另外 2 个文件只有会话元数据，没有可导入的执行。
- 最终保留 **2,582 条 Trace、83,837 个 Span**；时间范围从 2025-10-10 到 2026-09-15。
- 处理期间再次读取了 2 个变化文件，补充快照截止于北京时间 **2026-09-15 22:28:35**。原始文件未修改。
- 当前仍在执行的回合保持未完成状态；这次导入不会把未完成、取消或缺失结果标为成功。

## 兼容与验证

历史解析使用现有 Codex parser，修复了原生回合之外的启动/恢复上下文被误当成匿名回合、可见 UserMessage 与最终答复读取，以及分叉历史中的父会话头覆盖子会话身份的问题。新增了脱敏回归夹具，11 项解析测试、类型检查和插件构建通过。

导入按稳定的原生会话/回合关联生成 Trace/Span ID，并分批发送到原有 OTLP 接口。磁盘中断后，先比较完整 Span ID 集合，再补写 56 条缺少部分 Span 的 Trace。最终逐 Trace 数量全部一致，无缺失或多余记录；重复上传一条 Trace 后逻辑数量不变。

两个工具结果因同一内容在多个兼容字段中重复出现而超过单次请求限制。传输时只移除了与 `input.value` / `output.value` 完全相同的字段别名；完整输出保留，数据库输出与来源 SHA-256 一致，分别为 6,076,835 和 10,301,875 字节。

真实浏览器验证通过总览、精确 Trace 打开，以及手机端最早一条 Trace 的直达与刷新保持；未出现页面脚本错误或横向溢出。详细验证记录保存在 `.local/codex-import-20260915/`；其中包含私有快照和凭证，受文件权限保护，并排除于 Git 和 Docker 构建上下文之外。

## 本机服务与存储

本轮启动了 Catena Core、PostgreSQL 和 ClickHouse。记忆和分析执行服务未因导入任务自动启动。Windows 下 PostgreSQL 初始化脚本的 CRLF 问题已修复，并为该文件固定 LF。

导入期间 C 盘写满，导致 Docker/WSL 退出。Docker 数据盘已停机复制到 `E:\DockerData\catena-local\data\docker_data.vhdx`，原路径 `%LOCALAPPDATA%\Docker\wsl\disk` 通过目录联接保持兼容。原盘保留在 `E:\DockerData\catena-local\backup\docker_data.vhdx`，迁移时两个副本均通过 SHA-256 校验。已有 Docker 容器和镜像随数据盘保留，没有重置 Docker 或删除其他项目数据。

最终验收依据为 `final-verification.json`、`browser-verification.json` 和导入回执。首次数据库 ID 集合对账记录中发现的缺口已在后续补写中解决，不能把该中间记录当作最终状态。
