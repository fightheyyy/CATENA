package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type CreateAnalysisRequest struct {
	Pipeline    string          `json:"pipeline"`
	Adapter     string          `json:"adapter,omitempty"`
	TraceID     string          `json:"trace_id,omitempty"`
	AgentID     string          `json:"agent_id,omitempty"`
	WindowStart time.Time       `json:"window_start,omitempty"`
	WindowEnd   time.Time       `json:"window_end,omitempty"`
	Objective   string          `json:"objective,omitempty"`
	Options     json.RawMessage `json:"options,omitempty"`
}

func (s *HTTPServer) createAnalysisJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.evolutionRuntime == nil || s.traces == nil {
		writeProblem(w, 503, "Engine and Trace storage are required")
		return
	}
	var req CreateAnalysisRequest
	if e := decodeJSON(w, r, &req); e != nil {
		writeProblem(w, 400, e.Error())
		return
	}
	if req.Pipeline != "rsi" && req.Pipeline != "memory" {
		writeProblem(w, 400, "pipeline must be rsi or memory")
		return
	}
	if req.Adapter == "" {
		if req.Pipeline == "rsi" {
			req.Adapter = "harness2"
		} else {
			req.Adapter = "openviking"
		}
	}
	if len(req.Objective) > 4000 || len(req.Options) > 128*1024 {
		writeProblem(w, 400, "analysis input is too large")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if e := validateReplayIdempotencyKey(key); e != nil {
		writeProblem(w, 400, e.Error())
		return
	}
	owner := ""
	if user != nil {
		owner = user.ID
	}
	now := time.Now().UTC()
	var pack EvolutionEvidencePack
	var err error
	if req.AgentID != "" {
		if req.TraceID != "" || req.WindowStart.IsZero() || !req.WindowEnd.After(req.WindowStart) || req.WindowEnd.Sub(req.WindowStart) > 31*24*time.Hour || req.WindowEnd.After(now.Add(5*time.Minute)) {
			writeProblem(w, 400, "invalid Agent Trace window")
			return
		}
		var summaries []TraceSummary
		summaries, err = s.traces.ListAgentTraces(r.Context(), traceOwnerID(user), req.AgentID, req.WindowStart, req.WindowEnd, maxAgentEvolutionTraceScan)
		if err == nil {
			selected := selectAgentEvolutionTraces(summaries, maxAgentEvolutionTraces)
			traces := []TraceDetail{}
			for _, v := range selected {
				var t TraceDetail
				t, err = s.traces.GetTrace(r.Context(), traceOwnerID(user), v.TraceID)
				if err != nil {
					break
				}
				traces = append(traces, t)
			}
			if err == nil {
				pack, err = buildAgentTraceSetEvidencePack(req.AgentID, req.WindowStart, req.WindowEnd, traces, len(summaries), now)
			}
		}
	} else {
		var id string
		id, err = normalizedEvolutionTraceID(req.TraceID)
		if err == nil {
			var trace TraceDetail
			trace, err = s.traces.GetTrace(r.Context(), traceOwnerID(user), id)
			if err == nil {
				pack, err = buildEvolutionEvidencePack(EvolutionSourceTrace, nil, id, &trace, nil, now)
			}
		}
	}
	if err != nil {
		writeProblem(w, statusFor(err), "Evidence unavailable: "+bounded(err.Error(), 300))
		return
	}
	raw, _ := json.Marshal(req)
	hash := sha256.Sum256(append(raw, []byte(pack.SHA256)...))
	job := EvolutionJob{Schema: "catena.analysis_job.v2", ID: newID("analysis-job"), OwnerUserID: owner, Pipeline: req.Pipeline, Adapter: req.Adapter, Options: req.Options, SourceKind: pack.SourceKind, SourceTraceID: pack.SourceTraceID, SourceTraceIDs: pack.SourceTraceIDs, SourceAgentID: pack.SourceAgentID, SourceRuntimeKind: pack.SourceRuntimeKind, WindowStart: pack.WindowStart, WindowEnd: pack.WindowEnd, Objective: sanitizeEvolutionText(req.Objective, 4000), IdempotencyKey: key, RequestFingerprint: hex.EncodeToString(hash[:]), State: EvolutionJobQueued, Stages: []EvolutionStage{{Name: "generate", State: EvolutionStageQueued}}, Artifacts: []Artifact{}, EvidencePack: &pack, CreatedAt: now, UpdatedAt: now}
	stored, created, err := s.store.CreateEvolutionJob(r.Context(), job)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if created {
		s.jobs.notify()
		writeJSON(w, 202, stored)
	} else {
		writeJSON(w, 200, stored)
	}
}
func (s *HTTPServer) executeAnalysisJob(ctx context.Context, lease EvolutionLease, job EvolutionJob) error {
	if job.Schema != "catena.analysis_job.v2" || (job.Pipeline != "rsi" && job.Pipeline != "memory") {
		return errors.New("legacy execution retired; create a V2 Analysis Job")
	}
	if job.EvidencePack == nil || !validEvolutionEvidencePack(*job.EvidencePack, job) {
		return errors.New("invalid retained Evidence Pack")
	}
	if job.Pipeline == "memory" {
		return s.executeMemoryJob(ctx, lease, job)
	}
	var payload map[string]any
	if len(job.Options) > 0 {
		if json.Unmarshal(job.Options, &payload) != nil {
			return errors.New("invalid RSI options")
		}
	}
	if payload == nil {
		payload = map[string]any{}
	}
	for key := range payload {
		switch key {
		case "case_id", "baseline", "algorithms", "composition", "budget", "validation_case_ids":
		default:
			return errors.New("unsupported RSI option")
		}
	}
	if _, ok := payload["case_id"].(string); !ok {
		return errors.New("a registered evaluation case_id is required for RSI")
	}
	if payload["baseline"] == nil {
		payload["baseline"] = map[string]string{"AGENTS.md": "Complete the supplied task faithfully."}
	}
	if payload["algorithms"] == nil {
		payload["algorithms"] = []any{map[string]any{"name": job.Adapter}}
	}
	credentials, err := s.evolutionModelCredentials(ctx, job.OwnerUserID)
	if err != nil {
		return err
	}
	evidence, _ := json.Marshal(map[string]any{"pack": job.EvidencePack, "objective": job.Objective})
	payload["evidence"] = string(evidence)
	payload["evidence_refs"] = evolutionJobTraceIDs(job)
	payload["schema"] = "catena.rsi_request.v1"
	payload["request_id"] = job.ID
	payload["model"] = map[string]string{"provider": credentials.Provider, "base_url": credentials.BaseURL, "model": credentials.Model, "api_key": credentials.APIKey}
	job.State = EvolutionJobRunning
	job.CurrentStage = "generate"
	job.Stages[0].State = EvolutionStageRunning
	if err = s.saveEvolutionProgress(ctx, lease, job); err != nil {
		return err
	}
	owner := job.OwnerUserID
	if owner == "" {
		owner = "local"
	}
	result, err := s.evolutionRuntime.execute(ctx, owner, job.ID, payload)
	if err != nil {
		return err
	}
	var out struct {
		Status     string            `json:"status"`
		Candidates []json.RawMessage `json:"candidates"`
		Trials     []json.RawMessage `json:"trials"`
		Validation []string          `json:"validation_case_ids"`
	}
	if err = json.Unmarshal(result, &out); err != nil || out.Status != "completed" || len(out.Candidates) == 0 {
		return errors.New("RSI execution failed; no candidate adopted")
	}
	job.Checkpoint = result
	job.Artifacts = []Artifact{}
	for i, payload := range out.Candidates {
		status := "unverified"
		if len(out.Validation) > 0 && len(out.Trials) > 0 {
			status = "review_required"
		}
		provenance, _ := json.Marshal(map[string]any{"pipeline": "rsi", "adapter": job.Adapter, "evidence_sha256": job.EvidencePack.SHA256, "evaluation": out.Trials, "execution_id": job.ID})
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", job.ID, i)))
		job.Artifacts = append(job.Artifacts, Artifact{ID: "artifact-" + hex.EncodeToString(digest[:16]), OwnerID: job.OwnerUserID, JobID: job.ID, Type: "rsi_candidate", SchemaVersion: "catena.artifact.v1", EvidenceRefs: evolutionJobTraceIDs(job), Payload: payload, Provenance: provenance, ValidationStatus: status, CreatedAt: time.Now().UTC()})
	}
	job.State = EvolutionJobCompleted
	job.CurrentStage = "complete"
	job.Stages[0].State = EvolutionStageCompleted
	return s.saveEvolutionProgress(ctx, lease, job)
}

func (m *EvolutionRuntimeManager) execute(ctx context.Context, owner, id string, payload any) (json.RawMessage, error) {
	type receipt struct {
		RequestID string          `json:"request_id"`
		State     string          `json:"state"`
		Output    json.RawMessage `json:"output"`
	}
	var out receipt
	status, err := m.call(ctx, owner, http.MethodPost, "/v2/rsi", payload, &out)
	for err == nil && status == 202 {
		select {
		case <-ctx.Done():
			m.cancelExecution(owner, id)
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
		status, err = m.call(ctx, owner, http.MethodGet, "/v2/executions/"+id, nil, &out)
	}
	if err != nil {
		if ctx.Err() != nil {
			m.cancelExecution(owner, id)
		}
		return nil, err
	}
	if out.RequestID != id || out.State != "completed" {
		return nil, errors.New("invalid or failed Engine receipt")
	}
	return out.Output, nil
}
func (s *HTTPServer) listArtifacts(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	owner := ""
	if u != nil {
		owner = u.ID
	}
	rows, e := s.store.ListArtifactsByOwner(r.Context(), owner, r.URL.Query().Get("type"), 100)
	if e != nil {
		writeProblem(w, 500, "Artifacts unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"artifacts": rows})
}
func (s *HTTPServer) evaluationCases(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var out any
	_, e := s.evolutionRuntime.call(r.Context(), traceOwnerID(u), http.MethodGet, "/v2/cases", nil, &out)
	if e != nil {
		writeProblem(w, 503, "Evaluation Cases unavailable")
		return
	}
	writeJSON(w, 200, out)
}
