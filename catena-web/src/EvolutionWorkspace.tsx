import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { AgentEvolutionLauncher } from "./AgentEvolutionLauncher";
import { api } from "./api";
import {
  agentAssetArchive,
  agentAssets,
  agentAssetDownloadURL,
  agentAssetFiles,
  agentAssetFilename,
  agentAssetPath,
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
    assets: "资产",
    assetLibrary: "Agent 资产",
    assetLibraryBody: "这里保存 agent.md、Skill 包与 Role 包；DeepSeek Harness Agent 会额外生成可直接安装的 DSH Plugin。",
    assetLibraryEmpty: "还没有生成可复用资产。新建一次分析，Catena 会把最值得修复的问题写成文件。",
    assetLibraryEmptyTitle: "Trace 还没有变成资产",
    allAssets: "全部",
    allAgents: "全部 Agent",
    assetFilter: "资产类型",
    agentFilter: "Agent",
    assetDocument: "资产文件",
    packageContents: "包内文件",
    fileCount: "个文件",
    selectedFile: "当前文件",
    assetReason: "生成原因",
    assetQuality: "质量检查",
    openAnalysis: "查看来源分析",
    readAsset: "阅读",
    sourceAsset: "源码",
    generatedAt: "生成时间",
    noMatchingAssets: "没有符合当前筛选条件的资产。",
    traceSources: "条来源 Trace",
    recentJobs: "最近分析",
    recentJobsBody: "选择一条结果后，再查看生成的资产和来源证据。",
    newAnalysis: "新建分析",
    close: "关闭",
    noJobs: "还没有分析结果。选择一个 Agent，让 Catena 从近期 Trace 中提炼第一项资产。",
    noJobsTitle: "从一次 Agent 分析开始",
    loading: "正在读取分析记录",
    loadFailed: "无法读取分析记录",
    retry: "重试",
    deleteAnalysis: "删除分析",
    deleteAsset: "删除资产",
    deleteAssetTitle: "删除这项资产？",
    deleteAssetWarning: "资产和对应的来源分析将一起删除；来源 Trace 保留。此操作无法恢复。",
    confirmDelete: "确认删除",
    cancelDelete: "取消",
    deleting: "正在删除",
    deleteFailed: "无法删除分析",
    confirmDeleteTitle: "删除这条分析？",
    deleteWarning: "分析记录和生成资产会被删除；来源 Trace 保留。此操作无法恢复。",
    deleteAfterFinish: "分析完成后可删除",
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
    copyFile: "复制文件",
    copiedAsset: "已复制",
    copyAssetFailed: "复制失败，请检查浏览器剪贴板权限。",
    downloadAsset: "下载",
    downloadPackage: "下载整个包",
    agentMD: "agent.md",
    skill: "Skill",
    role: "Role",
    dshPlugin: "DSH Plugin",
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
    assets: "Assets",
    assetLibrary: "Agent assets",
    assetLibraryBody: "Catena stores agent.md, Skill, and Role packages. DeepSeek Harness Agents additionally produce installable DSH Plugins.",
    assetLibraryEmpty: "No reusable asset exists yet. Start an analysis and Catena will turn the highest-impact problem into a file.",
    assetLibraryEmptyTitle: "No Trace has become an asset yet",
    allAssets: "All",
    allAgents: "All Agents",
    assetFilter: "Asset kind",
    agentFilter: "Agent",
    assetDocument: "Asset file",
    packageContents: "Package contents",
    fileCount: "files",
    selectedFile: "Current file",
    assetReason: "Why it was generated",
    assetQuality: "Quality check",
    openAnalysis: "Open source analysis",
    readAsset: "Read",
    sourceAsset: "Source",
    generatedAt: "Generated",
    noMatchingAssets: "No asset matches the current filters.",
    traceSources: "source Traces",
    recentJobs: "Recent analyses",
    recentJobsBody: "Select a result to inspect its generated assets and source evidence.",
    newAnalysis: "New analysis",
    close: "Close",
    noJobs: "No result yet. Choose an Agent and let Catena distill the first asset from recent Traces.",
    noJobsTitle: "Start with an Agent analysis",
    loading: "Reading analysis",
    loadFailed: "Could not read the analysis",
    retry: "Retry",
    deleteAnalysis: "Delete analysis",
    deleteAsset: "Delete asset",
    deleteAssetTitle: "Delete this asset?",
    deleteAssetWarning: "The asset and its source analysis will be deleted. Source Traces stay intact. This cannot be undone.",
    confirmDelete: "Delete",
    cancelDelete: "Cancel",
    deleting: "Deleting",
    deleteFailed: "Could not delete analysis",
    confirmDeleteTitle: "Delete this analysis?",
    deleteWarning: "The analysis and generated assets will be deleted. Source Traces stay intact. This cannot be undone.",
    deleteAfterFinish: "Available after the analysis finishes",
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
    copyFile: "Copy file",
    copiedAsset: "Copied",
    copyAssetFailed: "Could not copy. Check browser clipboard permission.",
    downloadAsset: "Download",
    downloadPackage: "Download package",
    agentMD: "agent.md",
    skill: "Skill",
    role: "Role",
    dshPlugin: "DSH Plugin",
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

type AssetRecord = {
  candidate: EvolutionCandidate;
  job: EvolutionJob;
  agentName: string;
};

export function EvolutionWorkspace({
  locale,
  jobs,
  agents,
  initialJobID,
  initialAgentID,
  onJobStarted,
  onJobSelected,
  onJobDeleted,
}: {
  locale: Locale;
  jobs: EvolutionJob[];
  agents: AgentSummary[];
  initialJobID?: string;
  initialAgentID?: string;
  onJobStarted: (job: EvolutionJob) => void;
  onJobSelected: (jobID: string) => void;
  onJobDeleted: (jobID: string) => void;
}) {
  const t = workspaceCopy[locale];
  const [selectedID, setSelectedID] = useState(initialJobID || "");
  const [job, setJob] = useState<EvolutionJob | null>(
    jobs.find((item) => item.job_id === initialJobID) ?? null,
  );
  const [loading, setLoading] = useState(Boolean(selectedID));
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [view, setView] = useState<"assets" | "analyses" | "analysis">(
    initialJobID ? "analysis" : "assets",
  );
  const [drawer, setDrawer] = useState<"new" | "">("");
  const [deleteConfirming, setDeleteConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");

  useEffect(() => {
    setSelectedID(initialJobID || "");
    if (initialJobID) setView("analysis");
  }, [initialJobID]);

  useEffect(() => {
    setDeleteConfirming(false);
    setDeleteError("");
  }, [selectedID]);

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
    setView("analysis");
    setDrawer("");
    onJobStarted(next);
  };

  const handleDeleteJob = async () => {
    if (!job || !isEvolutionJobTerminal(job) || deleting) return;
    setDeleting(true);
    setDeleteError("");
    try {
      await api.deleteEvolutionJob(job.job_id);
      const deletedID = job.job_id;
      setSelectedID("");
      setJob(null);
      setView("analyses");
      setDeleteConfirming(false);
      onJobSelected("");
      onJobDeleted(deletedID);
    } catch (cause) {
      setDeleteError(cause instanceof Error ? cause.message : t.deleteFailed);
    } finally {
      setDeleting(false);
    }
  };

  const handleDeleteAsset = async (jobID: string) => {
    await api.deleteEvolutionJob(jobID);
    if (selectedID === jobID) {
      setSelectedID("");
      setJob(null);
      onJobSelected("");
    }
    onJobDeleted(jobID);
  };

  const agentName = (agentID?: string) => agents.find((agent) => (
    agent.agent_id === agentID || agent.sources?.some((source) => source.service_name === agentID)
  ))?.display_name || agentID || t.analysis;

  const assetRecords = useMemo<AssetRecord[]>(() => jobs.flatMap((sourceJob) => (
    agentAssets(sourceJob).map((candidate) => ({
      candidate,
      job: sourceJob,
      agentName: agentName(sourceJob.source_agent_id),
    }))
  )).sort((left, right) => (
    new Date(right.job.updated_at).getTime() - new Date(left.job.updated_at).getTime()
  )), [agents, jobs]);

  const openAnalysis = (jobID: string) => {
    setSelectedID(jobID);
    setView("analysis");
    onJobSelected(jobID);
  };

  return (
    <section className="page evolution-page">
      <header className="page-header evolution-page-header">
        <div>
          <h1>{t.title}</h1>
          <p>{t.body}</p>
        </div>
        <div className="evolution-page-actions">
          <div className="farm-view-switch" role="group" aria-label={t.title}>
            <button className={view === "assets" ? "active" : ""} type="button" onClick={() => {
              setView("assets");
              setSelectedID("");
              onJobSelected("");
            }}>{t.assets}<span>{assetRecords.length}</span></button>
            <button className={view === "analyses" ? "active" : ""} type="button" onClick={() => {
              setView("analyses");
              setSelectedID("");
              onJobSelected("");
            }}>{t.jobs}<span>{jobs.length}</span></button>
          </div>
          <button className="primary-button compact" type="button" onClick={() => setDrawer("new")}>{t.newAnalysis}</button>
        </div>
      </header>
      {view === "assets" && assetRecords.length > 0 ? (
        <AssetLibrary
          records={assetRecords}
          agents={agents}
          locale={locale}
          t={t}
          onOpenAnalysis={openAnalysis}
          onDeleteAsset={handleDeleteAsset}
        />
      ) : view === "assets" ? (
        <div className="evolution-empty">
          <h2>{t.assetLibraryEmptyTitle}</h2>
          <p>{t.assetLibraryEmpty}</p>
          <button className="primary-button compact" type="button" onClick={() => setDrawer("new")}>{t.newAnalysis}</button>
        </div>
      ) : view === "analyses" && jobs.length === 0 ? (
        <div className="evolution-empty">
          <h2>{t.noJobsTitle}</h2>
          <p>{t.noJobs}</p>
          <button className="primary-button compact" type="button" onClick={() => setDrawer("new")}>{t.newAnalysis}</button>
        </div>
      ) : view === "analyses" ? (
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
                <button className="job-row" type="button" key={item.job_id} onClick={() => openAnalysis(item.job_id)}>
                  <span className={`job-state state-${safeState(item.state)}`}>{stateLabel(t, item.state)}</span>
                  <strong>{agentName(item.source_agent_id) || item.finding?.title || item.objective || t.analysis}</strong>
                  <small>{item.source_agent_id ? `${traceCount} ${t.trace} · ${windowLabel || t.agentTraceSet}` : t.legacyTrace}</small>
                  <time dateTime={item.updated_at}>{formattedTime(item.updated_at, locale)}</time>
                </button>
              );
            })}
          </div>
        </section>
      ) : view === "analysis" ? (
        <div className="job-detail-column" aria-live="polite">
          {loading && !job ? <JobSkeleton label={t.loading} /> : null}
          {error ? (
            <div className="job-error" role="alert">
              <strong>{t.loadFailed}</strong>
              <span>{error}</span>
              <button className="text-button" type="button" onClick={() => setReload((value) => value + 1)}>{t.retry}</button>
            </div>
          ) : null}
          {deleteError ? <p className="inline-note error" role="alert">{t.deleteFailed}: {deleteError}</p> : null}
          {job ? <EvolutionJobDetail
            key={job.job_id}
            job={job}
            locale={locale}
            t={t}
            agentName={agentName(job.source_agent_id)}
            action={isEvolutionJobTerminal(job) ? (
              deleteConfirming ? null : <button className="text-button danger" type="button" onClick={() => setDeleteConfirming(true)}>{t.deleteAnalysis}</button>
            ) : <span className="job-delete-hint">{t.deleteAfterFinish}</span>}
            notice={deleteConfirming ? (
              <div className="job-delete-confirmation" role="group" aria-label={t.confirmDeleteTitle}>
                <div>
                  <strong>{t.confirmDeleteTitle}</strong>
                  <span>{t.deleteWarning}</span>
                </div>
                <div>
                  <button className="secondary-button" type="button" onClick={() => setDeleteConfirming(false)} disabled={deleting}>{t.cancelDelete}</button>
                  <button className="danger-button" type="button" onClick={() => void handleDeleteJob()} disabled={deleting}>{deleting ? t.deleting : t.confirmDelete}</button>
                </div>
              </div>
            ) : null}
          /> : null}
        </div>
      ) : null}
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

function assetKindLabel(t: Copy, kind: EvolutionCandidate["kind"]) {
  if (kind === "agent_md") return t.agentMD;
  if (kind === "skill") return t.skill;
  if (kind === "role") return t.role;
  if (kind === "dsh_plugin") return t.dshPlugin;
  return t.harness;
}

function assetRecordID(record: AssetRecord) {
  return record.candidate.candidate_id || [
    record.job.job_id,
    record.candidate.kind,
    agentAssetPath(record.candidate),
    record.candidate.title,
  ].join(":");
}

function AssetLibrary({
  records,
  agents,
  locale,
  t,
  onOpenAnalysis,
  onDeleteAsset,
}: {
  records: AssetRecord[];
  agents: AgentSummary[];
  locale: Locale;
  t: Copy;
  onOpenAnalysis: (jobID: string) => void;
  onDeleteAsset: (jobID: string) => Promise<void>;
}) {
  const [kind, setKind] = useState<"all" | EvolutionCandidate["kind"]>("all");
  const [agentID, setAgentID] = useState("all");
  const [selectedID, setSelectedID] = useState(assetRecordID(records[0]));
  const availableKinds = ["agent_md", "skill", "role", "dsh_plugin"] as const;
  const availableAgents = agents.filter((agent) => records.some((record) => (
    record.job.source_agent_id === agent.agent_id || record.agentName === agent.display_name
  )));
  const filtered = records.filter((record) => (
    (kind === "all" || record.candidate.kind === kind) &&
    (agentID === "all" || record.job.source_agent_id === agentID)
  ));
  const selected = filtered.find((record) => assetRecordID(record) === selectedID) || filtered[0];

  useEffect(() => {
    if (!selected) return;
    const nextID = assetRecordID(selected);
    if (nextID !== selectedID) setSelectedID(nextID);
  }, [filtered, selected, selectedID]);

  return (
    <section className="asset-library" aria-labelledby="asset-library-title">
      <header className="asset-library-header">
        <div>
          <h2 id="asset-library-title">{t.assetLibrary}</h2>
          <p>{t.assetLibraryBody}</p>
        </div>
        <strong>{records.length}</strong>
      </header>
      <div className="asset-library-filters">
        <div className="asset-kind-filter" role="group" aria-label={t.assetFilter}>
          <button className={kind === "all" ? "active" : ""} type="button" onClick={() => setKind("all")}>{t.allAssets}</button>
          {availableKinds.map((value) => (
            <button className={kind === value ? "active" : ""} type="button" key={value} onClick={() => setKind(value)}>
              {assetKindLabel(t, value)}
            </button>
          ))}
        </div>
        {availableAgents.length > 1 ? (
          <label className="asset-agent-filter">
            <span>{t.agentFilter}</span>
            <select value={agentID} onChange={(event) => setAgentID(event.target.value)}>
              <option value="all">{t.allAgents}</option>
              {availableAgents.map((agent) => <option key={agent.agent_id} value={agent.agent_id}>{agent.display_name}</option>)}
            </select>
          </label>
        ) : null}
      </div>
      {filtered.length === 0 ? <p className="asset-library-no-match">{t.noMatchingAssets}</p> : (
        <div className="asset-library-workspace">
          <nav className="asset-index" aria-label={t.assetLibrary}>
            {filtered.map((record) => {
              const recordID = assetRecordID(record);
              const active = selected === record;
              return (
                <button
                  className={active ? "asset-index-row selected" : "asset-index-row"}
                  type="button"
                  key={recordID}
                  aria-current={active ? "true" : undefined}
                  onClick={() => setSelectedID(recordID)}
                >
                  <span className="asset-index-file">
                    {record.candidate.kind === "agent_md"
                      ? "agent.md"
                      : locale === "zh"
                        ? `${assetKindLabel(t, record.candidate.kind)} 包`
                        : `${assetKindLabel(t, record.candidate.kind)} package`}
                  </span>
                  <strong>{record.candidate.title || assetKindLabel(t, record.candidate.kind)}</strong>
                  <span className="asset-index-meta">
                    <b>{record.agentName}</b>
                    <time dateTime={record.job.updated_at}>{formattedTime(record.job.updated_at, locale)}</time>
                  </span>
                </button>
              );
            })}
          </nav>
          {selected ? (
            <AssetDocument
              candidate={selected.candidate}
              job={selected.job}
              agentName={selected.agentName}
              locale={locale}
              t={t}
              onOpenAnalysis={() => onOpenAnalysis(selected.job.job_id)}
              onDeleteAsset={() => onDeleteAsset(selected.job.job_id)}
            />
          ) : null}
        </div>
      )}
    </section>
  );
}

function AssetDocument({
  candidate,
  job,
  agentName,
  locale,
  t,
  onOpenAnalysis,
  onDeleteAsset,
}: {
  candidate: EvolutionCandidate;
  job?: EvolutionJob;
  agentName?: string;
  locale: Locale;
  t: Copy;
  onOpenAnalysis?: () => void;
  onDeleteAsset?: () => Promise<void>;
}) {
  const [mode, setMode] = useState<"read" | "source">("read");
  const [copyState, setCopyState] = useState<"idle" | "copied" | "error">("idle");
  const [deleteConfirming, setDeleteConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const files = agentAssetFiles(candidate);
  const path = agentAssetPath(candidate);
  const [selectedFilePath, setSelectedFilePath] = useState(files[0]?.path || path);
  const selectedFile = files.find((file) => file.path === selectedFilePath) || files[0];
  const content = selectedFile?.content || agentAssetText(candidate);
  const filename = selectedFile?.path.split("/").at(-1) || agentAssetFilename(candidate);
  const archive = agentAssetArchive(candidate);
  const traceCount = candidate.source_trace_ids?.length
    || (candidate.source_trace_id ? 1 : 0)
    || job?.source_trace_ids?.length
    || 0;

  useEffect(() => {
    setCopyState("idle");
    setMode("read");
    setSelectedFilePath(files[0]?.path || path);
    setDeleteConfirming(false);
    setDeleteError("");
  }, [candidate.candidate_id, path]);

  const deleteAsset = async () => {
    if (!onDeleteAsset || deleting) return;
    setDeleting(true);
    setDeleteError("");
    try {
      await onDeleteAsset();
    } catch (cause) {
      setDeleteError(cause instanceof Error ? cause.message : t.deleteFailed);
      setDeleting(false);
    }
  };

  return (
    <article className="asset-document">
      <header className="asset-document-header">
        <div>
          <span className="asset-document-path">{path}</span>
          <h3>{candidate.title || filename}</h3>
          {candidate.summary ? <p>{candidate.summary}</p> : null}
        </div>
        <div className="asset-actions">
          <button className="asset-copy-button" type="button" disabled={!content} onClick={async () => {
            try {
              await navigator.clipboard.writeText(content);
              setCopyState("copied");
            } catch {
              setCopyState("error");
            }
          }}>{copyState === "copied" ? t.copiedAsset : t.copyFile}</button>
          <a
            className="asset-copy-button"
            aria-disabled={!content}
            href={content ? agentAssetDownloadURL(filename, content) : undefined}
            download={filename}
            onClick={(event) => { if (!content) event.preventDefault(); }}
          >{t.downloadAsset}</a>
          {archive ? (
            <button className="asset-copy-button" type="button" onClick={() => downloadAssetArchive(archive)}>
              {t.downloadPackage}
            </button>
          ) : null}
          {onDeleteAsset ? (
            <button className="asset-delete-button" type="button" onClick={() => setDeleteConfirming(true)}>{t.deleteAsset}</button>
          ) : null}
        </div>
      </header>
      {copyState === "error" ? <span className="asset-copy-error" role="alert">{t.copyAssetFailed}</span> : null}
      {deleteConfirming ? (
        <div className="asset-delete-confirmation" role="group" aria-label={t.deleteAssetTitle}>
          <div><strong>{t.deleteAssetTitle}</strong><span>{t.deleteAssetWarning}</span></div>
          <div>
            <button className="secondary-button" type="button" onClick={() => setDeleteConfirming(false)} disabled={deleting}>{t.cancelDelete}</button>
            <button className="danger-button" type="button" onClick={() => void deleteAsset()} disabled={deleting}>{deleting ? t.deleting : t.confirmDelete}</button>
          </div>
        </div>
      ) : null}
      {deleteError ? <p className="inline-note error asset-delete-error" role="alert">{t.deleteFailed}: {deleteError}</p> : null}
      <dl className="asset-document-meta">
        <div><dt>{t.assetFilter}</dt><dd>{assetKindLabel(t, candidate.kind)}</dd></div>
        {agentName ? <div><dt>{t.agent}</dt><dd>{agentName}</dd></div> : null}
        <div><dt>{t.packageContents}</dt><dd>{files.length} {t.fileCount}</dd></div>
        <div><dt>{t.sourceTraces}</dt><dd>{traceCount} {t.traceSources}</dd></div>
        {job ? <div className="asset-generated-at"><dt>{t.generatedAt}</dt><dd>{formattedTime(job.updated_at, locale)}</dd></div> : null}
      </dl>
      <div className={files.length > 1 ? "asset-package-browser has-tree" : "asset-package-browser"}>
        {files.length > 1 ? (
          <nav className="asset-file-tree" aria-label={t.packageContents}>
            <strong>{t.packageContents}</strong>
            {files.map((file) => (
              <button
                className={file.path === selectedFile?.path ? "selected" : ""}
                type="button"
                key={file.path}
                onClick={() => {
                  setSelectedFilePath(file.path);
                  setMode("read");
                  setCopyState("idle");
                }}
              >
                <span>{file.path.slice(path.length).replace(/^\//, "") || file.path}</span>
              </button>
            ))}
          </nav>
        ) : null}
        <section className="asset-file-viewer">
          <header className="asset-file-toolbar">
            <span><b>{t.selectedFile}</b>{selectedFile?.path || path}</span>
            <div className="asset-document-toolbar" role="tablist" aria-label={t.assetDocument}>
              <button className={mode === "read" ? "active" : ""} type="button" role="tab" aria-selected={mode === "read"} onClick={() => setMode("read")}>{t.readAsset}</button>
              <button className={mode === "source" ? "active" : ""} type="button" role="tab" aria-selected={mode === "source"} onClick={() => setMode("source")}>{t.sourceAsset}</button>
            </div>
          </header>
          <div className="asset-document-body" role="tabpanel">
            {mode === "source" ? <pre>{content}</pre> : <AgentAssetFileDocument filename={filename} content={content} />}
          </div>
        </section>
      </div>
      {job ? (
        <div className="asset-document-context">
          <section>
            <h4>{t.assetReason}</h4>
            <strong>{job.finding?.title || job.objective || t.defaultObjective}</strong>
            {job.finding?.summary ? <p>{job.finding.summary}</p> : null}
          </section>
          <section>
            <h4>{t.assetQuality}</h4>
            <strong className={`review-verdict verdict-${safeState(job.review?.verdict || "blocked")}`}>
              {job.review?.verdict === "pass" ? t.qualityPassed : t.qualityFailed}
            </strong>
            {job.review?.summary ? <p>{conciseReviewSummary(job.review.summary)}</p> : null}
          </section>
        </div>
      ) : null}
      <footer className="asset-document-footer">
        <ProposalProvenance
          sourceAgentID={candidate.source_agent_id}
          sourceTraceID={candidate.source_trace_id}
          sourceTraceIDs={candidate.source_trace_ids}
          evidenceSHA={candidate.evidence_pack_sha256}
          t={t}
        />
        {onOpenAnalysis ? <button className="text-button" type="button" onClick={onOpenAnalysis}>{t.openAnalysis}</button> : null}
      </footer>
    </article>
  );
}

function AgentAssetFileDocument({ filename, content }: { filename: string; content: string }) {
  if (/\.md$/i.test(filename)) return <MarkdownDocument value={content} />;
  if (/\.json$/i.test(filename)) {
    try {
      return <StructuredAssetDocument value={JSON.parse(content)} />;
    } catch {
      return <pre>{content}</pre>;
    }
  }
  return <pre className="asset-code-file">{content}</pre>;
}

function MarkdownDocument({ value }: { value: string }) {
  const normalized = value.replace(/\r\n/g, "\n").trim();
  const frontmatterMatch = normalized.match(/^---\n([\s\S]*?)\n---\n?/);
  const metadata = frontmatterMatch?.[1].split("\n").map((line) => {
    const separator = line.indexOf(":");
    return separator > 0 ? [line.slice(0, separator).trim(), line.slice(separator + 1).trim()] : [line.trim(), ""];
  }) || [];
  const body = frontmatterMatch ? normalized.slice(frontmatterMatch[0].length).trim() : normalized;
  const blocks = body.split(/\n{2,}/).filter(Boolean);
  return (
    <div className="markdown-document">
      {metadata.length ? <dl className="asset-frontmatter">{metadata.map(([key, entry]) => <div key={key}><dt>{key}</dt><dd>{entry}</dd></div>)}</dl> : null}
      {blocks.map((block, index) => {
        const heading = block.match(/^(#{1,4})\s+(.+)$/s);
        if (heading && !heading[2].includes("\n")) {
          const level = heading[1].length;
          if (level === 1) return <h1 key={index}>{heading[2]}</h1>;
          if (level === 2) return <h2 key={index}>{heading[2]}</h2>;
          return <h3 key={index}>{heading[2]}</h3>;
        }
        if (/^```/.test(block)) return <pre key={index}>{block.replace(/^```[^\n]*\n?/, "").replace(/\n?```$/, "")}</pre>;
        const lines = block.split("\n");
        if (lines.every((line) => /^[-*]\s+/.test(line))) {
          return <ul key={index}>{lines.map((line, itemIndex) => <li key={itemIndex}>{inlineMarkdown(line.replace(/^[-*]\s+/, ""))}</li>)}</ul>;
        }
        if (lines.every((line) => /^\d+[.)]\s+/.test(line))) {
          return <ol key={index}>{lines.map((line, itemIndex) => <li key={itemIndex}>{inlineMarkdown(line.replace(/^\d+[.)]\s+/, ""))}</li>)}</ol>;
        }
        return <p key={index}>{inlineMarkdown(block)}</p>;
      })}
    </div>
  );
}

function inlineMarkdown(value: string) {
  return value.split(/(`[^`]+`|\*\*[^*]+\*\*)/g).filter(Boolean).map((part, index) => {
    if (part.startsWith("`") && part.endsWith("`")) return <code key={index}>{part.slice(1, -1)}</code>;
    if (part.startsWith("**") && part.endsWith("**")) return <strong key={index}>{part.slice(2, -2)}</strong>;
    return part;
  });
}

function StructuredAssetDocument({ value }: { value: unknown }) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return <pre>{prettyJSON(value)}</pre>;
  const entries = Object.entries(value as Record<string, unknown>).filter(([key]) => key !== "path" && key !== "markdown");
  if (!entries.length) return <pre>{prettyJSON(value)}</pre>;
  return (
    <dl className="structured-asset-document">
      {entries.map(([key, entry]) => (
        <div key={key}>
          <dt>{key.replaceAll("_", " ")}</dt>
          <dd>{Array.isArray(entry) && entry.every((item) => ["string", "number", "boolean"].includes(typeof item))
            ? <ul>{entry.map((item, index) => <li key={index}>{String(item)}</li>)}</ul>
            : typeof entry === "string" || typeof entry === "number" || typeof entry === "boolean"
              ? String(entry)
              : <pre>{prettyJSON(entry)}</pre>}</dd>
        </div>
      ))}
    </dl>
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

function EvolutionJobDetail({ job, locale, t, agentName, action, notice }: { job: EvolutionJob; locale: Locale; t: Copy; agentName: string; action?: ReactNode; notice?: ReactNode }) {
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
        <div className="job-detail-actions">
          {updated ? <span className="job-updated">{t.updated} {updated}</span> : null}
          {action}
        </div>
      </header>
      {notice}
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
  const archive = agentAssetArchive(candidate);

  const assetLabel = (asset: EvolutionCandidate) => asset.kind === "agent_md"
    ? t.agentMD
    : asset.kind === "skill"
      ? t.skill
      : asset.kind === "role"
        ? t.role
        : asset.kind === "dsh_plugin"
          ? t.dshPlugin
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
          {archive ? (
            <button className="asset-copy-button" type="button" onClick={() => downloadAssetArchive(archive)}>
              {t.downloadPackage}
            </button>
          ) : null}
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

function downloadAssetArchive(archive: ReturnType<typeof agentAssetArchive>) {
  if (!archive) return;
  const bytes = archive.bytes.slice().buffer;
  const url = URL.createObjectURL(new Blob([bytes], { type: "application/x-tar" }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = archive.filename;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
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
