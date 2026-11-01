package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fastWorkerConfig() AnalysisWorkerConfig {
	c := DefaultAnalysisWorkerConfig()
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

func TestAnalysisWorkerBoundsConcurrencyAndDrainsPersistedQueue(t *testing.T) {
	s := NewMemoryStore()
	for i := 0; i < 12; i++ {
		seedQueueJob(t, s)
	}
	var active, peak, completed atomic.Int64
	w, err := newAnalysisWorker(s, fastWorkerConfig(), func(ctx context.Context, l AnalysisLease, j AnalysisJob) error {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		j.State = AnalysisJobCompleted
		err := s.SaveLeasedAnalysisJob(ctx, l, j)
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

func TestAnalysisWorkerCancellationAndShutdownRecovery(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	cfg := fastWorkerConfig()
	cfg.Workers = 1
	cfg.LeaseTTL = 200 * time.Millisecond
	cfg.HeartbeatInterval = 25 * time.Millisecond
	started := make(chan struct{})
	w, err := newAnalysisWorker(s, cfg, func(ctx context.Context, l AnalysisLease, j AnalysisJob) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := s.CancelAnalysisJob(context.Background(), "", j.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		got, _ := s.GetAnalysisJob(context.Background(), j.ID)
		return got.State == AnalysisJobCancelled
	})
	w.close()
	j = seedQueueJob(t, s)
	started = make(chan struct{})
	w, err = newAnalysisWorker(s, cfg, func(ctx context.Context, l AnalysisLease, j AnalysisJob) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	w.close()
	got, _ := s.GetAnalysisJob(context.Background(), j.ID)
	if got.State != AnalysisJobRunning {
		t.Fatal("shutdown incorrectly marked task terminal", got.State)
	}
	w, err = newAnalysisWorker(s, cfg, func(ctx context.Context, l AnalysisLease, j AnalysisJob) error {
		if j.Attempt != 2 {
			t.Errorf("takeover attempt=%d", j.Attempt)
		}
		j.State = AnalysisJobCompleted
		return s.SaveLeasedAnalysisJob(ctx, l, j)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	eventually(t, func() bool {
		got, _ := s.GetAnalysisJob(context.Background(), j.ID)
		return got.State == AnalysisJobCompleted
	})
}

type queueRoundTripFunc func(*http.Request) (*http.Response, error)

func (f queueRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAnalysisWorkerPermanentFailureDoesNotRetry(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	var calls atomic.Int64
	w, err := newAnalysisWorker(s, fastWorkerConfig(), func(context.Context, AnalysisLease, AnalysisJob) error {
		calls.Add(1)
		return errors.New("invalid deterministic input")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	eventually(t, func() bool {
		got, _ := s.GetAnalysisJob(context.Background(), j.ID)
		return got.State == AnalysisJobFailed
	})
	if calls.Load() != 1 {
		t.Fatal("permanent failure retried")
	}
}

func TestAnalysisCancelQueuedJobHTTP(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	h, err := NewHTTPHandler(s, HTTPConfig{Auth: AuthConfig{}, Engine: nil, Traces: nil, Memory: nil})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	r := httptest.NewRequest(http.MethodPost, "/v1/analysis-jobs/"+j.ID+"/cancel", nil)
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	if out.Code != http.StatusOK {
		t.Fatalf("cancel HTTP %d: %s", out.Code, out.Body.String())
	}
	var got AnalysisJob
	if err = json.Unmarshal(out.Body.Bytes(), &got); err != nil || got.State != AnalysisJobCancelled {
		t.Fatalf("cancel result: %#v %v", got, err)
	}
	stats := httptest.NewRecorder()
	h.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/v1/analysis-jobs/queue", nil))
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
	if _, _, err := s.CreateAnalysisJob(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	h, err := NewHTTPHandler(s, HTTPConfig{Auth: AuthConfig{}, Engine: nil, Traces: nil, Memory: nil})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	out := httptest.NewRecorder()
	h.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/v1/analysis-jobs/"+j.ID+"/cancel", nil))
	if out.Code != 200 {
		t.Fatalf("local owned job cancellation: HTTP %d", out.Code)
	}
}
