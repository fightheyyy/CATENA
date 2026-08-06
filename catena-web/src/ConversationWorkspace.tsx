import { useEffect, useMemo, useState } from "react";
import { api } from "./api";
import { conversationKey, orderedConversationMessages } from "./conversationView";
import type { ConversationDocument, ConversationMessage, ConversationSummary } from "./types";

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
    remembering: "正在提炼",
    memoryQueued: "已提交",
    memoryFailed: "记忆提炼失败",
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
    remembering: "Distilling",
    memoryQueued: "Submitted",
    memoryFailed: "Memory distillation failed",
  },
} as const;

export function ConversationWorkspace({ locale, memoryReady }: { locale: Locale; memoryReady: boolean }) {
  const t = copy[locale];
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [document, setDocument] = useState<ConversationDocument | null>(null);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState("");
  const [detailError, setDetailError] = useState("");

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
      <div className="conversation-browser">
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
                    onClick={() => setSelectedKey(key)}
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
          ) : document ? <ConversationThread document={document} locale={locale} memoryReady={memoryReady} /> : <ConversationState label={t.choose} />}
        </main>
      </div>
    </section>
  );
}

function ConversationThread({ document, locale, memoryReady }: { document: ConversationDocument; locale: Locale; memoryReady: boolean }) {
  const t = copy[locale];
  const messages = orderedConversationMessages(document.messages);
  const [memoryState, setMemoryState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const [memoryMessage, setMemoryMessage] = useState("");
  useEffect(() => {
    setMemoryState("idle");
    setMemoryMessage("");
  }, [document.conversation.agent_id, document.conversation.conversation_id]);
  return (
    <div className="conversation-thread">
      <header className="conversation-detail-header">
        <div>
          <span>{document.conversation.surface} · {document.conversation.agent_name || document.conversation.agent_id}</span>
          <h2>{document.conversation.title}</h2>
        </div>
        <div className="conversation-detail-actions">
          <strong>{document.conversation.message_count} {t.messages}</strong>
          {memoryReady ? (
            <button
              className="text-button"
              type="button"
              disabled={memoryState === "busy" || memoryState === "done"}
              onClick={async () => {
                setMemoryState("busy");
                setMemoryMessage("");
                try {
                  await api.rememberConversation(document.conversation.agent_id, document.conversation.conversation_id);
                  setMemoryState("done");
                  setMemoryMessage(t.memoryQueued);
                } catch (cause) {
                  setMemoryState("error");
                  setMemoryMessage(cause instanceof Error ? cause.message : t.memoryFailed);
                }
              }}
            >{memoryState === "busy" ? t.remembering : memoryState === "done" ? t.memoryQueued : t.remember}</button>
          ) : null}
        </div>
      </header>
      {memoryMessage && memoryState === "error" ? <p className="conversation-memory-note error">{memoryMessage}</p> : null}
      <ol className="conversation-message-list">
        {messages.map((message) => <ConversationMessageRow key={message.message_id} message={message} locale={locale} />)}
      </ol>
    </div>
  );
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
