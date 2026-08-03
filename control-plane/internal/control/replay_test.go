package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaseReplayPersistsOneEvaluationAndRelease(t *testing.T) {
	root := t.TempDir()
	store := NewMemoryStore()
	sourceCase := seedReplayCase(t, store, "")
	worker := writeReplayWorker(t, root, "cleared", false)
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

	missingKey, err := http.Post(
		server.URL+"/v1/cases/"+sourceCase.ID+"/replay",
		"application/json",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if missingKey.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(missingKey.Body)
		t.Fatalf("missing Idempotency-Key returned %d: %s", missingKey.StatusCode, body)
	}
	missingKey.Body.Close()

	first := postReplay(t, server.URL, sourceCase.ID, "replay-once")
	if first.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(first.Body)
		first.Body.Close()
		t.Fatalf("first Replay returned %d: %s", first.StatusCode, body)
	}
	var run Run
	if err := json.NewDecoder(first.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()

	retry := postReplay(t, server.URL, sourceCase.ID, "replay-once")
	if retry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retry.Body)
		retry.Body.Close()
		t.Fatalf("idempotent Replay returned %d: %s", retry.StatusCode, body)
	}
	var retried Run
	if err := json.NewDecoder(retry.Body).Decode(&retried); err != nil {
		t.Fatal(err)
	}
	retry.Body.Close()
	if retried.ID != run.ID {
		t.Fatalf("idempotent Replay changed Run: %s != %s", retried.ID, run.ID)
	}

	waitForState(t, store, run.ID, StateCompleted)
	completedRetry := postReplay(t, server.URL, sourceCase.ID, "replay-once")
	if completedRetry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(completedRetry.Body)
		completedRetry.Body.Close()
		t.Fatalf("completed Replay retry returned %d: %s", completedRetry.StatusCode, body)
	}
	var completedRun Run
	if err := json.NewDecoder(completedRetry.Body).Decode(&completedRun); err != nil {
		t.Fatal(err)
	}
	completedRetry.Body.Close()
	if completedRun.ID != run.ID || completedRun.State != StateCompleted {
		t.Fatalf("completed Replay retry changed state: %#v", completedRun)
	}
	var engineInput map[string]json.RawMessage
	if err := json.Unmarshal(run.Input, &engineInput); err != nil {
		t.Fatal(err)
	}
	if len(engineInput) != 2 ||
		len(engineInput["platform_case"]) == 0 ||
		len(engineInput["case_base_dir"]) == 0 {
		t.Fatalf("unexpected Replay input contract: %s", run.Input)
	}
	var baseDir string
	if err := json.Unmarshal(engineInput["case_base_dir"], &baseDir); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(baseDir) {
		t.Fatalf("case_base_dir is not absolute: %q", baseDir)
	}
	if !bytes.Equal(run.Runtime, sourceCase.Runtime) {
		t.Fatalf("Replay did not inherit Case runtime: %s != %s", run.Runtime, sourceCase.Runtime)
	}

	harness, err := store.GetHarnessVersionByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if harness.CaseID != sourceCase.ID ||
		harness.SourceRunID != sourceCase.SourceRunID ||
		harness.SourceTraceID != sourceCase.SourceTraceID {
		t.Fatalf("unexpected Harness lineage: %#v", harness)
	}
	evaluations, err := store.ListEvaluations(context.Background(), 100)
	if err != nil || len(evaluations) != 1 {
		t.Fatalf("expected one Evaluation, got %d: %v", len(evaluations), err)
	}
	releases, err := store.ListReleases(context.Background(), 100)
	if err != nil || len(releases) != 1 {
		t.Fatalf("expected one Release, got %d: %v", len(releases), err)
	}
	evaluation := evaluations[0]
	release := releases[0]
	if evaluation.Decision != DecisionCleared ||
		evaluation.PackageStatus != "complete" ||
		evaluation.ResultStatus != "pass" ||
		evaluation.ReplayTraceID != "11111111111111111111111111111111" ||
		evaluation.Summary != "Engine-owned release decision" {
		t.Fatalf("unexpected Evaluation facts: %#v", evaluation)
	}
	if release.Decision != DecisionCleared ||
		release.EvaluationID != evaluation.ID ||
		release.HarnessVersionID != harness.ID ||
		release.CaseID != sourceCase.ID ||
		release.SourceTraceID != sourceCase.SourceTraceID {
		t.Fatalf("unexpected Release: %#v", release)
	}

	var listed struct {
		Evaluations []Evaluation `json:"evaluations"`
	}
	decodeTestJSON(
		t,
		http.DefaultClient,
		http.MethodGet,
		server.URL+"/v1/evaluations",
		nil,
		&listed,
	)
	if len(listed.Evaluations) != 1 || listed.Evaluations[0].ID != evaluation.ID {
		t.Fatalf("unexpected Evaluation API response: %#v", listed)
	}
	var detailed Release
	decodeTestJSON(
		t,
		http.DefaultClient,
		http.MethodGet,
		server.URL+"/v1/releases/"+release.ID,
		nil,
		&detailed,
	)
	if detailed.ID != release.ID || detailed.Decision != release.Decision {
		t.Fatalf("unexpected Release detail: %#v", detailed)
	}

	fact := ReplayFact{
		TerminalEventID: evaluation.TerminalEventID,
		ReplayTraceID:   evaluation.ReplayTraceID,
		PackageStatus:   evaluation.PackageStatus,
		ResultStatus:    evaluation.ResultStatus,
		Decision:        evaluation.Decision,
		Summary:         evaluation.Summary,
		ResultRef:       evaluation.ResultRef,
	}
	duplicateEvaluation, duplicateRelease, created, err := store.FinalizeReplay(
		context.Background(),
		run.ID,
		fact,
		time.Now().UTC(),
	)
	if err != nil || created ||
		duplicateEvaluation.ID != evaluation.ID ||
		duplicateRelease.ID != release.ID {
		t.Fatalf(
			"duplicate finalization was not idempotent: created=%v evaluation=%#v release=%#v err=%v",
			created,
			duplicateEvaluation,
			duplicateRelease,
			err,
		)
	}
}

func TestUnsupportedPlatformReplayIsRejectedBeforeRunCreation(t *testing.T) {
	root := t.TempDir()
	store := NewMemoryStore()
	sourceCase := seedReplayCase(t, store, "")
	sourceCase.Runtime = json.RawMessage(`{
		"schema":"barena.platform_http_runtime.v1",
		"type":"http",
		"replay":{
			"supported":false,
			"reason":"Custom HTTP body templates are not supported by deterministic Replay."
		}
	}`)
	store.mu.Lock()
	store.cases[sourceCase.ID] = sourceCase
	store.mu.Unlock()
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  writeReplayWorker(t, root, "cleared", false),
		RunsRoot:    filepath.Join(root, "runs"),
		KillGrace:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(store, runner))
	defer server.Close()

	response := postReplay(t, server.URL, sourceCase.ID, "unsupported-replay")
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusConflict ||
		!strings.Contains(string(body), "Custom HTTP body templates") {
		t.Fatalf("unsupported Replay returned %d: %s", response.StatusCode, body)
	}
	runs, err := store.ListRuns(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != sourceCase.SourceRunID {
		t.Fatalf("unsupported Replay created a Run: %#v", runs)
	}
}

func TestReplayRejectsIllegalDecisionAndFailureCreatesNoRelease(t *testing.T) {
	tests := []struct {
		name     string
		decision string
		fail     bool
	}{
		{name: "illegal decision", decision: "pass"},
		{name: "worker failure", decision: "cleared", fail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewMemoryStore()
			sourceCase := seedReplayCase(t, store, "")
			runner, err := NewRunnerManager(store, RunnerConfig{
				NodeCommand: "node",
				WorkerPath:  writeReplayWorker(t, root, test.decision, test.fail),
				RunsRoot:    filepath.Join(root, "runs"),
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(NewHTTPHandler(store, runner))
			defer server.Close()
			response := postReplay(t, server.URL, sourceCase.ID, "no-release")
			if response.StatusCode != http.StatusAccepted {
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				t.Fatalf("Replay returned %d: %s", response.StatusCode, body)
			}
			var run Run
			if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			waitForState(t, store, run.ID, StateFailed)
			evaluations, _ := store.ListEvaluations(context.Background(), 100)
			releases, _ := store.ListReleases(context.Background(), 100)
			if len(evaluations) != 0 || len(releases) != 0 {
				t.Fatalf(
					"failed Replay fabricated records: evaluations=%d releases=%d",
					len(evaluations),
					len(releases),
				)
			}
		})
	}
}

func TestCancelledReplayCreatesNoRelease(t *testing.T) {
	root := t.TempDir()
	store := NewMemoryStore()
	sourceCase := seedReplayCase(t, store, "")
	worker := filepath.Join(root, "sleeping-replay-worker.js")
	if err := os.WriteFile(worker, []byte(`
process.stdin.resume();
process.on("SIGINT", () => process.exit(0));
setInterval(() => {}, 1000);
`), 0o755); err != nil {
		t.Fatal(err)
	}
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
	response := postReplay(t, server.URL, sourceCase.ID, "cancel-once")
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Replay returned %d: %s", response.StatusCode, body)
	}
	var run Run
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	waitForState(t, store, run.ID, StateRunning)
	cancelled := postJSON(t, server.URL+"/v1/runs/"+run.ID+"/cancel", map[string]any{})
	if cancelled.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(cancelled.Body)
		cancelled.Body.Close()
		t.Fatalf("cancel returned %d: %s", cancelled.StatusCode, body)
	}
	cancelled.Body.Close()
	waitForState(t, store, run.ID, StateCancelled)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runner.processMu.Lock()
		active := runner.processes[run.ID] != nil
		runner.processMu.Unlock()
		if !active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	runner.processMu.Lock()
	active := runner.processes[run.ID] != nil
	runner.processMu.Unlock()
	if active {
		t.Fatal("cancelled Replay worker did not exit")
	}
	evaluations, _ := store.ListEvaluations(context.Background(), 100)
	releases, _ := store.ListReleases(context.Background(), 100)
	if len(evaluations) != 0 || len(releases) != 0 {
		t.Fatalf(
			"cancelled Replay fabricated records: evaluations=%d releases=%d",
			len(evaluations),
			len(releases),
		)
	}
}

func TestReplayOwnerIsolation(t *testing.T) {
	root := t.TempDir()
	store := NewMemoryStore()
	firstCase := seedReplayCase(t, store, "owner-one")
	secondCase := seedReplayCase(t, store, "owner-two")
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  writeReplayWorker(t, root, "held", false),
		RunsRoot:    filepath.Join(root, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := runner.StartReplayOwned(
		context.Background(),
		firstCase,
		"owner-two",
		"cross-owner",
	); err != ErrConflict {
		t.Fatalf("cross-owner Replay should conflict, got %v", err)
	}
	firstRun, _, _, err := runner.StartReplayOwned(
		context.Background(),
		firstCase,
		"owner-one",
		"same-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	secondRun, _, _, err := runner.StartReplayOwned(
		context.Background(),
		secondCase,
		"owner-two",
		"same-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, store, firstRun.ID, StateCompleted)
	waitForState(t, store, secondRun.ID, StateCompleted)
	firstEvaluations, _ := store.ListEvaluationsByOwner(context.Background(), "owner-one", 100)
	secondEvaluations, _ := store.ListEvaluationsByOwner(context.Background(), "owner-two", 100)
	if len(firstEvaluations) != 1 ||
		firstEvaluations[0].RunID != firstRun.ID ||
		len(secondEvaluations) != 1 ||
		secondEvaluations[0].RunID != secondRun.ID {
		t.Fatalf(
			"owner-scoped Evaluations leaked: first=%#v second=%#v",
			firstEvaluations,
			secondEvaluations,
		)
	}

	now := time.Now().UTC()
	for index, user := range []User{
		{ID: "owner-one", GitHubID: 101, Login: "owner-one", CreatedAt: now, UpdatedAt: now},
		{ID: "owner-two", GitHubID: 202, Login: "owner-two", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
		token := fmt.Sprintf("barena_pat_owner_%d", index+1)
		if err := store.CreateAPIToken(context.Background(), APIToken{
			ID:        fmt.Sprintf("token-%d", index+1),
			TokenHash: sessionTokenHash(token),
			UserID:    user.ID,
			Name:      "test",
			CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := NewHTTPHandlerWithConfig(store, runner, AuthConfig{
		GitHubClientID:     "test-client",
		GitHubClientSecret: "test-secret",
		RedirectURL:        "http://127.0.0.1/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	crossOwner := bearerRequest(
		t,
		http.MethodGet,
		server.URL+"/v1/evaluations/"+firstEvaluations[0].ID,
		"barena_pat_owner_2",
		nil,
	)
	if crossOwner.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(crossOwner.Body)
		crossOwner.Body.Close()
		t.Fatalf("cross-owner Evaluation returned %d: %s", crossOwner.StatusCode, body)
	}
	crossOwner.Body.Close()
	ownerList := bearerRequest(
		t,
		http.MethodGet,
		server.URL+"/v1/evaluations",
		"barena_pat_owner_2",
		nil,
	)
	if ownerList.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ownerList.Body)
		ownerList.Body.Close()
		t.Fatalf("owner Evaluation list returned %d: %s", ownerList.StatusCode, body)
	}
	var ownerPayload struct {
		Evaluations []Evaluation `json:"evaluations"`
	}
	if err := json.NewDecoder(ownerList.Body).Decode(&ownerPayload); err != nil {
		t.Fatal(err)
	}
	ownerList.Body.Close()
	if len(ownerPayload.Evaluations) != 1 ||
		ownerPayload.Evaluations[0].RunID != secondRun.ID {
		t.Fatalf("owner-scoped API leaked Evaluations: %#v", ownerPayload.Evaluations)
	}
}

func postReplay(t *testing.T, serverURL, caseID, idempotencyKey string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(
		http.MethodPost,
		serverURL+"/v1/cases/"+caseID+"/replay",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func seedReplayCase(t *testing.T, store *MemoryStore, ownerUserID string) Case {
	t.Helper()
	now := time.Now().UTC()
	suffix := newID("seed")
	sourceRun := Run{
		ID:          "source-" + suffix,
		RequestID:   "request-" + suffix,
		OwnerUserID: ownerUserID,
		Origin:      OriginEdge,
		Operation:   OperationExplore,
		State:       StateCompleted,
		Input: json.RawMessage(`{
			"scenario":{
				"scenario_id":"replay-seed",
				"objective":"Create result.txt",
				"target":{"runtime":"xiaobaos","role":"assistant"},
				"isolation":{"level":"policy_only","network":"disabled"}
			}
		}`),
		Runtime:   json.RawMessage(`{"runtime":"xiaobaos","role":"assistant"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateRun(context.Background(), sourceRun); err != nil {
		t.Fatal(err)
	}
	sourceTraceID := "source-trace-" + suffix
	if err := store.AppendEvent(context.Background(), EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   sourceRun.ID + ".1",
		RunID:     sourceRun.ID,
		Sequence:  1,
		Timestamp: now,
		Operation: OperationExplore,
		Kind:      "terminal",
		Phase:     "complete",
		Actor:     "engine",
		TraceID:   sourceTraceID,
		Payload:   json.RawMessage(`{"status":"complete"}`),
	}); err != nil {
		t.Fatal(err)
	}
	value := Case{
		Schema:          "barena.case.v1",
		ID:              "case-" + suffix,
		Revision:        1,
		OwnerUserID:     ownerUserID,
		SourceIssueID:   "issue-" + suffix,
		SourceRunID:     sourceRun.ID,
		SourceTraceID:   sourceTraceID,
		Title:           "Replay regression",
		Operation:       OperationExplore,
		Input:           cloneJSON(sourceRun.Input),
		Runtime:         cloneJSON(sourceRun.Runtime),
		ReplayPrompt:    "Create result.txt",
		SuccessCriteria: "result.txt exists",
		Verifier: json.RawMessage(
			`{"kind":"artifact_assertions","artifacts":[{"path":"result.txt","exists":true}]}`,
		),
		CreatedAt: now,
	}
	store.mu.Lock()
	store.cases[value.ID] = value
	store.mu.Unlock()
	return value
}

func writeReplayWorker(t *testing.T, root, decision string, fail bool) string {
	t.Helper()
	worker := filepath.Join(root, "replay-worker.js")
	if fail {
		if err := os.WriteFile(
			worker,
			[]byte(`process.stderr.write("intentional Replay failure\n"); process.exit(1);`),
			0o755,
		); err != nil {
			t.Fatal(err)
		}
		return worker
	}
	source := fmt.Sprintf(`
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
let source = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", chunk => source += chunk);
process.stdin.on("end", () => {
  const request = JSON.parse(source);
  const keys = Object.keys(request.input).sort();
  if (request.operation !== "replay" ||
      JSON.stringify(keys) !== JSON.stringify(["case_base_dir", "platform_case"]) ||
      !path.isAbsolute(request.input.case_base_dir) ||
      request.input.platform_case.schema !== "barena.case.v1" ||
      !request.input.platform_case.replay_prompt ||
      request.runtime.runtime !== request.input.platform_case.runtime.runtime) {
    throw new Error("invalid Case Replay contract");
  }
  const runRoot = path.join(request.runs_root, request.run_id);
  fs.mkdirSync(runRoot);
  const resultRef = "result.json";
  fs.writeFileSync(path.join(runRoot, resultRef), JSON.stringify({
    status: "pass",
    decision: %q,
    summary: "Engine-owned release decision"
  }) + "\n");
  const event = {
    schema: "barena.engine_event.v1",
    event_id: request.run_id + ".1",
    run_id: request.run_id,
    sequence: 1,
    timestamp: new Date().toISOString(),
    operation: "replay",
    kind: "terminal",
    phase: "complete",
    actor: "engine",
    trace_id: "11111111111111111111111111111111",
    payload: {
      status: "complete",
      result_status: "pass",
      decision: %q,
      summary: "Engine-owned release decision",
      result_ref: resultRef
    }
  };
  fs.writeFileSync(path.join(runRoot, "events.ndjson"), JSON.stringify(event) + "\n");
  process.stdout.write(JSON.stringify(event) + "\n");
  const refs = [resultRef, "events.ndjson"];
  const files = refs.map(ref => {
    const bytes = fs.readFileSync(path.join(runRoot, ref));
    return {
      ref,
      kind: ref === resultRef ? "result" : "events",
      media_type: ref.endsWith(".json") ? "application/json" : "application/x-ndjson",
      size: bytes.length,
      sha256: crypto.createHash("sha256").update(bytes).digest("hex")
    };
  });
  fs.writeFileSync(path.join(runRoot, "run-package.json"), JSON.stringify({
    schema: "barena.run_package.v1",
    run_id: request.run_id,
    status: "complete",
    result_ref: resultRef,
    files
  }) + "\n");
});`, decision, decision)
	if err := os.WriteFile(worker, []byte(strings.TrimSpace(source)), 0o755); err != nil {
		t.Fatal(err)
	}
	return worker
}
