import { useEffect, useState } from "react";
import { api } from "./api";
import type { AgentSummary, AnalysisJob } from "./types";

export function AnalysisWorkspace({ locale, jobs, initialJobID, onAnalyze, onJobSelected, onJobDeleted, onJobsUpdated, refreshVersion }: {
 locale: "zh" | "en"; jobs: AnalysisJob[]; agents: AgentSummary[]; initialJobID?: string; analysisOnly?: boolean;
 onAnalyze: () => void; onJobSelected: (id: string) => void; onJobDeleted: (id: string) => void;
 onJobsUpdated: (jobs: AnalysisJob[]) => void; refreshVersion: string;
}) {
 const [error, setError] = useState("");
 const selected = initialJobID ? jobs.find(j => j.job_id === initialJobID) : jobs[0];
 useEffect(() => {
   const controller = new AbortController();
   const refresh = () => { void api.analysisJobs(controller.signal).then(async next => { if(initialJobID && !next.some(j => j.job_id === initialJobID)) next.push(await api.analysisJob(initialJobID, controller.signal)); onJobsUpdated(next); setError(""); }).catch(e => { if (!controller.signal.aborted) setError(String(e)); }); };
   refresh(); const timer = jobs.some(j => ["queued","running"].includes(j.state)) ? window.setInterval(refresh, 2000) : undefined;
   return () => { controller.abort(); if (timer !== undefined) clearInterval(timer); };
 }, [refreshVersion, initialJobID, onJobsUpdated, jobs.map(j => `${j.job_id}:${j.state}`).join("|")]);
 const terminal = selected && ["completed", "failed", "cancelled"].includes(selected.state);
 return <section className="workspace-panel analysis-workspace">
  <h2>{locale === "zh" ? "分析任务与产物" : "Analysis jobs and artifacts"}</h2>
  <button onClick={onAnalyze}>{locale === "zh" ? "创建分析" : "New analysis"}</button>
  {error && <p role="alert">{error}</p>}
  <nav>{jobs.map(j => <button key={j.job_id} onClick={() => onJobSelected(j.job_id)}>{j.pipeline ?? "Historical"} · {j.state} · {j.job_id}</button>)}</nav>
  {selected && <article><h3>{selected.pipeline} / {selected.adapter}</h3><p>{selected.objective}</p>
    <p>{selected.state} · attempt {selected.attempt ?? 0}</p>{selected.error && <p role="alert">{selected.error}</p>}
    <button disabled={!terminal && selected.cancel_requested} onClick={() => { if (terminal && !window.confirm("Delete this analysis and its artifacts?")) return; void (terminal ? api.deleteAnalysisJob(selected.job_id).then(() => onJobDeleted(selected.job_id)) : api.cancelAnalysisJob(selected.job_id).then(next => onJobsUpdated(jobs.map(j => j.job_id === next.job_id ? next : j)))).catch(e => setError(String(e))); }}>{terminal ? "Delete" : "Cancel"}</button>
    <p>Source: {selected.source_trace_ids.join(", ") || selected.source_trace_id || selected.source_conversation_id}</p>
    {(selected.artifacts ?? []).map(a => <section key={a.id}><h4>{a.type} · {a.validation_status}</h4>
      <ArtifactPayload value={a.payload} onError={setError} /><details><summary>Provenance / evaluation</summary><pre>{JSON.stringify(a.provenance, null, 2)}</pre></details>
    </section>)}
    <details><summary>Evidence</summary><pre>{JSON.stringify(selected.evidence_pack, null, 2)}</pre></details>
  </article>}
 </section>;
}

// Candidate file controls retain the former library's portable output behavior.
function ArtifactPayload({value,onError}:{value:unknown;onError:(message:string)=>void}) {
 const files=value && typeof value==="object" && "files" in value ? value.files : undefined;
 if(!files || typeof files!=="object" || Array.isArray(files)) return <pre>{JSON.stringify(value,null,2)}</pre>;
 return <div>{Object.entries(files).filter((entry):entry is [string,string]=>typeof entry[1]==="string").map(([path,text])=><section key={path}>
  <h5>{path}</h5><pre>{text}</pre>
  <button onClick={()=>{void navigator.clipboard.writeText(text).catch(()=>onError("Clipboard unavailable"));}}>Copy</button>
  <button onClick={()=>{const url=URL.createObjectURL(new Blob([text],{type:"text/plain;charset=utf-8"}));const link=document.createElement("a");link.href=url;link.download=path.split("/").at(-1)||"artifact.txt";link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}}>Download</button>
 </section>)}</div>;
}
