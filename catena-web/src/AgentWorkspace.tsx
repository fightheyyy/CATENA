import { useEffect, useMemo, useState } from "react";
import {
  canAnalyzeAgent,
  registeredAgentSummaries,
} from "./agentConnection";
import {
  agentIdentitySourceLabel,
  agentSourceKindLabel,
  agentSources,
} from "./agentView";
import type { WorkspaceData } from "./types";
import { useAgentTraceWindow } from "./useAgentTraceWindow";

type Locale = "zh" | "en";

const agentCopy = {
  zh: {
    title: "Agent",
    body: "接入、观测和进化你的 Agent。每个 Agent 只需要一个专属密钥。",
    targetAgents: "你的 Agent",
    emptyTitle: "还没有 Agent 数据",
    emptyBody: "Agent 上传 Trace 或对话后，会自动出现在这里。接入凭证请到 API 管理页面创建。",
    connect: "接入新 Agent",
    waiting: "等待数据",
    connected: "已连接",
    runtimeUnknown: "Runtime 待识别",
    traces: "Trace",
    conversations: "对话",
    spans: "Span",
    errors: "错误",
    lastSeen: "最近上传",
    recent: "最近 Trace",
    noRecent: "还没有 Trace 数据。",
    loadingTraces: "正在读取 Trace",
    loadTracesFailed: "无法读取这个 Agent 的 Trace",
    retry: "重试",
    openTraces: "查看 Trace",
    analyze: "开始进化",
    analyzeHint: "至少收到 2 条 Trace 后，才能从重复行为中提炼改进资产。",
    advanced: "高级信息",
    identity: "身份归属",
    agentID: "Agent ID",
    sources: "原始数据来源",
    generic: "通用 OTel Agent",
  },
  en: {
    title: "Agents",
    body: "Connect, observe, and evolve your Agents. Each Agent needs one dedicated key.",
    targetAgents: "Your Agents",
    emptyTitle: "No Agent data yet",
    emptyBody: "Agents appear here after uploading Trace or conversation data. Create credentials from API Management.",
    connect: "Connect an Agent",
    waiting: "Waiting for data",
    connected: "Connected",
    runtimeUnknown: "Runtime not detected",
    traces: "Traces",
    conversations: "Conversations",
    spans: "Spans",
    errors: "Errors",
    lastSeen: "Last upload",
    recent: "Recent Traces",
    noRecent: "No Trace data yet.",
    loadingTraces: "Loading Traces",
    loadTracesFailed: "Could not load this Agent's Traces",
    retry: "Retry",
    openTraces: "View Traces",
    analyze: "Start evolution",
    analyzeHint: "At least two Traces are required before Catena can distill recurring behavior into an improvement.",
    advanced: "Advanced details",
    identity: "Identity",
    agentID: "Agent ID",
    sources: "Raw evidence sources",
    generic: "Generic OTel Agent",
  },
} as const;

export function AgentWorkspace({
  locale,
  workspace,
  onAnalyze,
  onOpenTraces,
  onConnect,
}: {
  locale: Locale;
  workspace: WorkspaceData;
  onAnalyze: (agentID: string) => void;
  onOpenTraces: (agentID: string) => void;
  onConnect: () => void;
}) {
  const t = agentCopy[locale];
  const registeredAgents = useMemo(() => registeredAgentSummaries(workspace.agents), [workspace.agents]);
  const [selectedID, setSelectedID] = useState(registeredAgents[0]?.agent_id ?? "");

  useEffect(() => {
    if (registeredAgents.some((agent) => agent.agent_id === selectedID)) return;
    setSelectedID(registeredAgents[0]?.agent_id ?? "");
  }, [registeredAgents, selectedID]);

  const selected = registeredAgents.find((agent) => agent.agent_id === selectedID) ?? registeredAgents[0];
  const selectedSources = selected ? agentSources(selected) : [];
  const traceWindow = useAgentTraceWindow(selected?.agent_id ?? "", 100, selected?.trace_count ?? 0);
  const recentTraces = traceWindow.traces.slice(0, 6);

  return (
    <section className="page agent-page">
      <header className="agent-page-header">
        <div><h1>{t.title}</h1><p>{t.body}</p></div>
      </header>

      {registeredAgents.length === 0 ? (
        <section className="agent-empty">
          <h2>{t.emptyTitle}</h2>
          <p>{t.emptyBody}</p>
          <button className="primary-button compact" type="button" onClick={onConnect}>{t.connect}</button>
        </section>
      ) : (
        <div className="agent-workspace">
          <aside className="agent-index" aria-label={t.targetAgents}>
            <h2>{t.targetAgents}</h2>
            <div className="agent-index-list">
              {registeredAgents.map((agent) => (
                <button className={agent.agent_id === selected?.agent_id ? "agent-index-row selected" : "agent-index-row"} key={agent.agent_id} type="button" aria-pressed={agent.agent_id === selected?.agent_id} onClick={() => setSelectedID(agent.agent_id)}>
                  <span className="agent-index-heading"><strong>{agent.display_name}</strong><i className={agent.connected ? "agent-connection-mark connected" : "agent-connection-mark"} /></span>
                  <span>{runtimeLabel(agent.runtime_kind, locale, t.generic)}</span>
                  <span>{agent.connected ? `${agent.trace_count} ${t.traces}` : t.waiting}</span>
                </button>
              ))}
            </div>
          </aside>

          {selected ? <main className="agent-detail">
            <header className="agent-detail-header">
              <div>
                <div className="agent-runtime-line"><span className={selected.connected ? "agent-state connected" : "agent-state"}>{selected.connected ? t.connected : t.waiting}</span><span>{runtimeLabel(selected.runtime_kind, locale, t.generic)}</span></div>
                <h2>{selected.display_name}</h2>
                <p>{selected.connected ? `${t.lastSeen}: ${formatTime(selected.last_seen_at, locale)}` : (locale === "zh" ? "尚未收到这个 Agent 上传的数据。" : "This Agent has not uploaded data yet.")}</p>
              </div>
              <div className="agent-primary-actions">
                {selected.connected ? <button className="primary-button compact" type="button" onClick={() => onOpenTraces(selected.agent_id)}>{t.openTraces}</button> : null}
              </div>
            </header>

            <div className="agent-metrics">
              <AgentMetric value={selected.conversation_count ?? 0} label={t.conversations} />
              <AgentMetric value={selected.trace_count} label={t.traces} />
              <AgentMetric value={selected.span_count} label={t.spans} />
              <AgentMetric value={selected.error_count} label={t.errors} />
            </div>

            {canAnalyzeAgent(selected) ? (
              <section className="agent-next-action">
                <div><h3>{t.analyze}</h3><p>{locale === "zh" ? "从这个 Agent 最近一段时间的 Trace 中发现重复问题，并生成 agent.md、Skill、Role 或 Harness 建议。" : "Find repeated problems in this Agent's recent Traces and produce agent.md, Skill, Role, or Harness proposals."}</p></div>
                <button className="secondary-button" type="button" onClick={() => onAnalyze(selected.agent_id)}>{t.analyze}</button>
              </section>
            ) : <p className="agent-analysis-note">{t.analyzeHint}</p>}

            <section className="agent-recent-traces">
              <header><h3>{t.recent}</h3><span>{traceWindow.loading ? "..." : traceWindow.error ? "!" : traceWindow.traces.length}</span></header>
              {traceWindow.loading ? <p className="quiet-empty" role="status">{t.loadingTraces}</p> : null}
              {!traceWindow.loading && traceWindow.error ? <div className="agent-trace-error" role="alert"><p>{t.loadTracesFailed}</p><button className="text-button" type="button" onClick={traceWindow.retry}>{t.retry}</button></div> : null}
              {!traceWindow.loading && !traceWindow.error && recentTraces.length === 0 ? <p className="quiet-empty">{t.noRecent}</p> : null}
              {!traceWindow.loading && !traceWindow.error && recentTraces.length > 0 ? <div className="agent-trace-list">{recentTraces.map((trace) => <button key={trace.trace_id} type="button" onClick={() => onOpenTraces(selected.agent_id)}><div><strong>{trace.root_name || trace.trace_id}</strong><code>{trace.trace_id}</code></div><span>{trace.span_count} {t.spans}</span><span>{trace.error_count} {t.errors}</span><time dateTime={trace.end_time}>{formatTime(trace.end_time, locale)}</time></button>)}</div> : null}
            </section>

            <details className="agent-advanced">
              <summary>{t.advanced}</summary>
              <dl className="agent-identity">
                <div><dt>{t.identity}</dt><dd>{agentIdentitySourceLabel(selected.identity_source || "service.name", locale)}</dd></div>
                <div><dt>{t.agentID}</dt><dd>{selected.agent_id}</dd></div>
              </dl>
              {selectedSources.length > 0 ? <section className="agent-sources"><header><h3>{t.sources}</h3></header><div className="agent-source-list">{selectedSources.map((source) => <article key={source.service_name}><span>{agentSourceKindLabel(source.kind, locale)}</span><code>{source.service_name}</code></article>)}</div></section> : null}
            </details>
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
  if (!value) return locale === "zh" ? "Runtime 待识别" : "Runtime not detected";
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
