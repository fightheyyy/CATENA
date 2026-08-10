package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScenarioRunAdoptionIsTerminalIdempotentEvidence(t *testing.T) {
	store := NewMemoryStore()
	server := httptest.NewServer(NewHTTPHandler(store, nil))
	defer server.Close()

	traceID := "0123456789abcdef0123456789abcdef"
	startedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	payload := validScenarioAdoptionPayload(traceID, startedAt)

	first := adoptScenarioForTest(t, server.URL, "project-one", payload)
	if first.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(first.Body)
		first.Body.Close()
		t.Fatalf("first adoption returned %d: %s", first.StatusCode, body)
	}
	var adopted ScenarioRunAdoptionResponse
	if err := json.NewDecoder(first.Body).Decode(&adopted); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	if !adopted.Created || adopted.Run.Origin != OriginPlatform ||
		adopted.Run.State != StateCompleted || adopted.Run.Operation != OperationExplore {
		t.Fatalf("unexpected adopted Run: %#v", adopted)
	}
	var runInput struct {
		Source struct {
			Kind string `json:"kind"`
		} `json:"source"`
	}
	if err := json.Unmarshal(adopted.Run.Input, &runInput); err != nil {
		t.Fatal(err)
	}
	if runInput.Source.Kind != "catena_scenario_run" {
		t.Fatalf("source kind = %q, want catena_scenario_run", runInput.Source.Kind)
	}
	if retained, err := store.RunHasTrace(context.Background(), adopted.Run.ID, traceID); err != nil || !retained {
		t.Fatalf("adopted Trace was not retained: retained=%v err=%v", retained, err)
	}

	retry := adoptScenarioForTest(t, server.URL, "project-one", payload)
	if retry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retry.Body)
		retry.Body.Close()
		t.Fatalf("idempotent adoption returned %d: %s", retry.StatusCode, body)
	}
	var retried ScenarioRunAdoptionResponse
	if err := json.NewDecoder(retry.Body).Decode(&retried); err != nil {
		t.Fatal(err)
	}
	retry.Body.Close()
	if retried.Created || retried.Run.ID != adopted.Run.ID {
		t.Fatalf("retry was not idempotent: %#v", retried)
	}

	payload["scenario"].(map[string]any)["objective"] = "mutated source facts"
	conflict := adoptScenarioForTest(t, server.URL, "project-one", payload)
	if conflict.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(conflict.Body)
		conflict.Body.Close()
		t.Fatalf("mutated adoption returned %d: %s", conflict.StatusCode, body)
	}
	conflict.Body.Close()

	evaluations, err := store.ListEvaluations(context.Background(), 100)
	if err != nil || len(evaluations) != 0 {
		t.Fatalf("adoption created an Evaluation: %#v err=%v", evaluations, err)
	}
	releases, err := store.ListReleases(context.Background(), 100)
	if err != nil || len(releases) != 0 {
		t.Fatalf("adoption created a Release: %#v err=%v", releases, err)
	}
}

func TestScenarioRunAdoptionRejectsWrongProjectAndSecretFields(t *testing.T) {
	store := NewMemoryStore()
	server := httptest.NewServer(NewHTTPHandler(store, nil))
	defer server.Close()
	payload := validScenarioAdoptionPayload(
		"fedcba9876543210fedcba9876543210",
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	)

	wrongProject := adoptScenarioForTest(t, server.URL, "project-two", payload)
	if wrongProject.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong project returned %d", wrongProject.StatusCode)
	}
	wrongProject.Body.Close()

	payload["replay"].(map[string]any)["authorization"] = "Bearer secret"
	secret := adoptScenarioForTest(t, server.URL, "project-one", payload)
	if secret.StatusCode != http.StatusBadRequest {
		t.Fatalf("secret-bearing payload returned %d", secret.StatusCode)
	}
	secret.Body.Close()
}

func validScenarioAdoptionPayload(traceID string, startedAt time.Time) map[string]any {
	return map[string]any{
		"schema":            "barena.scenario_run_adoption.v1",
		"source_project_id": "project-one",
		"scenario_run_id":   "scenario-run-one",
		"scenario_id":       "scenario-one",
		"source_status":     "FAILED",
		"started_at":        startedAt,
		"completed_at":      startedAt.Add(2 * time.Second),
		"duration_in_ms":    2000,
		"scenario": map[string]any{
			"name": "Clarification behavior", "objective": "Ask before acting",
			"criteria": []string{"The Agent asks one clarifying question"},
		},
		"target": map[string]any{
			"type": "http", "reference_id": "agent-one", "name": "XiaoBa HTTP",
		},
		"trace_ids":        []string{traceID},
		"primary_trace_id": traceID,
		"judge": map[string]any{
			"verdict": "failure", "reasoning": "The Agent acted immediately.",
			"met_criteria": []string{}, "unmet_criteria": []string{"clarify first"},
		},
		"replay": map[string]any{
			"supported": true, "url": "http://127.0.0.1:9000/chat",
			"method": "POST", "output_path": "$.response", "timeout_ms": 5000,
		},
	}
}

func adoptScenarioForTest(
	t *testing.T,
	serverURL string,
	projectID string,
	payload map[string]any,
) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		serverURL+"/v1/platform/scenario-runs/adopt",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Barena-Project-ID", projectID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
