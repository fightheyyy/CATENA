package control

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func isolatedQueuePostgres(t *testing.T) (*PostgresStore, string) {
	t.Helper()
	dsn := os.Getenv("BARENA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BARENA_TEST_DATABASE_URL is not configured")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		t.Fatal("queue integration tests require a PostgreSQL URL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("catena_queue_test_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := OpenPostgres(context.Background(), u.String())
	if err != nil {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		t.Fatal(err)
	}
	store.db.SetMaxOpenConns(8)
	t.Cleanup(func() {
		store.Close()
		_, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		if err != nil {
			t.Error(err)
		}
	})
	return store, u.String()
}

func TestPostgresEvolutionQueueContract(t *testing.T) {
	queueContract(t, func(t *testing.T) Store { s, _ := isolatedQueuePostgres(t); return s })
}

func TestPostgresEvolutionQueueUpgradesLegacyRows(t *testing.T) {
	s, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	queued := seedQueueJob(t, s)
	running := seedQueueJob(t, s)
	running.State = EvolutionJobRunning
	running.CurrentStage = "inspector"
	if err := s.UpdateEvolutionJob(ctx, running); err != nil {
		t.Fatal(err)
	}
	// Reconstruct the pre-lease table shape while preserving real job rows.
	_, err := s.db.ExecContext(ctx, `ALTER TABLE spiral_evolution_jobs
 DROP COLUMN worker_id, DROP COLUMN fencing_token, DROP COLUMN lease_until,
 DROP COLUMN attempt, DROP COLUMN next_attempt_at, DROP COLUMN cancel_requested;
ALTER TABLE spiral_evolution_jobs DROP CONSTRAINT spiral_evolution_jobs_state_check;
ALTER TABLE spiral_evolution_jobs ADD CONSTRAINT spiral_evolution_jobs_state_check CHECK(state IN ('queued','running','completed','failed'));`)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err = s.migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for n := 0; n < 2; n++ {
		j, l, e := s.ClaimEvolutionJob(ctx, "upgraded-worker", 3*time.Second, 3)
		if e != nil {
			t.Fatal(e)
		}
		if j.Attempt != 1 || l.Fence != 1 || j.SourceTraceID != queued.SourceTraceID {
			t.Fatal("legacy evidence or lease defaults lost")
		}
		seen[j.ID] = true
		j.State = EvolutionJobCompleted
		if e = s.SaveLeasedEvolutionJob(ctx, l, j); e != nil {
			t.Fatal(e)
		}
	}
	if !seen[queued.ID] || !seen[running.ID] {
		t.Fatal("legacy queued/running rows were stranded")
	}
}

// This helper is a distinct OS process: killing it bypasses all Go defers,
// unlike context cancellation or a panic recovered in the same process.
func TestEvolutionQueueCrashProcessHelper(t *testing.T) {
	mode := os.Getenv("CATENA_QUEUE_CHILD_MODE")
	if mode == "" {
		return
	}
	s, err := OpenPostgres(context.Background(), os.Getenv("CATENA_QUEUE_CHILD_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	j, l, err := s.ClaimEvolutionJob(context.Background(), "killed-child", 500*time.Millisecond, 3)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "checkpoint" {
		j.Stages[0].State = EvolutionStageCompleted
		j.Stages[0].RawOutput = json.RawMessage(`{"checkpoint":true}`)
		if err = s.SaveLeasedEvolutionJob(context.Background(), l, j); err != nil {
			t.Fatal(err)
		}
	}
	if mode == "external_completed" {
		if err = os.WriteFile(os.Getenv("CATENA_QUEUE_CHILD_MARKER"), []byte("1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = json.NewEncoder(os.Stdout).Encode(l); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestPostgresEvolutionRecoversKilledProcess(t *testing.T) {
	for _, mode := range []string{"after_claim", "external_completed", "checkpoint"} {
		t.Run(mode, func(t *testing.T) {
			s, dsn := isolatedQueuePostgres(t)
			seedQueueJob(t, s)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEvolutionQueueCrashProcessHelper$", "-test.count=1")
			marker := filepath.Join(t.TempDir(), "external-call-count")
			cmd.Env = append(os.Environ(), "CATENA_QUEUE_CHILD_MODE="+mode, "CATENA_QUEUE_CHILD_DSN="+dsn, "CATENA_QUEUE_CHILD_MARKER="+marker)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() {
				cmd.Wait()
				t.Fatalf("child failed before acquisition: %s", stderr.String())
			}
			var old EvolutionLease
			if err = json.Unmarshal(scanner.Bytes(), &old); err != nil {
				t.Fatalf("invalid child lease: %s: %v", scanner.Text(), err)
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err = cmd.Wait(); err == nil {
				t.Fatal("child was not terminated")
			}
			var recovered EvolutionJob
			var fresh EvolutionLease
			eventually(t, func() bool {
				var err error
				recovered, fresh, err = s.ClaimEvolutionJob(context.Background(), "recovery", 3*time.Second, 3)
				if err != nil && !errors.Is(err, ErrNotFound) {
					t.Error(err)
				}
				return err == nil
			})
			if recovered.ID != old.JobID || recovered.Attempt != 2 || fresh.Fence <= old.Fence {
				t.Fatalf("incorrect recovery: %#v %#v", recovered, fresh)
			}
			if mode == "checkpoint" && (recovered.Stages[0].State != EvolutionStageCompleted || len(recovered.Stages[0].RawOutput) == 0) {
				t.Fatal("committed checkpoint was lost")
			}
			if mode == "external_completed" {
				// The model/provider side effect completed, but no checkpoint
				// committed: recovery must repeat it. We explicitly count that
				// window instead of claiming exactly-once external execution.
				b, e := os.ReadFile(marker)
				if e != nil || string(b) != "1" {
					t.Fatal("external stub did not complete")
				}
				if len(recovered.Stages[0].RawOutput) != 0 {
					t.Fatal("uncommitted response became a checkpoint")
				}
				if e = os.WriteFile(marker, []byte("2"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			recovered.State = EvolutionJobCompleted
			if err = s.SaveLeasedEvolutionJob(context.Background(), old, recovered); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("killed worker token still valid: %v", err)
			}
			if err = s.SaveLeasedEvolutionJob(context.Background(), fresh, recovered); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The proxy drops actual database TCP connections, then accepts them again.
// It leaves the database and other schemas untouched.
type queueDBProxy struct {
	listener    net.Listener
	upstream    string
	offline     atomic.Bool
	mu          sync.Mutex
	connections map[net.Conn]bool
	wg          sync.WaitGroup
}

func newQueueDBProxy(t *testing.T, upstream string) *queueDBProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &queueDBProxy{listener: listener, upstream: upstream, connections: map[net.Conn]bool{}}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				defer client.Close()
				if p.offline.Load() {
					return
				}
				server, err := net.DialTimeout("tcp", upstream, time.Second)
				if err != nil {
					return
				}
				defer server.Close()
				p.mu.Lock()
				p.connections[client] = true
				p.connections[server] = true
				offline := p.offline.Load()
				p.mu.Unlock()
				defer func() { p.mu.Lock(); delete(p.connections, client); delete(p.connections, server); p.mu.Unlock() }()
				if offline {
					return
				}
				done := make(chan struct{})
				go func() { io.Copy(server, client); server.Close(); close(done) }()
				io.Copy(client, server)
				client.Close()
				<-done
			}()
		}
	}()
	t.Cleanup(func() { p.listener.Close(); p.setOffline(true); p.wg.Wait() })
	return p
}

func (p *queueDBProxy) setOffline(value bool) {
	p.offline.Store(value)
	if value {
		p.mu.Lock()
		for c := range p.connections {
			c.Close()
		}
		p.mu.Unlock()
	}
}

func TestPostgresEvolutionWorkerRecoversDatabaseInterruption(t *testing.T) {
	direct, dsn := isolatedQueuePostgres(t)
	j := seedQueueJob(t, direct)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	upstream := u.Host
	if u.Port() == "" {
		upstream = net.JoinHostPort(u.Hostname(), "5432")
	}
	proxy := newQueueDBProxy(t, upstream)
	u.Host = proxy.listener.Addr().String()
	store, err := OpenPostgres(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.db.SetMaxOpenConns(4)
	cfg := fastWorkerConfig()
	cfg.Workers = 1
	cfg.LeaseTTL = 300 * time.Millisecond
	cfg.HeartbeatInterval = 40 * time.Millisecond
	started := make(chan struct{})
	var calls atomic.Int64
	w, err := newEvolutionWorker(store, cfg, func(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		j.State = EvolutionJobCompleted
		return store.SaveLeasedEvolutionJob(ctx, l, j)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	proxy.setOffline(true)
	time.Sleep(400 * time.Millisecond)
	proxy.setOffline(false)
	eventually(t, func() bool {
		got, err := direct.GetEvolutionJob(context.Background(), j.ID)
		return err == nil && got.State == EvolutionJobCompleted
	})
	got, _ := direct.GetEvolutionJob(context.Background(), j.ID)
	if got.Attempt != 2 || calls.Load() != 2 {
		t.Fatalf("unexpected recovery: attempt=%d calls=%d", got.Attempt, calls.Load())
	}
	if w.counters.LeaseLosses.Load() == 0 {
		t.Fatal("heartbeat outage was not observed")
	}
}
