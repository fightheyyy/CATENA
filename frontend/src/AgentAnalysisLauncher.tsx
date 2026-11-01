import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api";
import {
  agentAnalysisRequestSignature,
  agentAnalysisTraceSelection,
  agentAnalysisWindow,
  canStartAgentAnalysis,
  type AnalysisWindowPreset,
} from "./traceFarm";
import type { AgentSummary, AgentTraceWindow, AnalysisJob } from "./types";

type Locale = "zh" | "en";

const launcherCopy = {
  zh: {
    title: "开始一次分析",
    body: "选择 Agent 和时间范围，Catena 会自动发现重复问题并生成可复制资产。",
    agent: "Agent",
    chooseAgent: "选择 Agent",
    window: "分析范围",
    objective: "希望重点关注什么？（可选）",
    objectivePlaceholder: "不填写则自动发现最值得修复的问题",
    traces: "条 Trace",
    matched: "可用",
    loading: "正在读取近期 Trace",
    insufficient: "这个范围内还没有足够的 Trace，至少需要 2 条。",
    ready: "可以开始",
    unavailable: "暂时无法读取这个 Agent 的 Trace。",
    retry: "重试",
    start: "开始分析",
    starting: "正在创建分析",
    failed: "分析任务创建失败",
    noAgents: "接入第一个 Agent Trace 后，就可以从这里开始分析。",
  },
  en: {
    title: "Start an analysis",
    body: "Choose an Agent and time range. Catena will find recurring problems and produce copyable assets.",
    agent: "Agent",
    chooseAgent: "Choose an Agent",
    window: "Time range",
    objective: "What should Catena focus on? (optional)",
    objectivePlaceholder: "Leave blank to find the highest-impact problem",
    traces: "Traces",
    matched: "Available",
    loading: "Reading recent Traces",
    insufficient: "This range does not have enough evidence yet. At least two Traces are required.",
    ready: "Ready",
    unavailable: "Could not read this Agent's Traces.",
    retry: "Retry",
    start: "Start analysis",
    starting: "Creating analysis",
    failed: "Could not create the analysis",
    noAgents: "Connect the first Agent Trace to start an analysis here.",
  },
} as const;

const presets: AnalysisWindowPreset[] = ["24h", "7d", "30d"];

export function AgentAnalysisLauncher({
  locale,
  agents,
  initialAgentID,
  onStarted,
  embedded = false,
  initialObjective = "",
  onObjectiveChange,
  enabled = true,
}: {
  locale: Locale;
  agents: AgentSummary[];
  initialAgentID?: string;
  onStarted: (job: AnalysisJob) => void;
  embedded?: boolean;
  initialObjective?: string;
  onObjectiveChange?: (value: string) => void;
  enabled?: boolean;
}) {
  const t = launcherCopy[locale];
  const firstAgentID = agents[0]?.agent_id ?? "";
  const [agentID, setAgentID] = useState(
    agents.some((agent) => agent.agent_id === initialAgentID) ? initialAgentID ?? "" : firstAgentID,
  );
  const [preset, setPreset] = useState<AnalysisWindowPreset>("7d");
  const [pipeline, setPipeline] = useState<"rsi" | "memory">("rsi");
  const [caseID, setCaseID] = useState("");
  const [cases, setCases] = useState<string[]>([]);
  useEffect(() => { void api.evaluationCases().then(r => setCases((r.cases ?? []).map((c: {id: string}) => c.id))).catch(() => setCases([])); }, []);
  const [objective, setObjective] = useState(initialObjective);
  const [preview, setPreview] = useState<AgentTraceWindow | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState("");
  const [submitError, setSubmitError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const requestIdentity = useRef({ signature: "", key: "" });

  useEffect(() => {
    if (initialAgentID && agents.some((agent) => agent.agent_id === initialAgentID)) {
      setAgentID(initialAgentID);
    }
  }, [agents, initialAgentID]);

  useEffect(() => {
    if (!agents.some((agent) => agent.agent_id === agentID)) setAgentID(firstAgentID);
  }, [agentID, agents, firstAgentID]);

  const requestedWindow = useMemo(
    () => agentAnalysisWindow(new Date(), preset),
    [agentID, preset, refreshVersion],
  );

  useEffect(() => {
    if (!agentID) {
      setPreview(null);
      setPreviewLoading(false);
      setPreviewError("");
      return;
    }
    let active = true;
    setPreview(null);
    setPreviewLoading(true);
    setPreviewError("");
    setSubmitError("");
    void api.agentTraces(agentID, requestedWindow.window_start, requestedWindow.window_end).then((result) => {
      if (active) setPreview(result);
    }).catch((cause) => {
      if (!active) return;
      setPreviewError(cause instanceof Error ? cause.message : t.unavailable);
    }).finally(() => {
      if (active) setPreviewLoading(false);
    });
    return () => { active = false; };
  }, [agentID, requestedWindow, t.unavailable]);

  const traceCount = preview?.traces.length ?? 0;
  const traceSelection = agentAnalysisTraceSelection(traceCount);
  const canStart = (pipeline === "memory" || enabled) && canStartAgentAnalysis(agentID, traceCount, previewLoading) && !previewError && !submitting;
  const selectedAgent = agents.find((agent) => agent.agent_id === agentID);
  const activeWindow = preview ? {
    window_start: preview.window_start,
    window_end: preview.window_end,
  } : requestedWindow;

  return (
    <section className="agent-analysis-launcher" aria-label={embedded ? t.title : undefined} aria-labelledby={embedded ? undefined : "agent-analysis-launcher-title"}>
      <div className="launcher-heading">
        <div>{embedded ? null : <h2 id="agent-analysis-launcher-title">{t.title}</h2>}<p>{t.body}</p></div>
        {previewLoading ? <span>{t.loading}</span> : preview ? (
          <strong className={traceCount < 2 ? "insufficient" : ""}>{t.matched} {traceCount} {t.traces}</strong>
        ) : null}
      </div>
      {agents.length === 0 ? <p className="quiet-empty">{t.noAgents}</p> : (
        <form onSubmit={async (event) => {
          event.preventDefault();
          if (!canStart || !preview || (pipeline === "rsi" && !caseID)) return;
          setSubmitting(true);
          setSubmitError("");
          const input = {
            pipeline, options: pipeline === "rsi" ? { case_id: caseID } : {},
            window_start: preview.window_start,
            window_end: preview.window_end,
            output_language: locale === "zh" ? "zh-CN" as const : "en" as const,
            ...(objective.trim() ? { objective: objective.trim() } : {}),
          };
          const signature = JSON.stringify([agentAnalysisRequestSignature(agentID, input, objective, input.output_language), pipeline, caseID]);
          if (requestIdentity.current.signature !== signature) {
            requestIdentity.current = { signature, key: evolutionIdempotencyKey(agentID) };
          }
          try {
            onStarted(await api.startAgentAnalysisJob(agentID, input, requestIdentity.current.key));
          } catch (cause) {
            setSubmitError(cause instanceof Error ? cause.message : t.failed);
          } finally {
            setSubmitting(false);
          }
        }}>
          <div className="launcher-form-grid">
            <label>Pipeline<select value={pipeline} onChange={e => setPipeline(e.target.value as "rsi" | "memory")}><option value="rsi">RSI / Harness²</option><option value="memory">Engineering Memory</option></select></label>
            {pipeline === "rsi" && <label>Evaluation Case<select value={caseID} onChange={e => setCaseID(e.target.value)}><option value="">Select a registered Harbor case</option>{cases.map(id => <option key={id}>{id}</option>)}</select></label>}
            <label><span>{t.agent}</span><select value={agentID} onChange={(event) => setAgentID(event.target.value)} aria-label={t.chooseAgent}>
              {agents.map((agent) => <option value={agent.agent_id} key={agent.agent_id}>{agent.display_name}</option>)}
            </select></label>
            <fieldset>
              <legend>{t.window}</legend>
              <div className="window-presets">
                {presets.map((value) => (
                  <button className={preset === value ? "active" : ""} key={value} type="button" aria-pressed={preset === value} onClick={() => setPreset(value)}>{value}</button>
                ))}
              </div>
            </fieldset>
            <label className="launcher-objective"><span>{t.objective}</span><input value={objective} maxLength={4000} onChange={(event) => { setObjective(event.target.value); onObjectiveChange?.(event.target.value); }} placeholder={t.objectivePlaceholder} /></label>
          </div>
          <div className="launcher-submit">
            <div>
              {previewError ? <p className="inline-note error" role="alert">{previewError}</p> : null}
              {!previewLoading && preview && traceCount < 2 ? <p className="launcher-evidence-note insufficient">{t.insufficient}</p> : null}
              {!previewLoading && preview && traceCount >= 2 ? (
                <p className="launcher-evidence-note">
                  <strong>{t.ready}</strong>
                  <span>{traceSelectionMessage(traceSelection.matched, traceSelection.frozen, traceSelection.truncated, locale)}</span>
                  <span>{formatWindow(activeWindow, locale)}</span>
                </p>
              ) : null}
              {submitError ? <p className="inline-note error" role="alert">{submitError}</p> : null}
            </div>
            {previewError ? <button className="text-button" type="button" onClick={() => setRefreshVersion((value) => value + 1)}>{t.retry}</button> : null}
            <button className="primary-button compact" type="submit" disabled={!canStart || (pipeline === "rsi" && !caseID)}>{submitting ? t.starting : `${t.start}${selectedAgent ? ` ${selectedAgent.display_name}` : ""}`}</button>
          </div>
        </form>
      )}
    </section>
  );
}

function evolutionIdempotencyKey(agentID: string) {
  const nonce = typeof globalThis.crypto?.randomUUID === "function" ? globalThis.crypto.randomUUID() : `${Date.now()}`;
  const safeAgentID = agentID.replace(/[^a-zA-Z0-9._:-]/g, "").slice(0, 32) || "agent";
  return `catena-web-agent-${safeAgentID}-${nonce}`;
}

function formatWindow(value: { window_start: string; window_end: string }, locale: Locale) {
  const formatter = new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
  const start = formatter.format(new Date(value.window_start));
  const end = formatter.format(new Date(value.window_end));
  return locale === "zh" ? `${start} 至 ${end}` : `${start} to ${end}`;
}

function traceSelectionMessage(matched: number, frozen: number, truncated: boolean, locale: Locale) {
  if (locale === "zh") {
    return truncated
      ? `${matched} 条可用，本次分析 ${frozen} 条，优先包含错误`
      : `本次分析全部 ${frozen} 条 Trace`;
  }
  return truncated
    ? `${matched} available, analyzing ${frozen} with errors prioritized`
    : `Analyzing all ${frozen} Traces`;
}
