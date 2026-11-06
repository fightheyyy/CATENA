export type User = {
  id: string;
  login: string;
  display_name: string;
  avatar_url?: string;
};

export type Session = {
  mode: "local" | "github";
  authenticated: boolean;
  login_url?: string;
  user: User | null;
};

export type SystemStatus = {
  status: string;
  auth_mode: string;
  edge_ingest: string;
  analysis_protocol: string;
  analysis_engine: string;
  trace_store: string;
};

export type AnalysisModelSettings = {
  provider: string;
  base_url: string;
  model: string;
  api_key_configured: boolean;
  configured: boolean;
  updated_at?: string;
};

export type Artifact = {
 id: string; job_id: string; type: string; schema_version: string; evidence_refs: string[]; payload: unknown; provenance: unknown; validation_status: string; created_at: string;
};

export type HumanReview = { artifact_id: string; decision: "adopt" | "reject"; reason: string; reviewer: string; created_at: string };

export type AnalysisJob = {
 reviews?: HumanReview[]; checkpoint?: unknown;
 schema?: string; job_id: string; analysis_job_id?: string; pipeline?: string; adapter?: string; artifacts: Artifact[];
 state: string; attempt?: number; next_attempt_at?: string; cancel_requested?: boolean; error?: string;
 source_kind?: string; source_trace_id?: string; source_trace_ids: string[]; source_agent_id?: string; source_conversation_id?: string;
 source_runtime_kind?: string; window_start?: string; window_end?: string; objective?: string;
 current_stage?: string; stages: { name: string; state: string }[]; evidence_pack?: Record<string, unknown>;
 created_at: string; updated_at: string;
};

export type ApiToken = {
  id: string;
  agent_id?: string;
  name: string;
  masked_token: string;
  recoverable: boolean;
  created_at: string;
};

export type TraceSummary = {
  agent_id?: string;
  session_id?: string;
  trace_id: string;
  root_name: string;
  input_preview?: string;
  service_name: string;
  model?: string;
  start_time: string;
  end_time: string;
  duration_ms: number;
  span_count: number;
  error_count: number;
  last_ingested_at: string;
};

export type TraceSpan = {
  trace_id: string;
  span_id: string;
  parent_span_id?: string;
  name: string;
  kind: number;
  service_name: string;
  scope_name?: string;
  start_time: string;
  end_time: string;
  status_code: number;
  status_message?: string;
  attributes: Record<string, unknown>;
  resource_attributes: Record<string, unknown>;
  input?: string;
  output?: string;
  model?: string;
};

export type TraceDetail = {
  summary: TraceSummary;
  spans: TraceSpan[];
};












export type AgentSummary = {
  agent_id: string;
  display_name: string;
  identity_source: string;
  runtime_kind?: "xiaobaos" | "codex" | "claude_code" | "otel" | string;
  registered: boolean;
  connected: boolean;
  conversation_count: number;
  credential?: ApiToken;
  sources?: Array<{
    service_name: string;
    kind: "native_live" | "otel";
  }>;
  trace_count: number;
  span_count: number;
  error_count: number;
  last_seen_at: string;
};

export type RegisteredAgent = {
  agent_id: string;
  display_name: string;
  runtime_kind?: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
};

export type RegisteredAgentConnection = {
  agent: RegisteredAgent;
  connected: boolean;
  credential?: ApiToken;
};

export type AgentTraceWindow = {
  available: boolean;
  agent_id: string;
  window_start: string;
  window_end: string;
  traces: TraceSummary[];
};

export type WorkspaceData = {
  overviewErrors?: Partial<Record<"agents" | "traces" | "analysisJobs", string>>;
  system: SystemStatus;
  analysisJobs: AnalysisJob[];
  traceAvailable: boolean;
  agentAvailable: boolean;
  traces: TraceSummary[];
  agents: AgentSummary[];
};
