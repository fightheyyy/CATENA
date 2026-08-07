import { useEffect, useState } from "react";
import { api } from "./api";
import {
  agentIdentitySourceLabel,
  agentSourceKindLabel,
  agentSources,
} from "./agentView";
import { copyText } from "./clipboard";
import type { WorkspaceData } from "./types";
import { useAgentTraceWindow } from "./useAgentTraceWindow";

type Locale = "zh" | "en";

const agentCopy = {
  zh: {
    title: "Agent",
    body: "每个 Agent 都有固定身份。接入密钥决定 Conversation 与 Trace 归属于谁，Runtime 由数据自动识别。",
    connect: "接入新 Agent",
    connectBody: "只需要给它起一个名字。Catena 会创建固定 Agent ID，并生成专属接入密钥。",
    name: "Agent 名称",
    placeholder: "例如：大狗",
    generate: "生成接入密钥",
    creating: "正在创建…",
    created: "已接入。保存下面的密钥，然后把它交给你的 Agent。",
    targetAgents: "已接入 Agent",
    empty: "还没有 Agent。输入一个名称即可开始。",
    waiting: "等待首次上传",
    connected: "已连接",
    paused: "接入已暂停",
    runtime: "自动识别",
    traces: "Trace",
    conversations: "对话",
    spans: "Span",
    errors: "错误",
    sources: "数据来源",
    sourcesBody: "Catena 保留原始 service.name，同时用接入密钥把所有数据归到这个 Agent。",
    identity: "身份归属",
    agentID: "Agent ID",
    lastSeen: "最近上传",
    recent: "最近 30 天 Trace",
    noRecent: "还没有 Trace。密钥完成配置后，这里会自动更新。",
    loadingTraces: "正在读取 Trace",
    loadTracesFailed: "无法读取这个 Agent 的 Trace",
    retry: "重试",
    analyze: "送到 Trace Farm",
    analyzeBody: "按 Agent 聚合一段时间的证据，再提炼 agent.md、Skill、Role 或 Harness 建议。",
    credential: "接入密钥",
    credentialBody: "这个密钥固定绑定当前 Agent。Conversation 和 Trace 无需再携带可伪造的归属配置。",
    copy: "复制密钥",
    copied: "已复制",
    delete: "删除密钥",
    confirmDelete: "再次点击确认删除",
    recreate: "重新生成密钥",
    hide: "收起",
    revealHint: "可随时回来复制；不要提交到代码仓库。",
    keyDeleted: "密钥已删除，Agent 会停止接收新数据。",
    generic: "通用 OTel Agent",
  },
  en: {
    title: "Agents",
    body: "Every Agent has a stable identity. Its connection key owns incoming Conversations and Traces; Runtime is inferred from evidence.",
    connect: "Connect a new Agent",
    connectBody: "Give it a name. Catena creates a stable Agent ID and a dedicated connection key.",
    name: "Agent name",
    placeholder: "For example: Big Dog",
    generate: "Generate connection key",
    creating: "Creating…",
    created: "Connected. Save the key below, then give it to your Agent.",
    targetAgents: "Connected Agents",
    empty: "No Agent yet. Enter a name to get started.",
    waiting: "Waiting for first upload",
    connected: "Connected",
    paused: "Connection paused",
    runtime: "Auto detected",
    traces: "Traces",
    conversations: "Conversations",
    spans: "Spans",
    errors: "Errors",
    sources: "Evidence sources",
    sourcesBody: "Catena preserves service.name while the connection key assigns all evidence to this Agent.",
    identity: "Identity",
    agentID: "Agent ID",
    lastSeen: "Last upload",
    recent: "Traces from the last 30 days",
    noRecent: "No Trace yet. This view updates automatically after the key is configured.",
    loadingTraces: "Loading Traces",
    loadTracesFailed: "Could not load this Agent's Traces",
    retry: "Retry",
    analyze: "Send to Trace Farm",
    analyzeBody: "Aggregate an Agent's evidence window, then distill agent.md, Skill, Role, or Harness proposals.",
    credential: "Connection key",
    credentialBody: "This key is permanently bound to this Agent. Conversations and Traces need no client-controlled ownership field.",
    copy: "Copy key",
    copied: "Copied",
    delete: "Delete key",
    confirmDelete: "Click again to confirm",
    recreate: "Generate a new key",
    hide: "Hide",
    revealHint: "You can return to copy it later. Never commit it to source control.",
    keyDeleted: "Key deleted. The Agent will stop accepting new evidence.",
    generic: "Generic OTel Agent",
  },
} as const;

export function AgentWorkspace({
  locale,
  workspace,
  onAnalyze,
  onRefresh,
}: {
  locale: Locale;
  workspace: WorkspaceData;
  onAnalyze: (agentID: string) => void;
  onRefresh: () => Promise<void>;
}) {
  const t = agentCopy[locale];
  const [selectedID, setSelectedID] = useState(workspace.agents[0]?.agent_id ?? "");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [revealed, setRevealed] = useState<{ id: string; token: string } | null>(null);
  const [copiedID, setCopiedID] = useState("");
  const [confirmDeleteID, setConfirmDeleteID] = useState("");

  useEffect(() => {
    if (workspace.agents.some((agent) => agent.agent_id === selectedID)) return;
    setSelectedID(workspace.agents[0]?.agent_id ?? "");
  }, [selectedID, workspace.agents]);

  const selected = workspace.agents.find((agent) => agent.agent_id === selectedID) ?? workspace.agents[0];
  const selectedConnection = selected
    ? selected.registered && !selected.credential ? t.paused : selected.connected ? t.connected : t.waiting
    : "";
  const selectedSources = selected ? agentSources(selected) : [];
  const traceWindow = useAgentTraceWindow(selected?.agent_id ?? "", 100, selected?.trace_count ?? 0);
  const recentTraces = traceWindow.traces.slice(0, 8);

  const showKey = async (tokenID: string) => {
    setBusy(true);
    setError("");
    try {
      const result = await api.revealApiToken(tokenID);
      setRevealed({ id: tokenID, token: result.token });
      if (await copyText(result.token)) setCopiedID(tokenID);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="page agent-page">
      <header className="page-header"><h1>{t.title}</h1><p>{t.body}</p></header>

      <section className="agent-onboarding">
        <div><h2>{t.connect}</h2><p>{t.connectBody}</p></div>
        <form onSubmit={async (event) => {
          event.preventDefault();
          if (!name.trim() || busy) return;
          setBusy(true);
          setMessage("");
          setError("");
          try {
            const result = await api.createAgent(name.trim());
            setName("");
            setRevealed({ id: result.api_token.id, token: result.token });
            setSelectedID(result.agent.agent_id);
            setMessage(t.created);
            await onRefresh();
            setSelectedID(result.agent.agent_id);
          } catch (cause) {
            setError(cause instanceof Error ? cause.message : "Request failed");
          } finally {
            setBusy(false);
          }
        }}>
          <label><span>{t.name}</span><input autoComplete="off" value={name} maxLength={80} placeholder={t.placeholder} onChange={(event) => setName(event.target.value)} /></label>
          <button className="primary-button compact" disabled={!name.trim() || busy} type="submit">{busy ? t.creating : t.generate}</button>
        </form>
        {message ? <p className="agent-onboarding-message" role="status">{message}</p> : null}
        {error ? <p className="agent-onboarding-error" role="alert">{error}</p> : null}
      </section>

      {workspace.agents.length === 0 ? <div className="empty-state">{t.empty}</div> : (
        <div className="agent-workspace">
          <aside className="agent-index" aria-label={t.targetAgents}>
            <h2>{t.targetAgents}</h2>
            <div className="agent-index-list">
              {workspace.agents.map((agent) => (
                <button className={agent.agent_id === selected?.agent_id ? "agent-index-row selected" : "agent-index-row"} key={agent.agent_id} type="button" aria-pressed={agent.agent_id === selected?.agent_id} onClick={() => setSelectedID(agent.agent_id)}>
                  <strong>{agent.display_name}</strong>
                  <span>{runtimeLabel(agent.runtime_kind, locale, t.generic)} · {agent.registered && !agent.credential ? t.paused : agent.connected ? t.connected : t.waiting}</span>
                  <span>{agent.conversation_count ?? 0} {t.conversations} · {agent.trace_count} {t.traces}</span>
                  <time dateTime={agent.last_seen_at}>{agent.connected ? formatTime(agent.last_seen_at, locale) : "—"}</time>
                </button>
              ))}
            </div>
          </aside>

          {selected ? <main className="agent-detail">
            <header className="agent-detail-header">
              <div>
                <div className="agent-runtime-line"><span className={selected.connected && selectedConnection === t.connected ? "agent-state connected" : "agent-state"}>{selectedConnection}</span><span>{runtimeLabel(selected.runtime_kind, locale, t.generic)}</span></div>
                <h2>{selected.display_name}</h2>
                <code>{selected.agent_id}</code>
              </div>
              <button className="primary-button compact" type="button" onClick={() => onAnalyze(selected.agent_id)}>{t.analyze}</button>
            </header>

            {selected.registered ? <section className="agent-credential">
              <header><div><h3>{t.credential}</h3><p>{t.credentialBody}</p></div>
                {selected.credential ? <div className="agent-credential-actions">
                  <button className="text-button" type="button" disabled={busy} onClick={() => void showKey(selected.credential!.id)}>{copiedID === selected.credential.id ? t.copied : t.copy}</button>
                  <button className="text-button danger" type="button" disabled={busy} onClick={async () => {
                    if (confirmDeleteID !== selected.credential!.id) { setConfirmDeleteID(selected.credential!.id); return; }
                    setBusy(true); setError("");
                    try { await api.deleteApiToken(selected.credential!.id); setRevealed(null); setConfirmDeleteID(""); setMessage(t.keyDeleted); await onRefresh(); }
                    catch (cause) { setError(cause instanceof Error ? cause.message : "Request failed"); }
                    finally { setBusy(false); }
                  }}>{confirmDeleteID === selected.credential.id ? t.confirmDelete : t.delete}</button>
                </div> : <button className="text-button" type="button" disabled={busy} onClick={async () => {
                  setBusy(true); setError("");
                  try { const result = await api.createAgentConnectionKey(selected.agent_id); setRevealed({ id: result.api_token.id, token: result.token }); await onRefresh(); }
                  catch (cause) { setError(cause instanceof Error ? cause.message : "Request failed"); }
                  finally { setBusy(false); }
                }}>{t.recreate}</button>}
              </header>
              {revealed && revealed.id === selected.credential?.id ? <div className="agent-key-reveal" role="status"><input readOnly spellCheck={false} value={revealed.token} onFocus={(event) => event.currentTarget.select()} onClick={(event) => event.currentTarget.select()} /><span>{t.revealHint}</span><button className="text-button" type="button" onClick={() => setRevealed(null)}>{t.hide}</button></div> : selected.credential ? <code>{selected.credential.masked_token}</code> : null}
            </section> : null}

            <div className="agent-metrics">
              <AgentMetric value={selected.conversation_count ?? 0} label={t.conversations} />
              <AgentMetric value={selected.trace_count} label={t.traces} />
              <AgentMetric value={selected.span_count} label={t.spans} />
              <AgentMetric value={selected.error_count} label={t.errors} />
            </div>
            <dl className="agent-identity">
              <div><dt>{t.identity}</dt><dd>{agentIdentitySourceLabel(selected.identity_source || "service.name", locale)}</dd></div>
              <div><dt>{t.agentID}</dt><dd>{selected.agent_id}</dd></div>
              <div><dt>{t.lastSeen}</dt><dd>{selected.connected ? <time dateTime={selected.last_seen_at}>{formatTime(selected.last_seen_at, locale)}</time> : "—"}</dd></div>
            </dl>
            {selectedSources.length > 0 ? <section className="agent-sources"><header><h3>{t.sources}</h3><p>{t.sourcesBody}</p></header><div className="agent-source-list">{selectedSources.map((source) => <article key={source.service_name}><span>{agentSourceKindLabel(source.kind, locale)}</span><code>{source.service_name}</code></article>)}</div></section> : null}
            <section className="agent-recent-traces">
              <header><h3>{t.recent}</h3><span>{traceWindow.loading ? "…" : traceWindow.error ? "!" : traceWindow.traces.length}</span></header>
              {traceWindow.loading ? <p className="quiet-empty" role="status">{t.loadingTraces}</p> : null}
              {!traceWindow.loading && traceWindow.error ? <div className="agent-trace-error" role="alert"><p>{t.loadTracesFailed}</p><button className="text-button" type="button" onClick={traceWindow.retry}>{t.retry}</button></div> : null}
              {!traceWindow.loading && !traceWindow.error && recentTraces.length === 0 ? <p className="quiet-empty">{t.noRecent}</p> : null}
              {!traceWindow.loading && !traceWindow.error && recentTraces.length > 0 ? <div className="agent-trace-list">{recentTraces.map((trace) => <article key={trace.trace_id}><div><strong>{trace.root_name || trace.trace_id}</strong><code>{trace.trace_id}</code></div><span>{trace.service_name}{trace.model ? ` · ${trace.model}` : ""}</span><span>{trace.span_count} {t.spans} · {trace.error_count} {t.errors}</span><time dateTime={trace.end_time}>{formatTime(trace.end_time, locale)}</time></article>)}</div> : null}
            </section>
            <p className="agent-analysis-note">{t.analyzeBody}</p>
          </main> : null}
        </div>
      )}
    </section>
  );
}

function AgentMetric({ value, label }: { value: number; label: string }) {
  return <div><strong>{value}</strong><span>{label}</span></div>;
}

function runtimeLabel(value: string | undefined, locale: Locale, generic: string) {
  if (!value) return locale === "zh" ? "待识别" : "Not detected";
  if (value === "xiaobaos") return "XiaoBaOS";
  if (value === "codex") return "Codex";
  if (value === "claude_code") return "Claude Code";
  if (value === "otel") return generic;
  return value;
}

function formatTime(value: string, locale: Locale) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(date);
}
