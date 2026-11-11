package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type CreateAnalysisRequest struct {
	AnalysisJobID  string          `json:"analysis_job_id,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Pipeline       string          `json:"pipeline"`
	TraceID        string          `json:"trace_id,omitempty"`
	AgentID        string          `json:"agent_id,omitempty"`
	WindowStart    time.Time       `json:"window_start,omitempty"`
	WindowEnd      time.Time       `json:"window_end,omitempty"`
	Objective      string          `json:"objective,omitempty"`
	Options        json.RawMessage `json:"options,omitempty"`
}

func (s *HTTPServer) createAnalysisJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.analysisRuntime == nil || s.traces == nil {
		writeProblem(w, 503, "Engine and Trace storage are required")
		return
	}
	var req CreateAnalysisRequest
	if e := decodeJSON(w, r, &req); e != nil {
		writeProblem(w, 400, e.Error())
		return
	}
	if req.Pipeline != "improve" && req.Pipeline != "analyze" {
		writeProblem(w, 400, "supported pipelines: analyze, improve")
		return
	}
	if len(req.Objective) > 4000 || len(req.Options) > 128*1024 {
		writeProblem(w, 400, "analysis input is too large")
		return
	}
	if len(req.Options) > 0 {
		var options map[string]json.RawMessage
		if json.Unmarshal(req.Options, &options) != nil || options == nil {
			writeProblem(w, 400, "options must be an object")
			return
		}
		if req.Pipeline == "analyze" && len(options) != 0 {
			writeProblem(w, 400, "analysis does not accept improvement options")
			return
		}
		for key := range options {
			switch key {
			case "case_id", "baseline", "budget", "validation_case_ids":
			default:
				writeProblem(w, 400, "unsupported improvement option")
				return
			}
		}
	}
	if req.Pipeline == "analyze" && (req.AnalysisJobID != "") {
		writeProblem(w, 400, "analysis does not accept improvement options")
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
	var pack AnalysisEvidencePack
	var err error
	var analysisInput json.RawMessage
	if req.Pipeline == "improve" {
		if req.AnalysisJobID == "" || req.TraceID != "" || req.AgentID != "" || req.ConversationID != "" {
			writeProblem(w, 400, "improvement requires an explicit completed analysis_job_id, not new evidence")
			return
		}
		source, e := s.store.GetAnalysisJob(r.Context(), req.AnalysisJobID)
		if e != nil || source.OwnerUserID != owner {
			writeProblem(w, 404, "analysis unavailable")
			return
		}
		if source.Pipeline != "analyze" || source.State != AnalysisJobCompleted || source.EvidencePack == nil || !validAnalysisEvidencePack(*source.EvidencePack, source) || !json.Valid(source.Checkpoint) {
			writeProblem(w, 400, "a completed analysis with frozen evidence is required")
			return
		}
		pack = *source.EvidencePack
		analysisInput = append(json.RawMessage(nil), source.Checkpoint...)
	} else if req.ConversationID != "" {
		writeProblem(w, 400, "conversation extraction is retired; select Trace evidence")
		return
	} else if req.AgentID != "" {
		if req.TraceID != "" || req.WindowStart.IsZero() || !req.WindowEnd.After(req.WindowStart) || req.WindowEnd.Sub(req.WindowStart) > 31*24*time.Hour || req.WindowEnd.After(now.Add(5*time.Minute)) {
			writeProblem(w, 400, "invalid Agent Trace window")
			return
		}
		var summaries []TraceSummary
		summaries, err = s.traces.ListAgentTraces(r.Context(), traceOwnerID(user), req.AgentID, req.WindowStart, req.WindowEnd, maxAgentAnalysisTraceScan)
		if err == nil {
			selected := selectAgentAnalysisTraces(summaries, maxAgentAnalysisTraces)
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
		id, err = normalizedAnalysisTraceID(req.TraceID)
		if err == nil {
			var trace TraceDetail
			trace, err = s.traces.GetTrace(r.Context(), traceOwnerID(user), id)
			if err == nil {
				pack, err = buildAnalysisEvidencePack(AnalysisSourceTrace, id, &trace, now)
			}
		}
	}
	if err != nil {
		writeProblem(w, statusFor(err), "Evidence unavailable: "+bounded(err.Error(), 300))
		return
	}
	raw, _ := json.Marshal(req)
	hash := sha256.Sum256(append(raw, []byte(pack.SHA256)...))
	job := AnalysisJob{Schema: "catena.analysis_job.v2", ID: newID("analysis-job"), OwnerUserID: owner, Pipeline: req.Pipeline, Adapter: "codex-skill", SourceAnalysisJobID: req.AnalysisJobID, AnalysisInput: analysisInput, Options: req.Options, SourceKind: pack.SourceKind, SourceConversationID: pack.SourceConversationID, SourceTraceID: pack.SourceTraceID, SourceTraceIDs: pack.SourceTraceIDs, SourceAgentID: pack.SourceAgentID, SourceRuntimeKind: pack.SourceRuntimeKind, WindowStart: pack.WindowStart, WindowEnd: pack.WindowEnd, Objective: sanitizeAnalysisText(req.Objective, 4000), IdempotencyKey: key, RequestFingerprint: hex.EncodeToString(hash[:]), State: AnalysisJobQueued, Stages: []AnalysisStage{{Name: "generate", State: AnalysisStageQueued}}, Artifacts: []Artifact{}, EvidencePack: &pack, CreatedAt: now, UpdatedAt: now}
	stored, created, err := s.store.CreateAnalysisJob(r.Context(), job)
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
	_, e := s.analysisRuntime.call(r.Context(), traceOwnerID(u), http.MethodGet, "/v2/cases", nil, &out)
	if e != nil {
		writeProblem(w, 503, "Evaluation Cases unavailable")
		return
	}
	writeJSON(w, 200, out)
}
