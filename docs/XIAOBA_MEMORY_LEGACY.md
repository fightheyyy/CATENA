# XiaoBaOS 对话记忆兼容说明

旧功能保留兼容，当前产品主线参见 [README](../README.md)。

## 接入 XiaoBaOS 对话

XiaoBaOS 使用同一个 Agent 接入密钥增量同步用户可见的 Conversation Journal：

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
OTLP Trace          → Trace Farm → agent.md / Skill / Role / DSH Plugin
XiaoBaOS Conversation → GauzMem   → semantic / graph / temporal memory
```

MVP1 先为 XiaoBaOS 提供第一方 Conversation 协议，因为它具备稳定的用户可见消息日志；其他 Agent 若能提供同等语义的对话事件，后续也可以通过 Conversation Adapter 接入。

