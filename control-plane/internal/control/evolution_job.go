package control

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	evolutionJobSchema       = "spiral.evolution_job.v1"
	evolutionCandidateStatus = "draft/unverified"
	evolutionReviewScope     = "proposal_only"
	evolutionTurnTimeout     = 2 * time.Minute
	evolutionStageTimeout    = evolutionTurnTimeout + 10*time.Second
)

var evolutionJobStages = []EvolutionStage{
	{Name: "inspector", Role: "inspector-cat", State: EvolutionStageQueued},
	{Name: "evolution", Role: "evolution-cat", State: EvolutionStageQueued},
	{Name: "reviewer", Role: "reviewer-cat", State: EvolutionStageQueued},
}

type inspectorTurnOutput struct {
	Finding      EvolutionFinding      `json:"finding"`
	CaseProposal EvolutionCaseProposal `json:"case_proposal"`
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
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if err := validateReplayIdempotencyKey(idempotencyKey); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	fingerprint, err := evolutionRequestFingerprint(request)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Evolution request could not be fingerprinted")
		return
	}
	now := time.Now().UTC()
	job := EvolutionJob{
		Schema:             evolutionJobSchema,
		ID:                 newID("evolution-job"),
		OwnerUserID:        run.OwnerUserID,
		SourceRunID:        run.ID,
		SourceTraceID:      request.TraceID,
		Objective:          request.Objective,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		State:              EvolutionJobQueued,
		Stages:             cloneEvolutionStages(evolutionJobStages),
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
	run, err := s.store.GetRun(context.Background(), job.SourceRunID)
	if err != nil {
		s.failEvolutionJob(&job, -1, "source Run is unavailable")
		return
	}
	events, err := s.store.ListEventsAfter(context.Background(), run.ID, 0, 1000)
	if err != nil {
		s.failEvolutionJob(&job, -1, "source Run evidence is unavailable")
		return
	}
	evidence, err := evolutionEvidenceJSON(run, events, job.SourceTraceID)
	if err != nil {
		s.failEvolutionJob(&job, -1, "source Run evidence could not be encoded")
		return
	}

	inspectorPrompt := buildInspectorPrompt(job, run, evidence)
	inspectorRaw, ok := s.runEvolutionStage(&job, 0, inspectorPrompt)
	if !ok {
		return
	}
	finding, proposal := inspectorOutput(inspectorRaw, run)
	job.Finding = &finding
	job.CaseProposal = &proposal
	job.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateEvolutionJob(context.Background(), job); err != nil {
		return
	}

	evolutionPrompt := buildCandidatePrompt(job)
	candidateRaw, ok := s.runEvolutionStage(&job, 1, evolutionPrompt)
	if !ok {
		return
	}
	candidate := candidateOutput(candidateRaw)
	job.Candidate = &candidate
	job.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateEvolutionJob(context.Background(), job); err != nil {
		return
	}

	reviewerPrompt := buildReviewerPrompt(job, evidence)
	reviewerRaw, ok := s.runEvolutionStage(&job, 2, reviewerPrompt)
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
	})
	if err != nil {
		s.failEvolutionJob(job, index, bounded(err.Error(), 1000))
		return nil, false
	}
	finishedAt := time.Now().UTC()
	job.Stages[index].State = EvolutionStageCompleted
	job.Stages[index].RawOutput = cloneJSON(raw)
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

func evolutionRequestFingerprint(request CreateEvolutionJobRequest) (string, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func evolutionEvidenceJSON(
	run Run,
	events []EngineEvent,
	traceID string,
) (json.RawMessage, error) {
	relevant := make([]EngineEvent, 0)
	traceFound := false
	for _, event := range events {
		if event.TraceID == traceID || event.Kind == "terminal" {
			if event.TraceID == traceID {
				traceFound = true
			}
			copy := event
			if len(copy.Payload) > 12*1024 {
				digest := sha256.Sum256(copy.Payload)
				copy.Payload = json.RawMessage(fmt.Sprintf(
					`{"omitted":true,"reason":"payload exceeded evidence prompt limit","sha256":"%x"}`,
					digest[:],
				))
			}
			relevant = append(relevant, copy)
		}
	}
	if !traceFound {
		return nil, fmt.Errorf("source Trace evidence was not loaded")
	}
	if len(relevant) > 32 {
		relevant = append(
			append([]EngineEvent(nil), relevant[:16]...),
			relevant[len(relevant)-16:]...,
		)
	}
	return json.Marshal(map[string]any{
		"run": map[string]any{
			"run_id":     run.ID,
			"origin":     run.Origin,
			"operation":  run.Operation,
			"state":      run.State,
			"input":      run.Input,
			"runtime":    run.Runtime,
			"error":      run.Error,
			"created_at": run.CreatedAt,
			"updated_at": run.UpdatedAt,
		},
		"source_trace_id": traceID,
		"events":          relevant,
	})
}

func buildInspectorPrompt(job EvolutionJob, run Run, evidence json.RawMessage) string {
	focus := job.Objective
	if focus == "" {
		focus = "Find one evidence-backed failure mode or behavioral boundary."
	}
	return fmt.Sprintf(`You are InspectorCat in Catena. Analyze only the retained execution evidence below.
Do not invent tool calls, artifacts, verification, or outcomes. Focus: %s
Return one JSON object only with this exact shape:
{"finding":{"title":"...","summary":"...","severity":"low|medium|high|critical|unknown","evidence":["specific retained fact"]},"case_proposal":{"title":"...","replay_prompt":"...","success_criteria":"...","verifier":{"kind":"artifact_assertions","artifacts":[{"path":"relative/path","exists":true}]}}}
The Case is a proposal for human review, not an already-created or verified Case.
Source Run: %s
Evidence:
%s`, focus, run.ID, evidence)
}

func buildCandidatePrompt(job EvolutionJob) string {
	inputs, _ := json.Marshal(map[string]any{
		"finding":       job.Finding,
		"case_proposal": job.CaseProposal,
	})
	return fmt.Sprintf(`You are EvolutionCat in Barena. Produce the smallest draft improvement suggested by the accepted evidence analysis.
Return one JSON object only with this exact shape:
{"candidate":{"kind":"role|skill|memory|harness","title":"...","summary":"...","content":{}}}
The candidate is a draft proposal. Do not claim it was installed, applied, replayed, or verified.
Analysis:
%s`, inputs)
}

func buildReviewerPrompt(job EvolutionJob, evidence json.RawMessage) string {
	inputs, _ := json.Marshal(map[string]any{
		"finding":       job.Finding,
		"case_proposal": job.CaseProposal,
		"candidate":     job.Candidate,
	})
	return fmt.Sprintf(`You are ReviewerCat in Catena. Review whether this proposal is coherent and grounded in the retained evidence.
Return one JSON object only with this exact shape:
{"review":{"verdict":"pass|fail|blocked","summary":"..."}}
This is proposal review only. The candidate remains draft/unverified; do not claim execution or release verification.
Proposal:
%s
Retained evidence:
%s`, inputs, evidence)
}

func inspectorOutput(raw json.RawMessage, run Run) (EvolutionFinding, EvolutionCaseProposal) {
	var parsed inspectorTurnOutput
	if decodeRoleJSON(raw, &parsed) && validEvolutionFinding(parsed.Finding) &&
		validEvolutionCaseProposal(parsed.CaseProposal) {
		parsed.Finding = sanitizeEvolutionFinding(parsed.Finding)
		parsed.CaseProposal.Title = bounded(strings.TrimSpace(parsed.CaseProposal.Title), 160)
		parsed.CaseProposal.ReplayPrompt = bounded(strings.TrimSpace(parsed.CaseProposal.ReplayPrompt), 24000)
		parsed.CaseProposal.SuccessCriteria = bounded(strings.TrimSpace(parsed.CaseProposal.SuccessCriteria), 4000)
		parsed.CaseProposal.Verifier = boundedEvolutionJSON(
			parsed.CaseProposal.Verifier,
			64*1024,
			"verifier exceeded proposal limit",
		)
		parsed.CaseProposal.RequiresHumanReview = true
		return parsed.Finding, parsed.CaseProposal
	}
	replayPrompt, err := sourceExploreObjective(run.Input)
	if err != nil {
		replayPrompt = "Replay the source Run with its retained input."
	}
	return EvolutionFinding{
			Title:    "Unstructured InspectorCat result",
			Summary:  "InspectorCat returned output that requires human interpretation; its raw response is retained with the stage.",
			Severity: "unknown",
			Evidence: []string{"Raw InspectorCat output is retained in the inspector stage."},
		}, EvolutionCaseProposal{
			Title:               "Human review required before Case promotion",
			ReplayPrompt:        replayPrompt,
			SuccessCriteria:     "Define deterministic success criteria from the retained evidence before promotion.",
			Verifier:            json.RawMessage(`{"kind":"manual_review_required"}`),
			RequiresHumanReview: true,
		}
}

func candidateOutput(raw json.RawMessage) EvolutionCandidate {
	var parsed candidateTurnOutput
	if decodeRoleJSON(raw, &parsed) && validEvolutionCandidate(parsed.Candidate) {
		parsed.Candidate.ID = newID("candidate")
		parsed.Candidate.Title = bounded(strings.TrimSpace(parsed.Candidate.Title), 160)
		parsed.Candidate.Summary = bounded(strings.TrimSpace(parsed.Candidate.Summary), 4000)
		parsed.Candidate.Content = boundedEvolutionJSON(
			parsed.Candidate.Content,
			128*1024,
			"candidate content exceeded prompt limit",
		)
		parsed.Candidate.Status = evolutionCandidateStatus
		return parsed.Candidate
	}
	return EvolutionCandidate{
		ID:      newID("candidate"),
		Kind:    EvolutionCandidateHarness,
		Title:   "Unclassified EvolutionCat draft",
		Summary: "EvolutionCat returned unstructured output. Human review must classify and edit this draft before use.",
		Content: boundedEvolutionJSON(
			raw,
			128*1024,
			"unstructured candidate output exceeded prompt limit",
		),
		Status: evolutionCandidateStatus,
	}
}

func reviewOutput(raw json.RawMessage) EvolutionReview {
	var parsed reviewTurnOutput
	if decodeRoleJSON(raw, &parsed) && validEvolutionReview(parsed.Review) {
		parsed.Review.Summary = bounded(strings.TrimSpace(parsed.Review.Summary), 4000)
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
	value.Title = bounded(strings.TrimSpace(value.Title), 160)
	value.Summary = bounded(strings.TrimSpace(value.Summary), 4000)
	value.Severity = strings.ToLower(strings.TrimSpace(value.Severity))
	evidence := make([]string, 0, len(value.Evidence))
	for _, fact := range value.Evidence {
		fact = bounded(strings.TrimSpace(fact), 1000)
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

func validEvolutionCaseProposal(value EvolutionCaseProposal) bool {
	return strings.TrimSpace(value.Title) != "" &&
		strings.TrimSpace(value.ReplayPrompt) != "" &&
		strings.TrimSpace(value.SuccessCriteria) != "" &&
		jsonObject(value.Verifier)
}

func validEvolutionCandidate(value EvolutionCandidate) bool {
	return value.Kind.Valid() &&
		strings.TrimSpace(value.Title) != "" &&
		strings.TrimSpace(value.Summary) != "" &&
		len(value.Content) > 0 && json.Valid(value.Content)
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
