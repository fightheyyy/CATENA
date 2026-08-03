package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *HTTPServer) createIssue(w http.ResponseWriter, r *http.Request) {
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
		writeProblem(w, http.StatusConflict, "Issue evidence requires a terminal Run")
		return
	}

	var request CreateIssueRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Summary = strings.TrimSpace(request.Summary)
	request.TraceID = strings.TrimSpace(request.TraceID)
	request.Severity = IssueSeverity(strings.ToLower(string(request.Severity)))
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.TraceID != "" {
		retained, err := s.store.RunHasTrace(r.Context(), run.ID, request.TraceID)
		if err != nil {
			writeProblem(w, statusFor(err), err.Error())
			return
		}
		if !retained {
			writeProblem(
				w,
				http.StatusBadRequest,
				"trace_id is not retained by the source Run",
			)
			return
		}
	}

	now := time.Now().UTC()
	issue := Issue{
		ID:            newID("issue"),
		OwnerUserID:   run.OwnerUserID,
		SourceRunID:   run.ID,
		SourceTraceID: request.TraceID,
		Title:         request.Title,
		Summary:       request.Summary,
		Severity:      request.Severity,
		Status:        IssueOpen,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.CreateIssue(r.Context(), issue); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, issue)
}

func (s *HTTPServer) listIssues(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := evolutionListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var issues []Issue
	if user == nil {
		issues, err = s.store.ListIssues(r.Context(), limit)
	} else {
		issues, err = s.store.ListIssuesByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if issues == nil {
		issues = []Issue{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
}

func (s *HTTPServer) getIssue(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	issue, err := s.store.GetIssue(r.Context(), r.PathValue("issue_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(issue.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (s *HTTPServer) promoteIssue(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	issue, err := s.store.GetIssue(r.Context(), r.PathValue("issue_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(issue.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Issue not found")
		return
	}

	var request PromoteIssueRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.SuccessCriteria = strings.TrimSpace(request.SuccessCriteria)
	request.ReplayPrompt = strings.TrimSpace(request.ReplayPrompt)

	run, err := s.store.GetRun(r.Context(), issue.SourceRunID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if run.OwnerUserID != issue.OwnerUserID {
		writeProblem(w, http.StatusConflict, "Issue source ownership is inconsistent")
		return
	}
	if run.Operation != OperationExplore {
		writeProblem(w, http.StatusBadRequest, "MVP1 Cases require a source Explore Run")
		return
	}
	if request.ReplayPrompt == "" {
		request.ReplayPrompt, err = sourceExploreObjective(run.Input)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	promoted := Case{
		Schema:          "barena.case.v1",
		ID:              newID("case"),
		Revision:        1,
		OwnerUserID:     issue.OwnerUserID,
		SourceIssueID:   issue.ID,
		SourceRunID:     issue.SourceRunID,
		SourceTraceID:   issue.SourceTraceID,
		Title:           issue.Title,
		Operation:       run.Operation,
		Input:           cloneJSON(run.Input),
		Runtime:         cloneJSON(run.Runtime),
		ReplayPrompt:    request.ReplayPrompt,
		SuccessCriteria: request.SuccessCriteria,
		Verifier:        cloneJSON(request.Verifier),
		CreatedAt:       now,
	}
	value, created, err := s.store.PromoteIssue(
		r.Context(),
		issue.ID,
		promoted,
		now,
	)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, value)
}

func (s *HTTPServer) listCases(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := evolutionListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var cases []Case
	if user == nil {
		cases, err = s.store.ListCases(r.Context(), limit)
	} else {
		cases, err = s.store.ListCasesByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if cases == nil {
		cases = []Case{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": cases})
}

func (s *HTTPServer) getCase(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	value, err := s.store.GetCase(r.Context(), r.PathValue("case_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(value.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Case not found")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *HTTPServer) replayCase(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	sourceCase, err := s.store.GetCase(r.Context(), r.PathValue("case_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(sourceCase.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Case not found")
		return
	}
	if reason, unsupported := platformReplayUnsupported(sourceCase.Runtime); unsupported {
		writeProblem(w, http.StatusConflict, "Replay is unavailable: "+reason)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if err := validateReplayIdempotencyKey(idempotencyKey); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerUserID := ""
	if user != nil {
		ownerUserID = user.ID
	}
	run, _, created, err := s.runner.StartReplayOwned(
		r.Context(),
		sourceCase,
		ownerUserID,
		idempotencyKey,
	)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, run)
}

func platformReplayUnsupported(runtime json.RawMessage) (string, bool) {
	if len(runtime) == 0 {
		return "", false
	}
	var value struct {
		Schema string                `json:"schema"`
		Replay ScenarioReplaySupport `json:"replay"`
	}
	if err := json.Unmarshal(runtime, &value); err != nil {
		return "the retained Runtime snapshot is invalid", true
	}
	if value.Schema != "barena.platform_http_runtime.v1" {
		return "", false
	}
	if !value.Replay.Supported {
		reason := strings.TrimSpace(value.Replay.Reason)
		if reason == "" {
			reason = "this HTTP Agent does not satisfy the deterministic Replay contract"
		}
		return reason, true
	}
	return "", false
}

func (s *HTTPServer) listEvaluations(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := evolutionListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var values []Evaluation
	if user == nil {
		values, err = s.store.ListEvaluations(r.Context(), limit)
	} else {
		values, err = s.store.ListEvaluationsByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if values == nil {
		values = []Evaluation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": values})
}

func (s *HTTPServer) getEvaluation(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	value, err := s.store.GetEvaluation(r.Context(), r.PathValue("evaluation_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(value.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Evaluation not found")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *HTTPServer) listReleases(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := evolutionListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var values []Release
	if user == nil {
		values, err = s.store.ListReleases(r.Context(), limit)
	} else {
		values, err = s.store.ListReleasesByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if values == nil {
		values = []Release{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": values})
}

func (s *HTTPServer) getRelease(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	value, err := s.store.GetRelease(r.Context(), r.PathValue("release_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(value.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Release not found")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func validateReplayIdempotencyKey(value string) error {
	if value == "" || len(value) > 200 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("Idempotency-Key must contain from 1 to 200 safe characters")
	}
	return nil
}

func resourceOwnedBy(ownerUserID string, user *User) bool {
	return user == nil || ownerUserID == user.ID
}

func evolutionListLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 100, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 1000 {
		return 0, errors.New("limit must be from 1 to 1000")
	}
	return value, nil
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func sourceExploreObjective(input json.RawMessage) (string, error) {
	var value struct {
		Scenario struct {
			Objective string `json:"objective"`
		} `json:"scenario"`
	}
	if err := json.Unmarshal(input, &value); err != nil {
		return "", errors.New("source Explore input is invalid")
	}
	objective := strings.TrimSpace(value.Scenario.Objective)
	if objective == "" {
		return "", errors.New("replay_prompt is required when source Explore has no scenario.objective")
	}
	if len(objective) > 24000 {
		return "", errors.New("source Explore scenario.objective exceeds 24000 characters")
	}
	return objective, nil
}
