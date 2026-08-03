package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const platformExploreScenarioSchema = "barena.platform_explore_scenario.v1"

type ScenarioRunAdoptionRequest struct {
	Schema          string                 `json:"schema"`
	SourceProjectID string                 `json:"source_project_id"`
	ScenarioRunID   string                 `json:"scenario_run_id"`
	ScenarioID      string                 `json:"scenario_id"`
	SourceStatus    string                 `json:"source_status"`
	StartedAt       time.Time              `json:"started_at"`
	CompletedAt     time.Time              `json:"completed_at"`
	DurationInMS    int64                  `json:"duration_in_ms"`
	Scenario        ScenarioAdoptionPrompt `json:"scenario"`
	Target          ScenarioAdoptionTarget `json:"target"`
	TraceIDs        []string               `json:"trace_ids"`
	PrimaryTraceID  string                 `json:"primary_trace_id"`
	Judge           *ScenarioAdoptionJudge `json:"judge,omitempty"`
	Replay          ScenarioReplaySupport  `json:"replay"`
}

type ScenarioAdoptionPrompt struct {
	Name      string   `json:"name"`
	Objective string   `json:"objective"`
	Criteria  []string `json:"criteria"`
}

type ScenarioAdoptionTarget struct {
	Type        string `json:"type"`
	ReferenceID string `json:"reference_id"`
	Name        string `json:"name"`
}

type ScenarioAdoptionJudge struct {
	Verdict       string   `json:"verdict"`
	Reasoning     string   `json:"reasoning,omitempty"`
	MetCriteria   []string `json:"met_criteria"`
	UnmetCriteria []string `json:"unmet_criteria"`
	Error         string   `json:"error,omitempty"`
}

type ScenarioReplaySupport struct {
	Supported  bool   `json:"supported"`
	Reason     string `json:"reason,omitempty"`
	URL        string `json:"url,omitempty"`
	Method     string `json:"method,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	TimeoutMS  int    `json:"timeout_ms,omitempty"`
}

type ScenarioRunAdoptionResponse struct {
	Run            Run      `json:"run"`
	Created        bool     `json:"created"`
	TraceIDs       []string `json:"trace_ids"`
	PrimaryTraceID string   `json:"primary_trace_id"`
}

func (s *HTTPServer) adoptScenarioRun(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var request ScenarioRunAdoptionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.normalize()
	projectID := strings.TrimSpace(r.Header.Get("X-Barena-Project-ID"))
	if projectID == "" || projectID != request.SourceProjectID {
		writeProblem(w, http.StatusBadRequest, "source_project_id does not match the project context")
		return
	}
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerUserID := ""
	if user != nil {
		ownerUserID = user.ID
	}
	run, events, err := request.materialize(ownerUserID)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	value, created, err := s.store.AdoptScenarioRun(r.Context(), run, events)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, ScenarioRunAdoptionResponse{
		Run: value, Created: created, TraceIDs: append([]string(nil), request.TraceIDs...),
		PrimaryTraceID: request.PrimaryTraceID,
	})
}

func (r *ScenarioRunAdoptionRequest) normalize() {
	r.Schema = strings.TrimSpace(r.Schema)
	r.SourceProjectID = strings.TrimSpace(r.SourceProjectID)
	r.ScenarioRunID = strings.TrimSpace(r.ScenarioRunID)
	r.ScenarioID = strings.TrimSpace(r.ScenarioID)
	r.SourceStatus = strings.ToUpper(strings.TrimSpace(r.SourceStatus))
	r.Scenario.Name = strings.TrimSpace(r.Scenario.Name)
	r.Scenario.Objective = strings.TrimSpace(r.Scenario.Objective)
	r.Target.Type = strings.ToLower(strings.TrimSpace(r.Target.Type))
	r.Target.ReferenceID = strings.TrimSpace(r.Target.ReferenceID)
	r.Target.Name = strings.TrimSpace(r.Target.Name)
	r.PrimaryTraceID = strings.ToLower(strings.TrimSpace(r.PrimaryTraceID))
	for index := range r.TraceIDs {
		r.TraceIDs[index] = strings.ToLower(strings.TrimSpace(r.TraceIDs[index]))
	}
	slices.Sort(r.TraceIDs)
	r.TraceIDs = slices.Compact(r.TraceIDs)
	r.Replay.Reason = strings.TrimSpace(r.Replay.Reason)
	r.Replay.Method = strings.ToUpper(strings.TrimSpace(r.Replay.Method))
	r.Replay.URL = strings.TrimSpace(r.Replay.URL)
	r.Replay.OutputPath = strings.TrimSpace(r.Replay.OutputPath)
	if r.Judge != nil {
		r.Judge.Verdict = strings.ToLower(strings.TrimSpace(r.Judge.Verdict))
		r.Judge.Reasoning = strings.TrimSpace(r.Judge.Reasoning)
		r.Judge.Error = strings.TrimSpace(r.Judge.Error)
	}
}

func (r ScenarioRunAdoptionRequest) Validate() error {
	if r.Schema != "barena.scenario_run_adoption.v1" {
		return errors.New("unsupported Scenario adoption schema")
	}
	for _, value := range []string{r.SourceProjectID, r.ScenarioRunID, r.ScenarioID} {
		if value == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("Scenario source identity is invalid")
		}
	}
	if !scenarioStatusTerminal(r.SourceStatus) {
		return errors.New("only a terminal Scenario run can be adopted")
	}
	if r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || r.DurationInMS < 0 {
		return errors.New("Scenario timing is invalid")
	}
	if r.Scenario.Name == "" || len(r.Scenario.Name) > 300 ||
		r.Scenario.Objective == "" || len(r.Scenario.Objective) > 24000 ||
		len(r.Scenario.Criteria) > 100 {
		return errors.New("Scenario definition is invalid")
	}
	for _, criterion := range r.Scenario.Criteria {
		if strings.TrimSpace(criterion) == "" || len(criterion) > 4000 {
			return errors.New("Scenario criteria are invalid")
		}
	}
	if r.Target.Type != "http" || r.Target.ReferenceID == "" ||
		len(r.Target.ReferenceID) > 256 || r.Target.Name == "" || len(r.Target.Name) > 300 {
		return errors.New("only a named registered HTTP Agent can be adopted")
	}
	if len(r.TraceIDs) == 0 || len(r.TraceIDs) > 256 || !validOTelTraceID(r.PrimaryTraceID) {
		return errors.New("a retained primary OTel trace is required")
	}
	for _, traceID := range r.TraceIDs {
		if !validOTelTraceID(traceID) {
			return errors.New("trace_ids must contain valid W3C trace IDs")
		}
	}
	if !slices.Contains(r.TraceIDs, r.PrimaryTraceID) {
		return errors.New("primary_trace_id must be present in trace_ids")
	}
	if r.Judge != nil {
		switch r.Judge.Verdict {
		case "success", "failure", "inconclusive":
		default:
			return errors.New("Scenario Judge verdict is invalid")
		}
		if len(r.Judge.Reasoning) > 12000 || len(r.Judge.Error) > 2000 ||
			len(r.Judge.MetCriteria)+len(r.Judge.UnmetCriteria) > 200 {
			return errors.New("Scenario Judge result is too large")
		}
	}
	return r.Replay.Validate()
}

func (r ScenarioReplaySupport) Validate() error {
	if !r.Supported {
		if r.Reason == "" || len(r.Reason) > 500 || r.URL != "" || r.Method != "" || r.OutputPath != "" {
			return errors.New("unsupported Replay requires only a bounded reason")
		}
		return nil
	}
	if r.Reason != "" || r.Method != http.MethodPost || r.TimeoutMS < 1000 || r.TimeoutMS > 120000 {
		return errors.New("supported HTTP Replay requires POST and a timeout from 1000 to 120000 ms")
	}
	parsed, err := url.ParseRequestURI(r.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Replay URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if !supportedHTTPOutputPath(r.OutputPath) {
		return errors.New("HTTP Replay output_path is not supported")
	}
	return nil
}

func supportedHTTPOutputPath(value string) bool {
	switch value {
	case "", "$.response", "$.message", "$.content", "$.choices[0].message.content":
		return true
	default:
		return false
	}
}

func scenarioStatusTerminal(status string) bool {
	switch status {
	case "SUCCESS", "FAILED", "ERROR", "CANCELLED", "STALLED":
		return true
	default:
		return false
	}
}

func (r ScenarioRunAdoptionRequest) materialize(
	ownerUserID string,
) (Run, []EngineEvent, error) {
	snapshot, err := json.Marshal(r)
	if err != nil {
		return Run{}, nil, err
	}
	fingerprintBytes := sha256.Sum256(snapshot)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	identityBytes := sha256.Sum256([]byte(r.SourceProjectID + "\x00" + r.ScenarioRunID))
	identity := hex.EncodeToString(identityBytes[:])[:24]
	runID := "run-platform-" + identity
	requestID := "req-platform-" + identity

	input, err := json.Marshal(map[string]any{
		"schema": platformExploreScenarioSchema,
		"source": map[string]any{
			"kind": "langwatch_scenario_run", "project_id": r.SourceProjectID,
			"scenario_run_id": r.ScenarioRunID, "scenario_id": r.ScenarioID,
			"snapshot_sha256": fingerprint,
		},
		"scenario": r.Scenario,
		"target":   r.Target,
		"execution": map[string]any{
			"status": r.SourceStatus, "started_at": r.StartedAt,
			"completed_at": r.CompletedAt, "duration_in_ms": r.DurationInMS,
		},
		"judge": r.Judge,
		"evidence": map[string]any{
			"trace_ids": r.TraceIDs, "primary_trace_id": r.PrimaryTraceID,
		},
	})
	if err != nil {
		return Run{}, nil, err
	}
	runtime, err := json.Marshal(map[string]any{
		"schema": "barena.platform_http_runtime.v1",
		"type":   "http", "reference_id": r.Target.ReferenceID,
		"name": r.Target.Name, "replay": r.Replay,
	})
	if err != nil {
		return Run{}, nil, err
	}
	state, phase, actor, runError := adoptedScenarioState(r.SourceStatus)
	run := Run{
		ID: runID, RequestID: requestID, OwnerUserID: ownerUserID,
		Origin: OriginPlatform, Operation: OperationExplore, State: state,
		CurrentPhase: phase, CurrentActor: actor, Input: input, Runtime: runtime,
		Error: runError, CreatedAt: r.StartedAt.UTC(), UpdatedAt: r.CompletedAt.UTC(),
	}
	importPayload, _ := json.Marshal(map[string]any{
		"schema":            "barena.scenario_evidence_imported.v1",
		"source_project_id": r.SourceProjectID, "scenario_run_id": r.ScenarioRunID,
		"snapshot_sha256": fingerprint, "trace_ids": r.TraceIDs,
	})
	terminalPayload, _ := json.Marshal(map[string]any{
		"schema": "barena.scenario_terminal_fact.v1", "source_status": r.SourceStatus,
		"judge": r.Judge, "replay_supported": r.Replay.Supported,
		"replay_reason": r.Replay.Reason,
	})
	events := []EngineEvent{
		{
			Schema: "barena.engine_event.v1", EventID: "evt-platform-" + identity + "-1",
			RunID: runID, Sequence: 1, Timestamp: r.CompletedAt.UTC(),
			Operation: OperationExplore, Kind: "evidence_imported", Phase: "evidence",
			Actor: "platform", AttemptID: r.ScenarioRunID, TraceID: r.PrimaryTraceID,
			Payload: importPayload,
		},
		{
			Schema: "barena.engine_event.v1", EventID: "evt-platform-" + identity + "-2",
			RunID: runID, Sequence: 2, Timestamp: r.CompletedAt.UTC(),
			Operation: OperationExplore, Kind: "terminal", Phase: phase,
			Actor: actor, AttemptID: r.ScenarioRunID, TraceID: r.PrimaryTraceID,
			Payload: terminalPayload,
		},
	}
	return run, events, nil
}

func adoptedScenarioState(status string) (RunState, string, string, string) {
	switch status {
	case "SUCCESS", "FAILED":
		return StateCompleted, "complete", "scenario_judge", ""
	case "CANCELLED":
		return StateCancelled, "cancelled", "scenario", "Scenario run was cancelled"
	case "STALLED":
		return StateInterrupted, "interrupted", "scenario", "Scenario run stalled"
	default:
		return StateFailed, "failed", "scenario", "Scenario execution failed"
	}
}
