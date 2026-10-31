package control

import (
	"encoding/json"
	"time"
)

type User struct {
	ID          string    `json:"id"`
	GitHubID    int64     `json:"-"`
	Login       string    `json:"login"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Session struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type APIToken struct {
	ID             string    `json:"id"`
	TokenHash      string    `json:"-"`
	EncryptedToken string    `json:"-"`
	UserID         string    `json:"-"`
	AgentID        string    `json:"agent_id,omitempty"`
	Name           string    `json:"name"`
	MaskedToken    string    `json:"masked_token"`
	Recoverable    bool      `json:"recoverable"`
	CreatedAt      time.Time `json:"created_at"`
}

// AnalysisModelConfig is owner-scoped configuration for Catena's embedded
// Analysis Runtime. EncryptedAPIKey is storage-only and must never be
// serialized into a response, Job, Candidate, log or Evidence Pack.
type AnalysisModelConfig struct {
	OwnerUserID     string    `json:"-"`
	Provider        string    `json:"provider"`
	BaseURL         string    `json:"base_url"`
	Model           string    `json:"model"`
	EncryptedAPIKey string    `json:"-"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RegisteredAgent is the stable product identity chosen by a user. RuntimeKind
// is observed from accepted evidence; it is never supplied during onboarding.
type RegisteredAgent struct {
	ID          string    `json:"agent_id"`
	OwnerUserID string    `json:"-"`
	DisplayName string    `json:"display_name"`
	RuntimeKind string    `json:"runtime_kind,omitempty"`
	LastSeenAt  time.Time `json:"last_seen_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AgentProfile struct {
	OwnerUserID string    `json:"-"`
	Slug        string    `json:"slug"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CommunityProfile struct {
	Slug        string    `json:"slug"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	GitHubLogin string    `json:"github_login"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProfileRecord struct {
	Profile AgentProfile
	User    User
}

type AnalysisJobState string

const (
	AnalysisJobQueued    AnalysisJobState = "queued"
	AnalysisJobRunning   AnalysisJobState = "running"
	AnalysisJobCompleted AnalysisJobState = "completed"
	AnalysisJobFailed    AnalysisJobState = "failed"
	AnalysisJobCancelled AnalysisJobState = "cancelled"
)

func (s AnalysisJobState) Terminal() bool {
	return s == AnalysisJobCompleted || s == AnalysisJobFailed || s == AnalysisJobCancelled
}

type AnalysisSourceKind string

const (
	AnalysisSourceConversation  AnalysisSourceKind = "conversation"
	AnalysisSourceTrace         AnalysisSourceKind = "trace"
	AnalysisSourceAgentTraceSet AnalysisSourceKind = "agent_trace_set"
)

func (k AnalysisSourceKind) Valid() bool {
	return k == AnalysisSourceTrace || k == AnalysisSourceAgentTraceSet || k == AnalysisSourceConversation
}

type AnalysisStageState string

const (
	AnalysisStageQueued    AnalysisStageState = "queued"
	AnalysisStageRunning   AnalysisStageState = "running"
	AnalysisStageCompleted AnalysisStageState = "completed"
	AnalysisStageFailed    AnalysisStageState = "failed"
)

type AnalysisStage struct {
	Name       string             `json:"name"`
	State      AnalysisStageState `json:"state"`
	RawOutput  json.RawMessage    `json:"raw_output,omitempty"`
	Error      string             `json:"error,omitempty"`
	StartedAt  *time.Time         `json:"started_at,omitempty"`
	FinishedAt *time.Time         `json:"finished_at,omitempty"`
}

type AnalysisEvidenceBoundary struct {
	TargetAgentExecutedByCatena bool   `json:"target_agent_executed_by_catena"`
	CreatesRelease              bool   `json:"creates_release"`
	ReleaseAuthority            string `json:"release_authority"`
	CandidateStatus             string `json:"candidate_status"`
	ReviewScope                 string `json:"review_scope"`
}

type AnalysisEvidenceSpan struct {
	SpanID        string    `json:"span_id"`
	ParentSpanID  string    `json:"parent_span_id,omitempty"`
	Name          string    `json:"name"`
	ServiceName   string    `json:"service_name"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	StatusCode    int32     `json:"status_code"`
	StatusMessage string    `json:"status_message,omitempty"`
	Model         string    `json:"model,omitempty"`
	ToolName      string    `json:"tool_name,omitempty"`
	Input         string    `json:"input,omitempty"`
	Output        string    `json:"output,omitempty"`
	EventNames    []string  `json:"event_names,omitempty"`
}

type AnalysisEvidenceTrace struct {
	Summary           TraceSummary           `json:"summary"`
	Spans             []AnalysisEvidenceSpan `json:"spans"`
	IncludedSpanCount int                    `json:"included_span_count"`
	TotalSpanCount    uint64                 `json:"total_span_count"`
	Truncated         bool                   `json:"truncated"`
}

type AnalysisEvidencePack struct {
	SourceConversationID string                   `json:"source_conversation_id,omitempty"`
	Document             string                   `json:"document,omitempty"`
	Schema               string                   `json:"schema"`
	SourceKind           AnalysisSourceKind       `json:"source_kind"`
	SourceTraceID        string                   `json:"source_trace_id,omitempty"`
	SourceTraceIDs       []string                 `json:"source_trace_ids,omitempty"`
	SourceAgentID        string                   `json:"source_agent_id,omitempty"`
	SourceRuntimeKind    string                   `json:"source_runtime_kind,omitempty"`
	WindowStart          *time.Time               `json:"window_start,omitempty"`
	WindowEnd            *time.Time               `json:"window_end,omitempty"`
	TraceSummary         TraceSummary             `json:"trace_summary"`
	Spans                []AnalysisEvidenceSpan   `json:"spans"`
	Traces               []AnalysisEvidenceTrace  `json:"traces,omitempty"`
	IncludedTraceCount   int                      `json:"included_trace_count,omitempty"`
	TotalTraceCount      int                      `json:"total_trace_count,omitempty"`
	IncludedSpanCount    int                      `json:"included_span_count"`
	TotalSpanCount       uint64                   `json:"total_span_count"`
	Truncated            bool                     `json:"truncated"`
	Redacted             bool                     `json:"redacted"`
	Boundary             AnalysisEvidenceBoundary `json:"boundary"`
	SHA256               string                   `json:"sha256"`
	CreatedAt            time.Time                `json:"created_at"`
}

type AnalysisJob struct {
	SourceConversationID string          `json:"source_conversation_id,omitempty"`
	Pipeline             string          `json:"pipeline,omitempty"`
	Adapter              string          `json:"adapter,omitempty"`
	Options              json.RawMessage `json:"options,omitempty"`
	Artifacts            []Artifact      `json:"artifacts"`
	Checkpoint           json.RawMessage `json:"checkpoint,omitempty"`

	Attempt            int                      `json:"attempt,omitempty"`
	NextAttemptAt      *time.Time               `json:"next_attempt_at,omitempty"`
	CancelRequested    bool                     `json:"cancel_requested,omitempty"`
	Schema             string                   `json:"schema"`
	ID                 string                   `json:"job_id"`
	OwnerUserID        string                   `json:"-"`
	SourceKind         AnalysisSourceKind       `json:"source_kind"`
	SourceTraceID      string                   `json:"source_trace_id,omitempty"`
	SourceTraceIDs     []string                 `json:"source_trace_ids,omitempty"`
	SourceAgentID      string                   `json:"source_agent_id,omitempty"`
	SourceRuntimeKind  string                   `json:"source_runtime_kind,omitempty"`
	WindowStart        *time.Time               `json:"window_start,omitempty"`
	WindowEnd          *time.Time               `json:"window_end,omitempty"`
	Objective          string                   `json:"objective,omitempty"`
	IdempotencyKey     string                   `json:"-"`
	RequestFingerprint string                   `json:"-"`
	State              AnalysisJobState         `json:"state"`
	CurrentStage       string                   `json:"current_stage,omitempty"`
	Stages             []AnalysisStage          `json:"stages"`
	EvidencePack       *AnalysisEvidencePack    `json:"evidence_pack,omitempty"`
	Boundary           AnalysisEvidenceBoundary `json:"boundary"`
	Error              string                   `json:"error,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}
