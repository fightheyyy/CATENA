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

func TestMemoryTaskStatusPersistsProgressAcrossNavigation(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	record := MemoryTaskRecord{
		MemoryTaskStatus: MemoryTaskStatus{TaskID: "task-1", Status: "pending", Progress: 0, Steps: []MemoryTaskStep{}},
		OwnerUserID:      "local", SourceConversationID: "conversation-1", AgentID: "xiaoba-base",
		CreatedAtTime: now, UpdatedAtTime: now,
	}
	if err := store.UpsertMemoryTask(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	memory := &recordingMemoryBackend{task: MemoryTaskStatus{TaskID: "task-1", Status: "processing", Progress: 0.5, CurrentStep: "关系抽取", Steps: []MemoryTaskStep{}}}
	server := &HTTPServer{store: store, memory: memory, auth: AuthConfig{}.normalized()}
	request := httptest.NewRequest(http.MethodGet, "/v1/memories/tasks/task-1", nil)
	request.SetPathValue("task_id", "task-1")
	recorder := httptest.NewRecorder()
	server.memoryTaskStatus(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"source_conversation_id":"conversation-1"`) {
		t.Fatalf("durable task response lost source data: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	stored, err := store.GetMemoryTaskByOwner(t.Context(), "local", "task-1")
	if err != nil || stored.Status != "processing" || stored.Progress != 0.5 || stored.CurrentStep != "关系抽取" {
		t.Fatalf("task progress was not persisted: task=%#v err=%v", stored, err)
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
		!strings.Contains(recorder.Body.String(), "Memory provider recall failed") {
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

func TestMemoryFactGraphAddsTruthfulProvenanceAndConversationRelations(t *testing.T) {
	memory := &recordingMemoryBackend{
		graph: MemoryFactGraph{FactID: 27, Content: "用户要求整理发布说明。", Entities: []MemoryGraphEntity{}, Relations: []MemoryGraphRelation{}},
		list: MemoryList{Memories: []MemoryRecord{
			{ID: "27", Content: "用户要求整理发布说明。", Metadata: map[string]any{"conversation_id": "pet-42", "agent_id": "xiaoba-base", "agent_name": "小八"}},
			{ID: "28", Content: "小八交付了发布说明。", Metadata: map[string]any{"conversation_id": "pet-42", "agent_id": "xiaoba-base"}},
		}},
	}
	server := &HTTPServer{store: NewMemoryStore(), memory: memory, auth: AuthConfig{}.normalized()}
	request := httptest.NewRequest(http.MethodGet, "/v1/memories/facts/27/graph", nil)
	request.SetPathValue("fact_id", "27")
	recorder := httptest.NewRecorder()
	server.memoryFactGraph(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var graph MemoryFactGraph
	if err := json.Unmarshal(recorder.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	if graph.TotalEntities != 2 || graph.TotalRelations != 3 {
		t.Fatalf("expected Agent, Conversation, and same-conversation provenance: %#v", graph)
	}
	for _, relationType := range []string{"SOURCE_CONVERSATION", "SOURCE_AGENT", "SAME_CONVERSATION"} {
		found := false
		for _, relation := range graph.Relations {
			if relation.Type == relationType && relation.Origin == "provenance" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s provenance relation: %#v", relationType, graph.Relations)
		}
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
	graph          MemoryFactGraph
	list           MemoryList
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
	return s.list, nil
}
func (s *recordingMemoryBackend) Search(context.Context, string, MemorySearchRequest) (MemoryRecallBundle, error) {
	return MemoryRecallBundle{}, s.searchErr
}
func (s *recordingMemoryBackend) Graph(_ context.Context, ownerID string, factID int64) (MemoryFactGraph, error) {
	s.ownerID = ownerID
	s.factID = factID
	if s.graph.FactID != 0 {
		return s.graph, s.graphErr
	}
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
