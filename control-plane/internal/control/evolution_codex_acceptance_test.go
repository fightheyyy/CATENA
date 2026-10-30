package control

// Opt-in integration: real Engine HTTP + pinned Codex CLI + existing Catena API
// and store. By default only this test's model endpoint is a deterministic
// Responses SSE stub. Set CATENA_CODEX_ACCEPT_MODEL_* for a real provider.
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCodexEngineEvolutionAcceptance(t *testing.T) {
	engineURL := os.Getenv("CATENA_CODEX_ACCEPT_ENGINE_URL")
	if engineURL == "" {
		t.Skip("CATENA_CODEX_ACCEPT_ENGINE_URL is not configured")
	}
	ctx := context.Background()
	var store Store = NewMemoryStore()
	if databaseURL := os.Getenv("BARENA_TEST_DATABASE_URL"); databaseURL != "" {
		pg, err := OpenPostgres(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer pg.Close()
		store = pg
	}
	var mu sync.Mutex
	var seen []string
	slowCancelled := make(chan struct{}, 1)
	stopSlow := make(chan struct{})
	stub := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow/responses" {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"slow\",\"status\":\"in_progress\",\"output\":[]}}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				slowCancelled <- struct{}{}
			case <-stopSlow:
			}
			return
		}
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		encoded, _ := json.Marshal(payload)
		text := string(encoded)
		role, answer := "", ""
		switch {
		case strings.Contains(text, "As Inspector"):
			role, answer = "inspector", `{"finding":{"title":"Missing input boundary","summary":"The retained input is incomplete.","severity":"unknown","evidence":["The Run objective uses incomplete input."]}}`
		case strings.Contains(text, "As Evolution"):
			role, answer = "evolution", `{"candidate":{"kind":"agent_md","title":"Check required inputs","summary":"Stop when inputs are incomplete.","content":{"root":"agent.md","files":[{"path":"agent.md","content":"# Required inputs\nBefore creating a result file, check that all required inputs are present. Ask for missing inputs and stop until supplied. Never claim success without evidence.\n"}]}}}`
		case strings.Contains(text, "As Reviewer"):
			role, answer = "reviewer", `{"review":{"verdict":"pass","summary":"The input guard follows the retained incomplete-input evidence."}}`
		default:
			t.Error("Role developer instructions missing from Responses request")
			w.WriteHeader(400)
			return
		}
		// Verify Codex kept its own system instructions and did not advertise
		// host Skills or MCP to the provider.
		instructions, _ := payload["instructions"].(string)
		if instructions == "" || strings.Contains(instructions, "As Inspector") ||
			strings.Contains(text, "## Skills") || strings.Contains(text, "mcp__") {
			t.Errorf("Codex isolation: native_length=%d skill=%v mcp=%v", len(instructions), strings.Contains(text, "## Skills"), strings.Contains(text, "mcp__"))
		}
		if r.Header.Get("Authorization") != "Bearer stub-key" {
			t.Error("owner API key was not used")
		}

		mapped := false
		input, _ := payload["input"].([]any)
		for _, item := range input {
			message, _ := item.(map[string]any)
			content, _ := json.Marshal(message["content"])
			if message["role"] == "developer" && strings.Contains(string(content), "As "+map[string]string{"inspector": "Inspector", "evolution": "Evolution", "reviewer": "Reviewer"}[role]) {
				mapped = true
			}
		}
		if !mapped || payload["stream"] != true || !strings.Contains(text, `"type":"json_schema"`) {
			t.Error("developer mapping, SSE or structured-output contract missing")
		}
		mu.Lock()
		seen = append(seen, role)
		mu.Unlock()
		message := map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "status": "completed",
			"content": []any{map[string]any{"type": "output_text", "text": answer, "annotations": []any{}}}}
		response := map[string]any{"id": "resp_1", "object": "response", "created_at": time.Now().Unix(), "status": "completed",
			"output": []any{message}, "usage": map[string]any{"input_tokens": 100, "output_tokens": 50, "total_tokens": 150}}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": "resp_1", "object": "response", "status": "in_progress", "output": []any{}}},
			{"type": "response.output_item.added", "output_index": 0, "item": message},
			{"type": "response.output_text.delta", "item_id": "msg_1", "output_index": 0, "content_index": 0, "delta": answer},
			{"type": "response.output_item.done", "output_index": 0, "item": message},
			{"type": "response.completed", "response": response},
		}
		for _, event := range events {
			b, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], b)
		}
	}))
	stub.Listener.Close()
	listener, listenErr := net.Listen("tcp", "0.0.0.0:0")
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	stub.Listener = listener
	stub.Start()
	defer stub.Close()
	defer close(stopSlow)
	base := stub.URL
	if host := os.Getenv("CATENA_CODEX_ACCEPT_CALLBACK_HOST"); host != "" {
		base = "http://" + net.JoinHostPort(host, fmt.Sprint(stub.Listener.Addr().(*net.TCPAddr).Port))
	}
	credentials := EvolutionModelCredentials{Provider: "openai", BaseURL: base + "/v1", Model: "gpt-5.4", APIKey: "stub-key"}
	live := os.Getenv("CATENA_CODEX_ACCEPT_MODEL_KEY") != ""
	if live {
		credentials.BaseURL = os.Getenv("CATENA_CODEX_ACCEPT_MODEL_URL")
		credentials.Model = os.Getenv("CATENA_CODEX_ACCEPT_MODEL_NAME")
		credentials.APIKey = os.Getenv("CATENA_CODEX_ACCEPT_MODEL_KEY")
	}
	manager, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: engineURL, Token: os.Getenv("CATENA_CODEX_ACCEPT_ENGINE_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPHandlerWithRuntime(store, nil, testEvolutionAuthConfig(), manager)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	configured := putLLMConfig(t, server.URL, map[string]string{
		"provider": credentials.Provider, "base_url": credentials.BaseURL,
		"model": credentials.Model, "api_key": credentials.APIKey,
	})
	configured.Body.Close()
	if configured.StatusCode != http.StatusOK {
		t.Fatal("owner model API configuration failed")
	}
	// Use the same retained Run fixture as the existing Job tests, then copy
	// it into the selected persistence store without changing production code.
	fixture := NewMemoryStore()
	traceID := "11111111111111111111111111111111"
	run := seedEvolutionRun(t, fixture, "", StateCompleted, traceID)
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	events, err := fixture.ListEventsAfter(ctx, run.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := store.AppendEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	response := postEvolutionJob(t, server.URL+"/v1/runs/"+run.ID+"/evolution-jobs", newID("codex-accept"),
		map[string]any{"trace_id": traceID, "objective": "Add a narrow input guard grounded in this evidence."})
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start: %d %s", response.StatusCode, body)
	}
	var job EvolutionJob
	if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(16 * time.Minute)
	for time.Now().Before(deadline) {
		job, err = store.GetEvolutionJob(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == EvolutionJobFailed {
			t.Fatalf("stage %s: %s", job.CurrentStage, job.Error)
		}
		if job.State == EvolutionJobCompleted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if job.State != EvolutionJobCompleted || job.Finding == nil || job.Candidate == nil || job.Review == nil {
		t.Fatal("three-stage output was not persisted")
	}
	if job.Finding.Title == "Unstructured Inspector result" || len(job.Candidate.Content) == 0 || job.Review.Verdict == "blocked" {
		t.Fatal("structured output did not pass existing Catena validators")
	}
	for _, stage := range job.Stages {
		if stage.State != EvolutionStageCompleted || !strings.Contains(string(stage.RawOutput), "input_tokens") {
			t.Fatal("stage output or usage missing")
		}
	}
	report := map[string]any{"job_id": job.ID, "live_provider": live, "model": credentials.Model, "state": job.State, "stages": job.Stages}
	if reportPath := os.Getenv("CATENA_CODEX_ACCEPT_REPORT"); reportPath != "" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(reportPath, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, stage := range job.Stages {
		var result struct {
			DurationMS int64            `json:"duration_ms"`
			Usage      map[string]int64 `json:"usage"`
		}
		if err := json.Unmarshal(stage.RawOutput, &result); err != nil {
			t.Fatal(err)
		}
		t.Logf("role=%s elapsed=%s duration_ms=%d usage=%v", stage.Role, stage.FinishedAt.Sub(*stage.StartedAt), result.DurationMS, result.Usage)
	}
	mu.Lock()
	defer mu.Unlock()
	if !live && strings.Join(seen, ",") != "inspector,evolution,reviewer" {
		t.Fatalf("unexpected Role order: %v", seen)
	}
	// A fresh DB connection confirms this was committed, not just in memory.
	if databaseURL := os.Getenv("BARENA_TEST_DATABASE_URL"); databaseURL != "" {
		reopened, err := OpenPostgres(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		persisted, err := reopened.GetEvolutionJob(ctx, job.ID)
		if err != nil || persisted.Finding == nil || persisted.Candidate == nil || persisted.Review == nil {
			t.Fatal("reopened DB lost outputs")
		}
	}
	if !live {
		_, err := manager.RunRoleTurn(ctx, EvolutionRoleTurnInput{
			RequestID: newID("codex-unsupported"), RunID: job.ID, Role: "inspector",
			Prompt: "Return a JSON finding.", Timeout: 30 * time.Second,
			Model: EvolutionModelCredentials{Provider: "openai", BaseURL: base + "/unsupported", Model: credentials.Model, APIKey: credentials.APIKey},
		})
		if err == nil || !strings.Contains(err.Error(), "responses_incompatible") || strings.Contains(err.Error(), credentials.APIKey) {
			t.Fatal("unsupported Responses endpoint was not safely reported")
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		_, err = manager.RunRoleTurn(timeoutCtx, EvolutionRoleTurnInput{
			RequestID: newID("codex-timeout"), RunID: job.ID, Role: "inspector",
			Prompt: "Return a JSON finding.", Timeout: 5 * time.Second,
			Model: EvolutionModelCredentials{Provider: "openai", BaseURL: base + "/slow", Model: credentials.Model, APIKey: "stub-key"},
		})
		if err == nil || !strings.Contains(err.Error(), "turn timeout:") || timeoutCtx.Err() != nil {
			t.Fatalf("Engine did not terminate and return its own timeout within cleanup grace: %v", err)
		}
		select {
		case <-slowCancelled:
		case <-time.After(3 * time.Second):
			t.Fatal("timed-out CLI did not disconnect the stalled provider")
		}
	}
	t.Logf("job=%s completed Inspector → Evolution → Reviewer; candidate=%s review=%s live_provider=%v", job.ID, job.Candidate.Kind, job.Review.Verdict, live)
}
