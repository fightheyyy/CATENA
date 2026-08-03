package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestTraceIssuePromotesToOneImmutableCase(t *testing.T) {
	root := t.TempDir()
	worker := writeFakeWorker(t, root)
	store := NewMemoryStore()
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  worker,
		RunsRoot:    filepath.Join(root, "runs"),
		KillGrace:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(store, runner))
	defer server.Close()

	run := createTestRun(
		t,
		server.URL,
		`{"scenario":{"scenario_id":"trace-to-case","objective":"clarify an ambiguous task"}}`,
	)
	waitForState(t, store, run.ID, StateCompleted)
	traceID := "trace-" + run.ID

	badTrace := postJSON(t, server.URL+"/v1/runs/"+run.ID+"/issues", map[string]any{
		"trace_id": "trace-not-retained",
		"title":    "Agent skipped clarification",
		"summary":  "The target acted before confirming the missing constraint.",
		"severity": "high",
	})
	if badTrace.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(badTrace.Body)
		badTrace.Body.Close()
		t.Fatalf("unretained Trace returned %d: %s", badTrace.StatusCode, body)
	}
	badTrace.Body.Close()

	issueResponse := postJSON(t, server.URL+"/v1/runs/"+run.ID+"/issues", map[string]any{
		"trace_id": traceID,
		"title":    "Agent skipped clarification",
		"summary":  "The target acted before confirming the missing constraint.",
		"severity": "high",
	})
	if issueResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(issueResponse.Body)
		issueResponse.Body.Close()
		t.Fatalf("create Issue returned %d: %s", issueResponse.StatusCode, body)
	}
	var issue Issue
	if err := json.NewDecoder(issueResponse.Body).Decode(&issue); err != nil {
		t.Fatal(err)
	}
	issueResponse.Body.Close()
	if issue.SourceRunID != run.ID ||
		issue.SourceTraceID != traceID ||
		issue.Status != IssueOpen {
		t.Fatalf("unexpected Issue: %#v", issue)
	}

	promotion := map[string]any{
		"success_criteria": "The Agent asks for the missing constraint before changing files.",
		"verifier": map[string]any{
			"kind":      "artifact_assertions",
			"artifacts": []any{map[string]any{"path": "result.txt", "exists": true}},
		},
	}
	caseResponse := postJSON(
		t,
		server.URL+"/v1/issues/"+issue.ID+"/promote",
		promotion,
	)
	if caseResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(caseResponse.Body)
		caseResponse.Body.Close()
		t.Fatalf("promote Issue returned %d: %s", caseResponse.StatusCode, body)
	}
	var promoted Case
	if err := json.NewDecoder(caseResponse.Body).Decode(&promoted); err != nil {
		t.Fatal(err)
	}
	caseResponse.Body.Close()
	if promoted.Schema != "barena.case.v1" ||
		promoted.Revision != 1 ||
		promoted.SourceIssueID != issue.ID ||
		promoted.SourceRunID != run.ID ||
		promoted.SourceTraceID != traceID ||
		promoted.Operation != OperationExplore ||
		promoted.ReplayPrompt != "clarify an ambiguous task" {
		t.Fatalf("unexpected promoted Case: %#v", promoted)
	}
	if !bytes.Equal(promoted.Input, run.Input) {
		t.Fatalf("Case did not snapshot source Run input: %s", promoted.Input)
	}

	retryResponse := postJSON(
		t,
		server.URL+"/v1/issues/"+issue.ID+"/promote",
		map[string]any{
			"success_criteria": "A later request must not mutate revision 1.",
			"verifier": map[string]any{
				"kind":      "artifact_assertions",
				"artifacts": []any{map[string]any{"path": "other.txt", "exists": true}},
			},
		},
	)
	if retryResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retryResponse.Body)
		retryResponse.Body.Close()
		t.Fatalf("idempotent promotion returned %d: %s", retryResponse.StatusCode, body)
	}
	var retried Case
	if err := json.NewDecoder(retryResponse.Body).Decode(&retried); err != nil {
		t.Fatal(err)
	}
	retryResponse.Body.Close()
	if retried.ID != promoted.ID ||
		retried.SuccessCriteria != promoted.SuccessCriteria ||
		!bytes.Equal(retried.Verifier, promoted.Verifier) {
		t.Fatalf("promotion mutated immutable Case: before=%#v after=%#v", promoted, retried)
	}

	storedIssue, err := store.GetIssue(context.Background(), issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedIssue.Status != IssuePromoted ||
		storedIssue.PromotedCaseID != promoted.ID {
		t.Fatalf("Issue was not linked to the Case: %#v", storedIssue)
	}
	cases, err := store.ListCases(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected exactly one Case, got %d", len(cases))
	}
}

func TestMemoryStoreRejectsCrossOwnerIssue(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	run := Run{
		ID:          "run-owned",
		RequestID:   "request-owned",
		OwnerUserID: "user-one",
		Operation:   OperationExplore,
		State:       StateCompleted,
		Input:       json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	err := store.CreateIssue(context.Background(), Issue{
		ID:          "issue-cross-owner",
		OwnerUserID: "user-two",
		SourceRunID: run.ID,
		Title:       "Cross-owner evidence",
		Summary:     "Must fail closed.",
		Severity:    SeverityHigh,
		Status:      IssueOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != ErrConflict {
		t.Fatalf("cross-owner Issue should conflict, got %v", err)
	}
}

func TestCasePromotionRequiresReplayPromptAndArtifactAssertions(t *testing.T) {
	root := t.TempDir()
	store := NewMemoryStore()
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  writeFakeWorker(t, root),
		RunsRoot:    filepath.Join(root, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(store, runner))
	defer server.Close()
	run := createTestRun(
		t,
		server.URL,
		`{"scenario":{"scenario_id":"missing-objective"}}`,
	)
	waitForState(t, store, run.ID, StateCompleted)
	issueResponse := postJSON(t, server.URL+"/v1/runs/"+run.ID+"/issues", map[string]any{
		"title":    "Replay contract gap",
		"summary":  "Promotion must fail closed without a deterministic replay contract.",
		"severity": "high",
	})
	if issueResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(issueResponse.Body)
		issueResponse.Body.Close()
		t.Fatalf("create Issue returned %d: %s", issueResponse.StatusCode, body)
	}
	var issue Issue
	if err := json.NewDecoder(issueResponse.Body).Decode(&issue); err != nil {
		t.Fatal(err)
	}
	issueResponse.Body.Close()

	emptyAssertions := postJSON(
		t,
		server.URL+"/v1/issues/"+issue.ID+"/promote",
		map[string]any{
			"replay_prompt":    "Create result.txt",
			"success_criteria": "result.txt exists",
			"verifier": map[string]any{
				"kind":      "artifact_assertions",
				"artifacts": []any{},
			},
		},
	)
	if emptyAssertions.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(emptyAssertions.Body)
		emptyAssertions.Body.Close()
		t.Fatalf("empty assertions returned %d: %s", emptyAssertions.StatusCode, body)
	}
	emptyAssertions.Body.Close()

	unsafeAssertion := postJSON(
		t,
		server.URL+"/v1/issues/"+issue.ID+"/promote",
		map[string]any{
			"replay_prompt":    "Create result.txt",
			"success_criteria": "result.txt exists",
			"verifier": map[string]any{
				"kind":      "artifact_assertions",
				"artifacts": []any{map[string]any{"path": "../result.txt", "exists": true}},
			},
		},
	)
	if unsafeAssertion.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(unsafeAssertion.Body)
		unsafeAssertion.Body.Close()
		t.Fatalf("unsafe assertion returned %d: %s", unsafeAssertion.StatusCode, body)
	}
	unsafeAssertion.Body.Close()

	missingPrompt := postJSON(
		t,
		server.URL+"/v1/issues/"+issue.ID+"/promote",
		map[string]any{
			"success_criteria": "result.txt exists",
			"verifier": map[string]any{
				"kind":      "artifact_assertions",
				"artifacts": []any{map[string]any{"path": "result.txt", "exists": true}},
			},
		},
	)
	if missingPrompt.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(missingPrompt.Body)
		missingPrompt.Body.Close()
		t.Fatalf("missing replay prompt returned %d: %s", missingPrompt.StatusCode, body)
	}
	missingPrompt.Body.Close()

	valid := postJSON(
		t,
		server.URL+"/v1/issues/"+issue.ID+"/promote",
		map[string]any{
			"replay_prompt":    "Create result.txt",
			"success_criteria": "result.txt exists",
			"verifier": map[string]any{
				"kind":      "artifact_assertions",
				"artifacts": []any{map[string]any{"path": "result.txt", "exists": true}},
			},
		},
	)
	if valid.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(valid.Body)
		valid.Body.Close()
		t.Fatalf("valid promotion returned %d: %s", valid.StatusCode, body)
	}
	var promoted Case
	if err := json.NewDecoder(valid.Body).Decode(&promoted); err != nil {
		t.Fatal(err)
	}
	valid.Body.Close()
	if promoted.ReplayPrompt != "Create result.txt" {
		t.Fatalf("explicit replay_prompt was not snapshotted: %#v", promoted)
	}
}

func postJSON(t *testing.T, target string, value any) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(target, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return response
}
