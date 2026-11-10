package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func seedManaged(t *testing.T, s *PostgresStore, tenant string) string {
	t.Helper()
	ctx := context.Background()
	agent := newID("agent")
	session := newID("session")
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant_quotas(tenant_id) VALUES($1) ON CONFLICT DO NOTHING`, []any{tenant}},
		{`INSERT INTO managed_agents(agent_id,tenant_id,runtime_type,runtime_version,model_config_ref,instructions,tool_policy,timeout_seconds) VALUES($1,$2,'codex','0.160.0',$2,'test','sandbox-write',600)`, []any{agent, tenant}},
		{`INSERT INTO agent_sessions(session_id,tenant_id,agent_id) VALUES($1,$2,$3)`, []any{session, tenant, agent}},
	} {
		if _, err := s.db.ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	return session
}
func submitManagedTest(t *testing.T, s *PostgresStore, tenant, session, key string) managedRun {
	t.Helper()
	r, _, err := s.submitManaged(context.Background(), tenant, session, "write a file", key)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func claimManagedTest(t *testing.T, s *PostgresStore) *managedLease {
	t.Helper()
	l, err := s.claimManaged(context.Background(), "worker-a", 30*time.Second, 4)
	if err != nil || l == nil {
		t.Fatalf("claim: %v %v", l, err)
	}
	return l
}
func TestManagedAdmissionIdempotencyAndSessionMutex(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "tenant-a")
	var wg sync.WaitGroup
	ids := sync.Map{}
	var created atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, c, e := s.submitManaged(ctx, "tenant-a", session, "write a file", "same-key")
			if e != nil {
				t.Error(e)
				return
			}
			ids.Store(r.ID, true)
			if c {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	count := 0
	ids.Range(func(_, _ any) bool { count++; return true })
	if count != 1 || created.Load() != 1 {
		t.Fatalf("duplicate submissions: ids=%d created=%d", count, created.Load())
	}
	if _, _, err := s.submitManaged(ctx, "tenant-a", session, "different input", "same-key"); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload conflict: %v", err)
	}
	if _, _, err := s.submitManaged(ctx, "tenant-b", session, "wrong tenant", "key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant submit: %v", err)
	}
	submitManagedTest(t, s, "tenant-a", session, "second")
	first := claimManagedTest(t, s)
	if other, e := s.claimManaged(ctx, "worker-b", 30*time.Second, 4); e != nil || other != nil {
		t.Fatalf("session overlap: %v %v", other, e)
	}
	if _, err := s.db.Exec(`UPDATE managed_sandboxes SET state='BUSY',provider_sandbox_id='fake' WHERE sandbox_id=$1`, first.Box.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.finishManaged(ctx, first, "COMPLETED", "done", "", nil); err != nil {
		t.Fatal(err)
	}
	next := claimManagedTest(t, s)
	if next.Box.ID != first.Box.ID {
		t.Fatal("session sandbox was not reused")
	}
	if err := s.managedEvent(ctx, first, "stale", map[string]string{"bad": "write"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale event: %v", err)
	}
	if err := s.finishManaged(ctx, first, "COMPLETED", "overwrite", "", nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale finalization: %v", err)
	}
}
func TestManagedQuotaFairnessAndGlobalReservation(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		a := seedManaged(t, s, "a")
		submitManagedTest(t, s, "a", a, fmt.Sprint(i))
	}
	b := seedManaged(t, s, "b")
	submitManagedTest(t, s, "b", b, "b")
	first := claimManagedTest(t, s)
	second := claimManagedTest(t, s)
	if first.Run.Tenant == second.Run.Tenant {
		t.Fatal("tenant rotation did not schedule waiting tenant")
	}
	claimManagedTest(t, s)
	if l, e := s.claimManaged(ctx, "worker", 30*time.Second, 4); e != nil || l != nil {
		t.Fatalf("tenant sandbox quota bypassed: %v %v", l, e)
	}
	_, err := s.db.Exec(`UPDATE tenant_quotas SET max_sandboxes=8,max_running=8 WHERE tenant_id='a'`)
	if err != nil {
		t.Fatal(err)
	}
	claimManagedTest(t, s)
	if l, e := s.claimManaged(ctx, "worker", 30*time.Second, 4); e != nil || l != nil {
		t.Fatalf("global reservation exceeded: %v %v", l, e)
	}
}
func fakeManagedPlatform(t *testing.T, s *PostgresStore, failDelete *atomic.Bool) *managedPlatform {
	t.Helper()
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "list") {
			fmt.Fprint(w, `[]`)
			return
		}
		if failDelete != nil && failDelete.Load() {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(runner.Close)
	server := &HTTPServer{store: s}
	p := &managedPlatform{store: s, server: server, config: ManagedConfig{URL: runner.URL, Token: strings.Repeat("s", 32), Idle: time.Minute, Namespace: "test"}, client: runner.Client()}
	server.managed = p
	return p
}
func TestManagedCrashRecoveryFencingAndRestart(t *testing.T) {
	s, dsn := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "a")
	r := submitManagedTest(t, s, "a", session, "k")
	old := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE agent_runs SET execution_started_at=now(),lease_until=now()-interval '1 second' WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE managed_sandboxes SET provider_sandbox_id='gone',state='BUSY' WHERE sandbox_id=$1`, old.Box.ID); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	p := fakeManagedPlatform(t, restarted, nil)
	if err = p.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	var reserved int
	if err = s.db.QueryRow(`SELECT state FROM agent_runs WHERE run_id=$1`, r.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "NEEDS_REVIEW" {
		t.Fatalf("unsafe automatic replay: %s", state)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM resource_reservations WHERE released_at IS NULL`).Scan(&reserved); err != nil || reserved != 0 {
		t.Fatalf("reservation leak %d %v", reserved, err)
	}
	if err = s.heartbeatManaged(ctx, old, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old heartbeat: %v", err)
	}
	if err = s.finishManaged(ctx, old, "COMPLETED", "stale", "", nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old finalization: %v", err)
	}
	if l, e := s.claimManaged(ctx, "new-worker", time.Minute, 4); e != nil || l != nil {
		t.Fatalf("uncertain execution replayed: %v %v", l, e)
	}
	req := httptest.NewRequest("POST", "/internal/managed/authorize", strings.NewReader(fmt.Sprintf(`{"run_id":%q,"sandbox_id":%q,"generation":1,"token":%q}`, r.ID, old.Box.ID, old.Token)))
	req.Header.Set("Authorization", "Bearer "+p.config.Token)
	response := httptest.NewRecorder()
	p.server.managedAuthorize(response, req)
	if response.Code != 403 {
		t.Fatalf("expired executor authorized: %d", response.Code)
	}
}
func TestManagedUnknownCreateRetainsReservation(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "k")
	l := claimManagedTest(t, s)
	_, err := s.db.Exec(`INSERT INTO sandbox_operations(operation_id,sandbox_id,kind,state) VALUES($1,$1,'create','UNKNOWN')`, l.Box.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`UPDATE managed_sandboxes SET state='CREATING' WHERE sandbox_id=$1`, l.Box.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`UPDATE agent_runs SET lease_until=now()-interval '1 second' WHERE run_id=$1`, l.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := fakeManagedPlatform(t, s, nil)
	if err = p.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM resource_reservations WHERE released_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unknown create capacity released: %d %v", count, err)
	}
	if other, e := s.claimManaged(ctx, "worker-b", time.Minute, 4); e != nil || other != nil {
		t.Fatalf("unknown create blindly retried: %v %v", other, e)
	}
}
func TestManagedQueueLoad(t *testing.T) {
	for _, n := range []int{10, 50, 100} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s, _ := isolatedQueuePostgres(t)
			ctx := context.Background()
			for i := 0; i < n; i++ {
				tenant := fmt.Sprint("tenant-", i%5)
				session := seedManaged(t, s, tenant)
				submitManagedTest(t, s, tenant, session, fmt.Sprint(i))
			}
			var completed atomic.Int32
			var wg sync.WaitGroup
			started := time.Now()
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					for completed.Load() < int32(n) {
						l, e := s.claimManaged(ctx, fmt.Sprint("w", i), 30*time.Second, 4)
						if e != nil {
							t.Error(e)
							return
						}
						if l == nil {
							time.Sleep(time.Millisecond)
							continue
						}
						if e = s.finishManaged(ctx, l, "COMPLETED", "test", "", nil); e != nil {
							t.Error(e)
							return
						}
						tx, e := managedTx(ctx, s.db)
						if e != nil {
							t.Error(e)
							return
						}
						_, e = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATED' WHERE sandbox_id=$1`, l.Box.ID)
						if e == nil {
							_, e = tx.ExecContext(ctx, `UPDATE resource_reservations SET released_at=now() WHERE sandbox_id=$1`, l.Box.ID)
						}
						if e == nil {
							e = tx.Commit()
						} else {
							tx.Rollback()
						}
						if e != nil {
							t.Error(e)
							return
						}
						completed.Add(1)
					}
				}(i)
			}
			wg.Wait()
			if int(completed.Load()) != n {
				t.Fatalf("completed %d/%d", completed.Load(), n)
			}
			t.Logf("PostgreSQL scheduler only: %d runs, 8 claimants, capacity 4, duration %s; no model/provider performance claim", n, time.Since(started))
		})
	}
}
func TestManagedGatewayBudgetAndRevocation(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "k")
	l := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE agent_runs SET execution_started_at=now() WHERE run_id=$1`, l.Run.ID); err != nil {
		t.Fatal(err)
	}
	p := fakeManagedPlatform(t, s, nil)
	hash := sessionTokenHash(l.Token)
	if _, err := s.db.Exec(`UPDATE tenant_quotas SET max_model_requests=2 WHERE tenant_id='a'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, _, _, _, _, err := p.server.reserveManagedModel(context.Background(), hash); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, _, _, err := p.server.reserveManagedModel(context.Background(), hash); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("budget bypass: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE agent_runs SET cancel_requested=true,credential_hash=NULL WHERE run_id=$1`, l.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err := p.server.reserveManagedModel(context.Background(), hash); err == nil {
		t.Fatal("cancelled credential accepted")
	}
}
func TestManagedConfigValidation(t *testing.T) {
	c := ManagedConfig{URL: "http://localhost:8792", ControlURL: "http://192.0.2.1:18788", Token: strings.Repeat("x", 32), Workers: 2, Capacity: 4, Lease: 30 * time.Second, Idle: time.Minute}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	c.Capacity = 100
	if c.validate() == nil {
		t.Fatal("unbounded sandbox capacity accepted")
	}
	raw, _ := json.Marshal(managedRun{})
	if strings.Contains(string(raw), "credential") {
		t.Fatal("capability exposed")
	}
}

func TestManagedCleanupFailureRetainsCapacityAndRetries(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "k")
	l := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE managed_sandboxes SET provider_sandbox_id='provider-1',state='TERMINATING' WHERE sandbox_id=$1`, l.Box.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE agent_runs SET state='FAILED',lease_until=NULL WHERE run_id=$1`, l.Run.ID); err != nil {
		t.Fatal(err)
	}
	var fail atomic.Bool
	fail.Store(true)
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "list") {
			fmt.Fprintf(w, `[{"provider_id":"provider-1","sandbox_id":%q,"state":"Running","created_at":"2026-01-01T00:00:00Z"}]`, l.Box.ID)
			return
		}
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer runner.Close()
	server := &HTTPServer{store: s}
	p := &managedPlatform{store: s, server: server, config: ManagedConfig{URL: runner.URL, Idle: time.Minute}, client: runner.Client()}
	if err := p.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	var reserved int
	if err := s.db.QueryRow(`SELECT count(*) FROM resource_reservations WHERE released_at IS NULL`).Scan(&reserved); err != nil || reserved != 1 {
		t.Fatalf("unconfirmed deletion released capacity: %d %v", reserved, err)
	}
	fail.Store(false)
	if err := p.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM resource_reservations WHERE released_at IS NULL`).Scan(&reserved); err != nil || reserved != 0 {
		t.Fatalf("cleanup retry failed: %d %v", reserved, err)
	}
}
func TestManagedTenantReadIsolationAndModelListener(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	session := seedManaged(t, s, "other-owner")
	run := submitManagedTest(t, s, "other-owner", session, "key")
	p := fakeManagedPlatform(t, s, nil)
	req := httptest.NewRequest("GET", "/v1/agent-runs/"+run.ID, nil)
	req.SetPathValue("run_id", run.ID)
	response := httptest.NewRecorder()
	p.server.managedRunDetail(response, req)
	if response.Code != 404 {
		t.Fatalf("cross tenant run disclosure: %d", response.Code)
	}
	_ = claimManagedTest(t, s)
	requestAction := httptest.NewRequest("DELETE", "/v1/agent-sessions/"+session, nil)
	requestAction.SetPathValue("session_id", session)
	actionResponse := httptest.NewRecorder()
	p.server.managedSessionAction(actionResponse, requestAction)
	if actionResponse.Code != 404 {
		t.Fatalf("cross tenant active-session disclosure: %d", actionResponse.Code)
	}

	h := &ManagedHTTPHandler{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })}
	for _, path := range []string{"/v1/agents", "/internal/managed/authorize", "/v1/me/resources"} {
		response = httptest.NewRecorder()
		h.ModelGateway().ServeHTTP(response, httptest.NewRequest("POST", path, nil))
		if response.Code != 404 {
			t.Fatalf("gateway exposed %s", path)
		}
	}
	response = httptest.NewRecorder()
	h.ModelGateway().ServeHTTP(response, httptest.NewRequest("POST", "/internal/managed/model/responses", nil))
	if response.Code != 204 {
		t.Fatal("model path unavailable")
	}
}

func TestManagedGatewayForwardsScopedIdentityAndAccountsUsage(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "local")
	submitManagedTest(t, s, "local", session, "gateway")
	lease := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE agent_runs SET execution_started_at=now() WHERE run_id=$1`, lease.Run.ID); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer server-only-model-secret" {
			t.Error("upstream did not receive server credential")
		}
		if r.Header.Get("X-Catena-Run-ID") != lease.Run.ID || !strings.Contains(r.Header.Get("traceparent"), lease.Run.Trace) {
			t.Error("upstream execution identity missing")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"usage":{"input_tokens":12,"output_tokens":7},"output":[]}`)
	}))
	defer upstream.Close()
	p := fakeManagedPlatform(t, s, nil)
	p.server.auth = testAnalysisAuthConfig()
	if _, e := s.UpsertUser(ctx, User{ID: "local", GitHubID: 1, Login: "local", CreatedAt: time.Now(), UpdatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	encrypted, err := encryptAPIToken("server-only-model-secret", analysisModelEnvelopePrefix+"local", p.server.auth.APITokenEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpsertAnalysisModelConfig(ctx, AnalysisModelConfig{OwnerUserID: "local", Provider: "openai", BaseURL: upstream.URL, Model: "test-model", EncryptedAPIKey: encrypted, UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/internal/managed/model/responses", strings.NewReader(`{"model":"test-model","input":"hello"}`))
	request.Header.Set("Authorization", "Bearer "+lease.Token)
	response := httptest.NewRecorder()
	p.server.managedModel(response, request)
	if response.Code != 200 || calls.Load() != 1 {
		t.Fatalf("scoped gateway failed: status=%d calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
	}
	if strings.Contains(response.Body.String(), "server-only-model-secret") {
		t.Fatal("long-term credential exposed")
	}
	var input, output int64
	if err = s.db.QueryRow(`SELECT input_tokens,output_tokens FROM agent_runs WHERE run_id=$1`, lease.Run.ID).Scan(&input, &output); err != nil || input != 12 || output != 7 {
		t.Fatalf("usage missing: %d %d %v", input, output, err)
	}
	p.server.accountManagedUsage(ctx, 1, lease.Run.ID, []byte(`{"usage":{"input_tokens":12,"output_tokens":7}}`))
	if err = s.db.QueryRow(`SELECT input_tokens FROM agent_runs WHERE run_id=$1`, lease.Run.ID).Scan(&input); err != nil || input != 12 {
		t.Fatal("duplicate usage accounting")
	}
}

func TestManagedSSECursorAndIdempotentClose(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "local")
	run := submitManagedTest(t, s, "local", session, "key")
	l := claimManagedTest(t, s)
	if err := s.managedEvent(ctx, l, "runtime", map[string]string{"type": "actual-event"}); err != nil {
		t.Fatal(err)
	}
	var first, last int64
	if err := s.db.QueryRow(`SELECT min(event_id),max(event_id) FROM agent_run_events WHERE run_id=$1`, run.ID).Scan(&first, &last); err != nil {
		t.Fatal(err)
	}
	p := fakeManagedPlatform(t, s, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agent-runs/{run_id}/events", p.server.managedEvents)
	server := httptest.NewServer(mux)
	defer server.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(streamCtx, "GET", server.URL+"/v1/agent-runs/"+run.ID+"/events", nil)
	request.Header.Set("Last-Event-ID", fmt.Sprint(first))
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(response.Body)
	seen := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == fmt.Sprint("id: ", first) {
			t.Fatal("SSE replay ignored Last-Event-ID")
		}
		if line == fmt.Sprint("id: ", last) {
			seen = true
			break
		}
	}
	response.Body.Close()
	cancel()
	if !seen {
		t.Fatal("SSE cursor omitted next persisted event")
	}
	if err = s.finishManaged(ctx, l, "COMPLETED", "done", "", nil); err != nil {
		t.Fatal(err)
	}
	closeSession := func() {
		t.Helper()
		r := httptest.NewRequest("DELETE", "/v1/agent-sessions/"+session, nil)
		r.SetPathValue("session_id", session)
		w := httptest.NewRecorder()
		p.server.managedSessionAction(w, r)
		if w.Code != 202 {
			t.Fatalf("non-idempotent close: %d %s", w.Code, w.Body.String())
		}
	}
	closeSession()
	closeSession()
}
