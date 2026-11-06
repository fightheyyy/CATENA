package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type analysisTraceFixture struct{ recordingTraceStore }

func (s *analysisTraceFixture) GetTrace(ctx context.Context, owner, id string) (TraceDetail, error) {
	return TraceDetail{Summary: TraceSummary{TraceID: id, SpanCount: 2, StartTime: time.Unix(1700000000, 0).UTC(), EndTime: time.Unix(1700000000, 0).UTC()}, Spans: []TraceSpan{{TraceID: id, SpanID: "1122334455667788", Name: "synthetic trace", StartTime: time.Unix(1700000000, 0).UTC(), EndTime: time.Unix(1700000000, 0).UTC()}, {TraceID: id, SpanID: "8877665544332211", Name: "later evidence", StartTime: time.Unix(1700000001, 0).UTC(), EndTime: time.Unix(1700000001, 0).UTC()}}}, nil
}
func TestAnalysisWorkerPersistsCandidateAndEvaluationUnderLease(t *testing.T) {
	store := NewMemoryStore()
	seedTestAnalysisModelConfig(t, store, "local")
	calls := 0
	var controlURL string
	var capability string
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/improve" && r.URL.Path != "/v3/analyze" {
			writeProblem(w, 404, "unknown")
			return
		}
		if r.Header.Get("X-Catena-Owner") != "local" || r.Header.Get("Authorization") != "Bearer private" {
			t.Error("missing trusted identity")
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		calls++
		if r.URL.Path == "/v3/analyze" {
			if payload["schema"] != "catena.analysis_request.v2" || payload["evidence"] != nil {
				t.Error("analysis must start without preloaded evidence")
			}
			query := payload["trace_query"].(map[string]any)
			capability = query["token"].(string)
			for _, queryCase := range []struct {
				token, body string
				status      int
			}{
				{"wrong", `{"tool":"list_traces","arguments":{}}`, 403},
				{capability, `{"tool":"get_trace","arguments":{"trace_id":"00112233445566778899aabbccddeeff"}}`, 403},
				{capability, `{"tool":"list_traces","arguments":{}}`, 200},
				{capability, `{"tool":"get_trace","arguments":{"trace_id":"00112233445566778899aabbccddeeff"}}`, 200},
				{capability, `{"tool":"get_trace","arguments":{"trace_id":"00112233445566778899aabbccddeeff","offset":1,"limit":1}}`, 200},
				{capability, `{"tool":"get_trace","arguments":{"trace_id":"ffffffffffffffffffffffffffffffff"}}`, 403},
				{capability, `{"tool":"list_traces","arguments":{"agent_id":"foreign"}}`, 400},
			} {
				req, _ := http.NewRequest("POST", controlURL+"/internal/analysis-jobs/"+payload["request_id"].(string)+"/trace-query", strings.NewReader(queryCase.body))
				req.Header.Set("Authorization", "Bearer "+queryCase.token)
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				body, _ := io.ReadAll(res.Body)
				res.Body.Close()
				if strings.Contains(queryCase.body, `"offset":1`) {
					var page struct {
						Spans []AnalysisEvidenceSpan `json:"spans"`
						Next  *int                   `json:"next_offset"`
					}
					if json.Unmarshal(body, &page) != nil || len(page.Spans) != 1 || page.Spans[0].SpanID != "8877665544332211" || page.Next != nil {
						t.Error("later Trace page was not returned", string(body))
					}
				}
				if res.StatusCode != queryCase.status {
					t.Errorf("Trace query got %d want %d", res.StatusCode, queryCase.status)
				}
			}
			writeJSON(w, 200, map[string]any{"request_id": payload["request_id"], "state": "completed", "output": map[string]any{"schema": "catena.analysis_result.v1", "status": "completed", "summary": "Fixture observations only", "problems": []any{}}})
			return
		}
		var output map[string]any
		json.Unmarshal(comparisonFixture(), &output)
		output["status"] = "completed"
		output["candidates"] = []any{map[string]any{"id": "candidate-version", "files": map[string]string{"AGENTS.md": "Check tools"}}}
		writeJSON(w, 200, map[string]any{"request_id": payload["request_id"], "state": "completed", "output": output})
	}))
	defer engine.Close()
	manager, _ := NewEngineClient(EngineConfig{URL: engine.URL, Token: "private"})
	h, err := NewHTTPHandler(store, HTTPConfig{Auth: testAnalysisAuthConfig(), Engine: manager, Traces: &analysisTraceFixture{}, Workers: fastWorkerConfig(), Ingestion: DefaultTraceIngestConfig()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(h)
	defer server.Close()
	controlURL = server.URL
	first := httptest.NewRequest("POST", "/v1/analysis-jobs", bytes.NewBufferString(`{"pipeline":"analyze","trace_id":"00112233445566778899aabbccddeeff"}`))
	first.Header.Set("Idempotency-Key", "analysis-first")
	firstResponse := httptest.NewRecorder()
	h.ServeHTTP(firstResponse, first)
	if firstResponse.Code != 202 {
		t.Fatal(firstResponse.Body.String())
	}
	var analysis AnalysisJob
	json.Unmarshal(firstResponse.Body.Bytes(), &analysis)
	eventually(t, func() bool {
		j, _ := store.GetAnalysisJob(context.Background(), analysis.ID)
		return j.State.Terminal()
	})
	analysis, _ = store.GetAnalysisJob(context.Background(), analysis.ID)
	if analysis.State != AnalysisJobCompleted || calls != 1 {
		t.Fatal("analysis must finish without optimization", analysis.Error)
	}
	revoked, _ := http.NewRequest("POST", controlURL+"/internal/analysis-jobs/"+analysis.ID+"/trace-query", strings.NewReader(`{"tool":"list_traces","arguments":{}}`))
	revoked.Header.Set("Authorization", "Bearer "+capability)
	revokedResponse, err := http.DefaultClient.Do(revoked)
	if err != nil {
		t.Fatal(err)
	}
	revokedResponse.Body.Close()
	if revokedResponse.StatusCode != 403 {
		t.Fatal("completed task capability was not revoked")
	}
	foreign := analysis
	foreign.ID = "foreign-analysis"
	foreign.OwnerUserID = "foreign"
	foreign.Artifacts = nil
	foreign.IdempotencyKey = "foreign-key"
	if _, _, err := store.CreateAnalysisJob(context.Background(), foreign); err != nil {
		t.Fatal(err)
	}
	// The explicit owner request is the first human gate; missing/foreign reports cannot start improvement.
	for _, id := range []string{"", "foreign-analysis"} {
		denied := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/analysis-jobs", bytes.NewBufferString(`{"pipeline":"improve","analysis_job_id":"`+id+`","options":{"case_id":"approved-case"}}`))
		r.Header.Set("Idempotency-Key", "denied-"+id)
		h.ServeHTTP(denied, r)
		if denied.Code != 400 && denied.Code != 404 {
			t.Fatal("unapproved optimization accepted", denied.Code)
		}
	}
	raw := []byte(`{"pipeline":"improve","analysis_job_id":"` + analysis.ID + `","options":{"case_id":"approved-case"}}`)
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
	if calls != 2 {
		t.Fatal("unexpected executions")
	}
	if saved.SourceAnalysisJobID != analysis.ID || !bytes.Equal(saved.AnalysisInput, analysis.Checkpoint) || saved.EvidencePack.SHA256 != analysis.EvidencePack.SHA256 {
		t.Fatal("analysis snapshot was not retained")
	}
	if !bytes.Contains(saved.Checkpoint, []byte(`"rewards"`)) {
		t.Fatal("verifier evidence was not persisted")
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
	if _, err := manager.execute(ctx, "owner", "bounded-job", map[string]string{"request_id": "bounded-job"}, "/v3/improve"); err == nil {
		t.Fatal("timeout not propagated")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("cancel was not forwarded")
	}
}
