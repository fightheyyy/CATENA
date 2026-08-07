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
  run_bundle: "barena.run_bundle.v1";
  evolution_protocol: "barena.xiaoba_evolution_request.v1";
  evolution_runtime: string;
  trace_store: string;
  memory_store: string;
};

export type RuntimeRole = {
  id: string;
  display_name: string;
  responsibility: string;
  output: string;
};

export type Runtime = {
  runtime_id: string;
  display_name: string;
  kind: string;
  source: string;
  status: string;
  version?: string;
  detail: string;
  roles: RuntimeRole[];
  capabilities: {
    probe: boolean;
    role_turn: boolean;
    cancellation: boolean;
    telemetry: string;
    target_runtime_hosted: boolean;
  };
};

export type Run = {
  run_id: string;
  request_id: string;
  origin: string;
  operation: "explore" | "replay" | "compare";
  state: string;
  current_phase?: string;
  current_actor?: string;
  cancel_requested: boolean;
  error?: string;
  created_at: string;
  updated_at: string;
};

export type EvolutionStage = {
  name: string;
  role: string;
  state: string;
  raw_output?: unknown;
  error?: string;
  started_at?: string;
  finished_at?: string;
};

export type EvolutionFinding = {
  title: string;
  summary: string;
  severity: string;
  evidence: string[];
};

export type EvolutionCaseProposal = {
  candidate_id?: string;
  kind?: "case";
  title: string;
  replay_prompt: string;
  success_criteria: string;
  verifier?: unknown;
  requires_human_review?: boolean;
  status: string;
  source_trace_id?: string;
  source_trace_ids?: string[];
  source_agent_id?: string;
  source_run_id?: string;
  evidence_pack_sha256?: string;
};

export type EvolutionCandidateKind = "agent_md" | "memory" | "skill" | "role" | "harness" | "case" | "unknown";

export type EvolutionCandidate = {
  candidate_id: string;
  kind: EvolutionCandidateKind;
  title: string;
  summary: string;
  content?: unknown;
  status: string;
  source_trace_id?: string;
  source_trace_ids?: string[];
  source_agent_id?: string;
  source_run_id?: string;
  evidence_pack_sha256?: string;
};

export type EvolutionReview = {
  verdict: string;
  summary: string;
  scope: string;
  candidate_status: string;
};

export type EvolutionBoundary = {
  target_agent_executed_by_catena: boolean;
  creates_release: boolean;
  release_authority: string;
  candidate_status: string;
  review_scope: string;
};

export type EvolutionJob = {
  schema?: string;
  job_id: string;
  source_kind?: "trace" | "run_trace" | "agent_trace_set";
  source_run_id?: string;
  source_trace_id?: string;
  source_trace_ids: string[];
  source_agent_id?: string;
  window_start?: string;
  window_end?: string;
  objective?: string;
  state: string;
  current_stage?: string;
  stages: EvolutionStage[];
  finding?: EvolutionFinding;
  case_proposal?: EvolutionCaseProposal;
  candidates: EvolutionCandidate[];
  review?: EvolutionReview;
  error?: string;
  evidence_pack?: Record<string, unknown>;
  boundary?: EvolutionBoundary;
  created_at: string;
  updated_at: string;
};

export type Issue = {
  issue_id: string;
  source_run_id: string;
  source_trace_id?: string;
  title: string;
  summary: string;
  severity: string;
  status: string;
  promoted_case_id?: string;
  created_at: string;
};

export type RegressionCase = {
  case_id: string;
  revision: number;
  source_issue_id: string;
  title: string;
  operation: string;
  success_criteria: string;
  created_at: string;
};

export type Release = {
  release_id: string;
  case_id: string;
  run_id: string;
  decision: "cleared" | "held" | "rejected";
  summary?: string;
  created_at: string;
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
  trace_id: string;
  root_name: string;
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

export type MemoryIngestReceipt = {
  trace_id?: string;
  source_conversation_id?: string;
  conversation_id: number;
  task_id: string;
  status: string;
  indexed: boolean;
  message?: string;
};

export type MemoryRecallItem = {
  id: string;
  content: string;
  title?: string;
  score: number;
  metadata?: Record<string, unknown>;
};

export type MemoryRecallBundle = {
  success: boolean;
  query: string;
  facts: MemoryRecallItem[];
  conversations: MemoryRecallItem[];
  topics: MemoryRecallItem[];
  graph_expansion?: Record<string, unknown>;
  temporal_expansion?: Record<string, unknown>;
  search_time_ms?: number;
};

export type MemoryRecord = {
  id: string;
  content: string;
  created_at?: string;
  metadata?: Record<string, unknown>;
};

export type MemoryList = {
  memories: MemoryRecord[];
  total: number;
};

export type MemoryGraphEntity = {
  name: string;
  type: string;
  description?: string;
};

export type MemoryGraphRelation = {
  source: string;
  target: string;
  type: string;
  confidence: number;
};

export type MemoryFactGraph = {
  fact_id: number;
  content: string;
  entities: MemoryGraphEntity[];
  relations: MemoryGraphRelation[];
  total_entities: number;
  total_relations: number;
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
    kind: "native_live" | "history_backfill" | "otel";
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

export type AgentTraceWindow = {
  available: boolean;
  agent_id: string;
  window_start: string;
  window_end: string;
  traces: TraceSummary[];
};

export type ConversationContentPart = {
  type: "text" | "file";
  text?: string;
  name?: string;
  ref?: string;
  mime_type?: string;
};

export type ConversationMessage = {
  schema: "xiaoba.conversation_message.v1";
  message_id: string;
  conversation_id: string;
  sequence: number;
  occurred_at: string;
  received_at?: string;
  runtime: "xiaobaos";
  agent_id: string;
  agent_name?: string;
  surface: "cli" | "feishu" | "weixin" | "pet";
  role: "user" | "assistant";
  role_name?: string;
  content: ConversationContentPart[];
  delivery: {
    status: "received" | "delivered";
    platform_message_ids?: string[];
  };
  trace_id?: string;
};

export type ConversationSummary = {
  conversation_id: string;
  agent_id: string;
  agent_name?: string;
  runtime: "xiaobaos";
  surface: ConversationMessage["surface"];
  title: string;
  message_count: number;
  user_message_count: number;
  last_visible_message_preview?: string;
  created_at: string;
  updated_at: string;
};

export type ConversationDocument = {
  schema: "catena.conversation.v1";
  conversation: ConversationSummary;
  messages: ConversationMessage[];
};

export type WorkspaceData = {
  system: SystemStatus;
  runtimes: Runtime[];
  runs: Run[];
  evolutionJobs: EvolutionJob[];
  issues: Issue[];
  cases: RegressionCase[];
  releases: Release[];
  traceAvailable: boolean;
  agentAvailable: boolean;
  traces: TraceSummary[];
  agents: AgentSummary[];
};
