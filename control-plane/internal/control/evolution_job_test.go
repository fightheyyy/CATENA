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

func TestEvolutionJobRunsThreeEvidenceStagesAndIsIdempotent(t *testing.T) {
	store := NewMemoryStore()
	traceID := "11111111111111111111111111111111"
	run := seedEvolutionRun(t, store, "", StateCompleted, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, AuthConfig{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	first := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolve-once",
		map[string]any{"trace_id": traceID, "objective": "Find a replayable boundary."},
	)
	if first.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(first.Body)
		first.Body.Close()
		t.Fatalf("Evolution Job start returned %d: %s", first.StatusCode, body)
	}
	var started EvolutionJob
	if err := json.NewDecoder(first.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()

	retry := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolve-once",
		map[string]any{"trace_id": traceID, "objective": "Find a replayable boundary."},
	)
	if retry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retry.Body)
		retry.Body.Close()
		t.Fatalf("idempotent retry returned %d: %s", retry.StatusCode, body)
	}
	var duplicate EvolutionJob
	if err := json.NewDecoder(retry.Body).Decode(&duplicate); err != nil {
		t.Fatal(err)
	}
	retry.Body.Close()
	if duplicate.ID != started.ID {
		t.Fatalf("idempotent retry created another Job: %s != %s", duplicate.ID, started.ID)
	}

	mutated := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolve-once",
		map[string]any{"trace_id": traceID, "objective": "A different request."},
	)
	if mutated.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(mutated.Body)
		mutated.Body.Close()
		t.Fatalf("mutated idempotent request returned %d: %s", mutated.StatusCode, body)
	}
	mutated.Body.Close()

	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobCompleted)
	if job.Schema != evolutionJobSchema || job.SourceRunID != run.ID ||
		job.SourceTraceID != traceID || job.CurrentStage != "complete" {
		t.Fatalf("unexpected completed Job identity: %#v", job)
	}
	if len(job.Stages) != 3 ||
		job.Stages[0].Role != "inspector-cat" ||
		job.Stages[1].Role != "evolution-cat" ||
		job.Stages[2].Role != "reviewer-cat" {
		t.Fatalf("unexpected stage order: %#v", job.Stages)
	}
	for _, stage := range job.Stages {
		if stage.State != EvolutionStageCompleted || len(stage.RawOutput) == 0 ||
			stage.StartedAt == nil || stage.FinishedAt == nil {
			t.Fatalf("stage evidence is incomplete: %#v", stage)
		}
	}
	if job.Finding == nil || job.CaseProposal == nil || job.Candidate == nil || job.Review == nil {
		t.Fatalf("structured outputs are incomplete: %#v", job)
	}
	if job.Candidate.Kind != EvolutionCandidateSkill ||
		job.Candidate.Status != evolutionCandidateStatus ||
		job.Review.Verdict != "pass" ||
		job.Review.Scope != evolutionReviewScope ||
		job.Review.CandidateStatus != evolutionCandidateStatus ||
		!job.CaseProposal.RequiresHumanReview {
		t.Fatalf("Job overclaimed Candidate verification: %#v", job)
	}

	response, err := http.Get(server.URL + "/v1/evolution-jobs/" + job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Job GET returned %d: %s", response.StatusCode, body)
	}
	response.Body.Close()
	response, err = http.Get(server.URL + "/v1/evolution-jobs")
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Jobs []EvolutionJob `json:"evolution_jobs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(listed.Jobs) != 1 || listed.Jobs[0].ID != job.ID {
		t.Fatalf("unexpected Job list: %#v", listed.Jobs)
	}
}

func TestEvolutionJobRejectsNonterminalOrUnretainedEvidence(t *testing.T) {
	store := NewMemoryStore()
	traceID := "22222222222222222222222222222222"
	run := seedEvolutionRun(t, store, "", StateRunning, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, AuthConfig{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	missing := postEvolutionJob(
		t,
		server.URL+"/v1/runs/missing-run/evolution-jobs",
		"missing-run",
		map[string]any{"trace_id": traceID},
	)
	if missing.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(missing.Body)
		missing.Body.Close()
		t.Fatalf("missing Run returned %d: %s", missing.StatusCode, body)
	}
	missing.Body.Close()

	nonterminal := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"nonterminal",
		map[string]any{"trace_id": traceID},
	)
	if nonterminal.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(nonterminal.Body)
		nonterminal.Body.Close()
		t.Fatalf("nonterminal Run returned %d: %s", nonterminal.StatusCode, body)
	}
	nonterminal.Body.Close()

	run.State = StateFailed
	run.UpdatedAt = time.Now().UTC()
	if err := store.UpdateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	unretained := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"unretained",
		map[string]any{"trace_id": "33333333333333333333333333333333"},
	)
	if unretained.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(unretained.Body)
		unretained.Body.Close()
		t.Fatalf("unretained Trace returned %d: %s", unretained.StatusCode, body)
	}
	unretained.Body.Close()
}

func TestEvolutionJobPlatformTenantIsolation(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	projectA := platformProjectUser("evolution-project-a", now)
	projectB := platformProjectUser("evolution-project-b", now)
	for _, user := range []User{projectA, projectB} {
		if _, err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	traceID := "44444444444444444444444444444444"
	run := seedEvolutionRun(t, store, projectA.ID, StateCompleted, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	}, manager)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"trace_id": traceID})
	start := signedPlatformRequest(
		t,
		http.MethodPost,
		"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolution-project-a",
		"actor-a",
		body,
	)
	start.Header.Set("Idempotency-Key", "tenant-a-job")
	started := httptest.NewRecorder()
	handler.ServeHTTP(started, start)
	if started.Code != http.StatusAccepted {
		t.Fatalf("project A start returned %d: %s", started.Code, started.Body.String())
	}
	var job EvolutionJob
	if err := json.Unmarshal(started.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	waitEvolutionJobState(t, store, job.ID, EvolutionJobCompleted)

	crossGet := httptest.NewRecorder()
	handler.ServeHTTP(crossGet, signedPlatformRequest(
		t,
		http.MethodGet,
		"/v1/evolution-jobs/"+job.ID,
		"evolution-project-b",
		"actor-b",
		nil,
	))
	if crossGet.Code != http.StatusNotFound {
		t.Fatalf("cross-project GET returned %d: %s", crossGet.Code, crossGet.Body.String())
	}

	crossList := httptest.NewRecorder()
	handler.ServeHTTP(crossList, signedPlatformRequest(
		t,
		http.MethodGet,
		"/v1/evolution-jobs",
		"evolution-project-b",
		"actor-b",
		nil,
	))
	if crossList.Code != http.StatusOK ||
		!strings.Contains(crossList.Body.String(), `"evolution_jobs":[]`) {
		t.Fatalf("cross-project list leaked Jobs: %d %s", crossList.Code, crossList.Body.String())
	}

	crossStart := signedPlatformRequest(
		t,
		http.MethodPost,
		"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolution-project-b",
		"actor-b",
		body,
	)
	crossStart.Header.Set("Idempotency-Key", "tenant-b-cross-job")
	crossStartResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossStartResponse, crossStart)
	if crossStartResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-project start returned %d: %s", crossStartResponse.Code, crossStartResponse.Body.String())
	}
}

func TestEvolutionJobFailsTerminallyWhenRuntimeStageFails(t *testing.T) {
	store := NewMemoryStore()
	traceID := "55555555555555555555555555555555"
	run := seedEvolutionRun(t, store, "", StateCompleted, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "evolution-cat")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, AuthConfig{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"runtime-failure",
		map[string]any{"trace_id": traceID},
	)
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Evolution Job start returned %d: %s", response.StatusCode, body)
	}
	var started EvolutionJob
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobFailed)
	if job.CurrentStage != "evolution" || job.Error == "" ||
		job.Stages[0].State != EvolutionStageCompleted ||
		job.Stages[1].State != EvolutionStageFailed ||
		job.Stages[2].State != EvolutionStageQueued ||
		job.Candidate != nil || job.Review != nil {
		t.Fatalf("unexpected failed Job: %#v", job)
	}
}

func TestEvolutionJobUsesConservativeEnvelopeForUnstructuredRoleOutput(t *testing.T) {
	raw := json.RawMessage(`{"status":"completed","assistant":{"role":"assistant","content":"plain text only"}}`)
	run := Run{Input: json.RawMessage(`{"scenario":{"objective":"Replay this exact objective."}}`)}
	finding, proposal := inspectorOutput(raw, run)
	candidate := candidateOutput(raw)
	review := reviewOutput(raw)
	if finding.Severity != "unknown" || !proposal.RequiresHumanReview ||
		candidate.Kind != EvolutionCandidateHarness ||
		candidate.Status != evolutionCandidateStatus ||
		review.Verdict != "blocked" ||
		review.Scope != evolutionReviewScope ||
		review.CandidateStatus != evolutionCandidateStatus {
		t.Fatalf(
			"unstructured output must remain explicitly unverified: finding=%#v proposal=%#v candidate=%#v review=%#v",
			finding,
			proposal,
			candidate,
			review,
		)
	}
}

func seedEvolutionRun(
	t *testing.T,
	store *MemoryStore,
	ownerUserID string,
	state RunState,
	traceID string,
) Run {
	t.Helper()
	now := time.Now().UTC()
	run := Run{
		ID:          newID("evolution-source"),
		RequestID:   newID("evolution-request"),
		OwnerUserID: ownerUserID,
		Origin:      OriginPlatform,
		Operation:   OperationExplore,
		State:       state,
		Input: json.RawMessage(
			`{"scenario":{"objective":"Create result.txt from incomplete input."}}`,
		),
		Runtime:   json.RawMessage(`{"runtime":"xiaobaos","role":"base"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	kind := "terminal"
	phase := "complete"
	if !state.Terminal() {
		kind = "progress"
		phase = "agent"
	}
	if err := store.AppendEvent(context.Background(), EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   run.ID + ".1",
		RunID:     run.ID,
		Sequence:  1,
		Timestamp: now,
		Operation: run.Operation,
		Kind:      kind,
		Phase:     phase,
		Actor:     "engine",
		TraceID:   traceID,
		Payload:   json.RawMessage(`{"status":"retained","artifact":"result.txt"}`),
	}); err != nil {
		t.Fatal(err)
	}
	return run
}

func postEvolutionJob(
	t *testing.T,
	url string,
	idempotencyKey string,
	payload map[string]any,
) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func waitEvolutionJobState(
	t *testing.T,
	store Store,
	jobID string,
	want EvolutionJobState,
) EvolutionJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := store.GetEvolutionJob(context.Background(), jobID)
		if err == nil && job.State == want {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := store.GetEvolutionJob(context.Background(), jobID)
	t.Fatalf("Job did not reach %s: job=%#v err=%v", want, job, err)
	return EvolutionJob{}
}

func newStructuredEvolutionRuntimeManager(
	t *testing.T,
	failRole string,
) *EvolutionRuntimeManager {
	t.Helper()
	root := t.TempDir()
	worker := filepath.Join(root, "structured-evolution-worker.mjs")
	source := fmt.Sprintf(`
import fs from "node:fs";
const request = JSON.parse(fs.readFileSync(0, "utf8"));
if (request.telemetry !== undefined) process.exit(8);
if (request.operation === "turn" && request.role === %q) process.exit(7);
const content = request.role === "inspector-cat"
  ? JSON.stringify({
      finding: {title: "Ambiguous request was not clarified", summary: "The retained trace reached a terminal response without a clarification turn.", severity: "high", evidence: ["The retained trace contains the terminal response."]},
      case_proposal: {title: "Clarify incomplete input", replay_prompt: "Ask for missing constraints before writing result.txt.", success_criteria: "The Agent asks one clarification question.", verifier: {kind: "artifact_assertions", artifacts: [{path: "result.txt", exists: true}]}}
    })
  : request.role === "evolution-cat"
    ? JSON.stringify({candidate: {kind: "skill", title: "Clarify first", summary: "Draft a reusable clarification behavior.", content: {instruction: "Ask for missing constraints before acting."}}})
    : JSON.stringify({review: {verdict: "pass", summary: "The draft is grounded in the retained finding, but remains unverified."}});
console.log(JSON.stringify({
  schema: "barena.xiaoba_evolution_response.v1",
  request_id: request.request_id,
  operation: "turn",
  status: "ok",
  result: {status: "completed", assistant: {role: "assistant", content}, process: {exit_code: 0, signal: null, duration_ms: 1, stdout: "", stderr: ""}, telemetry: {mode: "native"}, native_trace_refs: []}
}));
`, failRole)
	if err := os.WriteFile(worker, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{
		NodeCommand:   "node",
		WorkerPath:    worker,
		XiaoBaCommand: "fake-xiaoba",
		WorkspaceRoot: filepath.Join(root, "workspaces"),
		ProbeTimeout:  2 * time.Second,
		CacheTTL:      time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
