import { useEffect, useState } from "react";
import { api } from "./api";
import type { AgentSummary, EvolutionJob } from "./types";

export function EvolutionWorkspace({ locale, jobs, initialJobID, onAnalyze, onJobSelected, onJobDeleted, onJobsUpdated, refreshVersion }: {
 locale: "zh" | "en"; jobs: EvolutionJob[]; agents: AgentSummary[]; initialJobID?: string; analysisOnly?: boolean;
 onAnalyze: () => void; onJobSelected: (id: string) => void; onJobDeleted: (id: string) => void;
 onJobsUpdated: (jobs: EvolutionJob[]) => void; refreshVersion: string;
}) {
 const [error, setError] = useState("");
 const selected = jobs.find(j => j.job_id === initialJobID) ?? jobs[0];
 useEffect(() => {
   const controller = new AbortController();
   const refresh = () => { void api.evolutionJobs(controller.signal).then(onJobsUpdated).catch(e => { if (!controller.signal.aborted) setError(String(e)); }); };
   refresh(); const timer = window.setInterval(refresh, 2000);
   return () => { controller.abort(); clearInterval(timer); };
 }, [refreshVersion, onJobsUpdated]);
 const terminal = selected && ["completed", "failed", "cancelled"].includes(selected.state);
 return <section className="workspace-panel">
  <h2>{locale === "zh" ? "分析任务与产物" : "Analysis jobs and artifacts"}</h2>
  <button onClick={onAnalyze}>{locale === "zh" ? "创建分析" : "New analysis"}</button>
  {error && <p role="alert">{error}</p>}
  <nav>{jobs.map(j => <button key={j.job_id} onClick={() => onJobSelected(j.job_id)}>{j.pipeline ?? "Historical"} · {j.state} · {j.job_id}</button>)}</nav>
  {selected && <article><h3>{selected.pipeline} / {selected.adapter}</h3><p>{selected.objective}</p>
    <p>{selected.state} · attempt {selected.attempt ?? 0}</p>{selected.error && <p role="alert">{selected.error}</p>}
    <button onClick={() => { void (terminal ? api.deleteEvolutionJob(selected.job_id).then(() => onJobDeleted(selected.job_id)) : api.cancelEvolutionJob(selected.job_id)).catch(e => setError(String(e))); }}>{terminal ? "Delete" : "Cancel"}</button>
    <p>Source: {selected.source_trace_ids.join(", ") || selected.source_trace_id}</p>
    {(selected.artifacts ?? []).map(a => <section key={a.id}><h4>{a.type} · {a.validation_status}</h4>
      <pre>{JSON.stringify(a.payload, null, 2)}</pre><details><summary>Provenance / evaluation</summary><pre>{JSON.stringify(a.provenance, null, 2)}</pre></details>
    </section>)}
    <details><summary>Evidence</summary><pre>{JSON.stringify(selected.evidence_pack, null, 2)}</pre></details>
  </article>}
 </section>;
}
