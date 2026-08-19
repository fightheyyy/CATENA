package control

import (
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"
)

type Operation string

const (
	OperationExplore Operation = "explore"
	OperationReplay  Operation = "replay"
	OperationCompare Operation = "compare"
)

func (o Operation) Valid() bool {
	return o == OperationExplore || o == OperationReplay || o == OperationCompare
}

type RunState string

const (
	StateQueued      RunState = "queued"
	StateRunning     RunState = "running"
	StateCompleted   RunState = "completed"
	StateInterrupted RunState = "interrupted"
	StateCancelled   RunState = "cancelled"
	StateFailed      RunState = "failed"
)

func (s RunState) Terminal() bool {
	return s == StateCompleted || s == StateInterrupted || s == StateCancelled || s == StateFailed
}

type RunOrigin string

const (
	OriginLocal    RunOrigin = "local"
	OriginEdge     RunOrigin = "edge"
	OriginPlatform RunOrigin = "platform"
)

func (o RunOrigin) Valid() bool {
	return o == OriginLocal || o == OriginEdge || o == OriginPlatform
}

type Run struct {
	ID              string          `json:"run_id"`
	RequestID       string          `json:"request_id"`
	OwnerUserID     string          `json:"-"`
	Origin          RunOrigin       `json:"origin"`
	Operation       Operation       `json:"operation"`
	State           RunState        `json:"state"`
	CurrentPhase    string          `json:"current_phase,omitempty"`
	CurrentActor    string          `json:"current_actor,omitempty"`
	Input           json.RawMessage `json:"input"`
	Runtime         json.RawMessage `json:"runtime,omitempty"`
	CancelRequested bool            `json:"cancel_requested"`
	Error           string          `json:"error,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

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

// EvolutionModelConfig is owner-scoped configuration for Catena's embedded
// Evolution Runtime. EncryptedAPIKey is storage-only and must never be
// serialized into a response, Job, Candidate, log or Evidence Pack.
type EvolutionModelConfig struct {
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

type CapabilitySummary struct {
	Key          string  `json:"key"`
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	Level        string  `json:"level"`
	SampleCount  int     `json:"sample_count"`
	SuccessCount int     `json:"success_count"`
	SuccessRate  float64 `json:"success_rate"`
	OTLPSpans    int     `json:"otlp_spans"`
}

type CommunityProfile struct {
	Slug         string              `json:"slug"`
	DisplayName  string              `json:"display_name"`
	Bio          string              `json:"bio"`
	GitHubLogin  string              `json:"github_login"`
	AvatarURL    string              `json:"avatar_url,omitempty"`
	Capabilities []CapabilitySummary `json:"capabilities"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type ProfileRecord struct {
	Profile AgentProfile
	User    User
}

type EngineEvent struct {
	Schema    string          `json:"schema"`
	EventID   string          `json:"event_id"`
	RunID     string          `json:"run_id"`
	Sequence  int64           `json:"sequence"`
	Timestamp time.Time       `json:"timestamp"`
	Operation Operation       `json:"operation"`
	Kind      string          `json:"kind"`
	Phase     string          `json:"phase"`
	Actor     string          `json:"actor"`
	AttemptID string          `json:"attempt_id,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

func (e EngineEvent) Validate(run Run) error {
	if e.Schema != "barena.engine_event.v1" {
		return errors.New("unsupported Engine Event schema")
	}
	if e.RunID != run.ID || e.Operation != run.Operation {
		return errors.New("Engine Event identity does not match the Run")
	}
	if e.EventID == "" || e.Sequence < 1 || e.Timestamp.IsZero() {
		return errors.New("Engine Event identity is incomplete")
	}
	if e.Kind == "" || e.Phase == "" || e.Actor == "" || len(e.Payload) == 0 {
		return errors.New("Engine Event body is incomplete")
	}
	return nil
}

type CreateRunRequest struct {
	Operation Operation       `json:"operation"`
	Input     json.RawMessage `json:"input"`
	Runtime   json.RawMessage `json:"runtime,omitempty"`
}

func (r CreateRunRequest) Validate() error {
	if !r.Operation.Valid() {
		return errors.New("operation must be explore, replay, or compare")
	}
	if !jsonObject(r.Input) {
		return errors.New("input must be a JSON object")
	}
	if len(r.Runtime) > 0 && !jsonObject(r.Runtime) {
		return errors.New("runtime must be a JSON object")
	}
	return nil
}

type FinishRunRequest struct {
	State RunState `json:"state"`
	Error string   `json:"error,omitempty"`
}

func (r FinishRunRequest) Validate() error {
	if !r.State.Terminal() {
		return errors.New("state must be completed, interrupted, cancelled, or failed")
	}
	if r.State == StateCompleted && r.Error != "" {
		return errors.New("a completed Run cannot include an error")
	}
	return nil
}

func jsonObject(value json.RawMessage) bool {
	if len(value) == 0 || !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

type EngineRequest struct {
	Schema    string          `json:"schema"`
	RequestID string          `json:"request_id"`
	RunID     string          `json:"run_id"`
	Operation Operation       `json:"operation"`
	RunsRoot  string          `json:"runs_root"`
	Input     json.RawMessage `json:"input"`
	Runtime   json.RawMessage `json:"runtime,omitempty"`
}

type IssueSeverity string

const (
	SeverityLow      IssueSeverity = "low"
	SeverityMedium   IssueSeverity = "medium"
	SeverityHigh     IssueSeverity = "high"
	SeverityCritical IssueSeverity = "critical"
)

func (s IssueSeverity) Valid() bool {
	return s == SeverityLow ||
		s == SeverityMedium ||
		s == SeverityHigh ||
		s == SeverityCritical
}

type IssueStatus string

const (
	IssueOpen      IssueStatus = "open"
	IssuePromoted  IssueStatus = "promoted"
	IssueDismissed IssueStatus = "dismissed"
)

type Issue struct {
	ID             string        `json:"issue_id"`
	OwnerUserID    string        `json:"-"`
	SourceRunID    string        `json:"source_run_id"`
	SourceTraceID  string        `json:"source_trace_id,omitempty"`
	Title          string        `json:"title"`
	Summary        string        `json:"summary"`
	Severity       IssueSeverity `json:"severity"`
	Status         IssueStatus   `json:"status"`
	PromotedCaseID string        `json:"promoted_case_id,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type CreateIssueRequest struct {
	TraceID  string        `json:"trace_id,omitempty"`
	Title    string        `json:"title"`
	Summary  string        `json:"summary"`
	Severity IssueSeverity `json:"severity"`
}

func (r CreateIssueRequest) Validate() error {
	if len(r.Title) < 3 || len(r.Title) > 160 {
		return errors.New("title must contain from 3 to 160 characters")
	}
	if len(r.Summary) < 1 || len(r.Summary) > 4000 {
		return errors.New("summary must contain from 1 to 4000 characters")
	}
	if !r.Severity.Valid() {
		return errors.New("severity must be low, medium, high, or critical")
	}
	if len(r.TraceID) > 256 {
		return errors.New("trace_id must not exceed 256 characters")
	}
	return nil
}

type Case struct {
	Schema          string          `json:"schema"`
	ID              string          `json:"case_id"`
	Revision        int             `json:"revision"`
	OwnerUserID     string          `json:"-"`
	SourceIssueID   string          `json:"source_issue_id"`
	SourceRunID     string          `json:"source_run_id"`
	SourceTraceID   string          `json:"source_trace_id,omitempty"`
	Title           string          `json:"title"`
	Operation       Operation       `json:"operation"`
	Input           json.RawMessage `json:"input"`
	Runtime         json.RawMessage `json:"runtime,omitempty"`
	ReplayPrompt    string          `json:"replay_prompt,omitempty"`
	SuccessCriteria string          `json:"success_criteria"`
	Verifier        json.RawMessage `json:"verifier"`
	CreatedAt       time.Time       `json:"created_at"`
}

type PromoteIssueRequest struct {
	ReplayPrompt    string          `json:"replay_prompt,omitempty"`
	SuccessCriteria string          `json:"success_criteria"`
	Verifier        json.RawMessage `json:"verifier,omitempty"`
}

func (r PromoteIssueRequest) Validate() error {
	if len(r.ReplayPrompt) > 24000 {
		return errors.New("replay_prompt must not exceed 24000 characters")
	}
	if len(r.SuccessCriteria) < 1 || len(r.SuccessCriteria) > 4000 {
		return errors.New("success_criteria must contain from 1 to 4000 characters")
	}
	var verifier struct {
		Kind      string            `json:"kind"`
		Artifacts []json.RawMessage `json:"artifacts"`
	}
	if !jsonObject(r.Verifier) || json.Unmarshal(r.Verifier, &verifier) != nil {
		return errors.New("verifier must be a JSON object")
	}
	if verifier.Kind != "artifact_assertions" || len(verifier.Artifacts) == 0 {
		return errors.New("verifier must contain non-empty artifact_assertions")
	}
	if err := validateMVPArtifactAssertions(verifier.Artifacts); err != nil {
		return err
	}
	return nil
}

func validateMVPArtifactAssertions(assertions []json.RawMessage) error {
	seen := make(map[string]bool)
	for _, encoded := range assertions {
		var assertion map[string]any
		if json.Unmarshal(encoded, &assertion) != nil || assertion == nil {
			return errors.New("each verifier artifact assertion must be a JSON object")
		}
		for key := range assertion {
			if key != "path" && key != "exists" && key != "contains" {
				return errors.New("MVP1 artifact assertions support only path, exists, and contains")
			}
		}
		rawPath, ok := assertion["path"].(string)
		cleaned := path.Clean(strings.TrimSpace(rawPath))
		if !ok ||
			cleaned == "." ||
			cleaned == ".." ||
			path.IsAbs(cleaned) ||
			strings.HasPrefix(cleaned, "../") ||
			strings.Contains(rawPath, "\\") {
			return errors.New("artifact assertion path must stay inside the Replay workspace")
		}
		if seen[cleaned] {
			return errors.New("artifact assertion paths must be unique")
		}
		seen[cleaned] = true
		exists := true
		if value, present := assertion["exists"]; present {
			typed, valid := value.(bool)
			if !valid {
				return errors.New("artifact assertion exists must be boolean")
			}
			exists = typed
		}
		if value, present := assertion["contains"]; present {
			typed, valid := value.(string)
			if !valid || strings.TrimSpace(typed) == "" {
				return errors.New("artifact assertion contains must be a non-empty string")
			}
			if !exists {
				return errors.New("artifact assertion cannot combine exists=false with contains")
			}
		}
	}
	return nil
}

type ReleaseDecision string

const (
	DecisionCleared  ReleaseDecision = "cleared"
	DecisionHeld     ReleaseDecision = "held"
	DecisionRejected ReleaseDecision = "rejected"
)

func (d ReleaseDecision) Valid() bool {
	return d == DecisionCleared || d == DecisionHeld || d == DecisionRejected
}

type HarnessVersion struct {
	ID             string          `json:"harness_version_id"`
	OwnerUserID    string          `json:"-"`
	CaseID         string          `json:"case_id"`
	RunID          string          `json:"run_id"`
	SourceRunID    string          `json:"source_run_id"`
	SourceTraceID  string          `json:"source_trace_id,omitempty"`
	IdempotencyKey string          `json:"-"`
	Runtime        json.RawMessage `json:"runtime"`
	CreatedAt      time.Time       `json:"created_at"`
}

type Evaluation struct {
	ID               string          `json:"evaluation_id"`
	OwnerUserID      string          `json:"-"`
	HarnessVersionID string          `json:"harness_version_id"`
	CaseID           string          `json:"case_id"`
	RunID            string          `json:"run_id"`
	SourceRunID      string          `json:"source_run_id"`
	SourceTraceID    string          `json:"source_trace_id,omitempty"`
	ReplayTraceID    string          `json:"replay_trace_id,omitempty"`
	TerminalEventID  string          `json:"terminal_event_id"`
	PackageStatus    string          `json:"package_status"`
	ResultStatus     string          `json:"result_status,omitempty"`
	Decision         ReleaseDecision `json:"decision"`
	Summary          string          `json:"summary,omitempty"`
	ResultRef        string          `json:"result_ref"`
	CreatedAt        time.Time       `json:"created_at"`
}

type Release struct {
	ID               string          `json:"release_id"`
	OwnerUserID      string          `json:"-"`
	HarnessVersionID string          `json:"harness_version_id"`
	EvaluationID     string          `json:"evaluation_id"`
	CaseID           string          `json:"case_id"`
	RunID            string          `json:"run_id"`
	SourceRunID      string          `json:"source_run_id"`
	SourceTraceID    string          `json:"source_trace_id,omitempty"`
	ReplayTraceID    string          `json:"replay_trace_id,omitempty"`
	TerminalEventID  string          `json:"terminal_event_id"`
	Decision         ReleaseDecision `json:"decision"`
	Summary          string          `json:"summary,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type ReplayFact struct {
	TerminalEventID string
	ReplayTraceID   string
	PackageStatus   string
	ResultStatus    string
	Decision        ReleaseDecision
	Summary         string
	ResultRef       string
}

type EvolutionJobState string

const (
	EvolutionJobQueued    EvolutionJobState = "queued"
	EvolutionJobRunning   EvolutionJobState = "running"
	EvolutionJobCompleted EvolutionJobState = "completed"
	EvolutionJobFailed    EvolutionJobState = "failed"
)

func (s EvolutionJobState) Terminal() bool {
	return s == EvolutionJobCompleted || s == EvolutionJobFailed
}

type EvolutionSourceKind string

const (
	EvolutionSourceTrace         EvolutionSourceKind = "trace"
	EvolutionSourceRunTrace      EvolutionSourceKind = "run_trace"
	EvolutionSourceAgentTraceSet EvolutionSourceKind = "agent_trace_set"
)

func (k EvolutionSourceKind) Valid() bool {
	return k == EvolutionSourceTrace || k == EvolutionSourceRunTrace ||
		k == EvolutionSourceAgentTraceSet
}

type EvolutionStageState string

const (
	EvolutionStageQueued    EvolutionStageState = "queued"
	EvolutionStageRunning   EvolutionStageState = "running"
	EvolutionStageCompleted EvolutionStageState = "completed"
	EvolutionStageFailed    EvolutionStageState = "failed"
)

type EvolutionStage struct {
	Name       string              `json:"name"`
	Role       string              `json:"role"`
	State      EvolutionStageState `json:"state"`
	RawOutput  json.RawMessage     `json:"raw_output,omitempty"`
	Error      string              `json:"error,omitempty"`
	StartedAt  *time.Time          `json:"started_at,omitempty"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
}

type EvolutionFinding struct {
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Severity string   `json:"severity"`
	Evidence []string `json:"evidence"`
}

type EvolutionCaseProposal struct {
	CandidateID         string                 `json:"candidate_id"`
	Kind                EvolutionCandidateKind `json:"kind"`
	Title               string                 `json:"title"`
	ReplayPrompt        string                 `json:"replay_prompt"`
	SuccessCriteria     string                 `json:"success_criteria"`
	Verifier            json.RawMessage        `json:"verifier"`
	Status              string                 `json:"status"`
	SourceRunID         string                 `json:"source_run_id,omitempty"`
	SourceTraceID       string                 `json:"source_trace_id,omitempty"`
	SourceTraceIDs      []string               `json:"source_trace_ids,omitempty"`
	SourceAgentID       string                 `json:"source_agent_id,omitempty"`
	EvidencePackSHA256  string                 `json:"evidence_pack_sha256"`
	RequiresHumanReview bool                   `json:"requires_human_review"`
}

type EvolutionCandidateKind string

const (
	EvolutionCandidateAgentMD   EvolutionCandidateKind = "agent_md"
	EvolutionCandidateRole      EvolutionCandidateKind = "role"
	EvolutionCandidateSkill     EvolutionCandidateKind = "skill"
	EvolutionCandidateDSHPlugin EvolutionCandidateKind = "dsh_plugin"
	// Memory and Case remain readable for historical Evolution Jobs only.
	EvolutionCandidateMemory  EvolutionCandidateKind = "memory"
	EvolutionCandidateHarness EvolutionCandidateKind = "harness"
	EvolutionCandidateCase    EvolutionCandidateKind = "case"
)

func (k EvolutionCandidateKind) Valid() bool {
	return k == EvolutionCandidateAgentMD ||
		k == EvolutionCandidateRole ||
		k == EvolutionCandidateSkill ||
		k == EvolutionCandidateDSHPlugin ||
		k == EvolutionCandidateMemory ||
		k == EvolutionCandidateHarness ||
		k == EvolutionCandidateCase
}

type EvolutionCandidate struct {
	ID                 string                 `json:"candidate_id"`
	Kind               EvolutionCandidateKind `json:"kind"`
	Title              string                 `json:"title"`
	Summary            string                 `json:"summary"`
	Content            json.RawMessage        `json:"content"`
	Status             string                 `json:"status"`
	SourceRunID        string                 `json:"source_run_id,omitempty"`
	SourceTraceID      string                 `json:"source_trace_id,omitempty"`
	SourceTraceIDs     []string               `json:"source_trace_ids,omitempty"`
	SourceAgentID      string                 `json:"source_agent_id,omitempty"`
	SourceRuntimeKind  string                 `json:"source_runtime_kind,omitempty"`
	EvidencePackSHA256 string                 `json:"evidence_pack_sha256"`
}

type EvolutionReview struct {
	Verdict         string `json:"verdict"`
	Summary         string `json:"summary"`
	Scope           string `json:"scope"`
	CandidateStatus string `json:"candidate_status"`
}

type EvolutionEvidenceBoundary struct {
	TargetAgentExecutedByCatena bool   `json:"target_agent_executed_by_catena"`
	CreatesRelease              bool   `json:"creates_release"`
	ReleaseAuthority            string `json:"release_authority"`
	CandidateStatus             string `json:"candidate_status"`
	ReviewScope                 string `json:"review_scope"`
}

type EvolutionEvidenceSpan struct {
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

type EvolutionEvidenceRun struct {
	RunID     string          `json:"run_id"`
	Origin    RunOrigin       `json:"origin"`
	Operation Operation       `json:"operation"`
	State     RunState        `json:"state"`
	Input     json.RawMessage `json:"input"`
	Runtime   json.RawMessage `json:"runtime,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type EvolutionEvidenceTrace struct {
	Summary           TraceSummary            `json:"summary"`
	Spans             []EvolutionEvidenceSpan `json:"spans"`
	IncludedSpanCount int                     `json:"included_span_count"`
	TotalSpanCount    uint64                  `json:"total_span_count"`
	Truncated         bool                    `json:"truncated"`
}

type EvolutionEvidencePack struct {
	Schema             string                    `json:"schema"`
	SourceKind         EvolutionSourceKind       `json:"source_kind"`
	SourceRunID        string                    `json:"source_run_id,omitempty"`
	SourceTraceID      string                    `json:"source_trace_id,omitempty"`
	SourceTraceIDs     []string                  `json:"source_trace_ids,omitempty"`
	SourceAgentID      string                    `json:"source_agent_id,omitempty"`
	SourceRuntimeKind  string                    `json:"source_runtime_kind,omitempty"`
	WindowStart        *time.Time                `json:"window_start,omitempty"`
	WindowEnd          *time.Time                `json:"window_end,omitempty"`
	Run                *EvolutionEvidenceRun     `json:"run,omitempty"`
	TraceSummary       TraceSummary              `json:"trace_summary"`
	Spans              []EvolutionEvidenceSpan   `json:"spans"`
	Traces             []EvolutionEvidenceTrace  `json:"traces,omitempty"`
	IncludedTraceCount int                       `json:"included_trace_count,omitempty"`
	TotalTraceCount    int                       `json:"total_trace_count,omitempty"`
	RunEvents          []EngineEvent             `json:"run_events"`
	IncludedSpanCount  int                       `json:"included_span_count"`
	TotalSpanCount     uint64                    `json:"total_span_count"`
	Truncated          bool                      `json:"truncated"`
	Redacted           bool                      `json:"redacted"`
	Boundary           EvolutionEvidenceBoundary `json:"boundary"`
	SHA256             string                    `json:"sha256"`
	CreatedAt          time.Time                 `json:"created_at"`
}

type EvolutionJob struct {
	Schema             string                    `json:"schema"`
	ID                 string                    `json:"job_id"`
	OwnerUserID        string                    `json:"-"`
	SourceKind         EvolutionSourceKind       `json:"source_kind"`
	SourceRunID        string                    `json:"source_run_id,omitempty"`
	SourceTraceID      string                    `json:"source_trace_id,omitempty"`
	SourceTraceIDs     []string                  `json:"source_trace_ids,omitempty"`
	SourceAgentID      string                    `json:"source_agent_id,omitempty"`
	SourceRuntimeKind  string                    `json:"source_runtime_kind,omitempty"`
	WindowStart        *time.Time                `json:"window_start,omitempty"`
	WindowEnd          *time.Time                `json:"window_end,omitempty"`
	Objective          string                    `json:"objective,omitempty"`
	OutputLanguage     string                    `json:"output_language,omitempty"`
	IdempotencyKey     string                    `json:"-"`
	RequestFingerprint string                    `json:"-"`
	State              EvolutionJobState         `json:"state"`
	CurrentStage       string                    `json:"current_stage,omitempty"`
	Stages             []EvolutionStage          `json:"stages"`
	Finding            *EvolutionFinding         `json:"finding,omitempty"`
	CaseProposal       *EvolutionCaseProposal    `json:"case_proposal,omitempty"`
	Candidate          *EvolutionCandidate       `json:"candidate,omitempty"`
	Review             *EvolutionReview          `json:"review,omitempty"`
	EvidencePack       *EvolutionEvidencePack    `json:"evidence_pack,omitempty"`
	Boundary           EvolutionEvidenceBoundary `json:"boundary"`
	Error              string                    `json:"error,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
}

type CreateEvolutionJobRequest struct {
	TraceID   string `json:"trace_id"`
	Objective string `json:"objective,omitempty"`
}

type CreateTraceEvolutionJobRequest struct {
	Objective string `json:"objective,omitempty"`
}

type CreateAgentEvolutionJobRequest struct {
	WindowStart    time.Time `json:"window_start"`
	WindowEnd      time.Time `json:"window_end"`
	Objective      string    `json:"objective,omitempty"`
	OutputLanguage string    `json:"output_language,omitempty"`
}

func (r CreateAgentEvolutionJobRequest) Validate(now time.Time) error {
	if r.WindowStart.IsZero() || r.WindowEnd.IsZero() || !r.WindowEnd.After(r.WindowStart) {
		return errors.New("window_start and window_end must define a valid time window")
	}
	if r.WindowEnd.Sub(r.WindowStart) > 31*24*time.Hour {
		return errors.New("Agent Trace window must not exceed 31 days")
	}
	if r.WindowEnd.After(now.Add(5 * time.Minute)) {
		return errors.New("window_end must not be in the future")
	}
	if len(r.Objective) > 4000 {
		return errors.New("objective must not exceed 4000 characters")
	}
	return nil
}

func (r CreateTraceEvolutionJobRequest) Validate() error {
	if len(r.Objective) > 4000 {
		return errors.New("objective must not exceed 4000 characters")
	}
	return nil
}

func (r CreateEvolutionJobRequest) Validate() error {
	if strings.TrimSpace(r.TraceID) == "" || len(r.TraceID) > 256 {
		return errors.New("trace_id must contain from 1 to 256 characters")
	}
	if len(r.Objective) > 4000 {
		return errors.New("objective must not exceed 4000 characters")
	}
	return nil
}
