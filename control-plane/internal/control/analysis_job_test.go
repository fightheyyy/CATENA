package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type analysisTraceFixture struct{ recordingTraceStore }

func (s *analysisTraceFixture) GetTrace(ctx context.Context, owner, id string) (TraceDetail, error) {
	return TraceDetail{Summary: TraceSummary{TraceID: id, SpanCount: 1, StartTime: time.Now(), EndTime: time.Now()}, Spans: []TraceSpan{{TraceID: id, SpanID: "1122334455667788", Name: "synthetic trace", StartTime: time.Now(), EndTime: time.Now()}}}, nil
}
func TestAnalysisWorkerPersistsCandidateAndEvaluationUnderLease(t *testing.T) {
	store := NewMemoryStore()
	seedTestEvolutionModelConfig(t, store, "local")
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
	manager, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: engine.URL, Token: "private"})
	h, err := NewHTTPHandlerWithBackendConfig(store, nil, testEvolutionAuthConfig(), manager, &analysisTraceFixture{}, nil, fastWorkerConfig(), DefaultTraceIngestConfig())
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
	var job EvolutionJob
	json.NewDecoder(response.Body).Decode(&job)
	eventually(t, func() bool { j, _ := store.GetEvolutionJob(context.Background(), job.ID); return j.State.Terminal() })
	saved, _ := store.GetEvolutionJob(context.Background(), job.ID)
	if saved.State != EvolutionJobCompleted || len(saved.Artifacts) != 1 {
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
			writeJSON(w, 200, MemoryIngestReceipt{TaskID: "native-extraction", Status: "pending"})
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
	manager, _ := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{URL: engine.URL, Token: "private"})
	h, err := NewHTTPHandlerWithBackendConfig(store, nil, testEvolutionAuthConfig(), manager, &analysisTraceFixture{}, nil, fastWorkerConfig(), DefaultTraceIngestConfig())
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
	var job EvolutionJob
	json.Unmarshal(out.Body.Bytes(), &job)
	eventually(t, func() bool { j, _ := store.GetEvolutionJob(context.Background(), job.ID); return j.State.Terminal() })
	saved, _ := store.GetEvolutionJob(context.Background(), job.ID)
	if saved.State != EvolutionJobCompleted || extracts != 1 || polls != 2 || saved.Attempt != 2 || len(saved.Artifacts) != 1 {
		t.Fatalf("state=%s attempts=%d extracts=%d polls=%d error=%s", saved.State, saved.Attempt, extracts, polls, saved.Error)
	}
	a := saved.Artifacts[0]
	if a.Type != "engineering_memory" || a.ValidationStatus != "extracted" || !bytes.Contains(a.Payload, []byte("memory_count")) {
		t.Fatal("missing provider evidence")
	}
}
