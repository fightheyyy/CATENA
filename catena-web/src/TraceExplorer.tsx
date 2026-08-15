import { useEffect, useMemo, useState } from "react";
import { api } from "./api";
import type { TraceDetail, TraceSpan, TraceSummary, WorkspaceData } from "./types";
import {
  TRACE_STEP_PAGE_SIZE,
  boundedTraceSteps,
  buildTraceSemanticView,
  filterTraceSummaries,
  formatTraceEvidence,
  groupTraceSummariesBySession,
  presentTraceEvidence,
  preferredTraceSpan,
  shortTraceID,
  tracesForAgentSelection,
  traceSpanDepth,
  traceSpanAttributeString,
  traceSpanToolName,
  traceSessionKey,
  traceSessionTitle,
  traceSummaryTitle,
  type TraceFilter,
  type TraceLens,
  type TraceSpanSemanticKind,
} from "./traceView";
import { useAgentTraceWindow } from "./useAgentTraceWindow";
import { TraceNarrative } from "./TraceNarrative";

type Locale = "zh" | "en";

const traceCopy = {
  zh: {
    title: "Trace",
    body: "从用户请求开始，顺着模型、工具和分支看完整执行过程。",
    recent: "最近 100 条",
    agentRecent: "所选 Agent · 最近 30 天",
    agentLoading: "正在读取这个 Agent 的 Trace",
    agentLoadFailed: "无法读取这个 Agent 的 Trace",
    search: "搜索 Trace、Agent 或模型",
    agentFilter: "Agent 筛选",
    allAgents: "全部 Agent",
    all: "全部",
    errorsOnly: "错误",
    multiStep: "多步骤",
    sessions: "Session",
    traces: "Trace",
    ungroupedSession: "未归组 Session",
    ungroupedHint: "这些 Trace 没有导出 Session 标识",
    hierarchy: "证据层级",
    agentLevel: "AGENT",
    sessionLevel: "SESSION",
    traceLevel: "TRACE",
    spanLevel: "SPAN",
    spans: "Span",
    errors: "错误",
    duration: "耗时",
    noMatch: "没有匹配的 Trace",
    choose: "选择一条 Trace 查看完整执行链",
    loading: "正在读取执行证据",
    retry: "重试",
    detailFailed: "Trace 详情读取失败",
    traceUnavailable: "Trace 存储尚未配置",
    traceUnavailableBody: "为 Catena Server 配置 ClickHouse 后，即可直接接收和查询 OTLP Trace。",
    noTraces: "等待第一条 Trace",
    noTracesBody: "从 Agent 页复制专属接入配置，把 OTLP/HTTP exporter 指向这个地址。",
    endpoint: "OTLP Endpoint",
    execution: "Agent 执行链",
    selectSpan: "默认折叠 Runtime 内部噪声，选择一步查看证据",
    agentLens: "关键链路",
    narrativeLens: "执行过程",
    toolsLens: "工具",
    errorsLens: "错误",
    rawLens: "原始 Span",
    turns: "Turn",
    models: "模型调用",
    tools: "工具与产物",
    checks: "验证",
    events: "分支与事件",
    rawSpans: "原始 Span",
    folded: "条 Runtime 内部 Span 已折叠",
    rawHint: "原始 OTel 视图用于排障，并按批次加载，避免超大 Trace 卡住浏览器。",
    noAgentSteps: "这条 Trace 没有导出可识别的 Agent、模型、工具或错误边界。你仍可查看原始 Span。",
    noTools: "这条 Trace 没有导出可识别的工具调用。",
    noErrors: "这条 Trace 没有错误 Span。",
    showMore: "继续显示 200 条",
    remaining: "条尚未显示",
    kinds: { run: "RUN", turn: "TURN", model: "MODEL", tool: "TOOL", artifact: "ARTIFACT", subagent: "SUBAGENT", retry: "RETRY", compact: "COMPACT", check: "CHECK", error: "ERROR", internal: "OTEL" },
    input: "输入",
    output: "输出",
    attributes: "属性",
    rawData: "原始数据",
    hiddenContext: "条系统、工具或注入上下文已折叠",
    hiddenFields: "个次要字段已折叠",
    truncatedEvidence: "结构化证据在导出时被截断，未上传的内容无法恢复。",
    roles: { user: "用户", assistant: "Agent", system: "系统", tool: "工具" },
    noEvidence: "这个 Span 没有导出输入或输出证据。",
    metadataOnly: "Runtime 只导出了工具名与状态，没有输入或输出正文。",
    modelMetadataOnly: "XiaoBaOS 只上传了模型调用的耗时、Token 和状态；对话正文请看所属 Turn。",
    runSummary: "测试摘要",
    caseLabel: "Case",
    runLabel: "Run",
    checkPassed: "通过",
    checkFailed: "未通过",
    checkRules: { contains_any: "至少包含一项", contains_all: "必须全部包含", excludes: "不得包含" },
    statusOk: "成功",
    statusError: "失败",
    statusAborted: "已中止",
    statusIncomplete: "未完成",
    backToList: "返回 Trace 列表",
  },
  en: {
    title: "Traces",
    body: "Start with the request, then follow every model call, tool, and branch.",
    recent: "Latest 100",
    agentRecent: "Selected Agent · last 30 days",
    agentLoading: "Loading this Agent's Traces",
    agentLoadFailed: "Could not load this Agent's Traces",
    search: "Search Trace, Agent, or model",
    agentFilter: "Agent filter",
    allAgents: "All Agents",
    all: "All",
    errorsOnly: "Errors",
    multiStep: "Multi-step",
    sessions: "Sessions",
    traces: "Traces",
    ungroupedSession: "Ungrouped Session",
    ungroupedHint: "These Traces exported no Session identity",
    hierarchy: "Evidence hierarchy",
    agentLevel: "AGENT",
    sessionLevel: "SESSION",
    traceLevel: "TRACE",
    spanLevel: "SPAN",
    spans: "Spans",
    errors: "Errors",
    duration: "Duration",
    noMatch: "No matching Traces",
    choose: "Select a Trace to inspect its execution chain",
    loading: "Reading execution evidence",
    retry: "Retry",
    detailFailed: "Could not read Trace detail",
    traceUnavailable: "Trace storage is not configured",
    traceUnavailableBody: "Configure ClickHouse for Catena Server to receive and query OTLP Traces directly.",
    noTraces: "Waiting for the first Trace",
    noTracesBody: "Copy the dedicated connection configuration from Agents and point the OTLP/HTTP exporter to this endpoint.",
    endpoint: "OTLP Endpoint",
    execution: "Agent execution chain",
    selectSpan: "Runtime internals are folded by default. Select a step to inspect evidence.",
    agentLens: "Critical path",
    narrativeLens: "Execution",
    toolsLens: "Tools",
    errorsLens: "Errors",
    rawLens: "Raw Spans",
    turns: "Turns",
    models: "Model calls",
    tools: "Tools & artifacts",
    checks: "Checks",
    events: "Branches & events",
    rawSpans: "Raw Spans",
    folded: "Runtime-internal Spans folded",
    rawHint: "The raw OTel view loads in bounded batches so a large Trace cannot freeze the browser.",
    noAgentSteps: "This Trace exported no recognizable Agent, model, tool, or error boundary. Raw Spans remain available.",
    noTools: "This Trace exported no recognizable tool calls.",
    noErrors: "This Trace contains no error Spans.",
    showMore: "Show 200 more",
    remaining: "not shown",
    kinds: { run: "RUN", turn: "TURN", model: "MODEL", tool: "TOOL", artifact: "ARTIFACT", subagent: "SUBAGENT", retry: "RETRY", compact: "COMPACT", check: "CHECK", error: "ERROR", internal: "OTEL" },
    input: "Input",
    output: "Output",
    attributes: "Attributes",
    rawData: "Raw data",
    hiddenContext: "system, tool, or injected context items folded",
    hiddenFields: "secondary fields folded",
    truncatedEvidence: "Structured evidence was truncated during export; content that was not uploaded cannot be recovered.",
    roles: { user: "User", assistant: "Agent", system: "System", tool: "Tool" },
    noEvidence: "This Span exported no input or output evidence.",
    metadataOnly: "The Runtime exported the tool name and status, but no input or output content.",
    modelMetadataOnly: "XiaoBaOS exported model duration, token usage, and status only. Read the owning Turn for the conversation.",
    runSummary: "Test summary",
    caseLabel: "Case",
    runLabel: "Run",
    checkPassed: "Passed",
    checkFailed: "Failed",
    checkRules: { contains_any: "Contains any", contains_all: "Contains all", excludes: "Excludes" },
    statusOk: "Success",
    statusError: "Failed",
    statusAborted: "Aborted",
    statusIncomplete: "Incomplete",
    backToList: "Back to Trace list",
  },
} as const;

export function TraceExplorer({
  locale,
  workspace,
  initialAgentID,
}: {
  locale: Locale;
  workspace: WorkspaceData;
  initialAgentID?: string;
}) {
  const t = traceCopy[locale];
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<TraceFilter>("all");
  const [agentID, setAgentID] = useState(initialAgentID ?? "");
  const [selectedTraceID, setSelectedTraceID] = useState(workspace.traces[0]?.trace_id ?? "");
  const [detail, setDetail] = useState<TraceDetail | null>(null);
  const [detailError, setDetailError] = useState("");
  const [detailLoading, setDetailLoading] = useState(false);
  const [selectedSpanID, setSelectedSpanID] = useState("");
  const [detailRequestVersion, setDetailRequestVersion] = useState(0);
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false);
  const [expandedSessionKey, setExpandedSessionKey] = useState(
    workspace.traces[0] ? traceSessionKey(workspace.traces[0], initialAgentID) : "",
  );
  const agentWindow = useAgentTraceWindow(agentID, 500);
  const sourceTraces = useMemo(
    () => tracesForAgentSelection(workspace.traces, agentID, agentWindow.traces),
    [agentID, agentWindow.traces, workspace.traces],
  );

  useEffect(() => {
    if (initialAgentID && workspace.agents.some((agent) => agent.agent_id === initialAgentID)) {
      setAgentID(initialAgentID);
    }
  }, [initialAgentID, workspace.agents]);

  const filteredTraces = useMemo(
    () => filterTraceSummaries(sourceTraces, query, filter),
    [sourceTraces, query, filter],
  );
  const sessionGroups = useMemo(
    () => groupTraceSummariesBySession(filteredTraces, agentID),
    [agentID, filteredTraces],
  );
  const agentNames = useMemo(
    () => new Map(workspace.agents.map((agent) => [agent.agent_id, agent.display_name])),
    [workspace.agents],
  );

  useEffect(() => {
    if (filteredTraces.length === 0) {
      setSelectedTraceID("");
      setDetail(null);
      return;
    }
    if (!filteredTraces.some((trace) => trace.trace_id === selectedTraceID)) {
      setSelectedTraceID(filteredTraces[0].trace_id);
    }
  }, [filteredTraces, selectedTraceID]);

  useEffect(() => {
    const selected = filteredTraces.find((trace) => trace.trace_id === selectedTraceID);
    if (selected) setExpandedSessionKey(traceSessionKey(selected, agentID));
  }, [agentID, filteredTraces, selectedTraceID]);

  useEffect(() => {
    if (!selectedTraceID) return;
    let active = true;
    setDetailError("");
    setDetailLoading(true);
    void api.trace(selectedTraceID).then((nextDetail) => {
      if (!active) return;
      setDetail(nextDetail);
      const firstUsefulSpan = preferredTraceSpan(buildTraceSemanticView(nextDetail.spans));
      setSelectedSpanID(firstUsefulSpan?.span_id ?? "");
      const detailShell = document.getElementById("selected-trace-detail");
      if (detailShell) detailShell.scrollTop = 0;
      if (window.matchMedia("(max-width: 720px)").matches) {
        window.requestAnimationFrame(() => {
          detailShell?.scrollIntoView({ block: "start" });
        });
      }
    }).catch((cause) => {
      if (!active) return;
      setDetail(null);
      setDetailError(cause instanceof Error ? cause.message : t.detailFailed);
    }).finally(() => {
      if (active) setDetailLoading(false);
    });
    return () => { active = false; };
  }, [selectedTraceID, detailRequestVersion, t.detailFailed]);

  return (
    <section className="page trace-page">
      <header className="trace-page-header">
        <div><h1>{t.title}</h1><p>{t.body}</p></div>
        <span>{agentID ? t.agentRecent : t.recent}</span>
      </header>
      {!workspace.traceAvailable ? (
        <TraceConnectState title={t.traceUnavailable} body={t.traceUnavailableBody} endpointLabel={t.endpoint} />
      ) : workspace.traces.length === 0 ? (
        <TraceConnectState title={t.noTraces} body={t.noTracesBody} endpointLabel={t.endpoint} />
      ) : (
        <div className={mobileDetailOpen ? "trace-browser detail-open" : "trace-browser"}>
          <aside className="trace-index" aria-label={t.title}>
            <div className="trace-index-tools">
              <label className="trace-agent-filter">
                <span>{t.agentFilter}</span>
                <select value={agentID} onChange={(event) => {
                  setAgentID(event.target.value);
                  setMobileDetailOpen(false);
                }}>
                  <option value="">{t.allAgents}</option>
                  {workspace.agents.map((agent) => <option value={agent.agent_id} key={agent.agent_id}>{agent.display_name}</option>)}
                </select>
              </label>
              <label className="trace-search">
                <span className="sr-only">{t.search}</span>
                <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t.search} />
              </label>
              <div className="trace-filter-row">
                {([
                  ["all", t.all],
                  ["errors", t.errorsOnly],
                  ["multi", t.multiStep],
                ] as Array<[TraceFilter, string]>).map(([value, label]) => (
                  <button
                    className={filter === value ? "trace-filter active" : "trace-filter"}
                    key={value}
                    type="button"
                    aria-pressed={filter === value}
                    onClick={() => setFilter(value)}
                  >{label}</button>
                ))}
                <span className="trace-result-count">
                  {agentWindow.loading ? "…" : agentWindow.error ? "!" : `${sessionGroups.length} ${t.sessions} · ${filteredTraces.length} ${t.traces}`}
                </span>
              </div>
            </div>
            <div className="trace-index-list">
              {agentWindow.loading ? <div className="trace-list-empty" role="status">{t.agentLoading}</div> : null}
              {!agentWindow.loading && agentWindow.error ? (
                <div className="trace-list-empty trace-list-error" role="alert">
                  <span>{t.agentLoadFailed}</span>
                  <button className="text-button" type="button" onClick={agentWindow.retry}>{t.retry}</button>
                </div>
              ) : null}
              {!agentWindow.loading && !agentWindow.error && filteredTraces.length === 0 ? <div className="trace-list-empty">{t.noMatch}</div> : null}
              {!agentWindow.loading && !agentWindow.error ? sessionGroups.map((group, groupIndex) => {
                const expanded = expandedSessionKey === group.key;
                const groupAgentName = agentNames.get(group.agentID) || group.agentID;
                const sessionIdentity = group.sessionID ? shortTraceID(group.sessionID) : t.ungroupedSession;
                const sessionTitle = traceSessionTitle(group);
                const tracePanelID = `trace-session-panel-${groupIndex}`;
                return (
                  <section className={expanded ? "trace-session expanded" : "trace-session"} key={group.key}>
                    <button
                      className="trace-session-heading"
                      type="button"
                      aria-expanded={expanded}
                      aria-controls={tracePanelID}
                      onClick={() => {
                        const nextExpanded = !expanded;
                        setExpandedSessionKey(nextExpanded ? group.key : "");
                        if (nextExpanded && !group.traces.some((trace) => trace.trace_id === selectedTraceID)) {
                          setSelectedTraceID(group.traces[0].trace_id);
                        }
                      }}
                    >
                      <span className="trace-session-identity">
                        <span className="trace-session-kicker">
                          <em>{t.sessionLevel}</em>
                          {groupAgentName || sessionTitle ? <small>{[groupAgentName, sessionTitle ? sessionIdentity : ""].filter(Boolean).join(" · ")}</small> : null}
                        </span>
                        <strong>{sessionTitle || sessionIdentity}</strong>
                      </span>
                      <span className="trace-session-facts">
                        <span>{group.traces.length} {t.traces}</span>
                        <span>{group.spanCount} {t.spans}</span>
                        <TraceTime value={group.endedAt} locale={locale} />
                        <i aria-hidden="true">⌄</i>
                      </span>
                    </button>
                    {!group.sessionID && expanded ? <p className="trace-session-hint">{t.ungroupedHint}</p> : null}
                    {expanded ? <div className="trace-session-traces" id={tracePanelID}>{group.traces.map((trace) => (
                      <TraceIndexRow
                        key={trace.trace_id}
                        trace={trace}
                        locale={locale}
                        selected={selectedTraceID === trace.trace_id}
                        labels={t}
                        onSelect={() => {
                          setSelectedTraceID(trace.trace_id);
                          setMobileDetailOpen(true);
                        }}
                      />
                    ))}</div> : null}
                  </section>
                );
              }) : null}
            </div>
          </aside>
          <main className="trace-detail-shell" id="selected-trace-detail">
            {detailLoading ? <TraceDetailLoading label={t.loading} /> : null}
            {!detailLoading && detailError ? (
              <div className="trace-detail-state" role="alert"><p>{detailError}</p><button className="text-button" type="button" onClick={() => setDetailRequestVersion((value) => value + 1)}>{t.retry}</button></div>
            ) : null}
            {!detailLoading && !detailError && !detail ? <div className="trace-detail-state"><p>{t.choose}</p></div> : null}
            {!detailLoading && detail ? (
              <TraceDetailWorkspace
                key={detail.summary.trace_id}
                detail={detail}
                locale={locale}
                selectedSpanID={selectedSpanID}
                onSelectSpan={setSelectedSpanID}
                onBack={() => {
                  setMobileDetailOpen(false);
                  window.requestAnimationFrame(() => {
                    document.querySelector<HTMLElement>(".trace-index")?.scrollIntoView({ block: "start" });
                  });
                }}
                agentName={agentNames.get(detail.summary.agent_id || agentID) || detail.summary.agent_id || agentNames.get(agentID) || agentID}
              />
            ) : null}
          </main>
        </div>
      )}
    </section>
  );
}

function TraceIndexRow({
  trace,
  locale,
  selected,
  labels,
  onSelect,
}: {
  trace: TraceSummary;
  locale: Locale;
  selected: boolean;
  labels: (typeof traceCopy)[Locale];
  onSelect: () => void;
}) {
  return (
    <button className={selected ? "trace-index-row selected" : "trace-index-row"} type="button" aria-current={selected ? "true" : undefined} onClick={onSelect}>
      <span className="trace-index-heading">
        <span className="trace-index-title">
          <em>{labels.traceLevel} · {shortTraceID(trace.trace_id)}</em>
          <strong>{traceSummaryTitle(trace, locale)}</strong>
        </span>
        <i className={trace.error_count > 0 ? "trace-state-dot error" : "trace-state-dot"} aria-label={trace.error_count > 0 ? labels.statusError : labels.statusOk} />
      </span>
      <span className="trace-index-source">{trace.service_name}{trace.model ? ` / ${trace.model}` : ""}</span>
      <span className="trace-index-facts">
        <span>{trace.span_count} {labels.spans}</span>
        {trace.error_count > 0 ? <span className="trace-error-fact">{trace.error_count} {labels.errors}</span> : null}
        <span>{formatDuration(trace.duration_ms)}</span>
        <TraceTime value={trace.end_time} locale={locale} />
      </span>
    </button>
  );
}

function TraceDetailWorkspace({
  detail,
  locale,
  selectedSpanID,
  onSelectSpan,
  onBack,
  agentName,
}: {
  detail: TraceDetail;
  locale: Locale;
  selectedSpanID: string;
  onSelectSpan: (spanID: string) => void;
  onBack: () => void;
  agentName: string;
}) {
  const t = traceCopy[locale];
  const spansByID = useMemo(() => new Map(detail.spans.map((span) => [span.span_id, span])), [detail.spans]);
  const semanticView = useMemo(() => buildTraceSemanticView(detail.spans), [detail.spans]);
  const hasCanonicalNarrative = semanticView.canonicalNodeCount > 0;
  const [lens, setLens] = useState<TraceLens>(hasCanonicalNarrative ? "narrative" : "agent");
  const [stepLimit, setStepLimit] = useState(TRACE_STEP_PAGE_SIZE);
  const visible = useMemo(
    () => boundedTraceSteps(semanticView, lens, stepLimit),
    [lens, semanticView, stepLimit],
  );
  const traceStart = new Date(detail.summary.start_time).getTime();
  const traceDuration = Math.max(detail.summary.duration_ms, 1);
  const primaryTurn = semanticView.agentSteps.find(({ span, kind }) => (
    kind === "turn" && traceSpanAttributeString(span, "catena.node.kind") === "turn"
  ))?.span ?? semanticView.agentSteps.find(({ kind }) => kind === "turn")?.span;
  const traceState = primaryTurn ? traceSpanAttributeString(primaryTurn, "catena.state") || (primaryTurn.status_code === 2 ? "error" : "ok") : detail.summary.error_count > 0 ? "error" : "ok";
  const traceStatus = traceState === "aborted"
    ? t.statusAborted
    : traceState === "incomplete"
      ? t.statusIncomplete
      : traceState === "error" || detail.summary.error_count > 0
        ? t.statusError
        : t.statusOk;
  const traceHeadline = primaryTurn?.input ? compactTraceHeadline(primaryTurn.input) : "";

  function selectLens(nextLens: TraceLens) {
    setLens(nextLens);
    setStepLimit(TRACE_STEP_PAGE_SIZE);
    const first = boundedTraceSteps(semanticView, nextLens, 1).steps[0]?.span;
    onSelectSpan(first?.span_id ?? "");
  }

  const emptyMessage = lens === "tools"
    ? t.noTools
    : lens === "errors"
      ? t.noErrors
      : t.noAgentSteps;

  return (
    <article className="trace-detail-workspace">
      <header className="trace-detail-header">
        <button className="trace-back-button" type="button" onClick={onBack}>{t.backToList}</button>
        <nav className="trace-hierarchy" aria-label={t.hierarchy}>
          <span><em>{t.agentLevel}</em><strong>{agentName || "—"}</strong></span>
          <i aria-hidden="true">›</i>
          <span><em>{t.sessionLevel}</em><strong>{detail.summary.session_id ? shortTraceID(detail.summary.session_id) : t.ungroupedSession}</strong></span>
          <i aria-hidden="true">›</i>
          <span><em>{t.traceLevel}</em><strong>{shortTraceID(detail.summary.trace_id)}</strong></span>
          <i aria-hidden="true">›</i>
          <span className="current"><em>{t.spanLevel}</em><strong>{selectedSpanID ? shortTraceID(selectedSpanID) : "—"}</strong></span>
        </nav>
        <div className="trace-detail-title-row">
          <div className="trace-detail-identity">
            <h2 title={primaryTurn?.input}>{traceHeadline || `${t.traceLevel} ${shortTraceID(detail.summary.trace_id)}`}</h2>
            <span title={detail.summary.trace_id}>{detail.summary.service_name}{detail.summary.model ? ` · ${detail.summary.model}` : ""} · {shortTraceID(detail.summary.trace_id)}</span>
          </div>
          <span className={traceState === "ok" && detail.summary.error_count === 0 ? "trace-detail-status" : "trace-detail-status error"}>
            {traceStatus}
          </span>
        </div>
        <div className="trace-detail-facts">
          <span>{detail.summary.span_count} {t.spans}</span>
          <span className={detail.summary.error_count > 0 ? "has-error" : ""}>{detail.summary.error_count} {t.errors}</span>
          <span>{t.duration} {formatDuration(detail.summary.duration_ms)}</span>
          <TraceTime value={detail.summary.end_time} locale={locale} />
        </div>
      </header>
      <section className="trace-semantic-overview" aria-label={t.execution}>
        <div><strong>{semanticView.turnCount}</strong><span>{t.turns}</span></div>
        <div><strong>{semanticView.counts.model}</strong><span>{t.models}</span></div>
        <div><strong>{semanticView.counts.tool + semanticView.counts.artifact}</strong><span>{t.tools}</span></div>
        <div><strong>{hasCanonicalNarrative ? semanticView.counts.subagent + semanticView.counts.retry + semanticView.counts.compact : semanticView.counts.check}</strong><span>{hasCanonicalNarrative ? t.events : t.checks}</span></div>
        <div className={semanticView.counts.error > 0 ? "has-error" : ""}><strong>{semanticView.counts.error}</strong><span>{t.errors}</span></div>
      </section>
      <section className="trace-execution">
        <div className="trace-execution-heading"><h3>{t.execution}</h3><span>{t.selectSpan}</span></div>
        <div className="trace-lens-tabs" role="tablist" aria-label={t.execution}>
          {((hasCanonicalNarrative ? [
            ["narrative", t.narrativeLens, semanticView.agentSteps.length],
            ["tools", t.toolsLens, semanticView.toolSteps.length],
            ["errors", t.errorsLens, semanticView.errorSteps.length],
            ["raw", t.rawLens, semanticView.rawSteps.length],
          ] : [
            ["agent", t.agentLens, semanticView.agentSteps.length],
            ["tools", t.toolsLens, semanticView.toolSteps.length],
            ["errors", t.errorsLens, semanticView.errorSteps.length],
            ["raw", t.rawLens, semanticView.rawSteps.length],
          ]) as Array<[TraceLens, string, number]>).map(([value, label, count]) => (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={lens === value}
              className={lens === value ? "active" : ""}
              onClick={() => selectLens(value)}
            >{label}<span>{count}</span></button>
          ))}
        </div>
        {lens === "agent" && semanticView.foldedInternalCount > 0 ? (
          <p className="trace-folded-note"><strong>{semanticView.foldedInternalCount}</strong> {t.folded}</p>
        ) : null}
        {lens === "raw" ? <p className="trace-raw-note">{t.rawHint}</p> : null}
        {lens === "narrative" ? (
          <TraceNarrative detail={detail} locale={locale} selectedSpanID={selectedSpanID} onSelectSpan={onSelectSpan} />
        ) : <>
          <TraceRuler duration={traceDuration} />
          <div className="trace-span-list">
          {visible.steps.length === 0 ? <div className="trace-lens-empty">{emptyMessage}</div> : null}
          {visible.steps.map(({ span, kind }) => {
            const start = new Date(span.start_time).getTime();
            const end = new Date(span.end_time).getTime();
            const left = Math.max(0, ((start - traceStart) / traceDuration) * 100);
            const width = Math.max(1.2, ((end - start) / traceDuration) * 100);
            const toolName = traceSpanToolName(span);
            const depth = traceSpanDepth(span, spansByID);
            const selected = selectedSpanID === span.span_id;
            return (
              <div className={selected ? "trace-span selected" : "trace-span"} key={span.span_id}>
                <button className="trace-span-row" type="button" onClick={() => onSelectSpan(span.span_id)} aria-expanded={selected}>
                  <span className="trace-span-name" style={{ paddingLeft: `${Math.min(depth, 6) * 15}px` }}>
                    <i className={`trace-span-kind ${kind}`} />
                    <span className="trace-span-copy">
                      <em>{t.kinds[kind]}</em>
                      <strong>{traceSpanDisplayName(span, kind, locale, detail.summary.model)}</strong>
                      {traceSpanDisplayName(span, kind, locale, detail.summary.model) !== span.name ? <small>{span.name}</small> : null}
                    </span>
                  </span>
                  <span className="trace-span-track"><i className={span.status_code === 2 ? "trace-span-bar error" : "trace-span-bar"} style={{ left: `${left}%`, width: `${Math.min(width, 100 - left)}%` }} /></span>
                  <span className="trace-span-duration">{formatDuration(Math.max(0, end - start))}</span>
                </button>
                {selected ? <SpanEvidence span={span} kind={kind} toolName={toolName} locale={locale} /> : null}
              </div>
            );
          })}
          </div>
          {visible.hiddenCount > 0 ? (
            <div className="trace-more">
              <span>{visible.hiddenCount} {t.remaining}</span>
              <button type="button" className="text-button" onClick={() => setStepLimit((value) => value + TRACE_STEP_PAGE_SIZE)}>{t.showMore}</button>
            </div>
          ) : null}
        </>}
      </section>
    </article>
  );
}

function traceSpanDisplayName(span: TraceSpan, kind: TraceSpanSemanticKind, locale: Locale, traceModel?: string) {
  const toolName = traceSpanToolName(span);
  if (toolName) return toolName;
  if (kind === "run") return locale === "zh" ? "测试运行" : "Test run";
  if (kind === "check") {
    const assertionKind = traceSpanAttributeString(span, "barena.assertion.kind");
    const rules = traceCopy[locale].checkRules;
    return rules[assertionKind as keyof typeof rules] ?? (locale === "zh" ? "结果验证" : "Result check");
  }
  if (kind === "turn") {
    const turn = traceSpanAttributeString(span, "barena.turn.index");
    if (turn) return `Turn ${turn}`;
  }
  if (kind === "subagent") {
    return traceSpanAttributeString(span, "agent.subagent.thread.id", "agent.turn.id")
      || (locale === "zh" ? "Subagent 分支" : "Subagent branch");
  }
  if (kind === "retry") return locale === "zh" ? "模型重试" : "Model retry";
  if (kind === "compact") return locale === "zh" ? "上下文压缩" : "Context compact";
  if (kind === "model") {
    return span.model
      || traceSpanAttributeString(span, "gen_ai.request.model", "gen_ai.response.model", "model")
      || traceModel
      || (span.name === "xiaoba.model.call" ? (locale === "zh" ? "模型调用" : "Model call") : span.name);
  }
  return span.name;
}

function compactTraceHeadline(value: string) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized.length > 88 ? `${normalized.slice(0, 88)}…` : normalized;
}

function SpanEvidence({
  span,
  kind,
  toolName,
  locale,
}: {
  span: TraceSpan;
  kind: TraceSpanSemanticKind;
  toolName: string;
  locale: Locale;
}) {
  const t = traceCopy[locale];
  const diagnosticAttributes = Object.fromEntries(Object.entries(span.attributes).filter(([key]) => ![
    "input.value",
    "output.value",
    "gen_ai.tool.call.arguments",
    "gen_ai.tool.call.result",
    "tool.call.arguments",
    "tool.call.result",
  ].includes(key)));
  const attributes = Object.keys(diagnosticAttributes).length > 0 ? JSON.stringify(diagnosticAttributes, null, 2) : "";
  if (kind === "run" && span.name === "barena.simulation") {
    return <BarenaRunEvidence span={span} locale={locale} attributes={attributes} />;
  }
  if (kind === "check" && span.name === "barena.assertion") {
    return <BarenaCheckEvidence span={span} locale={locale} attributes={attributes} />;
  }
  const missingEvidence = kind === "model" && span.name === "xiaoba.model.call"
    ? t.modelMetadataOnly
    : toolName
      ? t.metadataOnly
      : t.noEvidence;
  return (
    <div className="trace-span-inspector">
      {span.status_code === 2 && span.status_message ? <p className="trace-span-error">{span.status_message}</p> : null}
      <div className="trace-evidence-grid">
        {span.input ? <EvidenceBlock title={t.input} value={span.input} kind={kind} direction="input" locale={locale} /> : null}
        {span.output ? <EvidenceBlock title={t.output} value={span.output} kind={kind} direction="output" locale={locale} /> : null}
      </div>
      {!span.input && !span.output ? <p className="trace-no-evidence">{missingEvidence}</p> : null}
      {attributes ? <details className="trace-attributes"><summary>{t.attributes}</summary><pre>{attributes}</pre></details> : null}
    </div>
  );
}

function BarenaRunEvidence({ span, locale, attributes }: { span: TraceSpan; locale: Locale; attributes: string }) {
  const t = traceCopy[locale];
  const status = traceSpanAttributeString(span, "barena.result.status");
  const passed = status === "pass" && span.status_code !== 2;
  return (
    <div className="trace-run-evidence">
      <div className="trace-run-evidence-heading">
        <span>{t.runSummary}</span>
        <strong className={passed ? "pass" : "fail"}>{passed ? t.checkPassed : t.checkFailed}</strong>
      </div>
      <dl>
        <div><dt>{t.caseLabel}</dt><dd>{traceSpanAttributeString(span, "barena.case.id") || "—"}</dd></div>
        <div><dt>{t.runLabel}</dt><dd>{traceSpanAttributeString(span, "barena.run.id") || "—"}</dd></div>
        <div><dt>{t.turns}</dt><dd>{traceSpanAttributeString(span, "barena.turn.count") || "—"}</dd></div>
        <div><dt>{t.checks}</dt><dd>{traceSpanAttributeString(span, "barena.assertion.passed_count") || "—"} / {traceSpanAttributeString(span, "barena.assertion.count") || "—"}</dd></div>
      </dl>
      {span.output ? <p>{span.output}</p> : null}
      {attributes ? <details className="trace-attributes"><summary>{t.attributes}</summary><pre>{attributes}</pre></details> : null}
    </div>
  );
}

function BarenaCheckEvidence({ span, locale, attributes }: { span: TraceSpan; locale: Locale; attributes: string }) {
  const t = traceCopy[locale];
  const assertionKind = traceSpanAttributeString(span, "barena.assertion.kind");
  const passed = traceSpanAttributeString(span, "barena.assertion.status") === "pass" && span.status_code !== 2;
  const expected = parseStringList(span.input);
  const rule = t.checkRules[assertionKind as keyof typeof t.checkRules] ?? (locale === "zh" ? "验证条件" : "Rule");
  return (
    <div className="trace-check-evidence">
      <div className="trace-check-evidence-heading">
        <span>{rule}</span>
        <strong className={passed ? "pass" : "fail"}>{passed ? t.checkPassed : t.checkFailed}</strong>
      </div>
      {expected.length > 0 ? <ul>{expected.map((value) => <li key={value}>{value}</li>)}</ul> : null}
      {span.output ? <p>{span.output}</p> : null}
      {attributes ? <details className="trace-attributes"><summary>{t.attributes}</summary><pre>{attributes}</pre></details> : null}
    </div>
  );
}

function parseStringList(value?: string) {
  if (!value) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === "string") : [];
  } catch {
    return [];
  }
}

function EvidenceBlock({
  title,
  value,
  kind,
  direction,
  locale,
}: {
  title: string;
  value: string;
  kind: TraceSpanSemanticKind;
  direction: "input" | "output";
  locale: Locale;
}) {
  const t = traceCopy[locale];
  const evidence = presentTraceEvidence(value, kind, direction);
  return (
    <section className={`trace-evidence-block ${evidence.kind}`}>
      <h4>{title}</h4>
      {evidence.kind === "messages" ? (
        <div className="trace-evidence-messages">
          {evidence.messages.map((message, index) => (
            <article className={`trace-evidence-message ${message.role}`} key={`${message.role}-${index}`}>
              <span>{t.roles[message.role]}</span>
              <p>{message.text}</p>
            </article>
          ))}
        </div>
      ) : null}
      {evidence.kind === "fields" ? (
        <dl className="trace-evidence-fields">
          {evidence.fields.map((field) => (
            <div key={field.key}>
              <dt>{field.key}</dt>
              <dd className={field.code ? "code" : ""}>{field.value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {evidence.kind === "terminal" ? <pre className="trace-evidence-terminal">{evidence.text}</pre> : null}
      {evidence.kind === "text" ? <p className="trace-evidence-text">{evidence.text}</p> : null}
      {evidence.hiddenContextCount > 0 ? (
        <p className="trace-evidence-folded"><strong>{evidence.hiddenContextCount}</strong> {t.hiddenContext}</p>
      ) : null}
      {evidence.hiddenFieldCount > 0 ? (
        <p className="trace-evidence-folded"><strong>{evidence.hiddenFieldCount}</strong> {t.hiddenFields}</p>
      ) : null}
      {evidence.truncated ? <p className="trace-evidence-warning">{t.truncatedEvidence}</p> : null}
      {evidence.structured ? (
        <details className="trace-evidence-raw">
          <summary>{t.rawData}</summary>
          <pre>{formatTraceEvidence(value)}</pre>
        </details>
      ) : null}
    </section>
  );
}

function TraceRuler({ duration }: { duration: number }) {
  return (
    <div className="trace-ruler" aria-hidden="true">
      <span />
      <div>{[0, 0.25, 0.5, 0.75, 1].map((part) => <i key={part}>{formatDuration(duration * part)}</i>)}</div>
      <span />
    </div>
  );
}

function TraceConnectState({ title, body, endpointLabel }: { title: string; body: string; endpointLabel: string }) {
  return (
    <section className="migration-state">
      <div className="migration-mark" aria-hidden="true"><img src="/catena-mark.svg" alt="" /></div>
      <div><h2>{title}</h2><p>{body}</p></div>
      <div className="endpoint-code"><span>{endpointLabel}</span><code>/v1/otlp/v1/traces</code></div>
    </section>
  );
}

function TraceDetailLoading({ label }: { label: string }) {
  return <div className="trace-detail-loading" role="status"><span>{label}</span><i /><i /><i /><i /></div>;
}

function TraceTime({ value, locale }: { value: string; locale: Locale }) {
  return <time dateTime={value}>{new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value))}</time>;
}

function formatDuration(value: number) {
  if (value < 1000) return `${Math.max(0, Math.round(value))} ms`;
  if (value < 60_000) return `${(value / 1000).toFixed(1)} s`;
  return `${(value / 60_000).toFixed(1)} min`;
}
