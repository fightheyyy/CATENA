package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExperimentStoreIsolationIdempotencyAndCapacity(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()
	e := Experiment{ID: "one", Owner: "a", RequestID: "request", State: "running", CreatedAt: now, Trials: []ExperimentTrial{}}
	if _, created, err := store.CreateExperiment(ctx, e); err != nil || !created {
		t.Fatal(err)
	}
	e.ID = "duplicate"
	if old, created, err := store.CreateExperiment(ctx, e); err != nil || created || old.ID != "one" {
		t.Fatal("idempotency failed")
	}
	if _, err := store.GetExperiment(ctx, "b", "one"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign record exposed")
	}
	e.Owner = "b"
	if _, _, err := store.CreateExperiment(ctx, e); !errors.Is(err, ErrConflict) {
		t.Fatal("worker capacity not enforced")
	}
	rows, _ := store.ListExperiments(ctx, "b")
	if len(rows) != 0 {
		t.Fatal("foreign history exposed")
	}
	original, _ := store.GetExperiment(ctx, "a", "one")
	original.State = "completed"
	if err := store.UpdateExperiment(ctx, original); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.CreateExperiment(ctx, e); err != nil || !created {
		t.Fatal("capacity was not released", err)
	}
}

func TestExperimentPublicSubmissionOwnerModelAndNoSecret(t *testing.T) {
	var received map[string]any
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer engine-test" {
			t.Error("engine authentication missing")
		}
		if r.URL.Path == "/v1/harbor/submit" {
			_ = json.NewDecoder(r.Body).Decode(&received)
			writeJSON(w, 200, map[string]any{"job_id": "harbor-one", "status": "completed", "trials": []any{}, "tasks_unchanged": true})
			return
		}
		writeJSON(w, 404, map[string]string{"error": "unexpected"})
	}))
	defer engine.Close()
	runtime, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: engine.URL, Token: "engine-test"})
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithRuntime(store, nil, testEvolutionAuthConfig(), runtime)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := putLLMConfig(t, server.URL, map[string]string{"provider": "openai", "base_url": "https://model.example/v1", "model": "owner-model", "api_key": "private-test-secret"})
	response.Body.Close()
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/v1/experiments", strings.NewReader(`{"case_id":"rg-missing-search","request_id":"same-request"}`))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != 202 && recorder.Code != 200 {
			t.Fatal(recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "private-test-secret") {
			t.Fatal("secret exposed")
		}
	}
	model := received["model"].(map[string]any)
	if model["api_key"] != "private-test-secret" || model["model"] != "owner-model" {
		t.Fatal("owner model not forwarded")
	}
	rows, _ := store.ListExperiments(context.Background(), "local")
	if len(rows) != 1 {
		t.Fatal("duplicate execution record")
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/experiments", strings.NewReader(`{"case_id":"../../task","request_id":"unsafe"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 400 {
		t.Fatal("arbitrary task accepted")
	}
}

func TestExperimentPollingReadsEvidenceAndDoesNotResubmit(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/harbor/lookup" {
			t.Error("polling attempted submission")
		}
		writeJSON(w, 200, map[string]any{"job_id": "harbor-id", "status": "completed", "tasks_unchanged": true, "trials": []map[string]any{{"variant": "baseline", "valid": true, "rewards": map[string]float64{"reward": 0}}}})
	}))
	defer engine.Close()
	runtime, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: engine.URL})
	store := NewMemoryStore()
	e := Experiment{ID: "one", Owner: "a", RequestID: "request", State: "queued", CreatedAt: time.Now()}
	_, _, _ = store.CreateExperiment(context.Background(), e)
	server := HTTPServer{store: store, evolutionRuntime: runtime}
	got := server.refreshExperiment(context.Background(), e)
	if got.State != "invalid" || got.JobID != "harbor-id" || len(got.Trials) != 1 || got.Trials[0].Rewards["reward"] != 0 {
		t.Fatal("incomplete evidence was admitted or its reward changed", got)
	}
}

func TestExperimentCompletionRequiresValidEvidenceNotAllPassing(t *testing.T) {
	state := harborState{Status: "completed", TasksUnchanged: true}
	for _, variant := range []string{"baseline", "candidate"} {
		for range 3 {
			state.Trials = append(state.Trials, ExperimentTrial{Variant: variant, Valid: true, Rewards: map[string]float64{"reward": 0}})
		}
	}
	e := applyHarborState(Experiment{State: "running"}, state)
	if e.State != "completed" {
		t.Fatal("valid negative results must complete the experiment")
	}
	state.Trials[0].Valid = false
	if applyHarborState(Experiment{State: "running"}, state).State != "invalid" {
		t.Fatal("invalid reward counted as evidence")
	}
}

func TestMemoryExperimentRequiresEveryTaskAttemptAndArm(t *testing.T) {
	state := harborState{Status: "completed", TasksUnchanged: true}
	for _, arm := range []string{"baseline", "memory", "candidate"} {
		for attempt := 1; attempt <= 2; attempt++ {
			for _, task := range []string{"catena/command-handler", "catena/role-config", "catena/absent-symbol"} {
				state.Trials = append(state.Trials, ExperimentTrial{Variant: arm, Task: task, Attempt: attempt, Valid: true, Rewards: map[string]float64{"reward": 0}})
			}
		}
	}
	plan := Experiment{State: "running", Study: "memory_skill"}
	if applyHarborState(plan, state).State != "completed" {
		t.Fatal("valid negative results rejected")
	}
	state.Trials[17] = state.Trials[0]
	if applyHarborState(plan, state).State != "invalid" {
		t.Fatal("duplicate trials admitted")
	}
}

func TestCodexCompletionRequiresNativeSessionAndEnvironmentEvidence(t *testing.T) {
	state := harborState{Status: "completed", TasksUnchanged: true}
	for _, arm := range []string{"baseline", "candidate"} {
		for range 3 { state.Trials = append(state.Trials, ExperimentTrial{Variant: arm, Valid: true, Rewards: map[string]float64{"reward": 1}, NativeSessionVerified: true, EnvironmentVerified: true}) }
	}
	plan := Experiment{State: "running", TargetAgent: "codex"}
	if applyHarborState(plan, state).State != "completed" { t.Fatal("native evidence rejected") }
	state.Trials[0].NativeSessionVerified = false
	if applyHarborState(plan, state).State != "invalid" { t.Fatal("native identity not verified") }
}

func TestExperimentArtifactsRequireOwnerAndKnownTrial(t *testing.T) {
	calls := 0
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/harbor/evidence" {
			t.Error("wrong evidence route")
		}
		writeJSON(w, 200, map[string]any{"task": "fixture", "variant": "memory", "answer": "{}", "calls": []any{}, "bounded": true})
	}))
	defer engine.Close()
	runtime, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: engine.URL})
	store := NewMemoryStore()
	for _, e := range []Experiment{
		{ID: "own", Owner: "local", RequestID: "one", State: "completed", JobID: "job", Trials: []ExperimentTrial{{Task: "fixture", Variant: "memory"}}},
		{ID: "foreign", Owner: "other", RequestID: "two", State: "completed", JobID: "private"},
	} {
		_, _, _ = store.CreateExperiment(context.Background(), e)
	}
	handler, _ := NewHTTPHandlerWithRuntime(store, nil, testEvolutionAuthConfig(), runtime)
	for _, path := range []string{"/v1/experiments/foreign/trials/0", "/v1/experiments/own/trials/-1", "/v1/experiments/own/trials/1"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("foreign or invalid evidence admitted", w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("unowned artifact request reached worker")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/experiments/own/trials/0", nil))
	if w.Code != 200 || calls != 1 {
		t.Fatal("owned evidence unavailable", w.Code)
	}
}
