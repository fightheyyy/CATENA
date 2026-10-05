import type {
  ApiToken,
  AgentTraceWindow,
  RegisteredAgent,
  RegisteredAgentConnection,
  ConversationDocument,
  ConversationSummary,
  EvolutionJob,
  EvolutionModelSettings,
  MemoryIngestReceipt,
  MemoryTaskStatus,
  MemoryTaskRecord,
  MemoryFactGraph,
  MemoryList,
  MemoryRecord,
  MemoryRecallBundle,
  Session,
  TraceDetail,
  TraceSummary,
  WorkspaceData,
} from "./types";
import { normalizeEvolutionJob, normalizeEvolutionJobs } from "./evolution";
import { routeResources, workspaceResources } from "./workspace";
import type { Route } from "./navigation";
import type { Experiment, ExperimentCase } from "./experiments";

type Problem = {
  detail?: string;
  error?: string;
  title?: string;
};

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const read = !init?.method || init.method === "GET";
  const timeout = read ? AbortSignal.timeout(15000) : undefined;
  const response = await fetch(path, {
    credentials: "same-origin",
    ...init,
    signal: init?.signal && timeout ? AbortSignal.any([init.signal, timeout]) : init?.signal ?? timeout,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });

  if (!response.ok) {
    const problem = (await response.json().catch(() => ({}))) as Problem;
    throw new ApiError(
      response.status,
      problem.detail ?? problem.error ?? problem.title ?? `Request failed (${response.status})`,
    );
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export const api = {
  experimentCases: (signal?: AbortSignal) => request<{ cases: ExperimentCase[] }>("/v1/experiment-cases", { signal }),
  experiments: (signal?: AbortSignal) => request<{ experiments: Experiment[] }>("/v1/experiments", { signal }),
  createExperiment: (caseID: string, requestID: string, includeMemory = false, targetAgent = "sdk") => request<Experiment>("/v1/experiments", { method: "POST", body: JSON.stringify({ case_id: caseID, request_id: requestID, include_memory: includeMemory, target_agent: targetAgent }) }),
  experimentEvidence: (experimentID: string, index: number) => request<{ answer: string; calls: { command: string; exit_code: number | null; stdout: string; stderr: string }[] }>(`/v1/experiments/${encodeURIComponent(experimentID)}/trials/${index}`),
  session: () => request<Session>("/v1/auth/session"),
  logout: () => request<void>("/v1/auth/logout", { method: "POST" }),
  workspace: async (route: Route, signal: AbortSignal): Promise<Partial<WorkspaceData>> => {
    const resources = routeResources[route];
    const reads = resources.map(async (resource) => {
      const result = await request<Record<string, unknown>>(workspaceResources[resource], { signal });
      if (resource === "system") return { system: result };
      if (resource === "evolutionJobs") return { evolutionJobs: normalizeEvolutionJobs(result.evolution_jobs) };
      if (resource === "agents") return { agents: result.agents, agentAvailable: result.available };
      if (resource === "traces") return { traces: result.traces, traceAvailable: result.available };
      return { [resource]: result[resource] };
    });
    if (route === "home") {
      const parts = await Promise.allSettled(reads);
      signal.throwIfAborted();
      const patch: Partial<WorkspaceData> = { overviewErrors: {} };
      parts.forEach((part, index) => {
        if (part.status === "fulfilled") Object.assign(patch, part.value);
        else {
          const resource = resources[index] as "agents" | "traces" | "evolutionJobs";
          patch.overviewErrors![resource] = part.reason instanceof Error ? part.reason.message : "Request failed";
        }
      });
      return patch;
    }
    const parts = await Promise.all(reads);
    return Object.assign({}, ...parts) as Partial<WorkspaceData>;
  },
  trace: (traceID: string, signal?: AbortSignal) => request<TraceDetail>(`/v1/traces/${encodeURIComponent(traceID)}`, { signal }),
  rememberTrace: (traceID: string) => request<MemoryIngestReceipt>(`/v1/traces/${encodeURIComponent(traceID)}/memories`, {method:"POST"}),
  traces: (limit = 100, signal?: AbortSignal) => request<{ available: boolean; traces: TraceSummary[] }>(`/v1/traces?limit=${limit}`, { signal }),
  agentTraces: (agentID: string, windowStart: string, windowEnd: string, limit = 100, signal?: AbortSignal) => {
    const query = new URLSearchParams({
      from: windowStart,
      to: windowEnd,
      limit: String(limit),
    });
    return request<AgentTraceWindow>(`/v1/agents/${encodeURIComponent(agentID)}/traces?${query}`, { signal });
  },
  createAgent: (displayName: string) =>
    request<{ agent: RegisteredAgent; api_token: ApiToken; token: string }>("/v1/agents", {
      method: "POST",
      body: JSON.stringify({ display_name: displayName }),
    }),
  registeredAgent: (agentID: string, signal?: AbortSignal) =>
    request<RegisteredAgentConnection>(`/v1/agents/${encodeURIComponent(agentID)}`, { signal }),
  createAgentConnectionKey: (agentID: string) =>
    request<{ api_token: ApiToken; token: string }>(
      `/v1/agents/${encodeURIComponent(agentID)}/api-key`,
      { method: "POST" },
    ),
  evolutionJobs: async (signal?: AbortSignal) => {
    const result = await request<{ evolution_jobs: EvolutionJob[] }>(workspaceResources.evolutionJobs, { signal });
    return normalizeEvolutionJobs(result.evolution_jobs);
  },
  evolutionJob: async (jobID: string, signal?: AbortSignal) => normalizeEvolutionJob(
    await request<unknown>(`/v1/evolution-jobs/${encodeURIComponent(jobID)}`, { signal }),
  ),
  deleteEvolutionJob: (jobID: string) => request<void>(
    `/v1/evolution-jobs/${encodeURIComponent(jobID)}`,
    { method: "DELETE" },
  ),
  startAgentEvolutionJob: async (
    agentID: string,
    input: { window_start: string; window_end: string; objective?: string; output_language: "zh-CN" | "en" },
    idempotencyKey: string,
  ) => normalizeEvolutionJob(
    await request<unknown>(`/v1/agents/${encodeURIComponent(agentID)}/evolution-jobs`, {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify(input),
    }),
  ),
  rememberConversation: (agentID: string, conversationID: string) => {
    const query = new URLSearchParams({ agent_id: agentID });
    return request<MemoryIngestReceipt>(`/v1/conversations/${encodeURIComponent(conversationID)}/memories?${query}`, {
      method: "POST",
    });
  },
  memoryTask: (taskID: string) =>
    request<MemoryTaskStatus>(`/v1/memories/tasks/${encodeURIComponent(taskID)}`, {
      signal: AbortSignal.timeout(8000),
    }),
  memoryTasks: (limit = 20) =>
    request<{ tasks: MemoryTaskRecord[] }>(`/v1/memories/tasks?limit=${limit}`),
  memories: (limit = 24) => request<MemoryList>(`/v1/memories?limit=${limit}`),
  memoryStatus: () => request<{status: string; backend: string; capabilities: string[]}>("/v1/memories/status"),
  createMemory: (title: string, content: string) => request<MemoryRecord>("/v1/memories", {
    method: "POST", body: JSON.stringify({title, content}),
  }),
  memoryGraph: (factID: string | number) =>
    request<MemoryFactGraph>(`/v1/memories/facts/${encodeURIComponent(String(factID))}/graph`),
  searchMemories: (query: string, topK = 8) =>
    request<MemoryRecallBundle>("/v1/memories/search", {
      method: "POST",
      body: JSON.stringify({ query, top_k: topK }),
    }),
  conversations: (limit = 100) =>
    request<{ schema: string; conversations: ConversationSummary[] }>(`/v1/conversations?limit=${limit}`),
  conversation: (agentID: string, conversationID: string) => {
    const query = new URLSearchParams({ agent_id: agentID });
    return request<ConversationDocument>(
      `/v1/conversations/${encodeURIComponent(conversationID)}?${query}`,
    );
  },
  apiTokens: () => request<{ api_tokens: ApiToken[] }>("/v1/me/api-tokens"),
  createApiToken: (name: string) =>
    request<{ api_token: ApiToken; token: string }>("/v1/me/api-tokens", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  revealApiToken: (id: string) =>
    request<{ token: string }>(`/v1/me/api-tokens/${encodeURIComponent(id)}/reveal`, {
      method: "POST",
    }),
  deleteApiToken: (id: string) =>
    request<void>(`/v1/me/api-tokens/${encodeURIComponent(id)}`, { method: "DELETE" }),
  llmConfig: () => request<EvolutionModelSettings>("/v1/me/llm-config"),
  saveLLMConfig: (input: { provider: string; base_url: string; model: string; api_key: string }) =>
    request<EvolutionModelSettings>("/v1/me/llm-config", {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  deleteLLMConfig: () => request<void>("/v1/me/llm-config", { method: "DELETE" }),
};
