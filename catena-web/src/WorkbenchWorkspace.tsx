import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { AgentEvolutionLauncher } from "./AgentEvolutionLauncher";
import { traceSummaryTitle } from "./traceView";
import type { EvolutionJob, WorkspaceData } from "./types";

export function WorkbenchWorkspace({ locale, workspace, ownerID, initialAgentID, onOpenTrace, onOpenJob, onConnectAgent, onRefresh }: {
  locale: "zh" | "en";
  workspace: WorkspaceData;
  ownerID: string;
  initialAgentID: string;
  onOpenTrace: (agentID: string, traceID?: string) => void;
  onOpenJob: (job: EvolutionJob) => void;
  onConnectAgent: () => void;
  onRefresh: () => Promise<void>;
}) {
  const zh = locale === "zh";
  const storageKey = `catena.assistant.${ownerID}`;
  const [preferences, setPreferences] = useState<{ goal: string; watching: boolean } | null>(null);
  const [configured, setConfigured] = useState<boolean | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const newest = useRef("");
  useEffect(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) || "{}");
      setPreferences({ goal: typeof saved.goal === "string" ? saved.goal : "", watching: saved.watching === true });
    } catch { setPreferences({ goal: "", watching: false }); }
  }, [storageKey]);
  useEffect(() => {
    if (preferences) try { localStorage.setItem(storageKey, JSON.stringify(preferences)); } catch { /* Browser storage can be unavailable. */ }
  }, [preferences, storageKey]);
  useEffect(() => {
    let active = true;
    api.llmConfig().then((config) => { if (active) setConfigured(config.configured); })
      .catch((cause) => { if (active) setError(String(cause)); });
    return () => { active = false; };
  }, []);
  useEffect(() => {
    newest.current = workspace.traces.reduce((value, trace) => trace.last_ingested_at > value ? trace.last_ingested_at : value, newest.current);
  }, [workspace.traces]);
  useEffect(() => {
    if (!preferences?.watching) return;
    let stopped = false;
    const timer = window.setInterval(async () => {
      if (document.hidden) return;
      try {
        const result = await api.traces(100);
        const latest = result.traces.reduce((value, trace) => trace.last_ingested_at > value ? trace.last_ingested_at : value, "");
        if (!stopped && latest > newest.current) {
          newest.current = latest;
          await onRefresh();
          if (!stopped) setMessage(zh ? "新经历已更新。" : "New history has been updated.");
        }
      } catch (cause) { if (!stopped) setError(String(cause)); }
    }, 60000);
    return () => { stopped = true; window.clearInterval(timer); };
  }, [preferences?.watching, onRefresh, zh]);
  useEffect(() => {
    if (!workspace.evolutionJobs.some((job) => ["queued", "running"].includes(job.state))) return;
    const timer = window.setInterval(() => { if (!document.hidden) void onRefresh(); }, 5000);
    return () => window.clearInterval(timer);
  }, [workspace.evolutionJobs, onRefresh]);
  const agents = workspace.agents.filter((agent) => agent.registered);
  const recent = workspace.traces.filter((trace) => new Date(trace.end_time).getTime() >= Date.now() - 7 * 86400000);
  const signals = recent.filter((trace) => trace.error_count > 0);
  const jobs = [...workspace.evolutionJobs].sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const activeJobs = jobs.filter((job) => ["queued", "running"].includes(job.state));
  const lead = signals[0] ?? recent[0];
  const names = new Map(workspace.agents.map((agent) => [agent.agent_id, agent.display_name]));
  const states: Record<string, string> = zh ? { queued: "排队中", running: "分析中", completed: "已完成", succeeded: "已完成", failed: "失败", cancelled: "已取消" } : {};
  return <section className="page proactive-page workbench-page">
    <header className="proactive-hero"><span className="proactive-eyebrow">{zh ? "经历 → 分析 → 实验 → 产出" : "History → Analysis → Experiments → Outputs"}</span><h1>{zh ? "工作台" : "Workspace"}</h1><p>{zh ? "从真实经历中发现问题，把有用的发现带到验证和下一次工作。" : "Find problems in real history and bring useful findings into verification and your next task."}</p></header>
    {error && <p className="proactive-error" role="alert">{error}</p>}
    {Object.values(workspace.overviewErrors ?? {}).map((detail, i) => <p key={i} className="proactive-error" role="alert">{detail} <button className="text-button" onClick={() => void onRefresh()}>{zh ? "重试" : "Retry"}</button></p>)}
    {message && <p role="status" className="proactive-message">{message}</p>}
    <div className="proactive-metrics"><div><strong>{recent.length}</strong><span>{zh ? "近 7 天样本" : "7-day sample"}</span></div><div><strong>{signals.length}</strong><span>{zh ? "待核对信号" : "Signals to review"}</span></div><div><strong>{activeJobs.length}</strong><span>{zh ? "进行中的分析" : "Active analyses"}</span></div></div>
    <p className="proactive-caveat">{zh ? "统计基于最近 100 条 Trace；错误状态不等同于任务失败。" : "Counts use the latest 100 traces; an error state does not establish task failure."}</p>
    <div className="proactive-columns proactive-main">
      <section className="proactive-panel proactive-goal"><h2 className="proactive-section-title">{zh ? "开始一次分析" : "Start an analysis"}</h2>
        {preferences && <AgentEvolutionLauncher key={ownerID} locale={locale} agents={agents} initialAgentID={initialAgentID} enabled={configured === true} initialObjective={preferences.goal} onObjectiveChange={(goal) => setPreferences((current) => current ? { ...current, goal } : current)} onStarted={onOpenJob} embedded />}
        {configured === false && <p className="proactive-muted">{zh ? "开始前请保存分析模型配置。" : "Save a model configuration before starting."} <button className="text-button" onClick={onConnectAgent}>{zh ? "前往连接" : "Connections"}</button></p>}
        {!agents.length && <button className="secondary-button" onClick={onConnectAgent}>{zh ? "接入 Agent" : "Connect Agent"}</button>}
      </section>
      <section className="proactive-panel proactive-lead"><h2 className="proactive-section-title">{zh ? "接下来值得看" : "Worth a look"}</h2><div className="proactive-lead-content"><h2>{lead ? (lead.error_count ? (zh ? "有待核对的错误信号" : "An error signal to review") : (zh ? "有新的 Agent 经历" : "New Agent activity")) : (zh ? "等待第一条经历" : "Waiting for the first trace")}</h2><p>{zh ? "回看完整上下文，再判断是否需要提炼 Skill 或建立实验。" : "Review the full context before deciding to generate a Skill or create an experiment."}</p>{lead ? <button className="proactive-link" onClick={() => onOpenTrace(lead.agent_id ?? "", lead.trace_id)}>{zh ? "查看来源 →" : "Open evidence →"}</button> : <button className="proactive-link" onClick={onConnectAgent}>{zh ? "连接 Agent →" : "Connect Agent →"}</button>}</div></section>
    </div>
    <section className="proactive-watch"><div><h2>{zh ? "页面打开时关注新经历" : "Watch for new history"}</h2><p>{zh ? "每分钟检查一次。关闭页面后暂停；关注目标保存在此浏览器。" : "Checks every minute while this page is open. The focus is saved in this browser."}</p></div><label className="proactive-switch"><input type="checkbox" aria-label={zh ? "关注新经历" : "Watch new history"} checked={preferences?.watching ?? false} onChange={(event) => setPreferences((current) => current ? { ...current, watching: event.target.checked } : current)} /><span aria-hidden="true" /></label></section>
    <div className="proactive-columns proactive-bottom">
      <section className="proactive-list"><h2>{zh ? "最近经历" : "Recent history"}</h2>{recent.slice(0, 4).map((trace) => <button key={trace.trace_id} onClick={() => onOpenTrace(trace.agent_id ?? "", trace.trace_id)}><span><strong>{names.get(trace.agent_id ?? "") || trace.service_name}</strong><small>{traceSummaryTitle(trace, locale)}</small></span><time>{new Date(trace.end_time).toLocaleDateString(zh ? "zh-CN" : "en-US")}</time></button>)}{!recent.length && <p className="proactive-muted">{zh ? "等待 Agent 上传经历。" : "Waiting for Agent history."}</p>}</section>
      <section className="proactive-list"><h2>{zh ? "分析记录" : "Analysis history"}<span className="workbench-count">{jobs.length}</span></h2>{jobs.map((job) => <button key={job.job_id} onClick={() => onOpenJob(job)}><span><strong>{job.finding?.title || job.objective || names.get(job.source_agent_id ?? "") || (zh ? "Agent 分析" : "Agent analysis")}</strong><small>{states[job.state] ?? job.state} · {new Date(job.updated_at).toLocaleString(zh ? "zh-CN" : "en-US")}</small></span><span aria-hidden="true">↗</span></button>)}{!jobs.length && <p className="proactive-muted">{zh ? "还没有分析记录。从上方选择 Agent 和关注目标开始。" : "No analyses yet. Choose an Agent and focus above."}</p>}</section>
    </div>
  </section>;
}
