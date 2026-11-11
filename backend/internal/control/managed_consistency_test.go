package control

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func managedCleanupFixture(t *testing.T, s *PostgresStore) (*managedLease, string) {
	t.Helper()
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "k")
	l := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE agent_runs SET state='FAILED',lease_until=NULL WHERE run_id=$1`, l.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE managed_sandboxes SET state='TERMINATING',provider_sandbox_id='provider-1' WHERE sandbox_id=$1`, l.Box.ID); err != nil {
		t.Fatal(err)
	}
	return l, session
}
func TestManagedSlowProviderDoesNotHoldAdmissionLock(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	l, _ := managedCleanupFixture(t, s)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "list") {
			fmt.Fprintf(w, `[{"provider_id":"provider-1","sandbox_id":%q,"state":"Running"}]`, l.Box.ID)
			return
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer runner.Close()
	defer close(release)
	p := &managedPlatform{store: s, server: &HTTPServer{store: s}, config: ManagedConfig{URL: runner.URL, Idle: time.Minute, ProviderTimeout: 2 * time.Second, OperationLease: 3 * time.Second}, client: runner.Client()}
	done := make(chan error, 1)
	go func() { done <- p.reconcile(context.Background()) }()
	<-entered
	ctx, stop := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer stop()
	session := seedManaged(t, s, "b")
	if _, _, err := s.submitManaged(ctx, "b", session, "new task", "b"); err != nil {
		t.Fatalf("slow Provider blocked admission: %v", err)
	}
	if other, err := s.claimManaged(ctx, "second-worker", time.Minute, 4); err != nil || other == nil {
		t.Fatalf("slow cleanup blocked scheduling: %v %v", other, err)
	}
	// A second controller sees the durable live operation and does not duplicate it.
	if err := p.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("concurrent reconciler duplicated a live operation")
	}
	// Simulate process loss: let another controller take over the expired intent.
	if _, err := s.db.Exec(`UPDATE sandbox_operations SET lease_until=now()-interval '1 second' WHERE kind='terminate'`); err != nil {
		t.Fatal(err)
	}
	if err := p.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	var old managedOperation
	if err := managedJSON(s.db.QueryRow(`SELECT to_jsonb(o) FROM sandbox_operations o WHERE kind='terminate'`), &old); err != nil {
		t.Fatal(err)
	}
	if old.Fence != 2 || old.State != "SUCCEEDED" {
		t.Fatalf("intent not taken over: %+v", old)
	}
	// The original late response must not finalize the new operation generation.
	release <- struct{}{}
	if err := <-done; !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("late finalization accepted: %v", err)
	}
}
func TestManagedPauseTimeoutQuarantinesBeforeReuse(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "one")
	l := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE managed_sandboxes SET state='BUSY',provider_sandbox_id='provider-1' WHERE sandbox_id=$1`, l.Box.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.finishManaged(ctx, l, "COMPLETED", "done", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE agent_sessions SET state='PAUSING' WHERE session_id=$1`, session); err != nil {
		t.Fatal(err)
	}
	var pauses, deletes atomic.Int32
	released := make(chan struct{})
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "list"):
			fmt.Fprintf(w, `[{"provider_id":"provider-1","sandbox_id":%q,"state":"Running"}]`, l.Box.ID)
		case strings.HasSuffix(r.URL.Path, "pause"):
			pauses.Add(1)
			<-released
		default:
			deletes.Add(1)
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer runner.Close()
	defer close(released)
	p := &managedPlatform{store: s, server: &HTTPServer{store: s}, config: ManagedConfig{URL: runner.URL, Idle: time.Minute, ProviderTimeout: 50 * time.Millisecond, OperationLease: time.Second}, client: runner.Client()}
	if err := p.reconcile(ctx); err == nil {
		t.Fatal("timeout was not surfaced")
	}
	if _, err := s.db.Exec(`UPDATE agent_sessions SET state='READY' WHERE session_id=$1`, session); err != nil {
		t.Fatal(err)
	}
	submitManagedTest(t, s, "a", session, "two")
	if next, err := s.claimManaged(ctx, "new", time.Minute, 4); err != nil || next != nil {
		t.Fatal("uncertain pause allowed execution reuse")
	}
	if err := p.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if pauses.Load() != 1 || deletes.Load() != 1 {
		t.Fatal("uncertain pause was replayed instead of quarantined")
	}
	var state string
	if err := s.db.QueryRow(`SELECT state FROM managed_sandboxes WHERE sandbox_id=$1`, l.Box.ID).Scan(&state); err != nil || state != "TERMINATED" {
		t.Fatalf("quarantine failed %s %v", state, err)
	}
}
func executionFrame() string {
	return fmt.Sprintf(`{"kind":"result","success":true,"result":"done","snapshot":%q,"command_exited":true,"exit_code":0}`+"\n", base64.StdEncoding.EncodeToString([]byte("fixture snapshot")))
}
func TestManagedExecutionWaitsForEOFAndRejectsExpiredResult(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint(expired), func(t *testing.T) {
			s, _ := isolatedQueuePostgres(t)
			session := seedManaged(t, s, "a")
			submitManagedTest(t, s, "a", session, "one")
			submitManagedTest(t, s, "a", session, "two")
			l := claimManagedTest(t, s)
			if _, err := s.db.Exec(`UPDATE agent_runs SET execution_started_at=now() WHERE run_id=$1`, l.Run.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.finishManaged(context.Background(), l, "COMPLETED", "premature", "", nil); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("result committed before exit: %v", err)
			}
			p := &managedPlatform{store: s}
			reader, writer := io.Pipe()
			defer reader.Close()
			done := make(chan error, 1)
			go func() { done <- p.consumeManagedExecution(context.Background(), l, reader) }()
			if _, err := io.WriteString(writer, executionFrame()); err != nil {
				t.Fatal(err)
			}
			var state string
			if err := s.db.QueryRow(`SELECT state FROM agent_runs WHERE run_id=$1`, l.Run.ID).Scan(&state); err != nil || state != "RUNNING" {
				t.Fatal("result frame finalized before stream exit")
			}
			if next, err := s.claimManaged(context.Background(), "next", time.Minute, 4); err != nil || next != nil {
				t.Fatal("session execution overlapped")
			}
			if expired {
				if _, err := s.db.Exec(`UPDATE agent_runs SET lease_until=now()-interval '1 second' WHERE run_id=$1`, l.Run.ID); err != nil {
					t.Fatal(err)
				}
			}
			writer.Close()
			err := <-done
			if expired {
				if !errors.Is(err, ErrLeaseLost) {
					t.Fatalf("expired execution finalized: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestManagedAdapterCrashDoesNotPublishResult(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "one")
	l := claimManagedTest(t, s)
	p := &managedPlatform{store: s}
	frame := strings.Replace(executionFrame(), `"command_exited":true`, `"command_exited":false`, 1)
	if err := p.consumeManagedExecution(context.Background(), l, strings.NewReader(frame)); err == nil {
		t.Fatal("unconfirmed adapter result accepted")
	}
	if err := p.consumeManagedExecution(context.Background(), l, strings.NewReader(`{"kind":"runtime","data":{"type":"started"}}`+"\n")); err == nil {
		t.Fatal("crashed adapter stream accepted")
	}
}

type managedTraceRecorder struct {
	mu     sync.Mutex
	calls  int
	failAt int
	spans  map[string]TraceSpan
}

func (s *managedTraceRecorder) InsertSpans(_ context.Context, _ string, spans []TraceSpan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	// Emulate a receiver that stored the batch but lost its acknowledgement.
	for _, span := range spans {
		s.spans[span.SpanID] = span
	}
	if s.calls == s.failAt {
		return errors.New("collector acknowledgement lost")
	}
	return nil
}
func TestManagedTracePaginationCrossPageToolAndRetry(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "a")
	run := submitManagedTest(t, s, "a", session, "k")
	l := claimManagedTest(t, s)
	for i := 0; i < 253; i++ {
		data := map[string]any{"type": "diagnostic", "index": i}
		if i == 98 || i == 201 {
			kind := "item.started"
			status := "in_progress"
			if i == 201 {
				kind = "item.completed"
				status = "completed"
			}
			data = map[string]any{"type": kind, "item": map[string]any{"id": "cross-page", "type": "command_execution", "status": status, "exit_code": 0, "command": "true"}}
		}
		if err := s.managedEvent(ctx, l, "runtime", data); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.finishManaged(ctx, l, "COMPLETED", "done", "", nil); err != nil {
		t.Fatal(err)
	}
	recorder := &managedTraceRecorder{spans: map[string]TraceSpan{}, failAt: 3}
	p := &managedPlatform{store: s, server: &HTTPServer{store: s, traceSink: recorder}}
	if err := p.exportManagedTraces(ctx); err == nil {
		t.Fatal("lost acknowledgement was ignored")
	}
	var cursor int64
	var exported bool
	if err := s.db.QueryRow(`SELECT trace_event_cursor,trace_exported FROM agent_runs WHERE run_id=$1`, run.ID).Scan(&cursor, &exported); err != nil || cursor == 0 || exported {
		t.Fatal("export cursor advanced past failed batch")
	}
	// A fresh process resumes the persisted cursor and pending cross-page tool start.
	p = &managedPlatform{store: s, server: &HTTPServer{store: s, traceSink: recorder}}
	if err := p.exportManagedTraces(ctx); err != nil {
		t.Fatal(err)
	}
	events := map[int64]bool{}
	toolFound := false
	for _, span := range recorder.spans {
		for _, ev := range span.Events {
			id := ev.Attributes["catena.event.id"].(int64)
			if events[id] {
				t.Fatal("event page duplicated across retry")
			}
			events[id] = true
		}
		if span.Name == "managed.tool.shell" {
			toolFound = true
			if !span.EndTime.After(span.StartTime) {
				t.Fatal("tool start was lost across page/restart")
			}
		}
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM agent_run_events WHERE run_id=$1`, run.ID).Scan(&count); err != nil || len(events) != count || !toolFound {
		t.Fatalf("incomplete export: %d/%d tool=%v %v", len(events), count, toolFound, err)
	}
	if err := s.db.QueryRow(`SELECT trace_exported FROM agent_runs WHERE run_id=$1`, run.ID).Scan(&exported); err != nil || !exported {
		t.Fatal("complete trace not acknowledged")
	}
	if err := p.exportManagedTraces(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestManagedCancellationRemainsStoppingUntilProviderConfirmsExit(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	session := seedManaged(t, s, "a")
	submitManagedTest(t, s, "a", session, "one")
	submitManagedTest(t, s, "a", session, "two")
	l := claimManagedTest(t, s)
	if _, err := s.db.Exec(`UPDATE agent_runs SET execution_started_at=now(),cancel_requested=true WHERE run_id=$1`, l.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE managed_sandboxes SET state='BUSY',provider_sandbox_id='provider-1' WHERE sandbox_id=$1`, l.Box.ID); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "list") {
			fmt.Fprintf(w, `[{"provider_id":"provider-1","sandbox_id":%q,"state":"Running"}]`, l.Box.ID)
			return
		}
		close(entered)
		<-release
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer runner.Close()
	defer close(release)
	p := &managedPlatform{store: s, server: &HTTPServer{store: s}, config: ManagedConfig{URL: runner.URL, Idle: time.Minute}, client: runner.Client()}
	done := make(chan error, 1)
	go func() { done <- p.reconcile(ctx) }()
	<-entered
	var state string
	var finished, isolated bool
	if err := s.db.QueryRow(`SELECT state,finished_at IS NOT NULL,isolation_confirmed_at IS NOT NULL FROM agent_runs WHERE run_id=$1`, l.Run.ID).Scan(&state, &finished, &isolated); err != nil {
		t.Fatal(err)
	}
	if state != "STOPPING" || finished || isolated {
		t.Fatalf("premature terminal state: %s finished=%v isolated=%v", state, finished, isolated)
	}
	if next, err := s.claimManaged(ctx, "next", time.Minute, 4); err != nil || next != nil {
		t.Fatal("next session run started before isolation")
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT state,finished_at IS NOT NULL,isolation_confirmed_at IS NOT NULL FROM agent_runs WHERE run_id=$1`, l.Run.ID).Scan(&state, &finished, &isolated); err != nil {
		t.Fatal(err)
	}
	if state != "CANCELLED" || !finished || !isolated {
		t.Fatalf("confirmed termination missing: %s %v %v", state, finished, isolated)
	}
}
