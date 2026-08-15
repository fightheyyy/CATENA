package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	evolutionJobSchema             = "spiral.evolution_job.v1"
	evolutionCandidateStatus       = "draft/unverified"
	evolutionReviewScope           = "proposal_only"
	evolutionEvidenceSchema        = "catena.evolution_evidence_pack.v1"
	evolutionAgentEvidenceSchema   = "catena.agent_trace_set_evidence_pack.v1"
	evolutionReleaseAuthority      = "local_barena"
	evolutionTurnTimeout           = 2 * time.Minute
	evolutionStageTimeout          = evolutionTurnTimeout + 10*time.Second
	maxEvolutionEvidenceSpans      = 64
	maxEvolutionEvidenceEvents     = 32
	maxEvolutionEvidenceFieldBytes = 2 * 1024
	maxEvolutionEvidencePackBytes  = 256 * 1024
	maxAgentEvolutionTraceScan     = 100
	maxAgentEvolutionTraces        = 12
	maxAgentEvolutionEvidenceSpans = 64
)

var evolutionJobStages = []EvolutionStage{
	{Name: "inspector", Role: "inspector-cat", State: EvolutionStageQueued},
	{Name: "evolution", Role: "evolution-cat", State: EvolutionStageQueued},
	{Name: "reviewer", Role: "reviewer-cat", State: EvolutionStageQueued},
}

type inspectorTurnOutput struct {
	Finding EvolutionFinding `json:"finding"`
}

type candidateTurnOutput struct {
	Candidate EvolutionCandidate `json:"candidate"`
}

type reviewTurnOutput struct {
	Review EvolutionReview `json:"review"`
}

func (s *HTTPServer) createEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	run, err := s.store.GetRun(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(run.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Run not found")
		return
	}
	if !run.State.Terminal() {
		writeProblem(w, http.StatusConflict, "Evolution evidence requires a terminal Run")
		return
	}

	var request CreateEvolutionJobRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.TraceID = strings.TrimSpace(request.TraceID)
	request.Objective = strings.TrimSpace(request.Objective)
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	retained, err := s.store.RunHasTrace(r.Context(), run.ID, request.TraceID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !retained {
		writeProblem(w, http.StatusBadRequest, "trace_id is not retained by the source Run")
		return
	}
	var trace *TraceDetail
	if s.traces != nil {
		stored, traceErr := s.traces.GetTrace(r.Context(), traceOwnerID(user), request.TraceID)
		if traceErr != nil {
			writeProblem(w, statusFor(traceErr), traceErr.Error())
			return
		}
		trace = &stored
	}
	s.createEvolutionJobFromEvidence(
		w,
		r,
		user,
		EvolutionSourceRunTrace,
		&run,
		request.TraceID,
		request.Objective,
		trace,
	)
}

func (s *HTTPServer) createTraceEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.traces == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Trace storage is not configured")
		return
	}
	traceID, err := normalizedEvolutionTraceID(r.PathValue("trace_id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var request CreateTraceEvolutionJobRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Objective = strings.TrimSpace(request.Objective)
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	trace, err := s.traces.GetTrace(r.Context(), traceOwnerID(user), traceID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	sourceKind := EvolutionSourceTrace
	var run *Run
	if associated, found, findErr := s.findTerminalRunForTrace(r.Context(), user, traceID); findErr != nil {
		writeProblem(w, statusFor(findErr), findErr.Error())
		return
	} else if found {
		sourceKind = EvolutionSourceRunTrace
		run = &associated
	}
	s.createEvolutionJobFromEvidence(
		w,
		r,
		user,
		sourceKind,
		run,
		traceID,
		request.Objective,
		&trace,
	)
}

func (s *HTTPServer) createAgentEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.traces == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Trace storage is not configured")
		return
	}
	agentID, err := normalizedAgentID(r.PathValue("agent_id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var request CreateAgentEvolutionJobRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.WindowStart = request.WindowStart.UTC()
	request.WindowEnd = request.WindowEnd.UTC()
	request.Objective = strings.TrimSpace(request.Objective)
	request.OutputLanguage, err = normalizedEvolutionOutputLanguage(request.OutputLanguage)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := request.Validate(time.Now().UTC()); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if err := validateReplayIdempotencyKey(idempotencyKey); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	summaries, err := s.traces.ListAgentTraces(
		r.Context(), traceOwnerID(user), agentID,
		request.WindowStart, request.WindowEnd, maxAgentEvolutionTraceScan,
	)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Agent Trace query failed")
		return
	}
	if len(summaries) < 2 {
		writeProblem(w, http.StatusUnprocessableEntity, "At least two Traces are required to evolve an Agent")
		return
	}
	selected := selectAgentEvolutionTraces(summaries, maxAgentEvolutionTraces)
	details := make([]TraceDetail, 0, len(selected))
	for _, summary := range selected {
		detail, traceErr := s.traces.GetTrace(r.Context(), traceOwnerID(user), summary.TraceID)
		if traceErr != nil {
			writeProblem(w, statusFor(traceErr), traceErr.Error())
			return
		}
		if !traceSummaryBelongsToAgent(detail.Summary, agentID) {
			writeProblem(w, http.StatusConflict, "Agent Trace identity changed while evidence was being frozen")
			return
		}
		details = append(details, detail)
	}
	pack, err := buildAgentTraceSetEvidencePack(
		agentID, request.WindowStart, request.WindowEnd, details, len(summaries), time.Now().UTC(),
	)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	fingerprint, err := agentEvolutionSourceRequestFingerprint(
		agentID, request.WindowStart, request.WindowEnd, request.Objective, request.OutputLanguage,
	)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Evolution request could not be fingerprinted")
		return
	}
	now := time.Now().UTC()
	windowStart := request.WindowStart
	windowEnd := request.WindowEnd
	job := EvolutionJob{
		Schema:             evolutionJobSchema,
		ID:                 newID("evolution-job"),
		OwnerUserID:        traceOwnerID(user),
		SourceKind:         EvolutionSourceAgentTraceSet,
		SourceTraceIDs:     append([]string(nil), pack.SourceTraceIDs...),
		SourceAgentID:      agentID,
		WindowStart:        &windowStart,
		WindowEnd:          &windowEnd,
		Objective:          request.Objective,
		OutputLanguage:     request.OutputLanguage,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		State:              EvolutionJobQueued,
		Stages:             cloneEvolutionStages(evolutionJobStages),
		EvidencePack:       &pack,
		Boundary:           pack.Boundary,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	stored, created, err := s.store.CreateEvolutionJob(r.Context(), job)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if created {
		go s.executeEvolutionJob(stored.ID)
		writeJSON(w, http.StatusAccepted, stored)
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (s *HTTPServer) createEvolutionJobFromEvidence(
	w http.ResponseWriter,
	r *http.Request,
	user *User,
	sourceKind EvolutionSourceKind,
	run *Run,
	traceID string,
	objective string,
	trace *TraceDetail,
) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if err := validateReplayIdempotencyKey(idempotencyKey); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	sourceRunID := ""
	ownerUserID := ""
	if user != nil {
		ownerUserID = user.ID
	}
	if run != nil {
		sourceRunID = run.ID
		ownerUserID = run.OwnerUserID
	}
	events := []EngineEvent{}
	if sourceRunID != "" {
		var err error
		events, err = s.store.ListEventsAfter(r.Context(), sourceRunID, 0, 1000)
		if err != nil {
			writeProblem(w, statusFor(err), err.Error())
			return
		}
	}
	pack, err := buildEvolutionEvidencePack(sourceKind, run, traceID, trace, events, time.Now().UTC())
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	fingerprint, err := evolutionSourceRequestFingerprint(sourceKind, sourceRunID, traceID, objective)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Evolution request could not be fingerprinted")
		return
	}
	now := time.Now().UTC()
	job := EvolutionJob{
		Schema:             evolutionJobSchema,
		ID:                 newID("evolution-job"),
		OwnerUserID:        ownerUserID,
		SourceKind:         sourceKind,
		SourceRunID:        sourceRunID,
		SourceTraceID:      traceID,
		Objective:          objective,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		State:              EvolutionJobQueued,
		Stages:             cloneEvolutionStages(evolutionJobStages),
		EvidencePack:       &pack,
		Boundary:           pack.Boundary,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	stored, created, err := s.store.CreateEvolutionJob(r.Context(), job)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if created {
		go s.executeEvolutionJob(stored.ID)
		writeJSON(w, http.StatusAccepted, stored)
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (s *HTTPServer) findTerminalRunForTrace(
	ctx context.Context,
	user *User,
	traceID string,
) (Run, bool, error) {
	var (
		runs []Run
		err  error
	)
	if user == nil {
		runs, err = s.store.ListRuns(ctx, 1000)
	} else {
		runs, err = s.store.ListRunsByOwner(ctx, user.ID, 1000)
	}
	if err != nil {
		return Run{}, false, err
	}
	for _, run := range runs {
		if !run.State.Terminal() || !resourceOwnedBy(run.OwnerUserID, user) {
			continue
		}
		retained, err := s.store.RunHasTrace(ctx, run.ID, traceID)
		if err != nil {
			return Run{}, false, err
		}
		if retained {
			return run, true, nil
		}
	}
	return Run{}, false, nil
}

func (s *HTTPServer) listEvolutionJobs(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := evolutionListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var jobs []EvolutionJob
	if user == nil {
		jobs, err = s.store.ListEvolutionJobs(r.Context(), limit)
	} else {
		jobs, err = s.store.ListEvolutionJobsByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if jobs == nil {
		jobs = []EvolutionJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"evolution_jobs": jobs})
}

func (s *HTTPServer) getEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetEvolutionJob(r.Context(), r.PathValue("job_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Evolution Job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) deleteEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetEvolutionJob(r.Context(), r.PathValue("job_id"))
	if err != nil || !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Evolution Job not found")
		return
	}
	if !job.State.Terminal() {
		writeProblem(w, http.StatusConflict, "Evolution Job can only be deleted after it finishes")
		return
	}
	if err := s.store.DeleteEvolutionJob(r.Context(), job.OwnerUserID, job.ID); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) executeEvolutionJob(jobID string) {
	job, err := s.store.GetEvolutionJob(context.Background(), jobID)
	if err != nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.failEvolutionJob(&job, -1, "Evolution Job stopped unexpectedly")
		}
	}()
	if job.EvidencePack == nil || !validEvolutionEvidencePack(*job.EvidencePack, job) {
		s.failEvolutionJob(&job, -1, "stored Evolution Evidence Pack is unavailable")
		return
	}
	model, err := s.evolutionModelCredentials(context.Background(), job.OwnerUserID)
	if err != nil {
		s.failEvolutionJob(&job, -1, bounded(err.Error(), 1000))
		return
	}
	evidence, err := json.Marshal(job.EvidencePack)
	if err != nil {
		s.failEvolutionJob(&job, -1, "Evolution Evidence Pack could not be encoded")
		return
	}

	inspectorPrompt := buildInspectorPrompt(job, evidence)
	inspectorRaw, ok := s.runEvolutionStage(&job, 0, inspectorPrompt, model)
	if !ok {
		return
	}
	finding := inspectorOutput(inspectorRaw)
	job.Finding = &finding
	job.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateEvolutionJob(context.Background(), job); err != nil {
		return
	}

	evolutionPrompt := buildCandidatePrompt(job)
	candidateRaw, ok := s.runEvolutionStage(&job, 1, evolutionPrompt, model)
	if !ok {
		return
	}
	candidate := candidateOutput(candidateRaw, job.SourceAgentID)
	enrichEvolutionCandidate(&candidate, job)
	job.Candidate = &candidate
	job.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateEvolutionJob(context.Background(), job); err != nil {
		return
	}

	reviewerPrompt := buildReviewerPrompt(job, evidence)
	reviewerRaw, ok := s.runEvolutionStage(&job, 2, reviewerPrompt, model)
	if !ok {
		return
	}
	review := reviewOutput(reviewerRaw)
	job.Review = &review
	job.State = EvolutionJobCompleted
	job.CurrentStage = "complete"
	job.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateEvolutionJob(context.Background(), job)
}

func (s *HTTPServer) runEvolutionStage(
	job *EvolutionJob,
	index int,
	prompt string,
	model EvolutionModelCredentials,
) (json.RawMessage, bool) {
	now := time.Now().UTC()
	job.State = EvolutionJobRunning
	job.CurrentStage = job.Stages[index].Name
	job.Stages[index].State = EvolutionStageRunning
	job.Stages[index].StartedAt = &now
	job.UpdatedAt = now
	if err := s.store.UpdateEvolutionJob(context.Background(), *job); err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), evolutionStageTimeout)
	defer cancel()
	raw, err := s.evolutionRuntime.RunRoleTurn(ctx, EvolutionRoleTurnInput{
		RequestID: newID(job.Stages[index].Name),
		RunID:     job.ID,
		Role:      job.Stages[index].Role,
		Prompt:    prompt,
		Timeout:   evolutionTurnTimeout,
		Model:     model,
	})
	if err != nil {
		s.failEvolutionJob(job, index, bounded(err.Error(), 1000))
		return nil, false
	}
	finishedAt := time.Now().UTC()
	job.Stages[index].State = EvolutionStageCompleted
	job.Stages[index].RawOutput = sanitizeEvolutionJSON(raw, 128*1024)
	job.Stages[index].FinishedAt = &finishedAt
	job.UpdatedAt = finishedAt
	if err := s.store.UpdateEvolutionJob(context.Background(), *job); err != nil {
		return nil, false
	}
	return raw, true
}

func (s *HTTPServer) failEvolutionJob(job *EvolutionJob, stageIndex int, detail string) {
	now := time.Now().UTC()
	job.State = EvolutionJobFailed
	job.Error = bounded(strings.TrimSpace(detail), 1000)
	job.UpdatedAt = now
	if stageIndex >= 0 && stageIndex < len(job.Stages) {
		job.CurrentStage = job.Stages[stageIndex].Name
		job.Stages[stageIndex].State = EvolutionStageFailed
		job.Stages[stageIndex].Error = job.Error
		job.Stages[stageIndex].FinishedAt = &now
	}
	_ = s.store.UpdateEvolutionJob(context.Background(), *job)
}

func evolutionSourceRequestFingerprint(
	sourceKind EvolutionSourceKind,
	sourceRunID string,
	traceID string,
	objective string,
) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"source_kind":     sourceKind,
		"source_run_id":   sourceRunID,
		"source_trace_id": traceID,
		"objective":       objective,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func agentEvolutionSourceRequestFingerprint(
	agentID string,
	windowStart time.Time,
	windowEnd time.Time,
	objective string,
	outputLanguage string,
) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"source_kind":     EvolutionSourceAgentTraceSet,
		"source_agent_id": agentID,
		"window_start":    windowStart.UTC(),
		"window_end":      windowEnd.UTC(),
		"objective":       objective,
		"output_language": outputLanguage,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func normalizedEvolutionOutputLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "zh", "zh-cn", "zh-hans":
		return "zh-CN", nil
	case "en", "en-us", "en-gb":
		return "en", nil
	default:
		return "", errors.New("output_language must be zh-CN or en")
	}
}

func evolutionOutputLanguageInstruction(job EvolutionJob) string {
	if job.OutputLanguage == "en" {
		return "Write every human-readable title, summary, explanation, and asset body in English. Keep protocol keys, file paths, code identifiers, commands, and required schema values unchanged."
	}
	return "所有面向人的标题、摘要、说明和资产正文都必须使用简体中文。协议字段、文件路径、代码标识符、命令及规范要求的固定值保持原样。"
}

func selectAgentEvolutionTraces(values []TraceSummary, limit int) []TraceSummary {
	selected := append([]TraceSummary(nil), values...)
	sort.SliceStable(selected, func(i, j int) bool {
		leftHasError := selected[i].ErrorCount > 0
		rightHasError := selected[j].ErrorCount > 0
		if leftHasError != rightHasError {
			return leftHasError
		}
		if !selected[i].EndTime.Equal(selected[j].EndTime) {
			return selected[i].EndTime.After(selected[j].EndTime)
		}
		return selected[i].TraceID < selected[j].TraceID
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	return selected
}

func normalizedEvolutionTraceID(value string) (string, error) {
	traceID := strings.ToLower(strings.TrimSpace(value))
	if len(traceID) != 32 {
		return "", fmt.Errorf("trace_id must be 32 hexadecimal characters")
	}
	if _, err := hex.DecodeString(traceID); err != nil {
		return "", fmt.Errorf("trace_id must be 32 hexadecimal characters")
	}
	return traceID, nil
}

func buildEvolutionEvidencePack(
	sourceKind EvolutionSourceKind,
	run *Run,
	traceID string,
	trace *TraceDetail,
	events []EngineEvent,
	createdAt time.Time,
) (EvolutionEvidencePack, error) {
	if !sourceKind.Valid() || strings.TrimSpace(traceID) == "" {
		return EvolutionEvidencePack{}, fmt.Errorf("Evolution evidence source is invalid")
	}
	if sourceKind == EvolutionSourceTrace && run != nil {
		return EvolutionEvidencePack{}, fmt.Errorf("Trace-only Evolution cannot include a synthetic Run")
	}
	if sourceKind == EvolutionSourceRunTrace && run == nil {
		return EvolutionEvidencePack{}, fmt.Errorf("Run-backed Evolution requires a retained Run")
	}
	if trace == nil && sourceKind == EvolutionSourceTrace {
		return EvolutionEvidencePack{}, fmt.Errorf("stored Trace evidence is required")
	}
	pack := EvolutionEvidencePack{
		Schema:        evolutionEvidenceSchema,
		SourceKind:    sourceKind,
		SourceTraceID: traceID,
		Spans:         []EvolutionEvidenceSpan{},
		RunEvents:     []EngineEvent{},
		Redacted:      true,
		Boundary: EvolutionEvidenceBoundary{
			TargetAgentExecutedByCatena: false,
			CreatesRelease:              false,
			ReleaseAuthority:            evolutionReleaseAuthority,
			CandidateStatus:             evolutionCandidateStatus,
			ReviewScope:                 evolutionReviewScope,
		},
		CreatedAt: createdAt.UTC(),
	}
	if run != nil {
		if len(run.Input) > 24*1024 || len(run.Runtime) > 12*1024 {
			pack.Truncated = true
		}
		pack.SourceRunID = run.ID
		pack.Run = &EvolutionEvidenceRun{
			RunID:     run.ID,
			Origin:    run.Origin,
			Operation: run.Operation,
			State:     run.State,
			Input:     sanitizeEvolutionJSON(run.Input, 24*1024),
			Runtime:   sanitizeEvolutionJSON(run.Runtime, 12*1024),
			Error:     bounded(redactMemoryText(run.Error), 2000),
			CreatedAt: run.CreatedAt,
			UpdatedAt: run.UpdatedAt,
		}
	}
	if trace != nil {
		if trace.Summary.TraceID != traceID {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace identity does not match the Evolution source")
		}
		if len(trace.Spans) == 0 {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace contains no spans")
		}
		pack.TraceSummary = trace.Summary
		pack.TraceSummary.RootName = bounded(redactMemoryText(pack.TraceSummary.RootName), 1000)
		pack.TraceSummary.ServiceName = bounded(redactMemoryText(pack.TraceSummary.ServiceName), 1000)
		pack.TraceSummary.Model = bounded(redactMemoryText(pack.TraceSummary.Model), 1000)
		pack.TotalSpanCount = trace.Summary.SpanCount
		if pack.TotalSpanCount == 0 {
			pack.TotalSpanCount = uint64(len(trace.Spans))
		}
		spans := append([]TraceSpan(nil), trace.Spans...)
		sort.SliceStable(spans, func(i, j int) bool {
			if spans[i].StartTime.Equal(spans[j].StartTime) {
				return spans[i].SpanID < spans[j].SpanID
			}
			return spans[i].StartTime.Before(spans[j].StartTime)
		})
		if len(spans) > maxEvolutionEvidenceSpans {
			spans = spans[:maxEvolutionEvidenceSpans]
			pack.Truncated = true
		}
		for _, span := range spans {
			if span.TraceID != traceID {
				return EvolutionEvidencePack{}, fmt.Errorf("stored Trace contains a mismatched span")
			}
			eventNames := make([]string, 0, len(span.Events))
			for _, event := range span.Events {
				if name := bounded(redactMemoryText(event.Name), 256); name != "" {
					eventNames = append(eventNames, name)
				}
				if len(eventNames) == 16 {
					pack.Truncated = pack.Truncated || len(span.Events) > len(eventNames)
					break
				}
			}
			input := sanitizeEvolutionText(span.Input, maxEvolutionEvidenceFieldBytes)
			output := sanitizeEvolutionText(span.Output, maxEvolutionEvidenceFieldBytes)
			if len(span.Input) > len(input) || len(span.Output) > len(output) {
				pack.Truncated = true
			}
			pack.Spans = append(pack.Spans, EvolutionEvidenceSpan{
				SpanID:        span.SpanID,
				ParentSpanID:  span.ParentSpanID,
				Name:          bounded(redactMemoryText(span.Name), 1000),
				ServiceName:   bounded(redactMemoryText(span.ServiceName), 1000),
				StartTime:     span.StartTime,
				EndTime:       span.EndTime,
				StatusCode:    span.StatusCode,
				StatusMessage: bounded(redactMemoryText(span.StatusMessage), 1000),
				Model:         bounded(redactMemoryText(span.Model), 1000),
				ToolName: bounded(redactMemoryText(firstAttributeString(
					span.Attributes,
					"tool.name",
					"gen_ai.tool.name",
					"tool.call.name",
					"xiaoba.tool.name",
				)), 256),
				Input:      input,
				Output:     output,
				EventNames: eventNames,
			})
		}
	}
	pack.IncludedSpanCount = len(pack.Spans)
	relevant := make([]EngineEvent, 0)
	for _, event := range events {
		if event.TraceID == traceID || event.Kind == "terminal" {
			copy := event
			if len(copy.Payload) > 12*1024 {
				pack.Truncated = true
			}
			copy.Payload = sanitizeEvolutionJSON(copy.Payload, 12*1024)
			relevant = append(relevant, copy)
		}
	}
	if len(relevant) > maxEvolutionEvidenceEvents {
		relevant = append(
			append([]EngineEvent(nil), relevant[:16]...),
			relevant[len(relevant)-16:]...,
		)
		pack.Truncated = true
	}
	pack.RunEvents = relevant
	for {
		encoded, err := evolutionEvidenceDigestInput(pack)
		if err != nil {
			return EvolutionEvidencePack{}, err
		}
		if len(encoded) <= maxEvolutionEvidencePackBytes {
			digest := sha256.Sum256(encoded)
			pack.SHA256 = hex.EncodeToString(digest[:])
			return pack, nil
		}
		pack.Truncated = true
		if len(pack.Spans) > 1 {
			pack.Spans = pack.Spans[:len(pack.Spans)-1]
			pack.IncludedSpanCount = len(pack.Spans)
			continue
		}
		if len(pack.RunEvents) > 1 {
			pack.RunEvents = pack.RunEvents[:len(pack.RunEvents)-1]
			continue
		}
		return EvolutionEvidencePack{}, fmt.Errorf("Evolution Evidence Pack exceeds the safe prompt limit")
	}
}

func buildAgentTraceSetEvidencePack(
	agentID string,
	windowStart time.Time,
	windowEnd time.Time,
	traces []TraceDetail,
	totalTraceCount int,
	createdAt time.Time,
) (EvolutionEvidencePack, error) {
	if strings.TrimSpace(agentID) == "" || len(traces) < 2 || !windowEnd.After(windowStart) {
		return EvolutionEvidencePack{}, fmt.Errorf("Agent Trace Set evidence source is invalid")
	}
	windowStart = windowStart.UTC()
	windowEnd = windowEnd.UTC()
	pack := EvolutionEvidencePack{
		Schema:          evolutionAgentEvidenceSchema,
		SourceKind:      EvolutionSourceAgentTraceSet,
		SourceAgentID:   agentID,
		WindowStart:     &windowStart,
		WindowEnd:       &windowEnd,
		SourceTraceIDs:  []string{},
		Spans:           []EvolutionEvidenceSpan{},
		Traces:          []EvolutionEvidenceTrace{},
		RunEvents:       []EngineEvent{},
		TotalTraceCount: totalTraceCount,
		Redacted:        true,
		Boundary: EvolutionEvidenceBoundary{
			TargetAgentExecutedByCatena: false,
			CreatesRelease:              false,
			ReleaseAuthority:            evolutionReleaseAuthority,
			CandidateStatus:             evolutionCandidateStatus,
			ReviewScope:                 evolutionReviewScope,
		},
		CreatedAt: createdAt.UTC(),
	}
	if pack.TotalTraceCount < len(traces) {
		pack.TotalTraceCount = len(traces)
	}
	pack.Truncated = pack.TotalTraceCount > len(traces)
	perTraceBudget := maxAgentEvolutionEvidenceSpans / len(traces)
	if perTraceBudget < 1 {
		perTraceBudget = 1
	}
	if perTraceBudget > 12 {
		perTraceBudget = 12
	}
	for _, trace := range traces {
		if trace.Summary.TraceID == "" || !traceSummaryBelongsToAgent(trace.Summary, agentID) || len(trace.Spans) == 0 {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace does not belong to the selected Agent")
		}
		if trace.Summary.EndTime.Before(windowStart) || trace.Summary.StartTime.After(windowEnd) {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace is outside the selected Agent window")
		}
		legacy, err := buildEvolutionEvidencePack(
			EvolutionSourceTrace, nil, trace.Summary.TraceID, &trace, nil, createdAt,
		)
		if err != nil {
			return EvolutionEvidencePack{}, err
		}
		spans := selectAgentEvidenceSpans(legacy.Spans, perTraceBudget)
		traceEvidence := EvolutionEvidenceTrace{
			Summary:           legacy.TraceSummary,
			Spans:             spans,
			IncludedSpanCount: len(spans),
			TotalSpanCount:    legacy.TotalSpanCount,
			Truncated:         legacy.Truncated || len(spans) < len(legacy.Spans),
		}
		pack.SourceTraceIDs = append(pack.SourceTraceIDs, trace.Summary.TraceID)
		pack.Traces = append(pack.Traces, traceEvidence)
		pack.IncludedSpanCount += traceEvidence.IncludedSpanCount
		pack.TotalSpanCount += traceEvidence.TotalSpanCount
		pack.Truncated = pack.Truncated || traceEvidence.Truncated
	}
	pack.IncludedTraceCount = len(pack.Traces)
	for {
		encoded, err := evolutionEvidenceDigestInput(pack)
		if err != nil {
			return EvolutionEvidencePack{}, err
		}
		if len(encoded) <= maxEvolutionEvidencePackBytes {
			digest := sha256.Sum256(encoded)
			pack.SHA256 = hex.EncodeToString(digest[:])
			return pack, nil
		}
		pack.Truncated = true
		trimmed := false
		for index := len(pack.Traces) - 1; index >= 0; index-- {
			if len(pack.Traces[index].Spans) > 1 {
				pack.Traces[index].Spans = pack.Traces[index].Spans[:len(pack.Traces[index].Spans)-1]
				pack.Traces[index].IncludedSpanCount--
				pack.Traces[index].Truncated = true
				pack.IncludedSpanCount--
				trimmed = true
				break
			}
		}
		if trimmed {
			continue
		}
		if len(pack.Traces) > 1 {
			last := len(pack.Traces) - 1
			pack.IncludedSpanCount -= pack.Traces[last].IncludedSpanCount
			pack.Traces = pack.Traces[:last]
			pack.SourceTraceIDs = pack.SourceTraceIDs[:last]
			pack.IncludedTraceCount = len(pack.Traces)
			continue
		}
		return EvolutionEvidencePack{}, fmt.Errorf("Agent Trace Set Evidence Pack exceeds the safe prompt limit")
	}
}

func selectAgentEvidenceSpans(values []EvolutionEvidenceSpan, limit int) []EvolutionEvidenceSpan {
	selected := append([]EvolutionEvidenceSpan(nil), values...)
	sort.SliceStable(selected, func(i, j int) bool {
		leftPriority := agentEvidenceSpanPriority(selected[i])
		rightPriority := agentEvidenceSpanPriority(selected[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if !selected[i].StartTime.Equal(selected[j].StartTime) {
			return selected[i].StartTime.Before(selected[j].StartTime)
		}
		return selected[i].SpanID < selected[j].SpanID
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if !selected[i].StartTime.Equal(selected[j].StartTime) {
			return selected[i].StartTime.Before(selected[j].StartTime)
		}
		return selected[i].SpanID < selected[j].SpanID
	})
	return selected
}

func agentEvidenceSpanPriority(span EvolutionEvidenceSpan) int {
	if span.StatusCode == 2 {
		return 0
	}
	if span.ToolName != "" {
		return 1
	}
	if span.ParentSpanID == "" {
		return 2
	}
	if span.Input != "" || span.Output != "" {
		return 3
	}
	return 4
}

func evolutionEvidenceDigestInput(pack EvolutionEvidencePack) ([]byte, error) {
	pack.SHA256 = ""
	encoded, err := json.Marshal(pack)
	if err != nil {
		return nil, err
	}
	// PostgreSQL JSONB deliberately discards whitespace and object-key order,
	// including inside json.RawMessage fields. Hash a canonical decoded value so
	// an Evidence Pack keeps the same identity after a durable round trip.
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func validEvolutionEvidencePack(pack EvolutionEvidencePack, job EvolutionJob) bool {
	if pack.SourceKind != job.SourceKind ||
		pack.Boundary.TargetAgentExecutedByCatena || pack.Boundary.CreatesRelease ||
		pack.Boundary.ReleaseAuthority != evolutionReleaseAuthority ||
		pack.Boundary.CandidateStatus != evolutionCandidateStatus ||
		pack.Boundary.ReviewScope != evolutionReviewScope {
		return false
	}
	if job.SourceKind == EvolutionSourceAgentTraceSet {
		if pack.Schema != evolutionAgentEvidenceSchema ||
			pack.SourceAgentID == "" || pack.SourceAgentID != job.SourceAgentID ||
			pack.WindowStart == nil || pack.WindowEnd == nil ||
			job.WindowStart == nil || job.WindowEnd == nil ||
			!pack.WindowStart.Equal(*job.WindowStart) || !pack.WindowEnd.Equal(*job.WindowEnd) ||
			len(pack.SourceTraceIDs) < 2 ||
			!equalStringSlices(pack.SourceTraceIDs, job.SourceTraceIDs) ||
			pack.IncludedTraceCount != len(pack.Traces) || len(pack.Traces) < 2 {
			return false
		}
	} else if pack.Schema != evolutionEvidenceSchema ||
		pack.SourceRunID != job.SourceRunID || pack.SourceTraceID != job.SourceTraceID {
		return false
	}
	encoded, err := evolutionEvidenceDigestInput(pack)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(encoded)
	return pack.SHA256 == hex.EncodeToString(digest[:])
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sanitizeEvolutionJSON(value json.RawMessage, max int) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return boundedEvolutionJSON(value, max, "invalid evidence JSON omitted")
	}
	encoded, err := json.Marshal(sanitizeEvolutionValue("", decoded))
	if err != nil {
		return json.RawMessage(`{"omitted":true,"reason":"evidence sanitization failed"}`)
	}
	return boundedEvolutionJSON(encoded, max, "evidence exceeded prompt limit")
}

func sanitizeEvolutionText(value string, max int) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) == nil {
		if encoded, err := json.Marshal(sanitizeEvolutionValue("", decoded)); err == nil {
			return bounded(string(encoded), max)
		}
	}
	return bounded(redactMemoryText(value), max)
}

func sanitizeEvolutionValue(key string, value any) any {
	normalized := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(key))
	for _, marker := range []string{
		"authorization", "api_key", "access_token", "password", "secret",
		"chain_of_thought", "hidden_reasoning", "reasoning_content", "reasoning",
	} {
		if strings.Contains(normalized, marker) {
			return "[REDACTED]"
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			result[childKey] = sanitizeEvolutionValue(childKey, childValue)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, sanitizeEvolutionValue(key, child))
		}
		return result
	case string:
		return redactMemoryText(typed)
	default:
		return value
	}
}

func buildInspectorPrompt(job EvolutionJob, evidence json.RawMessage) string {
	focus := job.Objective
	if focus == "" {
		focus = "Find one repeated or high-impact evidence-backed failure mode or behavioral boundary across this Agent Trace Set."
	}
	return fmt.Sprintf(`You are InspectorCat in Catena. Analyze only the retained execution evidence below.
Do not invent tool calls, artifacts, verification, or outcomes. Focus: %s
Output language: %s
Return one JSON object only with this exact shape:
{"finding":{"title":"...","summary":"...","severity":"low|medium|high|critical|unknown","evidence":["specific retained fact"]}}
	Do not propose Memory, Case, or a Replay workflow. Conversation owns memory; EvolutionCat owns Agent assets.
	Catena did not execute the target Agent and this workflow cannot create a Release decision.
	Source Agent: %s
	Source Traces: %s
	Source Run (optional): %s
	Evidence:
	%s`, focus, evolutionOutputLanguageInstruction(job), job.SourceAgentID, strings.Join(evolutionJobTraceIDs(job), ","), job.SourceRunID, evidence)
}

func buildCandidatePrompt(job EvolutionJob) string {
	inputs, _ := json.Marshal(map[string]any{
		"finding":         job.Finding,
		"source_agent_id": job.SourceAgentID,
	})
	return fmt.Sprintf(`You are EvolutionCat in Catena's XiaoBaOS Evolution Runtime. Produce one small, reusable Agent asset that directly prevents the accepted failure mode.
	Output language: %s
	Return one JSON object only with this exact shape:
	{"candidate":{"kind":"agent_md|skill|role","title":"...","summary":"...","content":{"root":"...","files":[{"path":"...","content":"..."}]}}}
	The asset must be an immediately usable repository file or package, not an optimization report.
	Use exactly one of these XiaoBaOS-compatible contracts:
	- agent_md: root is "agent.md" and files contains exactly one file whose path is "agent.md". Its Markdown heading and instructions must use the requested output language.
	- skill: root is "skills/<kebab-name>" and files must contain "skills/<kebab-name>/SKILL.md" with YAML frontmatter name and description. It may also contain text files under scripts/, references/, or assets/ when they are necessary for the capability.
	- role: root is "roles/<kebab-name>". A Role is a complete specialist package above Skills, not a Markdown persona. Files must contain role.json and prompts/<prompt-file>.md; role.json must declare name, displayName, description and promptFile. It may define evidence-supported tool policy, confirmation gates and role-local skills/<skill-name>/SKILL.md. All Roles reuse the XiaoBaOS Agent Runtime.
	Every file path must stay below the declared root. Write concrete instructions, triggers, expected behavior, and failure guards. Keep the package narrow enough to review in one sitting. Do not paste Trace IDs or analysis prose into file bodies. Never invent tool names: omit optional tool policy fields when the retained evidence does not justify them.
	Never emit Memory or Case: Conversation owns memory, and Trace Farm owns Agent assets. Do not claim the asset was installed, applied, replayed, verified, or released.
Analysis:
%s`, evolutionOutputLanguageInstruction(job), inputs)
}

func buildReviewerPrompt(job EvolutionJob, evidence json.RawMessage) string {
	inputs, _ := json.Marshal(map[string]any{
		"finding":   job.Finding,
		"candidate": job.Candidate,
	})
	return fmt.Sprintf(`You are ReviewerCat in Catena. Review whether this proposal is coherent and grounded in the retained evidence.
Output language: %s
Return one JSON object only with this exact shape:
{"review":{"verdict":"pass|fail|blocked","summary":"..."}}
	This is grounding review only. Do not invent Replay, verification, adoption, or a Release decision.
Proposal:
%s
Retained evidence:
%s`, evolutionOutputLanguageInstruction(job), inputs, evidence)
}

func inspectorOutput(raw json.RawMessage) EvolutionFinding {
	var parsed inspectorTurnOutput
	if decodeRoleJSON(raw, &parsed) && validEvolutionFinding(parsed.Finding) {
		return sanitizeEvolutionFinding(parsed.Finding)
	}
	return EvolutionFinding{
		Title:    "Unstructured InspectorCat result",
		Summary:  "InspectorCat returned output that requires human interpretation; its raw response is retained with the stage.",
		Severity: "unknown",
		Evidence: []string{"Raw InspectorCat output is retained in the inspector stage."},
	}
}

func enrichEvolutionCandidate(candidate *EvolutionCandidate, job EvolutionJob) {
	candidate.Status = evolutionCandidateStatus
	candidate.SourceRunID = job.SourceRunID
	candidate.SourceTraceID = job.SourceTraceID
	candidate.SourceTraceIDs = evolutionJobTraceIDs(job)
	candidate.SourceAgentID = job.SourceAgentID
	if job.EvidencePack != nil {
		candidate.EvidencePackSHA256 = job.EvidencePack.SHA256
	}
}

func evolutionJobTraceIDs(job EvolutionJob) []string {
	if len(job.SourceTraceIDs) > 0 {
		return append([]string(nil), job.SourceTraceIDs...)
	}
	if job.SourceTraceID != "" {
		return []string{job.SourceTraceID}
	}
	return []string{}
}

func validEvolutionJobSource(job EvolutionJob) bool {
	switch job.SourceKind {
	case EvolutionSourceTrace:
		return job.SourceTraceID != "" && job.SourceRunID == "" &&
			job.SourceAgentID == "" && len(job.SourceTraceIDs) == 0
	case EvolutionSourceRunTrace:
		return job.SourceTraceID != "" && job.SourceRunID != "" &&
			job.SourceAgentID == "" && len(job.SourceTraceIDs) == 0
	case EvolutionSourceAgentTraceSet:
		return job.SourceRunID == "" && job.SourceTraceID == "" &&
			job.SourceAgentID != "" && len(job.SourceTraceIDs) >= 2 &&
			job.WindowStart != nil && job.WindowEnd != nil && job.WindowEnd.After(*job.WindowStart)
	default:
		return false
	}
}

func evolutionJobSourceKey(job EvolutionJob) string {
	if job.SourceKind == EvolutionSourceAgentTraceSet {
		return "agent:" + job.SourceAgentID
	}
	return "trace:" + job.SourceTraceID
}

func candidateOutput(raw json.RawMessage, sourceAgentID string) EvolutionCandidate {
	var parsed candidateTurnOutput
	if decodeRoleJSON(raw, &parsed) && validCurrentEvolutionCandidate(parsed.Candidate, sourceAgentID) {
		parsed.Candidate.ID = newID("candidate")
		parsed.Candidate.Title = bounded(redactMemoryText(strings.TrimSpace(parsed.Candidate.Title)), 160)
		parsed.Candidate.Summary = bounded(redactMemoryText(strings.TrimSpace(parsed.Candidate.Summary)), 4000)
		parsed.Candidate.Content = sanitizeEvolutionJSON(
			parsed.Candidate.Content,
			128*1024,
		)
		parsed.Candidate.Status = evolutionCandidateStatus
		return parsed.Candidate
	}
	return EvolutionCandidate{
		ID:      newID("candidate"),
		Kind:    EvolutionCandidateAgentMD,
		Title:   "Unclassified EvolutionCat draft",
		Summary: "EvolutionCat returned an invalid Agent asset. Review the retained stage output before use.",
		Content: json.RawMessage(`{"root":"agent.md","files":[{"path":"agent.md","content":"# Human review required\n\nEvolutionCat returned an invalid Agent asset. Review the retained stage output before use."}]}`),
		Status:  evolutionCandidateStatus,
	}
}

func reviewOutput(raw json.RawMessage) EvolutionReview {
	var parsed reviewTurnOutput
	if decodeRoleJSON(raw, &parsed) && validEvolutionReview(parsed.Review) {
		parsed.Review.Summary = bounded(redactMemoryText(strings.TrimSpace(parsed.Review.Summary)), 4000)
		parsed.Review.Scope = evolutionReviewScope
		parsed.Review.CandidateStatus = evolutionCandidateStatus
		return parsed.Review
	}
	return EvolutionReview{
		Verdict:         "blocked",
		Summary:         "ReviewerCat returned unstructured output; no proposal acceptance or verification is claimed.",
		Scope:           evolutionReviewScope,
		CandidateStatus: evolutionCandidateStatus,
	}
}

func decodeRoleJSON(raw json.RawMessage, destination any) bool {
	var envelope struct {
		Assistant struct {
			Content string `json:"content"`
		} `json:"assistant"`
	}
	if json.Unmarshal(raw, &envelope) != nil ||
		strings.TrimSpace(envelope.Assistant.Content) == "" {
		return json.Unmarshal(raw, destination) == nil
	}
	content := strings.TrimSpace(envelope.Assistant.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if start, end := strings.Index(content, "{"), strings.LastIndex(content, "}"); start >= 0 && end >= start {
		content = content[start : end+1]
	}
	return json.Unmarshal([]byte(content), destination) == nil
}

func validEvolutionFinding(value EvolutionFinding) bool {
	severity := strings.ToLower(strings.TrimSpace(value.Severity))
	return strings.TrimSpace(value.Title) != "" &&
		strings.TrimSpace(value.Summary) != "" &&
		len(value.Evidence) > 0 &&
		(severity == "low" || severity == "medium" || severity == "high" ||
			severity == "critical" || severity == "unknown")
}

func sanitizeEvolutionFinding(value EvolutionFinding) EvolutionFinding {
	value.Title = bounded(redactMemoryText(strings.TrimSpace(value.Title)), 160)
	value.Summary = bounded(redactMemoryText(strings.TrimSpace(value.Summary)), 4000)
	value.Severity = strings.ToLower(strings.TrimSpace(value.Severity))
	evidence := make([]string, 0, len(value.Evidence))
	for _, fact := range value.Evidence {
		fact = bounded(redactMemoryText(strings.TrimSpace(fact)), 1000)
		if fact != "" {
			evidence = append(evidence, fact)
		}
		if len(evidence) == 20 {
			break
		}
	}
	value.Evidence = evidence
	return value
}

func validEvolutionCandidate(value EvolutionCandidate) bool {
	return value.Kind.Valid() &&
		strings.TrimSpace(value.Title) != "" &&
		strings.TrimSpace(value.Summary) != "" &&
		len(value.Content) > 0 && json.Valid(value.Content)
}

func validCurrentEvolutionCandidate(value EvolutionCandidate, _ string) bool {
	if !validEvolutionCandidate(value) {
		return false
	}
	switch value.Kind {
	case EvolutionCandidateAgentMD:
		return validAgentMDPackage(value.Content)
	case EvolutionCandidateSkill:
		return validSkillPackage(value.Content)
	case EvolutionCandidateRole:
		return validRolePackage(value.Content)
	default:
		return false
	}
}

type portableAssetFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type portableAssetPackage struct {
	Root  string              `json:"root"`
	Files []portableAssetFile `json:"files"`
}

func decodePortableAssetPackage(raw json.RawMessage) (portableAssetPackage, map[string]string, bool) {
	var content portableAssetPackage
	if json.Unmarshal(raw, &content) != nil || len(content.Files) == 0 || len(content.Files) > 24 {
		return portableAssetPackage{}, nil, false
	}
	content.Root = strings.TrimSuffix(strings.TrimSpace(content.Root), "/")
	if !validRelativeAssetPath(content.Root) {
		return portableAssetPackage{}, nil, false
	}
	files := make(map[string]string, len(content.Files))
	for index := range content.Files {
		filePath := strings.TrimSpace(content.Files[index].Path)
		body := content.Files[index].Content
		if !validRelativeAssetPath(filePath) || strings.TrimSpace(body) == "" ||
			(filePath != content.Root && !strings.HasPrefix(filePath, content.Root+"/")) {
			return portableAssetPackage{}, nil, false
		}
		if _, duplicate := files[filePath]; duplicate {
			return portableAssetPackage{}, nil, false
		}
		files[filePath] = body
	}
	return content, files, true
}

func validRelativeAssetPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validAgentMDPackage(raw json.RawMessage) bool {
	content, files, ok := decodePortableAssetPackage(raw)
	return ok && content.Root == "agent.md" && len(files) == 1 && strings.TrimSpace(files["agent.md"]) != ""
}

func validSkillPackage(raw json.RawMessage) bool {
	content, files, ok := decodePortableAssetPackage(raw)
	parts := strings.Split(content.Root, "/")
	if !ok || len(parts) != 2 || parts[0] != "skills" || !validAssetName(parts[1]) {
		return false
	}
	skill, exists := files[content.Root+"/SKILL.md"]
	return exists && frontmatterValue(skill, "name") == parts[1] && frontmatterValue(skill, "description") != ""
}

func validRolePackage(raw json.RawMessage) bool {
	content, files, ok := decodePortableAssetPackage(raw)
	parts := strings.Split(content.Root, "/")
	if !ok || len(parts) != 2 || parts[0] != "roles" || !validAssetName(parts[1]) {
		return false
	}
	var role struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		PromptFile  string `json:"promptFile"`
	}
	if json.Unmarshal([]byte(files[content.Root+"/role.json"]), &role) != nil ||
		role.Name != parts[1] || strings.TrimSpace(role.DisplayName) == "" ||
		strings.TrimSpace(role.Description) == "" || !validAssetFilename(role.PromptFile) {
		return false
	}
	_, promptExists := files[content.Root+"/prompts/"+role.PromptFile]
	return promptExists
}

func validAssetName(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func validAssetFilename(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\")
}

func frontmatterValue(markdown string, key string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			return strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		}
	}
	return ""
}

func validEvolutionReview(value EvolutionReview) bool {
	return (value.Verdict == "pass" || value.Verdict == "fail" || value.Verdict == "blocked") &&
		strings.TrimSpace(value.Summary) != ""
}

func cloneEvolutionStages(input []EvolutionStage) []EvolutionStage {
	return append([]EvolutionStage(nil), input...)
}

func boundedEvolutionJSON(value json.RawMessage, max int, reason string) json.RawMessage {
	if len(value) <= max {
		return cloneJSON(value)
	}
	digest := sha256.Sum256(value)
	encoded, _ := json.Marshal(map[string]any{
		"omitted": true,
		"reason":  reason,
		"sha256":  fmt.Sprintf("%x", digest[:]),
	})
	return encoded
}
