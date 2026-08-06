import { useEffect, useState } from "react";
import {
  agentIdentitySourceLabel,
  agentSourceKindLabel,
  agentSources,
  agentSourceSummary,
} from "./agentView";
import type { WorkspaceData } from "./types";
import { useAgentTraceWindow } from "./useAgentTraceWindow";

type Locale = "zh" | "en";

const agentCopy = {
  zh: {
    title: "Agent",
    body: "选择一个目标 Agent，查看身份来源、运行指标与近期 Trace。编排器和评测角色仍可在 Trace 中查看。",
    observed: "目标 Agent",
    unavailable: "Agent 索引尚未配置",
    unavailableBody: "Trace 仍可查看；完成 Agent 索引配置后即可按 Agent 分析。",
    empty: "还没有收到 Agent Trace。创建 API 密钥并配置 OTLP 后，Agent 会自动出现。",
    traces: "Trace",
    spans: "Span",
    errors: "错误",
    sources: "Trace 来源",
    sourcesBody: "同一个 Agent 的实时与历史证据会统一分析；每条 Trace 仍保留原始来源。",
    sourceUnit: "个来源",
    identity: "身份来源",
    agentID: "Agent ID",
    lastSeen: "最近观测",
    recent: "当前空间的近期 Trace",
    noRecent: "最近 30 天没有这个 Agent 的 Trace。",
    loadingTraces: "正在读取最近 30 天的 Trace",
    loadTracesFailed: "无法读取这个 Agent 的近期 Trace",
    retry: "重试",
    analyze: "分析这个 Agent",
    analyzeBody: "前往 Trace Farm 选择 24 小时、7 天或 30 天的证据窗口。",
  },
  en: {
    title: "Agents",
    body: "Select a target Agent to inspect its identity, metrics, and recent Traces. Orchestrators and evaluator roles remain in Traces.",
    observed: "Target Agents",
    unavailable: "The Agent index is not configured",
    unavailableBody: "Traces remain available. Configure the Agent index to analyze evidence by Agent.",
    empty: "No Agent Trace yet. Create an API key and configure OTLP to register one automatically.",
    traces: "Traces",
    spans: "Spans",
    errors: "Errors",
    sources: "Trace sources",
    sourcesBody: "Live and historical evidence belong to one Agent, while every Trace preserves its original source.",
    sourceUnit: "sources",
    identity: "Identity source",
    agentID: "Agent ID",
    lastSeen: "Last seen",
    recent: "Recent Traces in this workspace",
    noRecent: "This Agent has no Traces in the last 30 days.",
    loadingTraces: "Loading Traces from the last 30 days",
    loadTracesFailed: "Could not load this Agent's recent Traces",
    retry: "Retry",
    analyze: "Analyze this Agent",
    analyzeBody: "Open Trace Farm and choose a 24-hour, 7-day, or 30-day evidence window.",
  },
} as const;

export function AgentWorkspace({
  locale,
  workspace,
  onAnalyze,
}: {
  locale: Locale;
  workspace: WorkspaceData;
  onAnalyze: (agentID: string) => void;
}) {
  const t = agentCopy[locale];
  const [selectedID, setSelectedID] = useState(workspace.agents[0]?.agent_id ?? "");

  useEffect(() => {
    if (workspace.agents.some((agent) => agent.agent_id === selectedID)) return;
    setSelectedID(workspace.agents[0]?.agent_id ?? "");
  }, [selectedID, workspace.agents]);

  const selected = workspace.agents.find((agent) => agent.agent_id === selectedID) ?? workspace.agents[0];
  const selectedSources = selected ? agentSources(selected) : [];
  const traceWindow = useAgentTraceWindow(selected?.agent_id ?? "", 100);
  const recentTraces = traceWindow.traces.slice(0, 8);

  return (
    <section className="page agent-page">
      <header className="page-header"><h1>{t.title}</h1><p>{t.body}</p></header>
      {!workspace.agentAvailable ? (
        <section className="agent-unavailable"><h2>{t.unavailable}</h2><p>{t.unavailableBody}</p></section>
      ) : workspace.agents.length === 0 ? (
        <div className="empty-state">{t.empty}</div>
      ) : (
        <div className="agent-workspace">
          <aside className="agent-index" aria-label={t.observed}>
            <h2>{t.observed}</h2>
            <div className="agent-index-list">
              {workspace.agents.map((agent) => (
                <button
                  className={agent.agent_id === selected?.agent_id ? "agent-index-row selected" : "agent-index-row"}
                  key={agent.agent_id}
                  type="button"
                  aria-pressed={agent.agent_id === selected?.agent_id}
                  onClick={() => setSelectedID(agent.agent_id)}
                >
                  <strong>{agent.display_name}</strong>
                  <span>{agent.trace_count} {t.traces} · {agent.error_count} {t.errors}</span>
                  {agentSources(agent).length > 0 ? (
                    <span>{agentSources(agent).length} {t.sourceUnit} · {agentSourceSummary(agent, locale)}</span>
                  ) : null}
                  <time dateTime={agent.last_seen_at}>{formatTime(agent.last_seen_at, locale)}</time>
                </button>
              ))}
            </div>
          </aside>
          {selected ? (
            <main className="agent-detail">
              <header className="agent-detail-header">
                <div><h2>{selected.display_name}</h2><code>{selected.agent_id}</code></div>
                <button className="primary-button compact" type="button" onClick={() => onAnalyze(selected.agent_id)}>{t.analyze}</button>
              </header>
              <div className="agent-metrics">
                <AgentMetric value={selected.trace_count} label={t.traces} />
                <AgentMetric value={selected.span_count} label={t.spans} />
                <AgentMetric value={selected.error_count} label={t.errors} />
              </div>
              <dl className="agent-identity">
                <div><dt>{t.identity}</dt><dd>{agentIdentitySourceLabel(selected.identity_source || "service.name", locale)}</dd></div>
                <div><dt>{t.agentID}</dt><dd>{selected.agent_id}</dd></div>
                <div><dt>{t.lastSeen}</dt><dd><time dateTime={selected.last_seen_at}>{formatTime(selected.last_seen_at, locale)}</time></dd></div>
              </dl>
              {selectedSources.length > 0 ? (
                <section className="agent-sources">
                  <header><h3>{t.sources}</h3><p>{t.sourcesBody}</p></header>
                  <div className="agent-source-list">
                    {selectedSources.map((source) => (
                      <article key={source.service_name}>
                        <span>{agentSourceKindLabel(source.kind, locale)}</span>
                        <code>{source.service_name}</code>
                      </article>
                    ))}
                  </div>
                </section>
              ) : null}
              <section className="agent-recent-traces">
                <header><h3>{t.recent}</h3><span>{traceWindow.loading ? "…" : traceWindow.error ? "!" : traceWindow.traces.length}</span></header>
                {traceWindow.loading ? <p className="quiet-empty" role="status">{t.loadingTraces}</p> : null}
                {!traceWindow.loading && traceWindow.error ? (
                  <div className="agent-trace-error" role="alert">
                    <p>{t.loadTracesFailed}</p>
                    <button className="text-button" type="button" onClick={traceWindow.retry}>{t.retry}</button>
                  </div>
                ) : null}
                {!traceWindow.loading && !traceWindow.error && recentTraces.length === 0 ? <p className="quiet-empty">{t.noRecent}</p> : null}
                {!traceWindow.loading && !traceWindow.error && recentTraces.length > 0 ? (
                  <div className="agent-trace-list">
                    {recentTraces.map((trace) => (
                      <article key={trace.trace_id}>
                        <div><strong>{trace.root_name || trace.trace_id}</strong><code>{trace.trace_id}</code></div>
                        <span>{trace.service_name}{trace.model ? ` · ${trace.model}` : ""}</span>
                        <span>{trace.span_count} {t.spans} · {trace.error_count} {t.errors}</span>
                        <time dateTime={trace.end_time}>{formatTime(trace.end_time, locale)}</time>
                      </article>
                    ))}
                  </div>
                ) : null}
              </section>
              <p className="agent-analysis-note">{t.analyzeBody}</p>
            </main>
          ) : null}
        </div>
      )}
    </section>
  );
}

function AgentMetric({ value, label }: { value: number; label: string }) {
  return <div><strong>{value}</strong><span>{label}</span></div>;
}

function formatTime(value: string, locale: Locale) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
