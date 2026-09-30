import { useMemo, useState } from "react";
import { Icon } from "./Icons";
import { agentAssets } from "./evolution";
import { traceSummaryTitle } from "./traceView";
import type { EvolutionJob, WorkspaceData } from "./types";
import type { Route } from "./navigation";

type Locale = "zh" | "en";

export function OverviewWorkspace({ locale, workspace, onNavigate, onOpenTrace, onOpenJob, onRetry }: {
  locale: Locale;
  workspace: WorkspaceData;
  onNavigate: (route: Route) => void;
  onOpenTrace: (agentID: string, traceID: string) => void;
  onOpenJob: (job: EvolutionJob) => void;
  onRetry: () => void;
}) {
  const zh = locale === "zh";
  const [query, setQuery] = useState("");
  const [agentID, setAgentID] = useState("");
  const errors = workspace.overviewErrors ?? {};
  const assets = useMemo(() => [...workspace.evolutionJobs].sort((a, b) => b.updated_at.localeCompare(a.updated_at))
    .flatMap((job) => agentAssets(job).map((candidate) => ({ job, candidate }))), [workspace.evolutionJobs]);
  const traces = useMemo(() => [...workspace.traces].sort((a, b) => b.start_time.localeCompare(a.start_time)), [workspace.traces]);
  const names = new Map(workspace.agents.map((agent) => [agent.agent_id, agent.display_name]));
  const matching = traces.filter((trace) => (!agentID || trace.agent_id === agentID) &&
    [traceSummaryTitle(trace, locale), names.get(trace.agent_id ?? "") ?? trace.service_name, trace.trace_id]
      .join(" ").toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  const activeJobs = workspace.evolutionJobs.filter((job) => ["queued", "running"].includes(job.state));
  const empty = !Object.keys(errors).length && workspace.agents.length === 0 && traces.length === 0 && workspace.evolutionJobs.length === 0;

  const errorNotice = (title: string, detail: string) => (
    <div className="overview-error" role="alert">
      <strong>{title}</strong><p>{detail}</p>
      <button type="button" className="text-button" onClick={onRetry}>{zh ? "重新读取" : "Try again"}<Icon name="refresh" /></button>
    </div>
  );

  return (
    <section className="page overview-page">
      <header className="overview-header">
        <h1>{zh ? "总览" : "Overview"}</h1>
        {!empty && <button className="text-button" type="button" onClick={() => onNavigate("apiKeys")}><Icon name="plus" />{zh ? "接入 Agent" : "Connect Agent"}</button>}
      </header>

      {empty ? (
        <section className="overview-welcome">
          <Icon name="history" />
          <h2>{zh ? "还没有经历" : "No history yet"}</h2>
          <p>{zh ? "接入一个 Agent，开始记录。" : "Connect an Agent to start collecting its work."}</p>
          <button className="primary-button" type="button" onClick={() => onNavigate("apiKeys")}><Icon name="plus" />{zh ? "接入新 Agent" : "Connect your first Agent"}</button>
        </section>
      ) : (
        <div className="overview-columns">
          <section className="overview-history" aria-labelledby="overview-history-title">
            <header className="panel-heading">
              <h2 id="overview-history-title">{zh ? "最近经历" : "Recent history"}</h2>
              <button className="text-button" type="button" onClick={() => onNavigate("traces")}>{zh ? "全部" : "View all"}<Icon name="arrow" /></button>
            </header>
            {errors.traces ? errorNotice(zh ? "经历暂时无法读取" : "History is unavailable", errors.traces) : (
              <>
                <div className="overview-history-tools">
                  <label className="search-field">
                    <Icon name="search" />
                    <input aria-label={zh ? "搜索最近经历" : "Search recent history"} placeholder={zh ? "搜索经历" : "Search history"} value={query} onChange={(event) => setQuery(event.target.value)} />
                  </label>
                  <select aria-label={zh ? "筛选 Agent" : "Filter Agent"} value={agentID} onChange={(event) => setAgentID(event.target.value)}>
                    <option value="">{zh ? "全部 Agent" : "All Agents"}</option>
                    {workspace.agents.map((agent) => <option key={agent.agent_id} value={agent.agent_id}>{agent.display_name}</option>)}
                  </select>
                </div>
                <div className="overview-trace-list">
                  {matching.slice(0, 7).map((trace) => (
                    <button type="button" className="overview-trace" key={trace.trace_id} onClick={() => onOpenTrace(trace.agent_id ?? "", trace.trace_id)}>
                      <span className="overview-trace-copy">
                        <strong>{traceSummaryTitle(trace, locale)}</strong>
                        <span>{names.get(trace.agent_id ?? "") ?? trace.service_name}
                          {trace.error_count > 0 && <em>{trace.error_count} {zh ? "处错误" : "errors"}</em>}
                        </span>
                      </span>
                      <time dateTime={trace.start_time}>{formatOverviewTime(trace.start_time, locale)}</time>
                    </button>
                  ))}
                </div>
                {matching.length === 0 && <div className="panel-empty">
                  <p>{query || agentID ? (zh ? "没有匹配的经历" : "No matching history") : (zh ? "等待第一条 Trace" : "Waiting for the first Trace")}</p>
                  {(query || agentID) && <button className="text-button" type="button" onClick={() => { setQuery(""); setAgentID(""); }}>{zh ? "清除筛选" : "Clear filters"}</button>}
                </div>}
              </>
            )}
            {errors.agents && errorNotice(zh ? "Agent 信息暂时无法读取" : "Agent information is unavailable", errors.agents)}
          </section>

          <aside className="overview-aside">
            <section className="overview-outputs" aria-labelledby="overview-outputs-title">
              <header className="panel-heading">
                <h2 id="overview-outputs-title">{zh ? "最近产出" : "Recent outputs"}</h2>
                <button className="text-button icon-button" type="button" aria-label={zh ? "查看全部产出" : "View all outputs"} title={zh ? "查看全部产出" : "View all outputs"} onClick={() => onNavigate("evolution")}><Icon name="arrow" /></button>
              </header>
              {errors.evolutionJobs ? errorNotice(zh ? "产出暂时无法读取" : "Outputs are unavailable", errors.evolutionJobs) : assets.length ? (
                <div className="overview-asset-list">
                  {assets.slice(0, 3).map(({ job, candidate }) => (
                    <button className="overview-asset" type="button" key={job.job_id + ":" + candidate.candidate_id} onClick={() => onOpenJob(job)}>
                      <Icon name="outputs" />
                      <span><strong>{candidate.title}</strong><small>{candidate.kind === "agent_md" ? "agent.md" : candidate.kind === "skill" ? "Skill" : candidate.kind === "role" ? "Role" : "Plugin"}</small></span>
                    </button>
                  ))}
                </div>
              ) : <p className="overview-no-output">{zh ? "暂无产出" : "No outputs yet"}</p>}
              {!errors.evolutionJobs && activeJobs.length > 0 && <p className="overview-running" role="status"><span />{activeJobs.length} {zh ? "项分析进行中" : "analyses in progress"}</p>}
            </section>
          </aside>
        </div>
      )}
    </section>
  );
}

function formatOverviewTime(value: string, locale: Locale) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "—";
  const today = date.toDateString() === new Date().toDateString();
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en",
    today ? { hour: "2-digit", minute: "2-digit" } : { month: "short", day: "numeric" }).format(date);
}
