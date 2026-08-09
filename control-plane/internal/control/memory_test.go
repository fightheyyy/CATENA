package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTraceMemoryDocumentIsBoundedOrderedAndRedacted(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	trace := TraceDetail{
		Summary: TraceSummary{
			TraceID:     "00112233445566778899aabbccddeeff",
			RootName:    "agent.run",
			ServiceName: "codex",
			StartTime:   now,
			EndTime:     now.Add(time.Second),
			SpanCount:   2,
		},
		Spans: []TraceSpan{
			{
				SpanID:      "02",
				Name:        "tool.second",
				ServiceName: "codex",
				StartTime:   now.Add(time.Second),
				Input:       "Authorization: Bearer abcdefghijklmnop",
			},
			{
				SpanID:      "01",
				Name:        "tool.first",
				ServiceName: "codex",
				StartTime:   now,
				Output:      "api_key=sk_abcdefghijklmnop",
			},
		},
	}

	document := traceMemoryDocument(trace)
	if len(document) > maxMemoryEvidenceBytes {
		t.Fatalf("memory document exceeded bound: %d", len(document))
	}
	if strings.Index(document, "tool.first") > strings.Index(document, "tool.second") {
		t.Fatalf("spans were not ordered by start time: %s", document)
	}
	for _, secret := range []string{"abcdefghijklmnop", "sk_abcdefghijklmnop"} {
		if strings.Contains(document, secret) {
			t.Fatalf("memory document leaked %q: %s", secret, document)
		}
	}
	if !strings.Contains(document, "[REDACTED]") {
		t.Fatalf("memory document did not mark redaction: %s", document)
	}
}

func TestGauzMemoryClientUsesThreePathRecallAndStableOwnerScope(t *testing.T) {
	var ingestProject string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" && r.Header.Get("X-API-Key") != "service-key" {
			t.Fatalf("missing GauzMem service API key for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/memories/extract":
			var body map[string]any
			decodeHTTPRequestJSON(t, r, &body)
			ingestProject, _ = body["project_id"].(string)
			metadata, _ := body["metadata"].(map[string]any)
			if metadata["trace_id"] != "00112233445566778899aabbccddeeff" {
				t.Fatalf("trace provenance missing: %#v", metadata)
			}
			if body["replace"] != true || metadata["chunk_index"] != float64(0) || metadata["file_hash"] == "" {
				t.Fatalf("trace ingest is not idempotent: %#v", body)
			}
			writeTestJSON(t, w, map[string]any{
				"success": true, "conversation_id": 42, "task_id": "memory-task-1",
				"status": "indexing", "indexed": false,
			})
		case "/api/v1/memories/get":
			var body map[string]any
			decodeHTTPRequestJSON(t, r, &body)
			if body["project_id"] != ingestProject {
				t.Fatalf("list scope %v did not match ingest scope %q", body["project_id"], ingestProject)
			}
			writeTestJSON(t, w, map[string]any{
				"memories": []map[string]any{{
					"id": "7", "memory": "The tool failed", "created_at": "2026-08-05T00:00:00Z",
					"metadata": map[string]any{"trace_id": "00112233445566778899aabbccddeeff"},
				}},
				"total": 1,
			})
		case "/api/v1/memories/search/bundle":
			var body map[string]any
			decodeHTTPRequestJSON(t, r, &body)
			if body["project_id"] != ingestProject {
				t.Fatalf("search scope %v did not match ingest scope %q", body["project_id"], ingestProject)
			}
			expansions, _ := body["expansions"].(map[string]any)
			graph, _ := expansions["graph"].(map[string]any)
			temporal, _ := expansions["temporal"].(map[string]any)
			if graph["enabled"] != true || temporal["enabled"] != true {
				t.Fatalf("three-path recall was not enabled: %#v", expansions)
			}
			writeTestJSON(t, w, map[string]any{
				"success": true,
				"query":   body["query"],
				"facts": []map[string]any{{
					"id": "7", "content": "The tool failed", "score": 0.93,
				}},
				"conversations": []any{},
				"topics":        []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewGauzMemoryClient(server.URL, "service-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	trace := TraceDetail{Summary: TraceSummary{
		TraceID:     "00112233445566778899aabbccddeeff",
		RootName:    "agent.run",
		ServiceName: "codex",
		StartTime:   time.Now().UTC(),
		EndTime:     time.Now().UTC(),
		SpanCount:   1,
	}}
	receipt, err := client.IngestTrace(context.Background(), "owner-one", trace)
	if err != nil || receipt.ConversationID != 42 || receipt.TraceID != trace.Summary.TraceID {
		t.Fatalf("unexpected receipt: %#v err=%v", receipt, err)
	}
	memories, err := client.List(context.Background(), "owner-one", 12)
	if err != nil || memories.Total != 1 || len(memories.Memories) != 1 || memories.Memories[0].Content != "The tool failed" {
		t.Fatalf("unexpected memories: %#v err=%v", memories, err)
	}
	bundle, err := client.Search(context.Background(), "owner-one", MemorySearchRequest{Query: "why", TopK: 5})
	if err != nil || len(bundle.Facts) != 1 || bundle.Facts[0].Content != "The tool failed" {
		t.Fatalf("unexpected bundle: %#v err=%v", bundle, err)
	}
	if ingestProject == "" || ingestProject == "owner-one" {
		t.Fatalf("owner scope was not converted to an opaque project: %q", ingestProject)
	}
	if ingestProject == memoryProjectID("owner-two") {
		t.Fatal("different owners resolved to the same memory project")
	}
}

func TestGauzMemoryClientProjectsOwnerScopedTaskProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/task-42" || r.Header.Get("X-API-Key") != "service-key" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(t, w, map[string]any{
			"task_id": "task-42", "project_id": memoryProjectID("owner-one"),
			"status": "processing", "current_step": "向量化", "progress": 0.75,
			"message": "api_key=sk_private_value", "conversation_id": 7,
			"steps": []map[string]any{{"step_name": "事实抽取", "status": "completed"}, {"step_name": "向量化", "status": "processing"}},
		})
	}))
	defer server.Close()
	client, err := NewGauzMemoryClient(server.URL, "service-key")
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.Task(t.Context(), "owner-one", "task-42")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "processing" || task.Progress != 0.75 || task.CurrentStep != "向量化" || len(task.Steps) != 2 {
		t.Fatalf("unexpected task: %#v", task)
	}
	encoded, _ := json.Marshal(task)
	if strings.Contains(string(encoded), "project_id") || strings.Contains(string(encoded), "sk_private_value") {
		t.Fatalf("private task data leaked: %s", encoded)
	}
	if _, err := client.Task(t.Context(), "owner-two", "task-42"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign owner task should be hidden, got %v", err)
	}
}

func TestGauzMemoryClientReadsOwnerScopedFactGraph(t *testing.T) {
	var projectID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/graph/facts/27" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-API-Key") != "service-key" {
			t.Fatal("missing private service credential")
		}
		projectID = r.URL.Query().Get("project_id")
		writeTestJSON(t, w, map[string]any{
			"fact_id":    27,
			"project_id": projectID,
			"content":    "The user requested a release brief.",
			"entities":   []map[string]any{{"name": "release brief", "type": "unknown"}},
			"relations": []map[string]any{{
				"source":     "The user requested a release brief.",
				"target":     "The assistant delivered release-brief.md.",
				"type":       "DELIVEREDAS",
				"confidence": 0.95,
			}},
			"total_entities":  1,
			"total_relations": 1,
		})
	}))
	defer server.Close()

	client, err := NewGauzMemoryClient(server.URL, "service-key")
	if err != nil {
		t.Fatal(err)
	}
	graph, err := client.Graph(context.Background(), "owner-one", 27)
	if err != nil {
		t.Fatal(err)
	}
	if projectID != memoryProjectID("owner-one") || projectID == "owner-one" {
		t.Fatalf("unexpected graph project scope: %q", projectID)
	}
	if graph.FactID != 27 || len(graph.Entities) != 1 || len(graph.Relations) != 1 || graph.Relations[0].Type != "DELIVEREDAS" {
		t.Fatalf("unexpected graph: %#v", graph)
	}
	result, _ := json.Marshal(graph)
	if strings.Contains(string(result), projectID) || strings.Contains(string(result), "project_id") {
		t.Fatalf("private project scope leaked: %s", result)
	}
}

func TestConversationMemoryDocumentContainsOnlyVisibleConversationEvidence(t *testing.T) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	document := ConversationDocument{
		Schema: conversationReadSchema,
		Summary: ConversationSummary{
			ConversationID: "feishu-thread-7", AgentID: "xiaoba-role", AgentName: "小八",
			Runtime: "xiaobaos", Surface: "feishu", CreatedAt: now, UpdatedAt: now.Add(time.Minute), MessageCount: 2,
		},
		Messages: []ConversationMessage{
			{Sequence: 2, OccurredAt: now.Add(time.Minute), Role: "assistant", Content: []ConversationContentPart{{Type: "file", Name: "report.pdf", MIMEType: "application/pdf"}}},
			{Sequence: 1, OccurredAt: now, Role: "user", Content: []ConversationContentPart{{Type: "text", Text: "Authorization: Bearer visible-secret-value"}}},
		},
	}

	result := conversationMemoryDocument(document)
	if strings.Index(result, "role: user") > strings.Index(result, "role: assistant") {
		t.Fatalf("messages were not ordered: %s", result)
	}
	for _, expected := range []string{"feishu-thread-7", "surface: feishu", "report.pdf", "[REDACTED]"} {
		if !strings.Contains(result, expected) {
			t.Fatalf("visible conversation evidence missing %q: %s", expected, result)
		}
	}
	if strings.Contains(result, "visible-secret-value") || strings.Contains(result, "Trace Evidence") {
		t.Fatalf("conversation memory leaked or used Trace evidence: %s", result)
	}
}

func TestGauzMemoryClientIngestsConversationWithStableProvenance(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/memories/extract" {
			http.NotFound(w, r)
			return
		}
		decodeHTTPRequestJSON(t, r, &captured)
		writeTestJSON(t, w, map[string]any{
			"success": true, "conversation_id": 77, "task_id": "conversation-memory-1", "status": "indexing",
		})
	}))
	defer server.Close()
	client, err := NewGauzMemoryClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	document := ConversationDocument{
		Schema: conversationReadSchema,
		Summary: ConversationSummary{
			ConversationID: "pet-42", AgentID: "xiaoba-base", Runtime: "xiaobaos", Surface: "pet",
			CreatedAt: now, UpdatedAt: now, MessageCount: 1,
		},
		Messages: []ConversationMessage{{Sequence: 1, OccurredAt: now, Role: "user", Content: []ConversationContentPart{{Type: "text", Text: "remember this"}}}},
	}
	receipt, err := client.IngestConversation(context.Background(), "owner-one", document)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SourceConversationID != "pet-42" || receipt.ConversationID != 77 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	metadata, _ := captured["metadata"].(map[string]any)
	if metadata["source_type"] != "catena_conversation" || metadata["conversation_id"] != "pet-42" || metadata["agent_id"] != "xiaoba-base" {
		t.Fatalf("conversation provenance missing: %#v", metadata)
	}
	if captured["replace"] != true || metadata["file_hash"] == "" {
		t.Fatalf("conversation ingest is not idempotent: %#v", captured)
	}
}

func TestRememberTraceReadsOnlyTheAuthenticatedOwnerScope(t *testing.T) {
	traceID := "00112233445566778899aabbccddeeff"
	traces := &memoryTraceStore{trace: TraceDetail{Summary: TraceSummary{TraceID: traceID}}}
	memory := &recordingMemoryBackend{}
	server := &HTTPServer{
		store:  NewMemoryStore(),
		traces: traces,
		memory: memory,
		auth:   AuthConfig{}.normalized(),
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/traces/"+traceID+"/memories", nil)
	request.SetPathValue("trace_id", traceID)
	recorder := httptest.NewRecorder()

	server.rememberTrace(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if traces.ownerID != "local" || memory.ownerID != "local" || memory.traceID != traceID {
		t.Fatalf("owner/trace boundary mismatch: traces=%q memory=%q trace=%q", traces.ownerID, memory.ownerID, memory.traceID)
	}
}

func TestRememberConversationReadsOnlyTheAuthenticatedOwnerScope(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	_, _, err := store.IngestConversationMessages(context.Background(), "", []ConversationMessage{{
		Schema: conversationMessageSchema, MessageID: "message-1", ConversationID: "conversation-1",
		Sequence: 1, OccurredAt: now, Runtime: "xiaobaos", AgentID: "xiaoba-base", Surface: "pet",
		Role: "user", Content: []ConversationContentPart{{Type: "text", Text: "please remember"}},
		Delivery: ConversationDelivery{Status: "received"},
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	memory := &recordingMemoryBackend{}
	server := &HTTPServer{store: store, memory: memory, auth: AuthConfig{}.normalized()}
	request := httptest.NewRequest(http.MethodPost, "/v1/conversations/conversation-1/memories?agent_id=xiaoba-base", nil)
	request.SetPathValue("conversation_id", "conversation-1")
	recorder := httptest.NewRecorder()

	server.rememberConversation(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if memory.ownerID != "local" || memory.conversationID != "conversation-1" {
		t.Fatalf("owner/conversation boundary mismatch: owner=%q conversation=%q", memory.ownerID, memory.conversationID)
	}
}

func TestMemoryTaskStatusUsesAuthenticatedOwnerAndHidesProviderErrors(t *testing.T) {
	memory := &recordingMemoryBackend{task: MemoryTaskStatus{TaskID: "task-1", Status: "processing", Progress: 0.5}}
	server := &HTTPServer{store: NewMemoryStore(), memory: memory, auth: AuthConfig{}.normalized()}
	request := httptest.NewRequest(http.MethodGet, "/v1/memories/tasks/task-1", nil)
	request.SetPathValue("task_id", "task-1")
	recorder := httptest.NewRecorder()
	server.memoryTaskStatus(recorder, request)
	if recorder.Code != http.StatusOK || memory.ownerID != "local" || memory.taskID != "task-1" || !strings.Contains(recorder.Body.String(), `"progress":0.5`) {
		t.Fatalf("unexpected task response: status=%d owner=%q task=%q body=%s", recorder.Code, memory.ownerID, memory.taskID, recorder.Body.String())
	}

	memory.taskErr = errors.New("provider api_key=sk_private_value")
	recorder = httptest.NewRecorder()
	server.memoryTaskStatus(recorder, request)
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "sk_private_value") {
		t.Fatalf("provider detail leaked: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMemoryRecallDoesNotExposeProviderErrors(t *testing.T) {
	memory := &recordingMemoryBackend{searchErr: errors.New("provider rejected api_key=sk_private_value")}
	server := &HTTPServer{
		store:  NewMemoryStore(),
		memory: memory,
		auth:   AuthConfig{}.normalized(),
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/memories/search",
		strings.NewReader(`{"query":"why did the release fail?"}`),
	)
	recorder := httptest.NewRecorder()

	server.searchMemories(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "sk_private_value") ||
		!strings.Contains(recorder.Body.String(), "GauzMem recall failed") {
		t.Fatalf("provider detail leaked or safe message missing: %s", recorder.Body.String())
	}
}

func TestMemoryFactGraphUsesAuthenticatedOwnerAndHidesProviderErrors(t *testing.T) {
	memory := &recordingMemoryBackend{graphErr: errors.New("provider rejected api_key=sk_private_value")}
	server := &HTTPServer{store: NewMemoryStore(), memory: memory, auth: AuthConfig{}.normalized()}
	request := httptest.NewRequest(http.MethodGet, "/v1/memories/facts/27/graph", nil)
	request.SetPathValue("fact_id", "27")
	recorder := httptest.NewRecorder()

	server.memoryFactGraph(recorder, request)

	if memory.ownerID != "local" || memory.factID != 27 {
		t.Fatalf("owner/fact boundary mismatch: owner=%q fact=%d", memory.ownerID, memory.factID)
	}
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "sk_private_value") || !strings.Contains(recorder.Body.String(), "Memory graph is unavailable") {
		t.Fatalf("provider detail leaked or safe message missing: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

type memoryTraceStore struct {
	ownerID string
	trace   TraceDetail
}

func (s *memoryTraceStore) Ping(context.Context) error { return nil }
func (s *memoryTraceStore) Close() error               { return nil }
func (s *memoryTraceStore) InsertSpans(context.Context, string, []TraceSpan) error {
	return nil
}
func (s *memoryTraceStore) ListTraces(context.Context, string, int) ([]TraceSummary, error) {
	return nil, nil
}
func (s *memoryTraceStore) ListAgentTraces(_ context.Context, ownerID string, agentID string, windowStart time.Time, windowEnd time.Time, _ int) ([]TraceSummary, error) {
	if s.ownerID != "" && ownerID != s.ownerID {
		return nil, nil
	}
	if s.trace.Summary.ServiceName != agentID || s.trace.Summary.EndTime.Before(windowStart) || s.trace.Summary.StartTime.After(windowEnd) {
		return nil, nil
	}
	return []TraceSummary{s.trace.Summary}, nil
}
func (s *memoryTraceStore) GetTrace(_ context.Context, ownerID string, traceID string) (TraceDetail, error) {
	s.ownerID = ownerID
	if traceID != s.trace.Summary.TraceID {
		return TraceDetail{}, ErrNotFound
	}
	return s.trace, nil
}
func (s *memoryTraceStore) ListAgents(context.Context, string, int) ([]AgentSummary, error) {
	return nil, nil
}

type recordingMemoryBackend struct {
	ownerID        string
	traceID        string
	conversationID string
	searchErr      error
	factID         int64
	graphErr       error
	taskID         string
	task           MemoryTaskStatus
	taskErr        error
}

func (s *recordingMemoryBackend) Task(_ context.Context, ownerID string, taskID string) (MemoryTaskStatus, error) {
	s.ownerID = ownerID
	s.taskID = taskID
	return s.task, s.taskErr
}

func (s *recordingMemoryBackend) IngestConversation(_ context.Context, ownerID string, document ConversationDocument) (MemoryIngestReceipt, error) {
	s.ownerID = ownerID
	s.conversationID = document.Summary.ConversationID
	return MemoryIngestReceipt{
		SourceConversationID: document.Summary.ConversationID, ConversationID: 1, TaskID: "task", Status: "indexing",
	}, nil
}

func (s *recordingMemoryBackend) Ping(context.Context) error { return nil }
func (s *recordingMemoryBackend) IngestTrace(_ context.Context, ownerID string, trace TraceDetail) (MemoryIngestReceipt, error) {
	s.ownerID = ownerID
	s.traceID = trace.Summary.TraceID
	return MemoryIngestReceipt{
		TraceID: trace.Summary.TraceID, ConversationID: 1, TaskID: "task", Status: "indexing",
	}, nil
}
func (s *recordingMemoryBackend) List(context.Context, string, int) (MemoryList, error) {
	return MemoryList{}, nil
}
func (s *recordingMemoryBackend) Search(context.Context, string, MemorySearchRequest) (MemoryRecallBundle, error) {
	return MemoryRecallBundle{}, s.searchErr
}
func (s *recordingMemoryBackend) Graph(_ context.Context, ownerID string, factID int64) (MemoryFactGraph, error) {
	s.ownerID = ownerID
	s.factID = factID
	return MemoryFactGraph{FactID: factID, Content: "fact"}, s.graphErr
}

func decodeHTTPRequestJSON(t *testing.T, r *http.Request, value any) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		t.Fatalf("decode request: %v body=%s", err, body)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
