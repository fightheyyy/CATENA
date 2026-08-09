import { useEffect, useMemo, useState } from "react";
import { api } from "./api";
import { conversationKey, orderedConversationMessages } from "./conversationView";
import { isMemoryTaskActive, memoryTaskDisplayPercent, memoryTaskFromReceipt, memoryTaskStorageKey } from "./memoryTaskView";
import type { ConversationDocument, ConversationMessage, ConversationSummary, MemoryTaskStatus } from "./types";

type Locale = "zh" | "en";

const copy = {
  zh: {
    title: "对话",
    body: "查看 XiaoBaOS 用户真正看到的消息。系统提示词、思考过程和工具内部数据不会出现在这里。",
    list: "最近对话",
    refresh: "刷新",
    loading: "正在读取对话",
    loadFailed: "无法读取对话记录",
    detailFailed: "无法读取这段对话",
    empty: "还没有 XiaoBaOS 对话。配置同一个 Catena API 密钥后，新消息会自动出现。",
    choose: "选择一段对话查看完整消息。",
    user: "你",
    agent: "XiaoBaOS",
    messages: "条消息",
    trace: "关联 Trace",
    exclusive: "XiaoBaOS 原生 Conversation",
    remember: "提炼为记忆",
    submitting: "正在提交",
    memoryWaiting: "等待开始",
    memoryWorking: "正在提炼",
    memoryDone: "记忆已生成",
    memoryFailed: "记忆提炼失败",
    memoryExpired: "任务状态已过期，请确认记忆结果或重新提炼。",
    retryMemory: "重新提炼",
    viewMemory: "查看记忆",
    backToList: "返回对话列表",
  },
  en: {
    title: "Conversations",
    body: "See what XiaoBaOS users actually saw. System prompts, reasoning, and tool internals never appear here.",
    list: "Recent conversations",
    refresh: "Refresh",
    loading: "Loading conversations",
    loadFailed: "Could not load conversations",
    detailFailed: "Could not load this conversation",
    empty: "No XiaoBaOS conversation yet. Configure the same Catena API key and new messages will appear automatically.",
    choose: "Choose a conversation to read its visible messages.",
    user: "You",
    agent: "XiaoBaOS",
    messages: "messages",
    trace: "Trace",
    exclusive: "XiaoBaOS native Conversation",
    remember: "Distill to memory",
    submitting: "Submitting",
    memoryWaiting: "Waiting to start",
    memoryWorking: "Distilling",
    memoryDone: "Memory created",
    memoryFailed: "Memory distillation failed",
    memoryExpired: "Task status expired. Check the memory result or retry.",
    retryMemory: "Retry",
    viewMemory: "View memory",
    backToList: "Back to conversations",
  },
} as const;

export function ConversationWorkspace({
  locale,
  memoryReady,
  onOpenMemory,
}: {
  locale: Locale;
  memoryReady: boolean;
  onOpenMemory: () => void;
}) {
  const t = copy[locale];
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [document, setDocument] = useState<ConversationDocument | null>(null);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false);

  const selected = useMemo(
    () => conversations.find((item) => conversationKey(item) === selectedKey) ?? conversations[0] ?? null,
    [conversations, selectedKey],
  );

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api.conversations(100);
      setConversations(response.conversations);
      setSelectedKey((current) => (
        response.conversations.some((item) => conversationKey(item) === current)
          ? current
          : response.conversations[0] ? conversationKey(response.conversations[0]) : ""
      ));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t.loadFailed);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (!selected) {
      setDocument(null);
      return;
    }
    let active = true;
    setDetailLoading(true);
    setDetailError("");
    void api.conversation(selected.agent_id, selected.conversation_id)
      .then((next) => {
        if (active) setDocument(next);
      })
      .catch((cause) => {
        if (!active) return;
        setDocument(null);
        setDetailError(cause instanceof Error ? cause.message : t.detailFailed);
      })
      .finally(() => {
        if (active) setDetailLoading(false);
      });
    return () => { active = false; };
  }, [selected?.agent_id, selected?.conversation_id, t.detailFailed]);

  return (
    <section className="page conversation-page">
      <header className="conversation-page-header">
        <div><p>{t.exclusive}</p><h1>{t.title}</h1><span>{t.body}</span></div>
        <button className="text-button" type="button" onClick={() => void load()} disabled={loading}>{t.refresh}</button>
      </header>
      <div className={mobileDetailOpen ? "conversation-browser detail-open" : "conversation-browser"}>
        <aside className="conversation-index">
          <header><h2>{t.list}</h2><span>{conversations.length}</span></header>
          {loading ? <ConversationState label={t.loading} /> : error ? <ConversationState label={error || t.loadFailed} tone="error" /> : conversations.length === 0 ? (
            <ConversationState label={t.empty} />
          ) : (
            <div className="conversation-index-list">
              {conversations.map((item) => {
                const key = conversationKey(item);
                return (
                  <button
                    className={selected && conversationKey(selected) === key ? "conversation-index-row selected" : "conversation-index-row"}
                    key={key}
                    type="button"
                    onClick={() => {
                      setSelectedKey(key);
                      setMobileDetailOpen(true);
                    }}
                  >
                    <span className="conversation-index-meta"><b>{item.surface}</b><time>{formatTime(item.updated_at, locale)}</time></span>
                    <strong>{item.title}</strong>
                    <span className="conversation-index-preview">{item.last_visible_message_preview || `${item.message_count} ${t.messages}`}</span>
                  </button>
                );
              })}
            </div>
          )}
        </aside>
        <main className="conversation-detail">
          {!selected ? <ConversationState label={t.choose} /> : detailLoading ? <ConversationState label={t.loading} /> : detailError ? (
            <ConversationState label={detailError || t.detailFailed} tone="error" />
          ) : document ? (
            <ConversationThread
              key={conversationKey(document.conversation)}
              document={document}
              locale={locale}
              memoryReady={memoryReady}
              onOpenMemory={onOpenMemory}
              onBack={() => setMobileDetailOpen(false)}
            />
          ) : <ConversationState label={t.choose} />}
        </main>
      </div>
    </section>
  );
}

function ConversationThread({
  document,
  locale,
  memoryReady,
  onOpenMemory,
  onBack,
}: {
  document: ConversationDocument;
  locale: Locale;
  memoryReady: boolean;
  onOpenMemory: () => void;
  onBack: () => void;
}) {
  const t = copy[locale];
  const messages = orderedConversationMessages(document.messages);
  const storageKey = memoryTaskStorageKey(document.conversation.agent_id, document.conversation.conversation_id);
  const [submittingMemory, setSubmittingMemory] = useState(false);
  const [memoryTask, setMemoryTask] = useState<MemoryTaskStatus | null>(null);
  const [memoryError, setMemoryError] = useState("");

  useEffect(() => {
    const taskID = readStoredTaskID(storageKey);
    if (taskID) {
      setMemoryTask({ task_id: taskID, status: "pending", progress: 0, steps: [] });
    }
  }, [storageKey]);

  useEffect(() => {
    if (!isMemoryTaskActive(memoryTask)) return;
    const taskID = memoryTask?.task_id;
    if (!taskID) return;
    let active = true;
    let failures = 0;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const next = await api.memoryTask(taskID);
        if (!active) return;
        failures = 0;
        setMemoryTask(next);
        setMemoryError("");
        if (next.status === "pending" || next.status === "processing") {
          timer = window.setTimeout(() => void poll(), 1250);
        } else {
          removeStoredTaskID(storageKey);
        }
      } catch (cause) {
        if (!active) return;
        failures += 1;
        if (failures < 3) {
          timer = window.setTimeout(() => void poll(), 1500);
          return;
        }
        removeStoredTaskID(storageKey);
        const message = cause instanceof Error && cause.message ? cause.message : t.memoryExpired;
        setMemoryTask((current) => current ? { ...current, status: "failed", error: message } : current);
        setMemoryError(message);
      }
    };
    timer = window.setTimeout(() => void poll(), 350);
    return () => {
      active = false;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [memoryTask?.task_id, memoryTask?.status, storageKey, t.memoryExpired]);

  const startMemoryTask = async () => {
    setSubmittingMemory(true);
    setMemoryError("");
    setMemoryTask(null);
    removeStoredTaskID(storageKey);
    try {
      const receipt = await api.rememberConversation(document.conversation.agent_id, document.conversation.conversation_id);
      const next = memoryTaskFromReceipt(receipt);
      setMemoryTask(next);
      storeTaskID(storageKey, next.task_id);
    } catch (cause) {
      setMemoryError(cause instanceof Error ? cause.message : t.memoryFailed);
    } finally {
      setSubmittingMemory(false);
    }
  };

  const activeTask = isMemoryTaskActive(memoryTask);
  const progress = memoryTaskDisplayPercent(memoryTask);
  const buttonLabel = submittingMemory
    ? t.submitting
    : activeTask
      ? `${t.memoryWorking} ${progress}%`
      : memoryTask?.status === "completed"
        ? t.memoryDone
        : memoryTask?.status === "failed" || memoryError
          ? t.retryMemory
          : t.remember;

  return (
    <div className="conversation-thread">
      <header className="conversation-detail-header">
        <div>
          <button className="conversation-back-button" type="button" onClick={onBack}>{t.backToList}</button>
          <span>{document.conversation.surface} · {document.conversation.agent_name || document.conversation.agent_id}</span>
          <h2>{document.conversation.title}</h2>
        </div>
        <div className="conversation-detail-actions">
          <strong>{document.conversation.message_count} {t.messages}</strong>
          {memoryReady ? (
            <button
              className="text-button"
              type="button"
              disabled={submittingMemory || activeTask || memoryTask?.status === "completed"}
              onClick={() => void startMemoryTask()}
            >{buttonLabel}</button>
          ) : null}
        </div>
      </header>
      {memoryTask || memoryError ? (
        <MemoryTaskProgress
          task={memoryTask}
          error={memoryError}
          locale={locale}
          onRetry={() => void startMemoryTask()}
          onOpenMemory={onOpenMemory}
        />
      ) : null}
      <ol className="conversation-message-list">
        {messages.map((message) => <ConversationMessageRow key={message.message_id} message={message} locale={locale} />)}
      </ol>
    </div>
  );
}

function MemoryTaskProgress({
  task,
  error,
  locale,
  onRetry,
  onOpenMemory,
}: {
  task: MemoryTaskStatus | null;
  error: string;
  locale: Locale;
  onRetry: () => void;
  onOpenMemory: () => void;
}) {
  const t = copy[locale];
  const status = task?.status ?? "failed";
  const progress = memoryTaskDisplayPercent(task);
  const title = status === "completed"
    ? t.memoryDone
    : status === "failed"
      ? t.memoryFailed
      : status === "pending"
        ? t.memoryWaiting
        : task?.current_step
          ? `${t.memoryWorking} · ${memoryStepLabel(task.current_step, locale)}`
          : t.memoryWorking;
  const detail = status === "failed" ? (task?.error || error || t.memoryFailed) : task?.message;

  return (
    <section className={`conversation-memory-progress ${status}`} aria-live="polite">
      <div className="conversation-memory-progress-heading">
        <div>
          <strong>{title}</strong>
          {detail ? <span>{detail}</span> : null}
        </div>
        {status === "pending" || status === "processing" ? <b>{progress}%</b> : null}
        {status === "completed" ? <button className="text-button" type="button" onClick={onOpenMemory}>{t.viewMemory}</button> : null}
        {status === "failed" ? <button className="text-button" type="button" onClick={onRetry}>{t.retryMemory}</button> : null}
      </div>
      {status === "pending" || status === "processing" ? (
        <div className="conversation-memory-meter" aria-label={`${progress}%`}>
          <i style={{ width: `${progress}%` }} />
        </div>
      ) : null}
      {task?.steps.length ? (
        <ol className="conversation-memory-steps">
          {task.steps.map((step, index) => (
            <li className={step.status} key={`${step.name}-${index}`}>
              <i />
              <span>{memoryStepLabel(step.name, locale)}</span>
            </li>
          ))}
        </ol>
      ) : null}
    </section>
  );
}

function memoryStepLabel(value: string, locale: Locale) {
  const normalized = value.trim().replaceAll("_", " ");
  if (locale === "en") return normalized;
  const labels: Array<[RegExp, string]> = [
    [/fact/i, "提取事实"],
    [/entit/i, "识别实体"],
    [/relation/i, "分析关系"],
    [/graph/i, "构建知识图谱"],
    [/topic|cluster/i, "聚合主题"],
    [/vector|embed/i, "生成语义索引"],
    [/memor/i, "写入长期记忆"],
  ];
  return labels.find(([pattern]) => pattern.test(normalized))?.[1] ?? normalized;
}

function readStoredTaskID(key: string) {
  try {
    return window.localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function storeTaskID(key: string, taskID: string) {
  try {
    window.localStorage.setItem(key, taskID);
  } catch {
    // Polling still works in the current tab when storage is unavailable.
  }
}

function removeStoredTaskID(key: string) {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Nothing to clean when storage is unavailable.
  }
}

function ConversationMessageRow({ message, locale }: { message: ConversationMessage; locale: Locale }) {
  const t = copy[locale];
  return (
    <li className={`conversation-message ${message.role}`}>
      <header>
        <strong>{message.role === "user" ? t.user : message.role_name || message.agent_name || t.agent}</strong>
        <time>{formatTime(message.occurred_at, locale, true)}</time>
      </header>
      <div className="conversation-content">
        {message.content.map((part, index) => part.type === "text" ? (
          <p key={`${message.message_id}-text-${index}`}>{part.text}</p>
        ) : (
          <span className="conversation-file" key={`${message.message_id}-file-${index}`}>{part.name}</span>
        ))}
      </div>
      {message.trace_id ? <footer><span>{t.trace}</span><code>{shortID(message.trace_id)}</code></footer> : null}
    </li>
  );
}

function ConversationState({ label, tone = "quiet" }: { label: string; tone?: "quiet" | "error" }) {
  return <div className={`conversation-state ${tone}`}><p>{label}</p></div>;
}

function formatTime(value: string, locale: Locale, includeTime = false) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    month: "short",
    day: "numeric",
    ...(includeTime ? { hour: "2-digit", minute: "2-digit" } : {}),
  }).format(date);
}

function shortID(value: string) {
  return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value;
}
