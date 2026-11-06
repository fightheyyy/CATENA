package control

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredMemoryAndCommunityCannotExecute(t *testing.T) {
	engine, _ := NewEngineClient(EngineConfig{URL: "http://127.0.0.1:1", Token: "private"})
	h, err := NewHTTPHandler(NewMemoryStore(), HTTPConfig{Engine: engine, Traces: &analysisTraceFixture{}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, path := range []string{"/v1/memories", "/v1/community/profiles", "/v1/me/profile"} {
		out := httptest.NewRecorder()
		h.ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if out.Code != 404 {
			t.Fatalf("%s: %d", path, out.Code)
		}
	}
	for _, raw := range []string{`{"pipeline":"memory"}`, `{"pipeline":"improve","adapter":"other"}`, `{"pipeline":"improve","conversation_id":"old-session"}`, `{"pipeline":"rsi"}`, `{"pipeline":"improve","options":{"harness2":{}}}`} {
		out := httptest.NewRecorder()
		h.ServeHTTP(out, httptest.NewRequest("POST", "/v1/analysis-jobs", strings.NewReader(raw)))
		if out.Code != 400 {
			t.Fatalf("retired input: %d %s", out.Code, out.Body.String())
		}
	}
	if value := redactEvidenceText("api_key=sk_abcdefghijklmnop Bearer abcdefghijklmnop"); strings.Contains(value, "abcdefghijklmnop") {
		t.Fatal("shared Evidence redaction was lost")
	}
}
