import { useEffect, useState } from "react";
import { api } from "./api";
import type { AgentSummary, AnalysisJob, HumanReview } from "./types";

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
  <button className="primary-button compact" onClick={onAnalyze}>{locale === "zh" ? "创建分析" : "New analysis"}</button>
  {error && <p role="alert">{error}</p>}
  <nav>{jobs.map(j => <button className="text-button" key={j.job_id} onClick={() => onJobSelected(j.job_id)}>{j.pipeline ?? "Historical"} · {j.state} · {j.job_id}</button>)}</nav>
  {selected && <article><h3>{selected.pipeline} / {selected.adapter}</h3><p>{selected.objective}</p>
    <p>{selected.state} · attempt {selected.attempt ?? 0}</p>{selected.error && <p role="alert">{selected.error}</p>}
    <button className="text-button" disabled={!!selected.reviews?.length || (!terminal && selected.cancel_requested)} onClick={() => { if (terminal && !window.confirm("Delete this analysis and its artifacts?")) return; void (terminal ? api.deleteAnalysisJob(selected.job_id).then(() => onJobDeleted(selected.job_id)) : api.cancelAnalysisJob(selected.job_id).then(next => onJobsUpdated(jobs.map(j => j.job_id === next.job_id ? next : j)))).catch(e => setError(String(e))); }}>{terminal ? "Delete" : "Cancel"}</button>
    <p>Source: {selected.source_trace_ids.join(", ") || selected.source_trace_id || selected.source_conversation_id}</p>
    {(selected.artifacts ?? []).map(a => <section key={a.id}><h4>{a.type} · {a.validation_status}</h4>
      <Compare value={a.provenance} />
      <Review key={a.id} jobID={selected.job_id} artifactID={a.id} ready={a.validation_status === "review_required"} review={selected.reviews?.find(r => r.artifact_id === a.id)} onSaved={job => onJobsUpdated([job])} />
      <ArtifactPayload value={a.payload} onError={setError} /><details><summary>Provenance / evaluation</summary><pre>{JSON.stringify(a.provenance, null, 2)}</pre></details>
    </section>)}
    {selected.state === "failed" && selected.checkpoint != null && <details><summary>Failed execution evidence</summary><pre>{JSON.stringify(selected.checkpoint, null, 2)}</pre></details>}
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
  <button className="text-button" onClick={()=>{void navigator.clipboard.writeText(text).catch(()=>onError("Clipboard unavailable"));}}>Copy</button>
  <button className="text-button" onClick={()=>{const url=URL.createObjectURL(new Blob([text],{type:"text/plain;charset=utf-8"}));const link=document.createElement("a");link.href=url;link.download=path.split("/").at(-1)||"artifact.txt";link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}}>Download</button>
 </section>)}</div>;
}

function Compare({ value }: { value: unknown }) {
 if (!value || typeof value !== "object" || !("compare" in value) || !Array.isArray(value.compare)) return null;
 return <section aria-label="Baseline / Candidate Compare"><h4>Baseline / Candidate Compare</h4>
  <p>每个 Case 各 1 次；不能证明普遍提升。Missing cost/token data is unknown.</p>
  {value.compare.map((pair, i) => <section key={i}><h5>{pair.case_id} · {pair.held_out ? "held-out" : "optimization Case reused"}</h5>
   <div className="comparison-table"><table><thead><tr><th scope="col">Arm</th><th scope="col">Verifier pass / trials</th><th scope="col">Nonzero command exits / tool calls</th><th scope="col">Input / output tokens</th><th scope="col">Seconds</th><th scope="col">USD</th></tr></thead>
    <tbody>{(["baseline", "candidate"] as const).map(arm => { const trial = pair[arm]; return <tr key={arm}><th scope="row">{arm}</th><td>{trial?.valid && [0, 1].includes(trial.rewards?.reward) ? `${trial.rewards.reward} / 1` : "unknown"}</td><td>{trial?.nonzero_command_exits ?? "?"} / {trial?.tool_calls ?? "?"}</td><td>{trial?.input_tokens ?? "?"} / {trial?.output_tokens ?? "?"}</td><td>{trial?.seconds?.toFixed(2) ?? "?"}</td><td>{trial?.cost_usd ?? "?"}</td></tr>; })}</tbody>
   </table></div>
   <p>Raw Harbor artifacts: {pair.baseline?.artifact_dir ?? "unknown"} / {pair.candidate?.artifact_dir ?? "unknown"}</p>
  </section>)}
 </section>;
}

function Review({ jobID, artifactID, ready, review, onSaved }: { jobID: string; artifactID: string; ready: boolean; review?: HumanReview; onSaved: (job: AnalysisJob) => void }) {
 const [reason, setReason] = useState("");
 const [decision, setDecision] = useState<"adopt" | "reject">("reject");
 const [busy, setBusy] = useState(false);
 const [error, setError] = useState("");
 if (review) return <section aria-label="Human Review"><h4>Human Review · {review.decision}</h4><p>{review.reason}</p><p>{review.reviewer} · {review.created_at}</p></section>;
 if (!ready) return <p>Unverified: independent Harbor comparison required before review.</p>;
 return <form aria-label="Human Review" onSubmit={async event => {
  event.preventDefault(); setBusy(true); setError("");
  try { onSaved(await api.reviewAnalysisJob(jobID, artifactID, decision, reason)); }
  catch (cause) { setError(String(cause)); }
  finally { setBusy(false); }
 }}><h4>Human Review</h4><p>Record a decision after reviewing verifier and trajectory. Adoption does not install files.</p>
  <label>Decision<select value={decision} disabled={busy} onChange={e => setDecision(e.target.value as "adopt" | "reject")}><option value="reject">Reject</option><option value="adopt">Adopt</option></select></label>
  <label>Reason<textarea required maxLength={4000} value={reason} disabled={busy} onChange={e => setReason(e.target.value)} /></label>
  <button className="primary-button compact" disabled={busy || !reason.trim()}>Record decision</button>{error && <p role="alert">{error}</p>}
 </form>;
}
