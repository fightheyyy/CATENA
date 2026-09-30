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
const baseJob = { schema: "catena.evolution_job.v1", job_id: "job-fixture", source_kind: "agent_trace_set",
  source_agent_id: agent.agent_id, source_trace_ids: traces.map((trace) => trace.trace_id),
  state: "running", current_stage: "inspector", stages: [], candidates: [], created_at: now, updated_at: now };
const completed = { ...baseJob, state: "completed", current_stage: "complete", updated_at: new Date(Date.now() + 1000).toISOString(),
  candidates: [{ candidate_id: "asset-fixture", kind: "agent_md", title: "先核对再回答", summary: "本地浏览器验收专用候选",
    status: "draft/unverified", content: { path: "agent.md", markdown: "# 浏览器测试候选\n\n核对执行结果后再回答。" },
    source_trace_ids: traces.map((trace) => trace.trace_id) }],
  finding: { title: "核对执行结果", summary: "本地测试", severity: "low", evidence: [] },
  review: { verdict: "pass", summary: "仅验证候选可读性", scope: "proposal_only", candidate_status: "draft/unverified" } };

export async function fixtureServer(overrides = {}) {
  const state = { requests: [], agents: [structuredClone(agent)], jobComplete: false, jobs: true,
    connected: false, failAgents: false, failConnection: false, failTrace: false, emptyTraces: false,
    holdAgents: false, heldClosed: false, failTraceList: false, failJobs: false,
    traces: structuredClone(traces), completedJob: structuredClone(completed),
    memoryReady: false, failMemories: false, failSearch: false, memories: [], memorySearch: null, conversations: [],
    ...overrides };
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
      if (/^\/v1\/agents\/[^/]+\/traces$/.test(pathname)) return json(res, { available: true, traces: state.emptyTraces ? [] : state.traces.filter((trace) => trace.agent_id === pathname.split('/')[3]) });
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
      if (pathname === "/v1/evolution-jobs") return state.failJobs ? json(res, { detail: "Output query temporarily failed" }, 503) : json(res, { evolution_jobs: state.jobs ? [state.jobComplete ? state.completedJob : baseJob] : [] });
      if (pathname === "/v1/evolution-jobs/job-fixture") return json(res, state.jobComplete ? state.completedJob : baseJob);
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
