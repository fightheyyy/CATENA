package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkflowHarnessCapacityToolScopeAndLeaseRevocation(t *testing.T) {
	store, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	managed := submitManagedTest(t, store, "local", seedManaged(t, store, "local"), "managed")
	persistent, err := store.claimHarness(ctx, "managed", time.Minute, 1, false)
	if err != nil || persistent == nil || persistent.Run.ID != managed.ID {
		t.Fatalf("managed claim: %v, %v", persistent, err)
	}
	job := seedQueueJob(t, store)
	job.Pipeline = "improve"
	if err := store.UpdateAnalysisJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	_, lease, err := store.ClaimAnalysisJob(ctx, "workflow-job", time.Minute, 3)
	if err != nil || job.ID != lease.JobID {
		t.Fatalf("workflow job lease: %v, %v", lease, err)
	}
	binding := workflowAccess{ctx, lease, "local", "improve"}
	input := workflowTurn{"bounded input", strings.Repeat("a", 32), 60}
	id, err := store.submitWorkflowTurn(ctx, binding, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.submitWorkflowTurn(ctx, binding, input); err == nil {
		t.Fatal("one business job admitted a duplicate turn")
	}
	ephemeral, err := store.claimHarness(ctx, "ephemeral", time.Minute, 1, true)
	if err != nil || ephemeral == nil || ephemeral.Run.ID != id || ephemeral.Agent.Profile != "improve" {
		t.Fatalf("reserved workflow claim: %v, %v", ephemeral, err)
	}
	if _, err = store.db.Exec(`UPDATE agent_runs SET execution_started_at=now() WHERE run_id=$1`, id); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/internal/harness/tool/"+job.ID+"/"+input.Callback ||
			r.Header.Get("X-Catena-Owner") != "local" || r.Header.Get("Authorization") != "Bearer engine-fixture" {
			t.Error("tool callback was not bound to server-owned identity")
		}
		writeJSON(w, 200, json.RawMessage(`{"validated":true}`))
	}))
	defer engine.Close()
	client, err := NewEngineClient(EngineConfig{URL: engine.URL, Token: "engine-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	server := &HTTPServer{store: store, analysisRuntime: client}
	server.managed = &managedPlatform{store: store, server: server}
	invoke := func(token, tool string) int {
		r := httptest.NewRequest("POST", "/internal/harness/tool", strings.NewReader(`{"tool":"`+tool+`","arguments":{}}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.workflowTool(w, r)
		return w.Code
	}
	public := httptest.NewRequest("GET", "/v1/agent-runs/"+id, nil)
	public.SetPathValue("run_id", id)
	unlisted := httptest.NewRecorder()
	server.managedRunDetail(unlisted, public)
	if unlisted.Code != 404 {
		t.Fatalf("private workflow run is reachable via Managed API: %d", unlisted.Code)
	}
	if code := invoke(ephemeral.Token, "evaluate_candidate"); code != 200 {
		t.Fatalf("authorized callback: %d", code)
	}
	if code := invoke(ephemeral.Token, "get_trace"); code != 403 {
		t.Fatalf("unexpected cross-profile permission: %d", code)
	}
	if code := invoke(persistent.Token, "evaluate_candidate"); code != 403 {
		t.Fatalf("persistent execution obtained trusted tool: %d", code)
	}
	if _, err = store.db.Exec(`UPDATE spiral_evolution_jobs SET fencing_token=fencing_token+1 WHERE job_id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if code := invoke(ephemeral.Token, "evaluate_candidate"); code != 403 || calls.Load() != 1 {
		t.Fatalf("stale Job gained tool access: %d, calls=%d", code, calls.Load())
	}
	if err := store.heartbeatManaged(ctx, ephemeral, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale Job renewed its Run: %v", err)
	}
	if err := server.managed.recoverManagedRuns(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := store.db.QueryRow(`SELECT state FROM agent_runs WHERE run_id=$1`, id).Scan(&state); err != nil || state != "STOPPING" {
		t.Fatalf("unconfirmed execution should remain STOPPING: %s, %v", state, err)
	}
}

func TestManagedProfileRejectsCapabilityEscapes(t *testing.T) {
	skill := harnessSkill{Name: "inspect", Description: "Inspect input", Content: "Review data."}
	if !validManagedProfile([]harnessSkill{skill}, []string{"data/note.md"}) {
		t.Fatal("valid Skill and Workspace reference were rejected")
	}
	for _, ref := range []string{"/etc/passwd", "../private", "src/../secret", "https://example.com", "path\\file"} {
		if validManagedProfile(nil, []string{ref}) {
			t.Fatalf("accepted unsafe knowledge reference %q", ref)
		}
	}
	if validManagedProfile([]harnessSkill{skill, skill}, nil) {
		t.Fatal("accepted ambiguous duplicate Skills")
	}
}
