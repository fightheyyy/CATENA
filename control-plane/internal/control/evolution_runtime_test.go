package control

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeEngine serves the catena.engine_*.v1 contract. answer returns the
// assistant content for a role, or ok=false to report a failed turn.
type fakeEngine struct {
	token    string
	answer   func(role string) (content string, ok bool)
	mu       sync.Mutex
	requests []map[string]any
}

func (f *fakeEngine) sent() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.requests...)
}

func (f *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.token != "" && r.Header.Get("Authorization") != "Bearer "+f.token {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/manifest":
		manifest := baseEvolutionManifest()
		manifest.Status = "ready"
		manifest.Version = "fake-engine 0.1.0"
		manifest.Detail = "ready at /host/secret/engine"
		writeJSON(w, http.StatusOK, manifest)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/turn":
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		f.mu.Lock()
		f.requests = append(f.requests, request)
		f.mu.Unlock()
		role, _ := request["role"].(string)
		content, ok := role+": complete", true
		if f.answer != nil {
			content, ok = f.answer(role)
		}
		result := map[string]any{"status": "completed", "assistant": map[string]string{"role": "assistant", "content": content}}
		if !ok {
			result = map[string]any{"status": "failed", "reason_code": "model_error", "detail": content}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema": engineTurnResponseSchema, "request_id": request["request_id"], "status": "ok", "result": result,
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newFakeEngineManager(t *testing.T, engine *fakeEngine) *EvolutionRuntimeManager {
	t.Helper()
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	manager, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{
		URL:          server.URL,
		Token:        engine.token,
		ProbeTimeout: 2 * time.Second,
		CacheTTL:     time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func newFakeEvolutionRuntimeManager(t *testing.T) *EvolutionRuntimeManager {
	return newFakeEngineManager(t, &fakeEngine{})
}

func TestEvolutionRuntimeManagerProbesSanitizedManifest(t *testing.T) {
	manifest := newFakeEvolutionRuntimeManager(t).Probe(context.Background())
	if manifest.Status != "ready" || manifest.RuntimeID != "catena-engine" || manifest.Version != "fake-engine 0.1.0" {
		t.Fatalf("unexpected Runtime manifest: %+v", manifest)
	}
	if len(manifest.Roles) != 3 || manifest.Capabilities.TargetRuntimeHosted {
		t.Fatalf("unexpected Runtime boundary: %+v", manifest)
	}
	encoded, _ := json.Marshal(manifest)
	if strings.Contains(string(encoded), "/host/secret") {
		t.Fatalf("engine detail leaked through Runtime manifest: %s", encoded)
	}
}

func TestEvolutionRuntimeManagerSendsTokenAndModelAndRunsOnlyKnownRoles(t *testing.T) {
	engine := &fakeEngine{token: "engine-token"}
	manager := newFakeEngineManager(t, engine)
	result, err := manager.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
		RequestID: "turn-001",
		RunID:     "run-001",
		Role:      "inspector",
		Prompt:    "Return one Finding.",
		Timeout:   2 * time.Second,
		Model:     testEvolutionModelCredentials(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"status":"completed"`) || !strings.Contains(string(result), "inspector: complete") {
		t.Fatalf("unexpected role turn result: %s", result)
	}
	sent := engine.sent()[0]
	model, _ := sent["model"].(map[string]any)
	if sent["schema"] != engineTurnRequestSchema || sent["timeout_ms"] != float64(2000) ||
		model["api_key"] != "secret-model-key" || model["base_url"] != "https://llm.example.test/v1" {
		t.Fatalf("unexpected engine request: %+v", sent)
	}
	for _, role := range []string{"engineer", "inspector-cat", "user-cat"} {
		if _, err := manager.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
			RequestID: "turn-blocked", RunID: "run-001", Role: role, Prompt: "x",
			Timeout: time.Second, Model: testEvolutionModelCredentials(),
		}); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("role %q should be rejected before reaching the engine, got %v", role, err)
		}
	}
	if len(engine.sent()) != 1 {
		t.Fatalf("rejected roles reached the engine: %d requests", len(engine.sent()))
	}
}

func TestEvolutionRuntimeManagerReportsFailedTurnWithReason(t *testing.T) {
	manager := newFakeEngineManager(t, &fakeEngine{answer: func(string) (string, bool) {
		return "the model did not answer before the turn timeout", false
	}})
	_, err := manager.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
		RequestID: "turn-timeout", RunID: "run-timeout", Role: "inspector", Prompt: "Inspect.",
		Timeout: 2 * time.Second, Model: testEvolutionModelCredentials(),
	})
	if err == nil || !strings.Contains(err.Error(), "model_error") || !strings.Contains(err.Error(), "turn timeout") {
		t.Fatalf("failed role turn must surface an auditable reason, got %v", err)
	}
}

func TestEvolutionRuntimeManagerBlocksOnWrongTokenOrUnreachableEngine(t *testing.T) {
	server := httptest.NewServer(&fakeEngine{token: "right"})
	defer server.Close()
	wrong, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: server.URL, Token: "wrong"})
	if manifest := wrong.Probe(context.Background()); manifest.Status != "blocked" {
		t.Fatalf("wrong token must block the Runtime: %+v", manifest)
	}
	down, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: "http://127.0.0.1:1", ProbeTimeout: time.Second})
	if manifest := down.Probe(context.Background()); manifest.Status != "blocked" || manifest.ReasonCode != "runtime_error" {
		t.Fatalf("unreachable engine must block the Runtime: %+v", manifest)
	}
	_, err := down.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
		RequestID: "t", RunID: "r", Role: "reviewer", Prompt: "x", Timeout: time.Second, Model: testEvolutionModelCredentials(),
	})
	if err == nil || strings.Contains(err.Error(), "secret-model-key") {
		t.Fatalf("unreachable engine must fail without echoing credentials, got %v", err)
	}
}

func TestNewEvolutionRuntimeManagerValidatesURL(t *testing.T) {
	if manager, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{}); manager != nil || err != nil {
		t.Fatalf("empty URL must mean not configured, got %v %v", manager, err)
	}
	for _, value := range []string{"ftp://engine", "http://user:pw@engine", "engine:8790", "http://engine?x=1"} {
		if _, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: value}); err == nil {
			t.Fatalf("invalid engine URL %q was accepted", value)
		}
	}
}

func TestRuntimeAPIExposesEmbeddedRuntimeAndExternalTargetBoundary(t *testing.T) {
	handler, err := NewHTTPHandlerWithRuntime(NewMemoryStore(), nil, AuthConfig{}, newFakeEvolutionRuntimeManager(t))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	body := getBody(t, server.URL+"/v1/runtimes")
	if !strings.Contains(body, `"runtime_id":"catena-engine"`) ||
		!strings.Contains(body, `"target_runtime_hosted":false`) ||
		strings.Contains(body, "/host/secret") {
		t.Fatalf("unexpected Runtime API response: %s", body)
	}
	body = getBody(t, server.URL+"/v1/system/status")
	if !strings.Contains(body, `"evolution_runtime":"ready"`) ||
		!strings.Contains(body, `"run_bundle":"barena.run_bundle.v1"`) ||
		!strings.Contains(body, `"evolution_protocol":"catena.engine_turn_request.v1"`) {
		t.Fatalf("system status does not include Runtime readiness: %s", body)
	}
}

func TestRuntimeAPIReportsBlockedWhenEvolutionRuntimeIsNotConfigured(t *testing.T) {
	server := httptest.NewServer(NewHTTPHandler(NewMemoryStore(), nil))
	defer server.Close()
	if body := getBody(t, server.URL+"/v1/runtimes"); !strings.Contains(body, `"reason_code":"not_configured"`) {
		t.Fatalf("unexpected unconfigured Runtime response: %s", body)
	}
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", url, response.StatusCode, body)
	}
	return string(body)
}

func testEvolutionModelCredentials() EvolutionModelCredentials {
	return EvolutionModelCredentials{
		Provider: "openai",
		BaseURL:  "https://llm.example.test/v1",
		Model:    "test-model",
		APIKey:   "secret-model-key",
	}
}
