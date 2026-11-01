package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func seedQueueJob(t *testing.T, s Store) AnalysisJob {
	t.Helper()
	now := time.Now().UTC()
	j := AnalysisJob{Schema: analysisJobSchema, ID: newID("queue-test"), SourceKind: AnalysisSourceTrace, SourceTraceID: "00112233445566778899aabbccddeeff", IdempotencyKey: newID("queue-key"), RequestFingerprint: "queue-contract", State: AnalysisJobQueued, Stages: []AnalysisStage{{Name: "generate", State: AnalysisStageQueued}}, CreatedAt: now, UpdatedAt: now}
	got, created, err := s.CreateAnalysisJob(context.Background(), j)
	if err != nil || !created {
		t.Fatalf("seed job: created=%v err=%v", created, err)
	}
	return got
}

func TestAnalysisQueueMemoryContract(t *testing.T) {
	queueContract(t, func(t *testing.T) Store { return NewMemoryStore() })
}

func queueContract(t *testing.T, makeStore func(*testing.T) Store) {
	t.Run("admission_limit_preserves_idempotent_retries", func(t *testing.T) {
		s := makeStore(t)
		first := seedQueueJob(t, s)
		for i := 1; i < analysisOwnerQueueCapacity; i++ {
			seedQueueJob(t, s)
		}
		_, created, err := s.CreateAnalysisJob(context.Background(), first)
		if err != nil || created {
			t.Fatalf("full queue rejected existing idempotent request: %v %v", created, err)
		}
		next := first
		next.ID = newID("over-capacity")
		next.IdempotencyKey = newID("over-capacity-key")
		if _, _, err := s.CreateAnalysisJob(context.Background(), next); !errors.Is(err, ErrQueueFull) {
			t.Fatalf("queue admitted over capacity: %v", err)
		}
		owner := ""
		stats, err := s.AnalysisQueueStats(context.Background(), &owner)
		if err != nil || stats.Queued != int64(analysisOwnerQueueCapacity) {
			t.Fatalf("queue stats: %#v %v", stats, err)
		}
		other := "other-owner"
		stats, err = s.AnalysisQueueStats(context.Background(), &other)
		if err != nil || stats.Queued != 0 {
			t.Fatalf("queue stats crossed owner boundary: %#v %v", stats, err)
		}
	})
	t.Run("concurrent_claim_and_commit", func(t *testing.T) {
		s := makeStore(t)
		for i := 0; i < 24; i++ {
			seedQueueJob(t, s)
		}
		var mu sync.Mutex
		seen := map[string]bool{}
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				for {
					j, l, err := s.ClaimAnalysisJob(context.Background(), fmt.Sprintf("worker-%d", i), 10*time.Second, 3)
					if errors.Is(err, ErrNotFound) {
						return
					}
					if err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					if seen[j.ID] {
						t.Error("duplicate live claim", j.ID)
					}
					seen[j.ID] = true
					mu.Unlock()
					j.State = AnalysisJobCompleted
					if err := s.SaveLeasedAnalysisJob(context.Background(), l, j); err != nil {
						t.Error(err)
						return
					}
				}
			}(i)
		}
		wg.Wait()
		if len(seen) != 24 {
			t.Fatalf("claimed %d/24 jobs", len(seen))
		}
	})
	t.Run("expired_worker_cannot_overwrite_successor", func(t *testing.T) {
		s := makeStore(t)
		seedQueueJob(t, s)
		ctx := context.Background()
		old, lease, err := s.ClaimAnalysisJob(ctx, "old", 150*time.Millisecond, 3)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(190 * time.Millisecond)
		if _, err := s.HeartbeatAnalysisJob(ctx, lease, time.Second); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("expired heartbeat: %v", err)
		}
		fresh, newLease, err := s.ClaimAnalysisJob(ctx, "new", 3*time.Second, 3)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.ID != old.ID || fresh.Attempt != 2 || newLease.Fence <= lease.Fence {
			t.Fatalf("invalid takeover: %#v %#v", fresh, newLease)
		}
		old.State = AnalysisJobCompleted
		if err := s.SaveLeasedAnalysisJob(ctx, lease, old); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale write: %v", err)
		}
		if err := s.UpdateAnalysisJob(ctx, old); err == nil {
			t.Fatal("unleased legacy update bypassed fencing")
		}
		fresh.State = AnalysisJobCompleted
		if err := s.SaveLeasedAnalysisJob(ctx, newLease, fresh); err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseAnalysisJob(ctx, lease, "old failure", false, 0, 3); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale failure: %v", err)
		}
		got, err := s.GetAnalysisJob(ctx, fresh.ID)
		if err != nil || got.State != AnalysisJobCompleted {
			t.Fatalf("success overwritten: %#v %v", got, err)
		}
	})
	t.Run("retry_checkpoint_backoff_and_limit", func(t *testing.T) {
		s := makeStore(t)
		seedQueueJob(t, s)
		ctx := context.Background()
		j, l, err := s.ClaimAnalysisJob(ctx, "retry", 3*time.Second, 2)
		if err != nil {
			t.Fatal(err)
		}
		j.Stages[0].State = AnalysisStageCompleted
		j.Stages[0].RawOutput = json.RawMessage(`{"checkpoint":true}`)
		if err := s.SaveLeasedAnalysisJob(ctx, l, j); err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseAnalysisJob(ctx, l, "transient", true, 150*time.Millisecond, 2); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.ClaimAnalysisJob(ctx, "early", 3*time.Second, 2); !errors.Is(err, ErrNotFound) {
			t.Fatalf("backoff ignored: %v", err)
		}
		time.Sleep(190 * time.Millisecond)
		j, l, err = s.ClaimAnalysisJob(ctx, "retry", 3*time.Second, 2)
		if err != nil {
			t.Fatal(err)
		}
		var checkpoint struct {
			Checkpoint bool `json:"checkpoint"`
		}
		decodeErr := json.Unmarshal(j.Stages[0].RawOutput, &checkpoint)
		if j.Attempt != 2 || j.Stages[0].State != AnalysisStageCompleted || decodeErr != nil || !checkpoint.Checkpoint {
			t.Fatalf("checkpoint lost: %#v", j)
		}
		if err := s.ReleaseAnalysisJob(ctx, l, "exhausted", true, 0, 2); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetAnalysisJob(ctx, j.ID)
		if got.State != AnalysisJobFailed {
			t.Fatalf("attempt limit ignored: %#v", got)
		}
		if _, _, err := s.ClaimAnalysisJob(ctx, "again", time.Second, 2); !errors.Is(err, ErrNotFound) {
			t.Fatalf("terminal job reclaimed: %v", err)
		}
	})
	t.Run("cancellation_ownership_and_commit_race", func(t *testing.T) {
		s := makeStore(t)
		ctx := context.Background()
		j := seedQueueJob(t, s)
		if _, err := s.CancelAnalysisJob(ctx, "other-owner", j.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("tenant isolation: %v", err)
		}
		got, err := s.CancelAnalysisJob(ctx, "", j.ID)
		if err != nil || got.State != AnalysisJobCancelled {
			t.Fatalf("queued cancel: %#v %v", got, err)
		}
		seedQueueJob(t, s)
		j, l, err := s.ClaimAnalysisJob(ctx, "cancel", 3*time.Second, 3)
		if err != nil {
			t.Fatal(err)
		}
		got, err = s.CancelAnalysisJob(ctx, "", j.ID)
		if err != nil || !got.CancelRequested || got.State != AnalysisJobRunning {
			t.Fatalf("running cancel: %#v %v", got, err)
		}
		requested, err := s.HeartbeatAnalysisJob(ctx, l, 3*time.Second)
		if err != nil || !requested {
			t.Fatalf("heartbeat failed to observe cancellation: %v %v", requested, err)
		}
		j.State = AnalysisJobCompleted
		if err := s.SaveLeasedAnalysisJob(ctx, l, j); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("completion raced past cancel: %v", err)
		}
		if err := s.ReleaseAnalysisJob(ctx, l, "context cancelled", false, 0, 3); err != nil {
			t.Fatal(err)
		}
		got, err = s.CancelAnalysisJob(ctx, "", j.ID)
		if err != nil || got.State != AnalysisJobCancelled {
			t.Fatalf("cancel ack: %#v %v", got, err)
		}
	})
	t.Run("crashed_final_attempt_is_reaped", func(t *testing.T) {
		s := makeStore(t)
		seedQueueJob(t, s)
		ctx := context.Background()
		j, l, err := s.ClaimAnalysisJob(ctx, "crashed", 150*time.Millisecond, 1)
		if err != nil {
			t.Fatal(err)
		}
		j.Stages[0].State = AnalysisStageRunning
		if err = s.SaveLeasedAnalysisJob(ctx, l, j); err != nil {
			t.Fatal(err)
		}
		time.Sleep(190 * time.Millisecond)
		if _, _, err := s.ClaimAnalysisJob(ctx, "reaper", time.Second, 1); !errors.Is(err, ErrNotFound) {
			t.Fatalf("attempt cap bypassed: %v", err)
		}
		got, _ := s.GetAnalysisJob(ctx, j.ID)
		if got.State != AnalysisJobFailed {
			t.Fatalf("dead job stuck running: %#v", got)
		}
		if got.Stages[0].State != AnalysisStageFailed || got.Stages[0].FinishedAt == nil {
			t.Fatal("reaped job retained a running stage")
		}
		if _, err := s.HeartbeatAnalysisJob(ctx, l, time.Second); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("dead worker revived: %v", err)
		}
	})
}
