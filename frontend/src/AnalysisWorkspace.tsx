import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { AgentSummary, AnalysisJob, HumanReview } from "./types";

const jobKind = (job: AnalysisJob, zh: boolean) => job.pipeline === "analyze" ? (zh ? "Trace 分析" : "Trace analysis") : (zh ? "候选改进" : "Improvement");
const jobState = (state: string, zh: boolean) => zh ? ({completed:"已完成",failed:"失败",cancelled:"已取消",queued:"排队中",running:"运行中"}[state] ?? state) : state;
const jobDate = (date: string, locale: "zh" | "en") => date && !Number.isNaN(Date.parse(date)) ? new Date(date).toLocaleString(locale === "zh" ? "zh-CN" : "en-US", {month:"short",day:"numeric",hour:"2-digit",minute:"2-digit"}) : "—";

function jobTitle(job: AnalysisJob, zh: boolean) {
 const report = job.artifacts.find(a => a.type === "analysis_report")?.payload as {problems?: {title?: string}[]} | undefined;
 const provenance = job.artifacts.find(a => a.type !== "analysis_report")?.provenance as {compare?: {case_id?: string}[]} | undefined;
 return report?.problems?.[0]?.title || provenance?.compare?.[0]?.case_id || job.objective || job.source_agent_id || jobKind(job,zh);
}

export function AnalysisWorkspace({ locale, jobs, initialJobID, onAnalyze, onJobSelected, onJobDeleted, onJobsUpdated, refreshVersion }: {
 locale: "zh" | "en"; jobs: AnalysisJob[]; agents: AgentSummary[]; initialJobID?: string; analysisOnly?: boolean;
 onAnalyze: () => void; onJobSelected: (id: string) => void; onJobDeleted: (id: string) => void;
 onJobsUpdated: (jobs: AnalysisJob[]) => void; refreshVersion: string;
}) {
 const zh = locale === "zh";
 const [error, setError] = useState("");
 const [historyOpen, setHistoryOpen] = useState(false);
 const selected = initialJobID ? jobs.find(j => j.job_id === initialJobID) : jobs[0];
 useEffect(() => {
   const controller = new AbortController();
   const refresh = () => { void api.analysisJobs(controller.signal).then(async next => { if(initialJobID && !next.some(j => j.job_id === initialJobID)) next.push(await api.analysisJob(initialJobID, controller.signal)); onJobsUpdated(next); setError(""); }).catch(e => { if (!controller.signal.aborted) setError(String(e)); }); };
   refresh(); const timer = jobs.some(j => ["queued","running"].includes(j.state)) ? window.setInterval(refresh, 2000) : undefined;
   return () => { controller.abort(); if (timer !== undefined) clearInterval(timer); };
 }, [refreshVersion, initialJobID, onJobsUpdated, jobs.map(j => `${j.job_id}:${j.state}`).join("|")]);
 const terminal = selected && ["completed", "failed", "cancelled"].includes(selected.state);
 return <section className="page analysis-workspace">
  <header className="analysis-page-header"><div><span className="eyebrow">CATENA / {zh ? "分析与改进" : "ANALYSIS & IMPROVEMENT"}</span><h1>{zh ? "从证据到改进" : "From evidence to improvement"}</h1><p>{zh ? "先读懂问题，再决定是否优化。" : "Understand the problem. Then decide what to improve."}</p></div>
   <button className="primary-button" onClick={onAnalyze}>{zh ? "＋ 创建分析" : "+ New analysis"}</button>
  </header>
  {error && <p className="analysis-alert" role="alert">{error}</p>}
  <div className="analysis-layout">
   <aside className={`analysis-history ${historyOpen ? "is-open" : ""}`}>
    <h2 className="analysis-history-heading">{zh ? "任务记录" : "Job history"}<span>{jobs.length}</span></h2>
    <button className="analysis-history-toggle" aria-expanded={historyOpen} aria-controls="analysis-job-list" onClick={()=>setHistoryOpen(!historyOpen)}>{zh ? "任务记录" : "Job history"}<span>{jobs.length}<span className="analysis-history-indicator"> {historyOpen ? "−" : "+"}</span></span></button>
    <nav id="analysis-job-list" aria-label={zh ? "任务记录" : "Job history"}>{jobs.map(j => <button className={`analysis-job-row ${selected?.job_id === j.job_id ? "is-selected" : ""}`} aria-current={selected?.job_id === j.job_id ? "true" : undefined} key={j.job_id} onClick={() => {onJobSelected(j.job_id);setHistoryOpen(false);}}>
     <span className="analysis-job-meta"><span>{jobKind(j,zh)}</span><span className={`analysis-status status-${j.state}`}>{jobState(j.state,zh)}</span></span>
     <strong>{jobTitle(j,zh)}</strong><time dateTime={j.created_at}>{jobDate(j.created_at,locale)}</time>
    </button>)}</nav>
   </aside>
   {selected ? <article className="analysis-document" key={selected.job_id}>
    <header className="analysis-document-header"><div className="analysis-job-meta"><span>{jobKind(selected,zh)}</span><span className={`analysis-status status-${selected.state}`}>{jobState(selected.state,zh)}</span><time dateTime={selected.created_at}>{jobDate(selected.created_at,locale)}</time></div>
     <h2>{selected.pipeline === "analyze" ? (zh ? "Trace 分析报告" : "Trace analysis report") : (zh ? "候选改进与验证" : "Candidate improvement & evaluation")}</h2>
     <div className="analysis-context"><span>{selected.source_trace_ids.length || (selected.source_trace_id ? 1 : 0)} Trace</span><span>{zh ? "执行次数" : "Attempts"} {selected.attempt ?? 0}</span>{selected.analysis_job_id && <button className="text-button" onClick={()=>onJobSelected(selected.analysis_job_id!)}>{zh ? "↗ 查看来源分析" : "↗ Source analysis"}</button>}</div>
    </header>
    {selected.error && <p className="analysis-alert" role="alert">{selected.error}</p>}
    {!terminal && <div className="analysis-empty" role="status"><h3>{zh ? (selected.cancel_requested ? "正在取消任务" : "正在处理证据") : (selected.cancel_requested ? "Cancellation requested" : "Processing evidence")}</h3><p>{zh ? "结果会自动更新，你可以离开此页面。" : "Results update automatically. You can leave this page."}</p></div>}
    {(selected.artifacts ?? []).map(a => <section className="analysis-artifact" key={a.id}>
      {a.type === "analysis_report" ? <ProblemReport value={a.payload} locale={locale} /> : <>
       <div className="analysis-section-title"><h3>{zh ? "候选与验证" : "Candidate & evaluation"}</h3><span className="eyebrow">{selected.reviews?.some(r => r.artifact_id === a.id) ? (zh ? "已完成审阅" : "Reviewed") : a.validation_status === "review_required" ? (zh ? "等待人工审阅" : "Awaiting review") : (zh ? "尚未独立验证" : "Unverified")}</span></div>
       <Compare value={a.provenance} locale={locale} />
       <ArtifactPayload value={a.payload} onError={setError} />
       <Review locale={locale} key={a.id} jobID={selected.job_id} artifactID={a.id} ready={a.validation_status === "review_required"} review={selected.reviews?.find(r => r.artifact_id === a.id)} onSaved={job => onJobsUpdated([job])} />
      </>}
      <details className="analysis-technical"><summary>{zh ? "来源与执行详情" : "Provenance / evaluation"}</summary><pre>{JSON.stringify(a.provenance, null, 2)}</pre></details>
    </section>)}
    {terminal && !selected.artifacts?.length && <div className="analysis-empty"><h3>{zh ? "暂无分析产物" : "No artifacts"}</h3><p>{zh ? "本次执行未生成报告，可查看下方执行记录。" : "This execution produced no report. Inspect the execution details below."}</p></div>}
    {selected.pipeline === "analyze" && selected.state === "completed" && <StartImprovement jobID={selected.job_id} locale={locale} onStarted={job=>{onJobsUpdated([job]);onJobSelected(job.job_id);}} />}
    <footer className="analysis-document-footer">
     <details className="analysis-technical"><summary>{zh ? "任务与原始证据" : "Job & raw evidence"}</summary><dl><dt>Job ID</dt><dd>{selected.job_id}</dd><dt>{zh ? "分析目标" : "Objective"}</dt><dd>{selected.objective || "—"}</dd><dt>Runtime</dt><dd>{selected.adapter ?? "—"}</dd><dt>Trace</dt><dd>{selected.source_trace_ids.join(", ") || selected.source_trace_id || selected.source_conversation_id || "—"}</dd></dl><pre>{JSON.stringify(selected.evidence_pack, null, 2)}</pre></details>
     {selected.state === "failed" && selected.checkpoint != null && <details className="analysis-technical"><summary>{zh ? "失败执行记录" : "Failed execution evidence"}</summary><pre>{JSON.stringify(selected.checkpoint, null, 2)}</pre></details>}
     <button className="text-button analysis-delete" disabled={!!selected.reviews?.length || (!terminal && selected.cancel_requested)} onClick={() => { if (terminal && !window.confirm(zh ? "删除此任务及其产物？此操作无法撤销。" : "Delete this analysis and its artifacts?")) return; void (terminal ? api.deleteAnalysisJob(selected.job_id).then(() => onJobDeleted(selected.job_id)) : api.cancelAnalysisJob(selected.job_id).then(next => onJobsUpdated(jobs.map(j => j.job_id === next.job_id ? next : j)))).catch(e => setError(String(e))); }}>{terminal ? (zh ? "删除任务" : "Delete") : (zh ? "取消任务" : "Cancel")}</button>
    </footer>
   </article> : <div className="analysis-document analysis-empty"><h2>{zh ? "从一条真实 Trace 开始" : "Start with a real Trace"}</h2><p>{zh ? "创建分析，定位问题并保留证据。是否优化，由你决定。" : "Create an analysis to identify problems and preserve evidence. You decide whether to improve."}</p><button className="primary-button" onClick={onAnalyze}>{zh ? "创建分析" : "New analysis"}</button></div>}
  </div>
 </section>;
}

function ProblemReport({value,locale}:{value:unknown;locale:"zh"|"en"}) {
 const zh=locale==="zh";
 const report=value as {summary?:string;problems?:{title:string;evidence:string;evidence_refs:string[];hypothesis:string;uncertainty:string}[]};
 return <section className="analysis-report" aria-label="Analysis report">
  <section className="analysis-summary"><span className="eyebrow">{zh ? "分析结论" : "SUMMARY"}</span><p>{report?.summary || (zh ? "报告未提供摘要。" : "No summary provided.")}</p><span className="analysis-caveat">{zh ? "归因假设尚未经评测验证" : "Attribution hypotheses have not been independently evaluated"}</span></section>
  <div className="analysis-section-title"><h3>{zh ? "发现的问题" : "Findings"}</h3><span>{report?.problems?.length ?? 0} {zh ? "项" : "findings"}</span></div>
  {report?.problems?.map((p,i)=><article className="analysis-problem" key={i}>
   <header><span className="analysis-problem-number">{String(i+1).padStart(2,"0")}</span><h4>{p.title}</h4></header>
   <div className="analysis-problem-body"><section><h5>{zh ? "观察到的证据" : "Observed evidence"}</h5><p>{p.evidence}</p></section><section><h5>{zh ? "根因假设" : "Root-cause hypothesis"}</h5><p>{p.hypothesis}</p></section><section className="analysis-uncertainty"><h5>{zh ? "仍需确认" : "Uncertainty"}</h5><p>{p.uncertainty}</p></section>
   <details className="analysis-evidence-refs"><summary>{zh ? "证据引用" : "Evidence references"} · {p.evidence_refs.length}</summary><ul>{p.evidence_refs.map(ref=><li key={ref}><code>{ref}</code></li>)}</ul></details></div>
  </article>)}
 </section>;
}

function StartImprovement({jobID,locale,onStarted}:{jobID:string;locale:"zh"|"en";onStarted:(job:AnalysisJob)=>void}) {
 const [opened,setOpened]=useState(false),[cases,setCases]=useState<string[]>([]),[caseID,setCaseID]=useState(""),[baseline,setBaseline]=useState("Complete the supplied task faithfully."),[error,setError]=useState(""),[busy,setBusy]=useState(false);
 const replay=useRef({signature:"",key:""});
 useEffect(()=>{if(opened)void api.evaluationCases().then(r=>setCases(r.cases.map(c=>c.id))).catch(e=>setError(String(e)));},[opened]);
 return <section className="analysis-decision" aria-label="Human improvement decision"><span className="eyebrow">{locale==="zh"?"下一步 · 由你决定":"NEXT · YOUR DECISION"}</span><h3>{locale==="zh"?"要针对这些问题生成候选吗？":"Generate a candidate for these findings?"}</h3><p>{locale==="zh"?"分析已保存。由你决定是否优化；也可以保留报告，结束本次分析。":"Analysis saved. You decide whether to improve; keeping the report ends the analysis."}</p>
 {!opened?<button className="primary-button compact" onClick={()=>setOpened(true)}>{locale==="zh"?"选择 Case，启动优化":"Choose to improve…"}</button>:<form onSubmit={async e=>{e.preventDefault();setBusy(true);setError("");const signature=JSON.stringify([jobID,caseID,baseline]);if(replay.current.signature!==signature)replay.current={signature,key:crypto.randomUUID()};try{onStarted(await api.startImprovementJob(jobID,caseID,baseline,replay.current.key));}catch(cause){setError(String(cause));}finally{setBusy(false);}}}>
 <label htmlFor="improvement-case">Harbor Case</label><select id="improvement-case" required value={caseID} onChange={e=>setCaseID(e.target.value)}><option value="">{locale === "zh" ? "选择已确认的 Case" : "Select a confirmed Case"}</option>{cases.map(id=><option key={id}>{id}</option>)}</select>
 <label>Baseline AGENTS.md<textarea required maxLength={32768} value={baseline} onChange={e=>setBaseline(e.target.value)} /></label>
 <button className="primary-button compact" type="submit" disabled={busy||!caseID}>{locale==="zh"?"人工确认，启动优化":"Confirm and start improvement"}</button><button className="secondary-button compact" type="button" onClick={()=>setOpened(false)} disabled={busy}>{locale==="zh"?"暂不优化":"Keep analysis only"}</button>
 {error&&<p role="alert">{error}</p>}</form>}</section>;
}

// Candidate file controls retain the former library's portable output behavior.
function ArtifactPayload({value,onError}:{value:unknown;onError:(message:string)=>void}) {
 const files=value && typeof value==="object" && "files" in value ? value.files : undefined;
 if(!files || typeof files!=="object" || Array.isArray(files)) return <pre>{JSON.stringify(value,null,2)}</pre>;
 return <div className="analysis-files">{Object.entries(files).filter((entry):entry is [string,string]=>typeof entry[1]==="string").map(([path,text])=><details key={path}><summary>{path}</summary><pre>{text}</pre>
  <button className="text-button" onClick={()=>{void navigator.clipboard.writeText(text).catch(()=>onError("Clipboard unavailable"));}}>Copy</button>
  <button className="text-button" onClick={()=>{const url=URL.createObjectURL(new Blob([text],{type:"text/plain;charset=utf-8"}));const link=document.createElement("a");link.href=url;link.download=path.split("/").at(-1)||"artifact.txt";link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}}>Download</button>
 </details>)}</div>;
}

function Compare({ value, locale }: { value: unknown; locale: "zh" | "en" }) {
 const zh = locale === "zh";
 if (!value || typeof value !== "object" || !("compare" in value) || !Array.isArray(value.compare)) return null;
 return <section className="analysis-compare" aria-label="Baseline / Candidate Compare"><h4>{zh ? "基线 / 候选对比" : "Baseline / Candidate Compare"}</h4>
  <p>{zh ? "每个 Case 各执行 1 次，不能证明普遍提升。缺失指标显示为 ?。" : "One trial per Case and arm does not establish general improvement. Missing metrics are unknown."}</p>
  {value.compare.map((pair, i) => <section key={i}><h5>{pair.case_id} · {pair.held_out ? (zh ? "独立验证 Case" : "held-out") : (zh ? "复用优化 Case" : "optimization Case reused")}</h5>
   <div className="comparison-table"><table><thead><tr><th scope="col">{zh ? "方案" : "Arm"}</th><th scope="col">{zh ? "通过 / 执行次数" : "Verifier pass / trials"}</th><th scope="col">{zh ? "命令错误 / 工具调用" : "Nonzero command exits / tool calls"}</th><th scope="col">{zh ? "输入 / 输出 Token" : "Input / output tokens"}</th><th scope="col">{zh ? "耗时（秒）" : "Seconds"}</th><th scope="col">{zh ? "费用（USD）" : "USD"}</th></tr></thead>
    <tbody>{(["baseline", "candidate"] as const).map(arm => { const trial = pair[arm]; return <tr key={arm}><th scope="row">{zh ? (arm === "baseline" ? "基线" : "候选") : arm}</th><td>{trial?.valid && [0, 1].includes(trial.rewards?.reward) ? `${trial.rewards.reward} / 1` : "unknown"}</td><td>{trial?.nonzero_command_exits ?? "?"} / {trial?.tool_calls ?? "?"}</td><td>{trial?.input_tokens ?? "?"} / {trial?.output_tokens ?? "?"}</td><td>{trial?.seconds?.toFixed(2) ?? "?"}</td><td>{trial?.cost_usd?.toFixed(8) ?? "?"}</td></tr>; })}</tbody>
   </table></div>
   <details className="analysis-technical"><summary>{zh ? "Harbor 原始产物路径" : "Raw Harbor artifacts"}</summary><p>{pair.baseline?.artifact_dir ?? "unknown"}<br />{pair.candidate?.artifact_dir ?? "unknown"}</p></details>
  </section>)}
 </section>;
}

function Review({ locale, jobID, artifactID, ready, review, onSaved }: { locale: "zh" | "en"; jobID: string; artifactID: string; ready: boolean; review?: HumanReview; onSaved: (job: AnalysisJob) => void }) {
 const zh = locale === "zh";
 const [reason, setReason] = useState("");
 const [decision, setDecision] = useState<"adopt" | "reject">("reject");
 const [busy, setBusy] = useState(false);
 const [error, setError] = useState("");
 if (review) return <section className="analysis-review" aria-label="Human Review"><h4>{zh ? "人工审阅" : "Human Review"} · {zh ? (review.decision === "adopt" ? "已采纳" : "已拒绝") : review.decision}</h4><p>{review.reason}</p><p>{review.reviewer} · {review.created_at}</p></section>;
 if (!ready) return <p className="analysis-caveat">{zh ? "尚未验证：完成独立 Harbor 对比后才能审阅。" : "Unverified: independent Harbor comparison required before review."}</p>;
 return <form className="analysis-review" aria-label="Human Review" onSubmit={async event => {
  event.preventDefault(); setBusy(true); setError("");
  try { onSaved(await api.reviewAnalysisJob(jobID, artifactID, decision, reason)); }
  catch (cause) { setError(String(cause)); }
  finally { setBusy(false); }
 }}><h4>{zh ? "人工审阅" : "Human Review"}</h4><p>{zh ? "核对 Verifier 和执行轨迹后记录决定。采纳仅保存决定，不会安装文件。" : "Record a decision after reviewing verifier and trajectory. Adoption does not install files."}</p>
  <label>{zh ? "审阅决定" : "Decision"}<select value={decision} disabled={busy} onChange={e => setDecision(e.target.value as "adopt" | "reject")}><option value="reject">{zh ? "拒绝" : "Reject"}</option><option value="adopt">{zh ? "采纳" : "Adopt"}</option></select></label>
  <label>{zh ? "决定理由" : "Reason"}<textarea required maxLength={4000} value={reason} disabled={busy} onChange={e => setReason(e.target.value)} /></label>
  <button className="primary-button compact" disabled={busy || !reason.trim()}>{zh ? "保存审阅决定" : "Record decision"}</button>{error && <p role="alert">{error}</p>}
 </form>;
}
