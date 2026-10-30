package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fastWorkerConfig() EvolutionWorkerConfig {
	c := DefaultEvolutionWorkerConfig()
	c.Workers = 2
	c.LeaseTTL = time.Second
	c.HeartbeatInterval = 50 * time.Millisecond
	c.PollInterval = 5 * time.Millisecond
	c.RetryBase = 20 * time.Millisecond
	c.RetryMax = 50 * time.Millisecond
	c.JobTimeout = 3 * time.Second
	return c
}

func eventually(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if f() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestEvolutionWorkerBoundsConcurrencyAndDrainsPersistedQueue(t *testing.T) {
	s := NewMemoryStore()
	for i := 0; i < 12; i++ {
		seedQueueJob(t, s)
	}
	var active, peak, completed atomic.Int64
	w, err := newEvolutionWorker(s, fastWorkerConfig(), func(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		j.State = EvolutionJobCompleted
		err := s.SaveLeasedEvolutionJob(ctx, l, j)
		if err == nil {
			completed.Add(1)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	eventually(t, func() bool { return completed.Load() == 12 })
	if peak.Load() != 2 {
		t.Fatalf("expected exactly 2 active workers, peak=%d", peak.Load())
	}
}

func TestEvolutionWorkerCancellationAndShutdownRecovery(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	cfg := fastWorkerConfig()
	cfg.Workers = 1
	cfg.LeaseTTL = 200 * time.Millisecond
	cfg.HeartbeatInterval = 25 * time.Millisecond
	started := make(chan struct{})
	w, err := newEvolutionWorker(s, cfg, func(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := s.CancelEvolutionJob(context.Background(), "", j.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		got, _ := s.GetEvolutionJob(context.Background(), j.ID)
		return got.State == EvolutionJobCancelled
	})
	w.close()
	j = seedQueueJob(t, s)
	started = make(chan struct{})
	w, err = newEvolutionWorker(s, cfg, func(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	w.close()
	got, _ := s.GetEvolutionJob(context.Background(), j.ID)
	if got.State != EvolutionJobRunning {
		t.Fatal("shutdown incorrectly marked task terminal", got.State)
	}
	w, err = newEvolutionWorker(s, cfg, func(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
		if j.Attempt != 2 {
			t.Errorf("takeover attempt=%d", j.Attempt)
		}
		j.State = EvolutionJobCompleted
		return s.SaveLeasedEvolutionJob(ctx, l, j)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	eventually(t, func() bool {
		got, _ := s.GetEvolutionJob(context.Background(), j.ID)
		return got.State == EvolutionJobCompleted
	})
}

type queueRoundTripFunc func(*http.Request) (*http.Response, error)

func (f queueRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEvolutionTransientStageRetryReusesCommittedInspector(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		for _, role := range []string{"evolution", "reviewer"} {
			t.Run(fmt.Sprintf("%d/%s", status, role), func(t *testing.T) { testEvolutionStageRetry(t, role, status) })
		}
	}
}

func testEvolutionStageRetry(t *testing.T, failedRole string, status int) {
	s := NewMemoryStore()
	seedTestEvolutionModelConfig(t, s, "local")
	run := seedEvolutionRun(t, s, "", StateCompleted, "00112233445566778899aabbccddeeff")
	manager := newStructuredEvolutionRuntimeManager(t, "")
	var mu sync.Mutex
	calls := map[string]int{}
	ids := map[string][]string{}
	manager.config.Client = &http.Client{Transport: queueRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/turn" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var req struct {
				Role string `json:"role"`
				ID   string `json:"request_id"`
			}
			if err = json.Unmarshal(body, &req); err != nil {
				return nil, err
			}
			mu.Lock()
			calls[req.Role]++
			n := calls[req.Role]
			ids[req.Role] = append(ids[req.Role], req.ID)
			mu.Unlock()
			if req.Role == failedRole && n == 1 {
				return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(`{"error":"temporary"}`)), Header: make(http.Header)}, nil
			}
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	handler, err := NewHTTPHandlerWithWorkerConfig(s, nil, testEvolutionAuthConfig(), manager, nil, nil, fastWorkerConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	r := postEvolutionJob(t, server.URL+"/v1/runs/"+run.ID+"/evolution-jobs", "checkpoint-retry", map[string]any{"trace_id": "00112233445566778899aabbccddeeff"})
	defer r.Body.Close()
	if r.StatusCode != http.StatusAccepted {
		t.Fatal("create failed", r.StatusCode)
	}
	var j EvolutionJob
	if err = json.NewDecoder(r.Body).Decode(&j); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		got, _ := s.GetEvolutionJob(context.Background(), j.ID)
		return got.State == EvolutionJobCompleted
	})
	got, _ := s.GetEvolutionJob(context.Background(), j.ID)
	if got.Attempt != 2 {
		t.Fatalf("attempt=%d", got.Attempt)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, role := range []string{"inspector", "evolution", "reviewer"} {
		want := 1
		if role == failedRole {
			want = 2
		}
		if calls[role] != want {
			t.Fatalf("stages repeated unexpectedly: %v", calls)
		}
	}
	if got.Candidate == nil || got.Candidate.ID != got.ID+"-candidate" {
		t.Fatal("candidate identity is not stable")
	}
	if ids[failedRole][0] != ids[failedRole][1] {
		t.Fatal("stage correlation id changed across retries")
	}
}

func TestEvolutionWorkerPermanentFailureDoesNotRetry(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	var calls atomic.Int64
	w, err := newEvolutionWorker(s, fastWorkerConfig(), func(context.Context, EvolutionLease, EvolutionJob) error {
		calls.Add(1)
		return errors.New("invalid deterministic input")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	eventually(t, func() bool {
		got, _ := s.GetEvolutionJob(context.Background(), j.ID)
		return got.State == EvolutionJobFailed
	})
	if calls.Load() != 1 {
		t.Fatal("permanent failure retried")
	}
}

func TestEvolutionCancelQueuedJobHTTP(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	h, err := NewHTTPHandlerWithMemory(s, nil, AuthConfig{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	r := httptest.NewRequest(http.MethodPost, "/v1/evolution-jobs/"+j.ID+"/cancel", nil)
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	if out.Code != http.StatusOK {
		t.Fatalf("cancel HTTP %d: %s", out.Code, out.Body.String())
	}
	var got EvolutionJob
	if err = json.Unmarshal(out.Body.Bytes(), &got); err != nil || got.State != EvolutionJobCancelled {
		t.Fatalf("cancel result: %#v %v", got, err)
	}
	stats := httptest.NewRecorder()
	h.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/v1/evolution-jobs/queue", nil))
	if stats.Code != 200 {
		t.Fatalf("queue HTTP %d", stats.Code)
	}
}

func TestLocalModeCanCancelLocalTraceSetOwner(t *testing.T) {
	s, user, _, _ := registeredAgentFixture(t, "local trace set")
	j := seedQueueJob(t, s)
	j.ID = newID("owned-queue")
	j.IdempotencyKey = newID("owned-key")
	j.OwnerUserID = user.ID
	if _, _, err := s.CreateEvolutionJob(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	h, err := NewHTTPHandlerWithMemory(s, nil, AuthConfig{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	out := httptest.NewRecorder()
	h.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/v1/evolution-jobs/"+j.ID+"/cancel", nil))
	if out.Code != 200 {
		t.Fatalf("local owned job cancellation: HTTP %d", out.Code)
	}
}
