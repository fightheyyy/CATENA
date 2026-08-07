import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { AgentEvolutionLauncher } from "./AgentEvolutionLauncher";
import { api } from "./api";
import {
  agentAssets,
  agentAssetDownloadURL,
  agentAssetFilename,
  agentAssetText,
  evolutionStages,
  evolutionTraceCounts,
  isEvolutionJobTerminal,
  prettyJSON,
} from "./evolution";
import type { AgentSummary, EvolutionCandidate, EvolutionJob, EvolutionStage } from "./types";

type Locale = "zh" | "en";

const workspaceCopy = {
  zh: {
    title: "Trace Farm",
    body: "选择一个 Agent，找出近期反复出现的问题，并生成可以直接使用的改进资产。",
    jobs: "分析记录",
    recentJobs: "最近分析",
    recentJobsBody: "选择一条结果后，再查看生成的资产和来源证据。",
    newAnalysis: "新建分析",
    close: "关闭",
    noJobs: "还没有分析结果。选择一个 Agent，让 Catena 从近期 Trace 中提炼第一项资产。",
    noJobsTitle: "从一次 Agent 分析开始",
    loading: "正在读取分析记录",
    loadFailed: "无法读取分析记录",
    retry: "重试",
    analysis: "Agent 分析",
    analyzing: "正在分析",
    analysisCompleted: "分析完成",
    analysisFailed: "分析失败",
    resultReady: "已生成可用资产",
    resultPending: "完成质量检查后，资产会出现在这里。",
    assetCount: "项资产",
    objective: "分析目标",
    defaultObjective: "自动发现最值得修复的重复问题",
    progress: "分析进度",
    progressBody: "Catena 正在从 Trace 中发现问题、生成资产并检查质量。",
    details: "查看证据与运行详情",
    detailsBody: "Trace 来源、阶段原始输出和协议字段仅用于审计与排障。",
    evidence: "本次使用的证据",
    agentTraceSet: "Agent Trace 集",
    agent: "Agent",
    traceCount: "Trace 数量",
    sourceTraces: "来源 Trace",
    provenance: "查看资产来源",
    window: "时间范围",
    legacyTrace: "历史单 Trace 分析",
    traceOnly: "独立 OTLP Trace",
    runTrace: "Run 关联 Trace",
    trace: "Trace",
    run: "Run",
    evidencePack: "Evidence Pack",
    evidencePackMissing: "没有可展示的 Evidence Pack 元数据。",
    executionBoundary: "执行边界",
    targetExecution: "Catena 运行被测 Agent",
    createsRelease: "自动创建 Release",
    releaseAuthority: "Release 权限",
    yes: "是",
    no: "否",
    stages: "阶段原始输出",
    rawOutput: "查看原始输出",
    waitingOutput: "等待该角色返回输出。",
    outputMissing: "该阶段没有可展示的输出。",
    stageError: "阶段失败",
    finding: "发现的问题",
    findingMissing: "暂时还没有形成明确的问题结论。",
    evidenceItems: "关键证据",
    proposals: "生成的资产",
    proposalsBody: "复制后即可应用到对应 Agent。每项资产都保留来源 Trace，方便回查。",
    proposalMissing: "这次分析还没有生成资产。",
    content: "资产内容",
    copyAsset: "复制资产",
    copiedAsset: "已复制",
    copyAssetFailed: "复制失败，请检查浏览器剪贴板权限。",
    downloadAsset: "下载",
    agentMD: "agent.md",
    skill: "Skill",
    role: "Role",
    harness: "XiaoBaOS Harness",
    review: "质量检查",
    reviewMissing: "质量检查尚未完成。",
    qualityPassed: "已通过",
    qualityFailed: "需要复核",
    severity: { high: "高优先", medium: "中优先", low: "低优先", unknown: "待判断" },
    legacyReview: "该历史任务的审查文本来自旧协议，当前仅保留 Agent 资产与来源证据。",
    updated: "更新于",
    stageLabels: {
      inspector: "发现问题",
      evolution: "生成资产",
      reviewer: "检查质量",
    },
    stageDescriptions: {
      inspector: "检查重复失败与行为边界",
      evolution: "把问题转成可复用资产",
      reviewer: "核对资产是否有证据支撑",
    },
    states: {
      queued: "等待中",
      running: "进行中",
      completed: "已完成",
      failed: "失败",
      not_reported: "未开始",
    },
  },
  en: {
    title: "Trace Farm",
    body: "Choose an Agent, find recurring problems in recent behavior, and generate improvements you can use directly.",
    jobs: "Analysis history",
    recentJobs: "Recent analyses",
    recentJobsBody: "Select a result to inspect its generated assets and source evidence.",
    newAnalysis: "New analysis",
    close: "Close",
    noJobs: "No result yet. Choose an Agent and let Catena distill the first asset from recent Traces.",
    noJobsTitle: "Start with an Agent analysis",
    loading: "Reading analysis",
    loadFailed: "Could not read the analysis",
    retry: "Retry",
    analysis: "Agent analysis",
    analyzing: "Analyzing",
    analysisCompleted: "Analysis complete",
    analysisFailed: "Analysis failed",
    resultReady: "Usable assets are ready",
    resultPending: "Assets will appear here after the quality check.",
    assetCount: "assets",
    objective: "Analysis objective",
    defaultObjective: "Automatically find the highest-impact recurring problem",
    progress: "Analysis progress",
    progressBody: "Catena is finding problems, generating assets, and checking their quality.",
    details: "View evidence and run details",
    detailsBody: "Trace sources, raw stage output, and protocol fields are kept for audits and debugging.",
    evidence: "Evidence used",
    agentTraceSet: "Agent Trace set",
    agent: "Agent",
    traceCount: "Trace count",
    sourceTraces: "Source Traces",
    provenance: "View asset sources",
    window: "Time range",
    legacyTrace: "Historical single-Trace analysis",
    traceOnly: "Standalone OTLP Trace",
    runTrace: "Run-linked Trace",
    trace: "Trace",
    run: "Run",
    evidencePack: "Evidence Pack",
    evidencePackMissing: "No Evidence Pack metadata is available.",
    executionBoundary: "Execution boundary",
    targetExecution: "Catena runs target Agent",
    createsRelease: "Creates Release automatically",
    releaseAuthority: "Release authority",
    yes: "Yes",
    no: "No",
    stages: "Raw stage output",
    rawOutput: "View raw output",
    waitingOutput: "Waiting for this role to return output.",
    outputMissing: "No output is available for this stage.",
    stageError: "Stage failed",
    finding: "Problem found",
    findingMissing: "No clear problem has been identified yet.",
    evidenceItems: "Key evidence",
    proposals: "Generated assets",
    proposalsBody: "Copy an asset into the target Agent. Every asset keeps its source Trace provenance for review.",
    proposalMissing: "This analysis has not generated an asset yet.",
    content: "Asset content",
    copyAsset: "Copy asset",
    copiedAsset: "Copied",
    copyAssetFailed: "Could not copy. Check browser clipboard permission.",
    downloadAsset: "Download",
    agentMD: "agent.md",
    skill: "Skill",
    role: "Role",
    harness: "XiaoBaOS Harness",
    review: "Quality check",
    reviewMissing: "The quality check is not complete yet.",
    qualityPassed: "Passed",
    qualityFailed: "Needs review",
    severity: { high: "High priority", medium: "Medium priority", low: "Low priority", unknown: "Unclassified" },
    legacyReview: "This historical review used the previous contract. Only the Agent asset and source evidence remain current.",
    updated: "Updated",
    stageLabels: {
      inspector: "Find problems",
      evolution: "Generate assets",
      reviewer: "Check quality",
    },
    stageDescriptions: {
      inspector: "Inspect recurring failures and behavior boundaries",
      evolution: "Turn the problem into a reusable asset",
      reviewer: "Check that the asset is supported by evidence",
    },
    states: {
      queued: "Waiting",
      running: "In progress",
      completed: "Completed",
      failed: "Failed",
      not_reported: "Not started",
    },
  },
} as const;

type Copy = typeof workspaceCopy.zh | typeof workspaceCopy.en;

function stateLabel(t: Copy, state: string) {
  return t.states[state as keyof typeof t.states] ?? (state || t.states.not_reported);
}

function safeState(state: string) {
  return state.toLowerCase().replace(/[^a-z0-9_-]/g, "-");
}

function stageKey(stage: EvolutionStage) {
  if (stage.name === "inspector" || stage.role === "inspector-cat") return "inspector";
  if (stage.name === "evolution" || stage.role === "evolution-cat") return "evolution";
  if (stage.name === "reviewer" || stage.role === "reviewer-cat") return "reviewer";
  return stage.name;
}

function stageLabel(t: Copy, stage: EvolutionStage) {
  const key = stageKey(stage);
  return t.stageLabels[key as keyof typeof t.stageLabels] ?? stage.role ?? stage.name;
}

function stageDescription(t: Copy, stage: EvolutionStage) {
  const key = stageKey(stage);
  return t.stageDescriptions[key as keyof typeof t.stageDescriptions] ?? stage.role ?? stage.name;
}

function formattedTime(value: string, locale: Locale) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formattedWindow(job: EvolutionJob, locale: Locale) {
  if (!job.window_start || !job.window_end) return "";
  const start = formattedTime(job.window_start, locale);
  const end = formattedTime(job.window_end, locale);
  if (!start || !end) return "";
  return locale === "zh" ? `${start} 至 ${end}` : `${start} to ${end}`;
}

function traceCountDisplay(value: { frozen: number; matched: number }, locale: Locale) {
  if (value.matched <= value.frozen) return String(value.frozen);
  return locale === "zh"
    ? `使用 ${value.frozen} 条，共 ${value.matched} 条可用`
    : `Using ${value.frozen} of ${value.matched} available`;
}

function conciseReviewSummary(value: string) {
  const summary = value.trim();
  if (!summary) return "";
  const firstSentence = summary.match(/^.*?[.!?。！？](?:\s|$)/)?.[0]?.trim();
  if (firstSentence) return firstSentence;
  return summary.length > 240 ? `${summary.slice(0, 237).trim()}...` : summary;
}

export function EvolutionWorkspace({
  locale,
  jobs,
  agents,
  initialJobID,
  initialAgentID,
  onJobStarted,
  onJobSelected,
}: {
  locale: Locale;
  jobs: EvolutionJob[];
  agents: AgentSummary[];
  initialJobID?: string;
  initialAgentID?: string;
  onJobStarted: (job: EvolutionJob) => void;
  onJobSelected: (jobID: string) => void;
}) {
  const t = workspaceCopy[locale];
  const [selectedID, setSelectedID] = useState(initialJobID || "");
  const [job, setJob] = useState<EvolutionJob | null>(
    jobs.find((item) => item.job_id === initialJobID) ?? null,
  );
  const [loading, setLoading] = useState(Boolean(selectedID));
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [drawer, setDrawer] = useState<"new" | "">("");

  useEffect(() => {
    setSelectedID(initialJobID || "");
  }, [initialJobID]);

  useEffect(() => {
    if (!selectedID) {
      setJob(null);
      setLoading(false);
      return;
    }
    let active = true;
    let timer = 0;
    const summary = jobs.find((item) => item.job_id === selectedID);
    setJob(summary ?? null);
    setLoading(!summary);
    setError("");

    const refresh = async () => {
      try {
        const next = await api.evolutionJob(selectedID);
        if (!active) return;
        setJob(next);
        setLoading(false);
        setError("");
        if (!isEvolutionJobTerminal(next)) timer = window.setTimeout(() => void refresh(), 1600);
      } catch (cause) {
        if (!active) return;
        setLoading(false);
        setError(cause instanceof Error ? cause.message : t.loadFailed);
      }
    };
    void refresh();
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [jobs, reload, selectedID, t.loadFailed]);

  const handleJobStarted = (next: EvolutionJob) => {
    setSelectedID(next.job_id);
    setJob(next);
    setDrawer("");
    onJobStarted(next);
  };

  const agentName = (agentID?: string) => agents.find((agent) => (
    agent.agent_id === agentID || agent.sources?.some((source) => source.service_name === agentID)
  ))?.display_name || agentID || t.analysis;

  return (
    <section className="page evolution-page">
      <header className="page-header evolution-page-header">
        <div>
          <h1>{t.title}</h1>
          <p>{t.body}</p>
        </div>
        <div className="evolution-page-actions">
          {jobs.length > 0 && selectedID ? <button className="secondary-button" type="button" onClick={() => {
            setSelectedID("");
            onJobSelected("");
          }}>{t.jobs}</button> : null}
          <button className="primary-button compact" type="button" onClick={() => setDrawer("new")}>{t.newAnalysis}</button>
        </div>
      </header>
      {!selectedID && jobs.length === 0 ? (
        <div className="evolution-empty">
          <h2>{t.noJobsTitle}</h2>
          <p>{t.noJobs}</p>
          <button className="primary-button compact" type="button" onClick={() => setDrawer("new")}>{t.newAnalysis}</button>
        </div>
      ) : !selectedID ? (
        <section className="farm-overview" aria-labelledby="farm-recent-title">
          <header>
            <div>
              <h2 id="farm-recent-title">{t.recentJobs}</h2>
              <p>{t.recentJobsBody}</p>
            </div>
            <span>{jobs.length}</span>
          </header>
          <div className="farm-overview-list">
            {jobs.map((item) => {
              const traceCount = evolutionTraceCounts(item).frozen;
              const windowLabel = formattedWindow(item, locale);
              return (
                <button className="job-row" type="button" key={item.job_id} onClick={() => {
                  setSelectedID(item.job_id);
                  onJobSelected(item.job_id);
                }}>
                  <span className={`job-state state-${safeState(item.state)}`}>{stateLabel(t, item.state)}</span>
                  <strong>{agentName(item.source_agent_id) || item.finding?.title || item.objective || t.analysis}</strong>
                  <small>{item.source_agent_id ? `${traceCount} ${t.trace} · ${windowLabel || t.agentTraceSet}` : t.legacyTrace}</small>
                  <time dateTime={item.updated_at}>{formattedTime(item.updated_at, locale)}</time>
                </button>
              );
            })}
          </div>
        </section>
      ) : (
        <div className="job-detail-column" aria-live="polite">
          {loading && !job ? <JobSkeleton label={t.loading} /> : null}
          {error ? (
            <div className="job-error" role="alert">
              <strong>{t.loadFailed}</strong>
              <span>{error}</span>
              <button className="text-button" type="button" onClick={() => setReload((value) => value + 1)}>{t.retry}</button>
            </div>
          ) : null}
          {job ? <EvolutionJobDetail key={job.job_id} job={job} locale={locale} t={t} agentName={agentName(job.source_agent_id)} /> : null}
        </div>
      )}
      <FarmDrawer open={drawer === "new"} title={t.newAnalysis} closeLabel={t.close} onClose={() => setDrawer("")}>
        <AgentEvolutionLauncher
          locale={locale}
          agents={agents}
          initialAgentID={initialAgentID}
          onStarted={handleJobStarted}
          embedded
        />
      </FarmDrawer>
    </section>
  );
}

function FarmDrawer({
  open,
  title,
  closeLabel,
  onClose,
  children,
}: {
  open: boolean;
  title: string;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (!open) return;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
      if (event.key === "Tab" && panelRef.current) {
        const focusable = Array.from(panelRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), select:not([disabled]), input:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])',
        ));
        const first = focusable[0];
        const last = focusable.at(-1);
        if (event.shiftKey && document.activeElement === first && last) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last && first) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
      previousFocus?.focus();
    };
  }, [onClose, open]);

  if (!open) return null;
  return (
    <div className="farm-drawer-layer" role="presentation" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose();
    }}>
      <aside ref={panelRef} className="farm-drawer" role="dialog" aria-modal="true" aria-labelledby="farm-drawer-title">
        <header className="farm-drawer-header">
          <h2 id="farm-drawer-title">{title}</h2>
          <button ref={closeRef} type="button" aria-label={closeLabel} onClick={onClose}>×</button>
        </header>
        <div className="farm-drawer-body">{children}</div>
      </aside>
    </div>
  );
}

function JobSkeleton({ label }: { label: string }) {
  return (
    <div className="job-skeleton" aria-label={label}>
      <i /><i /><i /><i />
    </div>
  );
}

function EvolutionJobDetail({ job, locale, t, agentName }: { job: EvolutionJob; locale: Locale; t: Copy; agentName: string }) {
  const stages = evolutionStages(job);
  const assets = agentAssets(job);
  const updated = formattedTime(job.updated_at, locale);
  const traceSet = Boolean(job.source_agent_id || job.source_kind === "agent_trace_set");
  const traceCounts = evolutionTraceCounts(job);
  const windowLabel = formattedWindow(job, locale);
  const title = job.source_agent_id ? `${agentName} ${t.analysis}` : t.analysis;
  const objective = job.objective || job.finding?.title || t.defaultObjective;
  const resultLabel = job.state === "completed"
    ? assets.length > 0 ? t.resultReady : t.analysisCompleted
    : job.state === "failed" ? t.analysisFailed : t.analyzing;

  return (
    <article className="evolution-job-detail">
      <header className="job-detail-header">
        <div>
          <div className="job-heading-line">
            <span className={`job-state state-${safeState(job.state)}`}>{stateLabel(t, job.state)}</span>
            <span>{resultLabel}</span>
            {traceSet ? <span>{traceCountDisplay(traceCounts, locale)} {t.trace}</span> : null}
          </div>
          <h2>{title}</h2>
          <p className="job-objective">{objective}</p>
        </div>
        {updated ? <span className="job-updated">{t.updated} {updated}</span> : null}
      </header>
      {job.error ? <p className="inline-note error" role="alert">{job.error}</p> : null}

      {job.state !== "completed" ? <section className="job-progress-section">
        <div className="section-heading stacked">
          <h3>{t.progress}</h3>
          <p>{job.state === "completed" ? resultLabel : t.progressBody}</p>
        </div>
        <div className="progress-steps">
          {stages.map((stage) => (
            <StageProgress
              key={`${stage.name}-${stage.role}`}
              stage={stage}
              current={job.current_stage === stage.name || job.current_stage === stage.role}
              t={t}
            />
          ))}
        </div>
      </section> : null}

      <section className="proposal-section">
        <div className="section-heading stacked">
          <h3>{t.proposals}</h3>
          <p>{t.proposalsBody}</p>
        </div>
        {assets.length === 0 ? <p className="quiet-empty">{t.proposalMissing}</p> : null}
        {assets.length > 0 ? <AssetWorkspace assets={assets} t={t} /> : null}
      </section>

      <section className="finding-section">
        <h3>{t.finding}</h3>
        {job.finding ? (
          <div className="finding-content">
            <div>
              <span className="severity-label">{t.severity[job.finding.severity.toLowerCase() as keyof typeof t.severity] || job.finding.severity}</span>
              <h4>{job.finding.title}</h4>
              {job.finding.summary ? <p>{job.finding.summary}</p> : null}
            </div>
            {job.finding.evidence.length ? (
              <details className="finding-evidence">
                <summary>{t.evidenceItems} ({job.finding.evidence.length})</summary>
                <ul>{job.finding.evidence.map((item, index) => <li key={`${index}-${item}`}>{item}</li>)}</ul>
              </details>
            ) : null}
          </div>
        ) : <p className="quiet-empty">{t.findingMissing}</p>}
      </section>

      <section className="quality-section">
        <h3>{t.review}</h3>
        {job.review ? (
          <div className="quality-result">
            <strong className={`review-verdict verdict-${safeState(job.review.verdict)}`}>
              {job.review.verdict === "pass" ? t.qualityPassed : t.qualityFailed}
            </strong>
            <p>{conciseReviewSummary(job.review.summary) || (job.case_proposal ? t.legacyReview : "")}</p>
          </div>
        ) : <p className="quiet-empty">{t.reviewMissing}</p>}
      </section>

      <details className="run-details">
        <summary>
          <strong>{t.details}</strong>
          <span>{t.detailsBody}</span>
        </summary>
        <div className="run-details-content">
          <code className="job-id">{job.job_id}</code>
          <section className="evidence-summary">
            <div className="section-heading">
              <h3>{t.evidence}</h3>
              <span>{traceSet ? t.agentTraceSet : job.source_kind === "run_trace" ? t.runTrace : t.legacyTrace}</span>
            </div>
            <dl>
              {job.source_agent_id ? <div><dt>{t.agent}</dt><dd>{job.source_agent_id}</dd></div> : null}
              {traceSet ? <div><dt>{t.traceCount}</dt><dd>{traceCountDisplay(traceCounts, locale)}</dd></div> : null}
              {windowLabel ? <div><dt>{t.window}</dt><dd>{windowLabel}</dd></div> : null}
              {job.source_trace_id ? <div><dt>{t.trace}</dt><dd>{job.source_trace_id}</dd></div> : null}
              {job.source_run_id ? <div><dt>{t.run}</dt><dd>{job.source_run_id}</dd></div> : null}
              {job.objective ? <div><dt>{t.objective}</dt><dd>{job.objective}</dd></div> : null}
            </dl>
            {job.source_trace_ids.length ? (
              <details className="source-trace-ids">
                <summary>{t.sourceTraces} ({job.source_trace_ids.length})</summary>
                <div>{job.source_trace_ids.map((traceID) => <code key={traceID}>{traceID}</code>)}</div>
              </details>
            ) : null}
            {job.evidence_pack ? (
              <details className="evidence-pack">
                <summary>{t.evidencePack}</summary>
                <pre>{prettyJSON(job.evidence_pack)}</pre>
              </details>
            ) : <p className="quiet-empty">{t.evidencePackMissing}</p>}
            {job.boundary ? (
              <div className="boundary-data" aria-label={t.executionBoundary}>
                <span><b>{t.targetExecution}</b>{job.boundary.target_agent_executed_by_catena ? t.yes : t.no}</span>
                <span><b>{t.createsRelease}</b>{job.boundary.creates_release ? t.yes : t.no}</span>
                <span><b>{t.releaseAuthority}</b>{job.boundary.release_authority || "-"}</span>
              </div>
            ) : null}
          </section>
          <section className="stage-section">
            <h3>{t.stages}</h3>
            <div className="stage-list">
              {stages.map((stage) => (
                <StageResult
                  key={`${stage.name}-${stage.role}`}
                  stage={stage}
                  current={job.current_stage === stage.name || job.current_stage === stage.role}
                  t={t}
                />
              ))}
            </div>
          </section>
        </div>
      </details>
    </article>
  );
}

function StageProgress({ stage, current, t }: { stage: EvolutionStage; current: boolean; t: Copy }) {
  return (
    <div className={`progress-step state-${safeState(stage.state)}${current ? " current" : ""}`}>
      <i aria-hidden="true" />
      <div>
        <strong>{stageLabel(t, stage)}</strong>
        <small>{stageDescription(t, stage)}</small>
      </div>
      <span>{stateLabel(t, stage.state)}</span>
    </div>
  );
}

function StageResult({ stage, current, t }: { stage: EvolutionStage; current: boolean; t: Copy }) {
  const output = prettyJSON(stage.raw_output);
  return (
    <article className={current ? "stage-result current" : "stage-result"}>
      <header>
        <div><strong>{stage.role || stage.name}</strong><span>{stage.name}</span></div>
        <span className={`stage-state state-${safeState(stage.state)}`}>{stateLabel(t, stage.state)}</span>
      </header>
      {stage.error ? <p className="stage-error"><strong>{t.stageError}</strong>{stage.error}</p> : output ? (
        <details className="stage-raw">
          <summary>{t.rawOutput}</summary>
          <pre>{output}</pre>
        </details>
      ) : (
        <p className="stage-waiting">{stage.state === "queued" || stage.state === "running" ? t.waitingOutput : t.outputMissing}</p>
      )}
    </article>
  );
}

function AssetWorkspace({ assets, t }: { assets: EvolutionCandidate[]; t: Copy }) {
  const [selectedAssetID, setSelectedAssetID] = useState(assets[0]?.candidate_id || "asset-0");
  const selectedIndex = Math.max(0, assets.findIndex((candidate, index) => (candidate.candidate_id || `asset-${index}`) === selectedAssetID));
  const candidate = assets[selectedIndex] || assets[0];
  const [copyState, setCopyState] = useState<"idle" | "copied" | "error">("idle");
  const content = agentAssetText(candidate);
  const filename = agentAssetFilename(candidate);

  const assetLabel = (asset: EvolutionCandidate) => asset.kind === "agent_md"
    ? t.agentMD
    : asset.kind === "skill"
      ? t.skill
      : asset.kind === "role"
        ? t.role
        : t.harness;
  const kind = assetLabel(candidate);

  return (
    <div className="asset-workspace">
      {assets.length > 1 ? (
        <div className="asset-tabs" role="tablist" aria-label={t.proposals}>
          {assets.map((asset, index) => {
            const assetID = asset.candidate_id || `asset-${index}`;
            return (
              <button
                key={assetID}
                type="button"
                role="tab"
                aria-selected={assetID === (candidate.candidate_id || `asset-${selectedIndex}`)}
                className={assetID === (candidate.candidate_id || `asset-${selectedIndex}`) ? "active" : ""}
                onClick={() => {
                  setSelectedAssetID(assetID);
                  setCopyState("idle");
                }}
              >
                {assetLabel(asset)}
              </button>
            );
          })}
        </div>
      ) : null}
      <article className="proposal-card" role="tabpanel">
      <header className="proposal-header">
        <div>
          <span className="proposal-kind">{filename}</span>
          <h4>{candidate.title || kind}</h4>
        </div>
        <div className="asset-actions">
          <button className="asset-copy-button" type="button" disabled={!content} onClick={async () => {
            try {
              await navigator.clipboard.writeText(content);
              setCopyState("copied");
            } catch {
              setCopyState("error");
            }
          }}>{copyState === "copied" ? t.copiedAsset : t.copyAsset}</button>
          <a
            className="asset-copy-button"
            aria-disabled={!content}
            href={content ? agentAssetDownloadURL(filename, content) : undefined}
            download={filename}
            onClick={(event) => { if (!content) event.preventDefault(); }}
          >{t.downloadAsset}</a>
        </div>
      </header>
      {copyState === "error" ? <span className="asset-copy-error" role="alert">{t.copyAssetFailed}</span> : null}
      {candidate.summary ? <p className="proposal-summary">{candidate.summary}</p> : null}
      {content ? <ProposalCode label={t.content} value={content} /> : null}
      <ProposalProvenance
        sourceAgentID={candidate.source_agent_id}
        sourceTraceID={candidate.source_trace_id}
        sourceTraceIDs={candidate.source_trace_ids}
        evidenceSHA={candidate.evidence_pack_sha256}
        t={t}
      />
      </article>
    </div>
  );
}

function ProposalCode({ label, value }: { label: string; value: unknown }) {
  return <div className="proposal-field"><h5>{label}</h5><pre>{prettyJSON(value)}</pre></div>;
}

function ProposalProvenance({
  sourceAgentID,
  sourceTraceID,
  sourceTraceIDs,
  evidenceSHA,
  t,
}: {
  sourceAgentID?: string;
  sourceTraceID?: string;
  sourceTraceIDs?: string[];
  evidenceSHA?: string;
  t: Copy;
}) {
  if (!sourceAgentID && !sourceTraceID && !sourceTraceIDs?.length && !evidenceSHA) return null;
  return (
    <details className="proposal-provenance-disclosure">
      <summary>{t.provenance}</summary>
      <footer className="proposal-provenance">
        {sourceAgentID ? <span><b>{t.agent}</b><code>{sourceAgentID}</code></span> : null}
        {sourceTraceIDs?.length ? <span><b>{t.sourceTraces}</b><code>{sourceTraceIDs.join(", ")}</code></span> : null}
        {sourceTraceID && !sourceTraceIDs?.length ? <span><b>{t.trace}</b><code>{sourceTraceID}</code></span> : null}
        {evidenceSHA ? <span><b>{t.evidencePack}</b><code>{evidenceSHA}</code></span> : null}
      </footer>
    </details>
  );
}
