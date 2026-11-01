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

type analysisTraceFixture struct{ recordingTraceStore }

func (s *analysisTraceFixture) GetTrace(ctx context.Context, owner, id string) (TraceDetail, error) {
	return TraceDetail{Summary: TraceSummary{TraceID: id, SpanCount: 1, StartTime: time.Unix(1700000000, 0).UTC(), EndTime: time.Unix(1700000000, 0).UTC()}, Spans: []TraceSpan{{TraceID: id, SpanID: "1122334455667788", Name: "synthetic trace", StartTime: time.Unix(1700000000, 0).UTC(), EndTime: time.Unix(1700000000, 0).UTC()}}}, nil
}
func TestAnalysisWorkerPersistsCandidateAndEvaluationUnderLease(t *testing.T) {
	store := NewMemoryStore()
	seedTestAnalysisModelConfig(t, store, "local")
	calls := 0
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/rsi" {
			writeProblem(w, 404, "unknown")
			return
		}
		if r.Header.Get("X-Catena-Owner") != "local" || r.Header.Get("Authorization") != "Bearer private" {
			t.Error("missing trusted identity")
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		calls++
		writeJSON(w, 200, map[string]any{"request_id": payload["request_id"], "state": "completed", "output": map[string]any{"status": "completed", "candidates": []any{map[string]any{"id": "candidate-version", "files": map[string]string{"AGENTS.md": "Check tools"}}}, "trials": []any{map[string]any{"phase": "validation", "valid": true, "rewards": map[string]int{"reward": 1}}}, "validation_case_ids": []string{"heldout"}}})
	}))
	defer engine.Close()
	manager, _ := NewEngineClient(EngineConfig{URL: engine.URL, Token: "private"})
	h, err := NewHTTPHandler(store, HTTPConfig{Auth: testAnalysisAuthConfig(), Engine: manager, Traces: &analysisTraceFixture{}, Memory: nil, Workers: fastWorkerConfig(), Ingestion: DefaultTraceIngestConfig()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(h)
	defer server.Close()
	raw := []byte(`{"pipeline":"rsi","trace_id":"00112233445566778899aabbccddeeff","options":{"case_id":"approved-case"}}`)
	req, _ := http.NewRequest("POST", server.URL+"/v1/analysis-jobs", bytes.NewReader(raw))
	req.Header.Set("Idempotency-Key", "stable-job")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatal(response.Status)
	}
	var job AnalysisJob
	json.NewDecoder(response.Body).Decode(&job)
	retry := httptest.NewRequest("POST", "/v1/analysis-jobs", bytes.NewReader(raw))
	retry.Header.Set("Idempotency-Key", "stable-job")
	again := httptest.NewRecorder()
	h.ServeHTTP(again, retry)
	if again.Code != 200 {
		t.Fatalf("idempotent HTTP retry: %d %s", again.Code, again.Body.String())
	}
	eventually(t, func() bool { j, _ := store.GetAnalysisJob(context.Background(), job.ID); return j.State.Terminal() })
	saved, _ := store.GetAnalysisJob(context.Background(), job.ID)
	if saved.State != AnalysisJobCompleted || len(saved.Artifacts) != 1 {
		t.Fatalf("job failed: %s", saved.Error)
	}
	if saved.Artifacts[0].ValidationStatus != "review_required" || len(saved.Artifacts[0].EvidenceRefs) != 1 {
		t.Fatal("missing source or verification")
	}
	if calls != 1 {
		t.Fatal("unexpected executions")
	}
	if !bytes.Contains(saved.Checkpoint, []byte(`"rewards"`)) {
		t.Fatal("verifier evidence was not persisted")
	}
}

func TestMemoryJobResumesNativeTaskWithoutRepeatingExtraction(t *testing.T) {
	store := NewMemoryStore()
	extracts, polls := 0, 0
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Catena-Owner") != "local" {
			t.Error("wrong owner")
		}
		switch r.URL.Path {
		case "/v2/memory/extract":
			extracts++
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			writeJSON(w, 200, map[string]any{"request_id": payload["request_id"], "state": "completed", "output": MemoryIngestReceipt{TaskID: "native-extraction", Status: "pending"}})
		case "/v2/memory/task":
			polls++
			if polls == 1 {
				writeProblem(w, 503, "temporary provider outage")
			} else {
				writeJSON(w, 200, MemoryTaskStatus{TaskID: "native-extraction", Status: "completed", Result: json.RawMessage(`{"memory_count":1}`)})
			}
		default:
			writeProblem(w, 404, "unknown")
		}
	}))
	defer engine.Close()
	manager, _ := NewEngineClient(EngineConfig{URL: engine.URL, Token: "private"})
	h, err := NewHTTPHandler(store, HTTPConfig{Auth: testAnalysisAuthConfig(), Engine: manager, Traces: &analysisTraceFixture{}, Memory: nil, Workers: fastWorkerConfig(), Ingestion: DefaultTraceIngestConfig()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	req := httptest.NewRequest("POST", "/v1/analysis-jobs", bytes.NewBufferString(`{"pipeline":"memory","trace_id":"00112233445566778899aabbccddeeff"}`))
	req.Header.Set("Idempotency-Key", "memory-retry")
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != 202 {
		t.Fatal(out.Body.String())
	}
	var job AnalysisJob
	json.Unmarshal(out.Body.Bytes(), &job)
	eventually(t, func() bool { j, _ := store.GetAnalysisJob(context.Background(), job.ID); return j.State.Terminal() })
	saved, _ := store.GetAnalysisJob(context.Background(), job.ID)
	if saved.State != AnalysisJobCompleted || extracts != 1 || polls != 2 || saved.Attempt != 2 || len(saved.Artifacts) != 1 {
		t.Fatalf("state=%s attempts=%d extracts=%d polls=%d error=%s", saved.State, saved.Attempt, extracts, polls, saved.Error)
	}
	a := saved.Artifacts[0]
	if a.Type != "engineering_memory" || a.ValidationStatus != "extracted" || !bytes.Contains(a.Payload, []byte("memory_count")) {
		t.Fatal("missing provider evidence")
	}
}

func TestEngineClientTimeoutCancelsExternalExecution(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			cancelled <- struct{}{}
			writeJSON(w, 202, map[string]string{"state": "cancelling"})
			return
		}
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer engine.Close()
	manager, _ := NewEngineClient(EngineConfig{URL: engine.URL, Token: "private", RequestTimeout: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.execute(ctx, "owner", "bounded-job", map[string]string{"request_id": "bounded-job"}, "/v2/rsi"); err == nil {
		t.Fatal("timeout not propagated")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("cancel was not forwarded")
	}
}

func TestConversationEvidenceKeepsSourceAndRedactsSecrets(t *testing.T) {
	now := time.Now().UTC()
	pack, err := conversationEvidencePack("registered-agent", "session", []ConversationMessage{{Sequence: 1, Role: "user", OccurredAt: now, Content: []ConversationContentPart{{Type: "text", Text: "api_key=sk_abcdefghijklmnop"}}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	job := AnalysisJob{Pipeline: "memory", SourceKind: AnalysisSourceConversation, SourceAgentID: pack.SourceAgentID, SourceConversationID: pack.SourceConversationID, EvidencePack: &pack}
	if !validAnalysisJobSource(job) || !validAnalysisEvidencePack(pack, job) || bytes.Contains([]byte(pack.Document), []byte("abcdefghijklmnop")) {
		t.Fatal("invalid or unredacted conversation evidence")
	}
	job.SourceConversationID = "foreign"
	if validAnalysisEvidencePack(pack, job) {
		t.Fatal("foreign session evidence accepted")
	}
}
