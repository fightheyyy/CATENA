import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { Route } from "./navigation";
import type { WorkspaceData } from "./types";

type Definition = { agent_id: string; version: number; runtime_version: string };
type AgentSession = { session_id: string; agent_id: string; state: string; sandbox_binding: string | null; checkpoint_ref: string | null };
type Run = { run_id: string; session_id: string; state: string; input: string; result: string; error: string; trace_id: string; result_ref: string | null; attempt: number; input_tokens: number; output_tokens: number; model_requests: number };
type Box = { sandbox_id: string; provider_sandbox_id: string | null; session_id: string; state: string; cpu: string; memory: string; generation: number; error: string };
type Resources = { sandboxes: Box[]; quota: { max_sandboxes: number; max_running: number; max_queued: number; reserved_sandboxes: number; running: number; queued: number; max_model_requests: number; max_output_tokens: number; global_capacity: number } | null };
const active = (run: Run) => ["QUEUED", "RUNNING"].includes(run.state);

export function ManagedWorkspace({ route, workspace }: { route: Route; workspace: WorkspaceData }) {
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [selected, setSelected] = useState("");
  const [runs, setRuns] = useState<Run[]>([]);
  const [run, setRun] = useState<Run | null>(null);
  const [resources, setResources] = useState<Resources | null>(null);
  const [agent, setAgent] = useState("");
  const [instructions, setInstructions] = useState("");
  const [input, setInput] = useState("");
  const [events, setEvents] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const key = useRef(crypto.randomUUID());
  const detailID = new URLSearchParams(window.location.search).get("run") ?? "";
  const runID = route === "runDetail" ? detailID : run?.run_id;
  const refresh = useCallback(async () => {
    if (route === "resources" || route === "sandboxMonitor") {
      setResources(await api.managedRead<Resources>("/v1/me/resources"));
    } else if (route === "runDetail") {
      if (!detailID) throw new Error("Select a run from Agent Sessions.");
      setRun(await api.managedRead<Run>(`/v1/agent-runs/${encodeURIComponent(detailID)}`));
    } else {
      const [s, d] = await Promise.all([
        api.managedRead<{ sessions: AgentSession[] }>("/v1/agent-sessions"),
        api.managedRead<{ agents: Definition[] }>("/v1/managed-agents"),
      ]);
      setSessions(s.sessions); setDefinitions(d.agents);
      if (selected) {
        const detail = await api.managedRead<{ runs: Run[] }>(`/v1/agent-sessions/${encodeURIComponent(selected)}`);
        setRuns(detail.runs);
        setRun((previous) => detail.runs.find((r) => r.run_id === previous?.run_id) ?? detail.runs[0] ?? null);
      }
    }
  }, [route, selected, detailID]);
  useEffect(() => {
    let disposed = false;
    const load = () => { void refresh().then(() => { if (!disposed) setError(""); }).catch((e: unknown) => { if (!disposed) setError(String(e)); }); };
    load(); const timer = window.setInterval(load, 5000);
    return () => { disposed = true; window.clearInterval(timer); };
  }, [refresh]);
  useEffect(() => {
    setEvents([]);
    if (!runID) return;
    const source = new EventSource(`/v1/agent-runs/${encodeURIComponent(runID)}/events`);
    const receive = (event: MessageEvent<string>) => setEvents((previous) => [...previous.slice(-499), event.data]);
    for (const name of ["runtime", "queued", "claimed", "finished", "recovery"]) source.addEventListener(name, receive as EventListener);
    return () => source.close();
  }, [runID]);
  async function perform(action: () => Promise<unknown>) {
    setBusy(true); setError("");
    try { await action(); await refresh(); } catch (e) { setError(String(e)); } finally { setBusy(false); }
  }
  const selectedSession = sessions.find((s) => s.session_id === selected);
  const runDetail = run && <section className="panel managed-panel" aria-label="Run detail">
    <h2>Run · {run.state}</h2><code>{run.run_id}</code>
    <p>Attempt {run.attempt} · Model requests {run.model_requests} · Observed tokens {run.input_tokens} in / {run.output_tokens} out</p>
    <pre>{run.input}</pre>{run.result && <pre>{run.result}</pre>}
    {run.error && <p role="alert">{run.error}</p>}
    <div className="managed-actions">
      {active(run) && <button disabled={busy} onClick={() => void perform(() => api.managedWrite(`/v1/agent-runs/${run.run_id}/cancel`))}>Cancel run</button>}
      <a href={`/agent-runs?run=${encodeURIComponent(run.run_id)}`}>Run detail</a>
      <a href={`/traces?trace=${encodeURIComponent(run.trace_id)}`}>Trace / investigate</a>
      {run.result_ref && <a href={`/v1/agent-runs/${run.run_id}/artifact`}>Download workspace checkpoint</a>}
    </div>
    <h3>Execution events</h3><pre className="managed-events" aria-live="polite">{events.join("\n") || "Waiting for events…"}</pre>
  </section>;
  return <div className="managed-workspace">
    <header><h1>{route === "sandboxMonitor" ? "Sandbox Monitor" : route === "resources" ? "Tenant Resources" : route === "runDetail" ? "Run Detail" : "Agent Sessions"}</h1>
      <p>Managed Codex · trusted local Docker execution</p></header>
    {error && <p role="alert">{error}</p>}
    {(route === "resources" || route === "sandboxMonitor") && resources && <>
      <section className="panel managed-panel"><h2>Resource quota</h2>
        {resources.quota ? <dl className="managed-quota">
          <dt>Sandboxes reserved / tenant limit</dt><dd>{resources.quota.reserved_sandboxes} / {resources.quota.max_sandboxes}</dd>
          <dt>Concurrent runs</dt><dd>{resources.quota.running} / {resources.quota.max_running}</dd>
          <dt>Queued runs</dt><dd>{resources.quota.queued} / {resources.quota.max_queued}</dd>
          <dt>Global sandbox capacity</dt><dd>{resources.quota.global_capacity}</dd>
          <dt>Model budget per run</dt><dd>{resources.quota.max_model_requests} requests, at most {resources.quota.max_output_tokens} output tokens each</dd>
        </dl> : <p>Create a managed Agent definition to initialize your quota.</p>}
        <p>CPU and memory limits are passed to Docker. Live utilization metrics are unavailable.</p>
      </section>
      <section className="panel managed-panel"><h2>Sandboxes</h2>{!resources.sandboxes.length && <p>No sandboxes.</p>}
        {resources.sandboxes.map((box) => <article key={box.sandbox_id}><h3>{box.state}</h3><code>{box.sandbox_id}</code><p>Session {box.session_id}</p><p>{box.cpu} CPU / {box.memory} · Generation {box.generation}</p>{box.error && <p>{box.error}</p>}</article>)}
      </section>
    </>}
    {route === "sessions" && <>
      <section className="panel managed-panel"><h2>Managed Agent definition</h2>
        <p>Register an Agent in Connections and configure your model first. Definition version 1 is immutable.</p>
        <form onSubmit={(e) => { e.preventDefault(); void perform(() => api.managedWrite("/v1/managed-agents", { agent_id: agent, instructions })); }}>
          <label>Registered Agent<select required value={agent} onChange={(e) => setAgent(e.target.value)}><option value="">Select Agent</option>{workspace.agents.map((a) => <option key={a.agent_id} value={a.agent_id}>{a.display_name}</option>)}</select></label>
          <label>Instructions<textarea maxLength={32768} value={instructions} onChange={(e) => setInstructions(e.target.value)} /></label>
          <button disabled={busy || !agent}>Create definition</button>
        </form>
        <h3>Start a session</h3>{definitions.map((d) => <button key={d.agent_id} disabled={busy} onClick={() => void perform(async () => { const s = await api.managedWrite<AgentSession>("/v1/agent-sessions", { agent_id: d.agent_id }); setSelected(s.session_id); })}>{d.agent_id} · Codex {d.runtime_version}</button>)}
      </section>
      <section className="panel managed-panel"><h2>Sessions</h2>{!sessions.length && <p>No sessions yet.</p>}
        {sessions.map((s) => <button key={s.session_id} aria-pressed={s.session_id === selected} onClick={() => { setSelected(s.session_id); setRun(null); }}>{s.session_id} · {s.state}</button>)}
      </section>
      {selectedSession && <section className="panel managed-panel"><h2>{selectedSession.state}</h2><code>{selected}</code><p>Sandbox {selectedSession.sandbox_binding ?? "allocated when a run starts"}</p><p>Checkpoint {selectedSession.checkpoint_ref ?? "none"}</p>
        <div className="managed-actions">{["pause", "resume"].map((action) => <button key={action} disabled={busy} onClick={() => void perform(() => api.managedWrite(`/v1/agent-sessions/${selected}/${action}`))}>{action}</button>)}<button disabled={busy} onClick={() => void perform(() => api.managedWrite(`/v1/agent-sessions/${selected}`, undefined, undefined, "DELETE"))}>Close session</button></div>
        <form onSubmit={(e) => { e.preventDefault(); void perform(async () => { const created = await api.managedWrite<Run>(`/v1/agent-sessions/${selected}/runs`, { input }, key.current); setRun(created); setInput(""); key.current = crypto.randomUUID(); }); }}>
          <label>Task / follow-up<textarea required maxLength={65536} value={input} onChange={(e) => { setInput(e.target.value); key.current = crypto.randomUUID(); }} /></label><button disabled={busy || selectedSession.state !== "READY"}>Queue run</button>
        </form>
        {runs.map((r) => <button key={r.run_id} onClick={() => setRun(r)}>{r.state} · {r.run_id}</button>)}
      </section>}
    </>}
    {(route === "runDetail" || route === "sessions") && runDetail}
  </div>;
}
