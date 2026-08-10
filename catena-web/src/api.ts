import type {
  ApiToken,
  AgentTraceWindow,
  AgentSummary,
  RegisteredAgent,
  RegisteredAgentConnection,
  ConversationDocument,
  ConversationSummary,
  EvolutionJob,
  EvolutionModelSettings,
  Issue,
  RegressionCase,
  Release,
  MemoryIngestReceipt,
  MemoryTaskStatus,
  MemoryTaskRecord,
  MemoryFactGraph,
  MemoryList,
  MemoryRecallBundle,
  Run,
  Runtime,
  Session,
  SystemStatus,
  TraceDetail,
  TraceSummary,
  WorkspaceData,
} from "./types";
import { normalizeEvolutionJob, normalizeEvolutionJobs } from "./evolution";

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
  const response = await fetch(path, {
    credentials: "same-origin",
    ...init,
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
  session: () => request<Session>("/v1/auth/session"),
  logout: () => request<void>("/v1/auth/logout", { method: "POST" }),
  workspace: async (): Promise<WorkspaceData> => {
    const [system, runtimes, runs, jobs, issues, cases, releases, traces, agents] = await Promise.all([
      request<SystemStatus>("/v1/system/status"),
      request<{ runtimes: Runtime[] }>("/v1/runtimes"),
      request<{ runs: Run[] }>("/v1/runs?limit=40"),
      request<{ evolution_jobs: EvolutionJob[] }>("/v1/evolution-jobs?limit=40"),
      request<{ issues: Issue[] }>("/v1/issues?limit=40"),
      request<{ cases: RegressionCase[] }>("/v1/cases?limit=40"),
      request<{ releases: Release[] }>("/v1/releases?limit=40"),
      request<{ available: boolean; traces: TraceSummary[] }>("/v1/traces?limit=100"),
      request<{ available: boolean; agents: AgentSummary[] }>("/v1/agents?limit=100"),
    ]);
    return {
      system,
      runtimes: runtimes.runtimes,
      runs: runs.runs,
      evolutionJobs: normalizeEvolutionJobs(jobs.evolution_jobs),
      issues: issues.issues,
      cases: cases.cases,
      releases: releases.releases,
      traceAvailable: traces.available,
      agentAvailable: agents.available,
      traces: traces.traces,
      agents: agents.agents,
    };
  },
  trace: (traceID: string) => request<TraceDetail>(`/v1/traces/${encodeURIComponent(traceID)}`),
  agentTraces: (agentID: string, windowStart: string, windowEnd: string, limit = 100) => {
    const query = new URLSearchParams({
      from: windowStart,
      to: windowEnd,
      limit: String(limit),
    });
    return request<AgentTraceWindow>(`/v1/agents/${encodeURIComponent(agentID)}/traces?${query}`);
  },
  createAgent: (displayName: string) =>
    request<{ agent: RegisteredAgent; api_token: ApiToken; token: string }>("/v1/agents", {
      method: "POST",
      body: JSON.stringify({ display_name: displayName }),
    }),
  registeredAgent: (agentID: string) =>
    request<RegisteredAgentConnection>(`/v1/agents/${encodeURIComponent(agentID)}`),
  createAgentConnectionKey: (agentID: string) =>
    request<{ api_token: ApiToken; token: string }>(
      `/v1/agents/${encodeURIComponent(agentID)}/api-key`,
      { method: "POST" },
    ),
  evolutionJob: async (jobID: string) => normalizeEvolutionJob(
    await request<unknown>(`/v1/evolution-jobs/${encodeURIComponent(jobID)}`),
  ),
  deleteEvolutionJob: (jobID: string) => request<void>(
    `/v1/evolution-jobs/${encodeURIComponent(jobID)}`,
    { method: "DELETE" },
  ),
  startAgentEvolutionJob: async (
    agentID: string,
    input: { window_start: string; window_end: string; objective?: string },
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
