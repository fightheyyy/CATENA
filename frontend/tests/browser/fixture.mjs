import http from "node:http";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../dist/", import.meta.url));
const now = new Date().toISOString();
export const fakeToken = "catena_agent_browser_fixture_only";
const credential = { id: "key-fixture", masked_token: "catena_***fixture", name: "Fixture", created_at: now };
export const agent = { agent_id: "agent-fixture", display_name: "浏览器测试 Agent", registered: true, connected: true,
  credential, runtime_kind: "codex", identity_source: "registered", conversation_count: 0,
  trace_count: 2, span_count: 2, error_count: 0, last_seen_at: now };
export const traces = ["first", "second"].map((name, index) => ({ trace_id: `trace-${name}`, agent_id: agent.agent_id,
  session_id: "session-fixture", root_name: "agent.turn", input_preview: `请求 ${index + 1}：核对工具结果`,
  service_name: agent.display_name, span_count: 1, error_count: 0, duration_ms: 120,
  start_time: now, end_time: now }));
const baseJob = { schema: "catena.analysis_job.v2", job_id: "job-fixture", pipeline: "rsi", adapter: "harness2", source_kind: "agent_trace_set",
 source_agent_id: agent.agent_id, source_trace_ids: traces.map(t=>t.trace_id), state: "running", stages: [], artifacts: [], created_at: now, updated_at: now };
const completed = { ...baseJob, state: "completed", artifacts: [{id:"artifact-fixture",job_id:"job-fixture",type:"rsi_candidate",schema_version:"catena.artifact.v1",
 evidence_refs:baseJob.source_trace_ids,payload:{files:{"AGENTS.md":"Inspect evidence before acting."}},provenance:{evaluation:[{fixture:true,valid:false,error:"No real verifier executed"}]},validation_status:"unverified",created_at:now}] };

export async function fixtureServer(overrides = {}) {
  const state = { requests: [], agents: [structuredClone(agent)], jobComplete: false, jobs: true,
    jobCancelRequested: false, jobCancelled: false, failCancel: false,
    connected: false, failAgents: false, failConnection: false, failTrace: false, emptyTraces: false,
    holdAgents: false, heldClosed: false, failTraceList: false, failJobs: false,
    traces: structuredClone(traces), completedJob: structuredClone(completed),
    memoryReady: false, failMemories: false, failSearch: false, memories: [], memorySearch: null, conversations: [],
    ...overrides };
  const currentJob = () => state.jobCancelled ? { ...baseJob, state: "cancelled", cancel_requested: true }
    : state.jobComplete ? state.completedJob : { ...baseJob, cancel_requested: state.jobCancelRequested };
  const json = (res, value, status = 200) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(value)); };
  const server = http.createServer(async (req, res) => {
    const pathname = new URL(req.url, "http://localhost").pathname;
    if (pathname.startsWith("/v1/")) {
      state.requests.push({ path: pathname, method: req.method });
      if (pathname === "/v1/auth/session") return json(res, { authenticated: true, mode: "local", user: null });
      if (pathname === "/v1/agents" && req.method === "POST") {
        let raw = "";
        for await (const chunk of req) raw += chunk;
        const created = { ...agent, agent_id: "new-agent", display_name: JSON.parse(raw).display_name,
          connected: false, trace_count: 0, span_count: 0, last_seen_at: "" };
        state.agents.unshift(created);
        return json(res, { agent: created, api_token: credential, token: fakeToken }, 201);
      }
      if (pathname === "/v1/agents") {
        if (state.holdAgents) { res.on("close", () => { state.heldClosed = true; }); return; }
        if (state.failAgents) return json(res, { detail: "Agent query temporarily failed" }, 503);
        return json(res, { available: true, agents: state.agents.map((item) => item.agent_id === "new-agent" ? { ...item, connected: state.connected } : item) });
      }
      if (/^\/v1\/agents\/[^/]+\/traces$/.test(pathname)) {
        const query = new URL(req.url, "http://localhost").searchParams;
        const agentID = pathname.split('/')[3];
        return json(res, { available: true, agent_id: agentID,
          window_start: query.get("from") ?? now, window_end: query.get("to") ?? now,
          traces: state.emptyTraces ? [] : state.traces.filter((trace) => trace.agent_id === agentID) });
      }
      if (/^\/v1\/agents\/[^/]+$/.test(pathname)) {
        if (state.failConnection) return json(res, { detail: "Connection check failed" }, 503);
        return json(res, { agent: state.agents.find((item) => item.agent_id === pathname.split("/").at(-1)), connected: state.connected, credential });
      }
      if (pathname.endsWith("/reveal")) return json(res, { token: fakeToken });
      if (pathname === "/v1/traces") return state.failTraceList ? json(res, { detail: "Trace query temporarily failed" }, 503) : json(res, { available: true, traces: state.emptyTraces ? [] : state.traces });
      if (pathname.startsWith("/v1/traces/")) {
        if (state.failTrace) return json(res, { detail: "Requested Trace was not found" }, 404);
        const id = pathname.split("/").at(-1);
        const summary = state.traces.find((trace) => trace.trace_id === id) ?? { ...traces[1], trace_id: id };
        return json(res, { summary, spans: [{ trace_id: id, span_id: `span-${id}`, parent_span_id: "", name: "agent.turn",
          service_name: agent.display_name, kind: 1, status_code: 1, resource_attributes: {}, start_time: now, end_time: now, duration_ms: 120,
          input: summary.input_preview, output: `完成 ${id}`,
          attributes: { "catena.node.kind": "turn", "catena.state": "ok" } }] });
      }
      if (pathname === "/v1/evaluation-cases") return json(res, {cases:[{id:"fixture-case"}]});
      if (pathname === "/v1/analysis-jobs") return state.failJobs ? json(res, { detail: "Output query temporarily failed" }, 503) : json(res, { analysis_jobs: state.jobs ? [currentJob()] : [] });
      if (pathname === "/v1/analysis-jobs/job-fixture") return json(res, currentJob());
      if (pathname === "/v1/analysis-jobs/job-fixture/cancel" && req.method === "POST") {
        if (state.failCancel) return json(res, { detail: "Cancellation temporarily unavailable" }, 503);
        state.jobCancelRequested = true;
        return json(res, currentJob());
      }
      if (pathname === "/v1/me/llm-config") return json(res, { provider: "", base_url: "", model: "", api_key_configured: false, configured: false });
      if (pathname === "/v1/system/status") return json(res, { status: "ok", memory_store: state.memoryReady ? "available" : "unavailable", trace_store: "available" });
      if (pathname === "/v1/memories") return state.failMemories ? json(res, { detail: "Memory query temporarily failed" }, 503) : json(res, { memories: state.memories, total: state.memories.length });
      if (pathname === "/v1/memories/tasks") return json(res, { tasks: [] });
      if (pathname === "/v1/memories/search") return state.failSearch ? json(res, { detail: "Memory search temporarily failed" }, 503) : json(res, state.memorySearch ?? { success: true, query: "", facts: [], conversations: [], topics: [] });
      if (/^\/v1\/memories\/facts\/\d+\/graph$/.test(pathname)) {
        const factID = Number(pathname.split('/')[4]);
        return json(res, { fact_id: factID, content: state.memories.find((memory) => memory.id === String(factID))?.content ?? "Fixture memory",
          entities: [{ name: "Catena", type: "project" }, { name: "阅读体验", type: "topic" }],
          relations: [{ source: "Catena", target: "阅读体验", type: "focuses_on", confidence: .95 }], total_entities: 2, total_relations: 1 });
      }
      if (pathname === "/v1/conversations") return json(res, { conversations: state.conversations });
      // Unrelated legacy endpoints deliberately fail, exposing accidental global fan-out.
      return json(res, { detail: "Unrelated fixture endpoint is unavailable" }, 503);
    }
    const candidate = path.resolve(root, `.${decodeURIComponent(pathname)}`);
    if (candidate !== path.resolve(root) && !candidate.startsWith(path.resolve(root) + path.sep)) { res.writeHead(403); res.end(); return; }
    try {
      const file = path.extname(candidate) ? candidate : path.join(root, "index.html");
      const content = await fs.readFile(file);
      const mime = { ".html": "text/html", ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml" };
      res.writeHead(200, { "Content-Type": `${mime[path.extname(file)] || "application/octet-stream"}; charset=utf-8` });
      res.end(content);
    } catch { res.writeHead(404); res.end(); }
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  return { state, origin: `http://127.0.0.1:${server.address().port}`,
    close: async () => { server.closeAllConnections(); await new Promise((resolve) => server.close(resolve)); } };
}
