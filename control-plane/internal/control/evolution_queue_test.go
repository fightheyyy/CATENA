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

func seedQueueJob(t *testing.T, s Store) EvolutionJob {
	t.Helper()
	now := time.Now().UTC()
	j := EvolutionJob{Schema: evolutionJobSchema, ID: newID("queue-test"), SourceKind: EvolutionSourceTrace, SourceTraceID: "00112233445566778899aabbccddeeff", IdempotencyKey: newID("queue-key"), RequestFingerprint: "queue-contract", State: EvolutionJobQueued, Stages: cloneEvolutionStages(evolutionJobStages), CreatedAt: now, UpdatedAt: now}
	got, created, err := s.CreateEvolutionJob(context.Background(), j)
	if err != nil || !created {
		t.Fatalf("seed job: created=%v err=%v", created, err)
	}
	return got
}

func TestEvolutionQueueMemoryContract(t *testing.T) {
	queueContract(t, func(t *testing.T) Store { return NewMemoryStore() })
}

func queueContract(t *testing.T, makeStore func(*testing.T) Store) {
	t.Run("admission_limit_preserves_idempotent_retries", func(t *testing.T) {
		s := makeStore(t)
		first := seedQueueJob(t, s)
		for i := 1; i < evolutionOwnerQueueCapacity; i++ {
			seedQueueJob(t, s)
		}
		_, created, err := s.CreateEvolutionJob(context.Background(), first)
		if err != nil || created {
			t.Fatalf("full queue rejected existing idempotent request: %v %v", created, err)
		}
		next := first
		next.ID = newID("over-capacity")
		next.IdempotencyKey = newID("over-capacity-key")
		if _, _, err := s.CreateEvolutionJob(context.Background(), next); !errors.Is(err, ErrQueueFull) {
			t.Fatalf("queue admitted over capacity: %v", err)
		}
		owner := ""
		stats, err := s.EvolutionQueueStats(context.Background(), &owner)
		if err != nil || stats.Queued != int64(evolutionOwnerQueueCapacity) {
			t.Fatalf("queue stats: %#v %v", stats, err)
		}
		other := "other-owner"
		stats, err = s.EvolutionQueueStats(context.Background(), &other)
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
					j, l, err := s.ClaimEvolutionJob(context.Background(), fmt.Sprintf("worker-%d", i), 10*time.Second, 3)
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
					j.State = EvolutionJobCompleted
					if err := s.SaveLeasedEvolutionJob(context.Background(), l, j); err != nil {
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
		old, lease, err := s.ClaimEvolutionJob(ctx, "old", 150*time.Millisecond, 3)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(190 * time.Millisecond)
		if _, err := s.HeartbeatEvolutionJob(ctx, lease, time.Second); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("expired heartbeat: %v", err)
		}
		fresh, newLease, err := s.ClaimEvolutionJob(ctx, "new", 3*time.Second, 3)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.ID != old.ID || fresh.Attempt != 2 || newLease.Fence <= lease.Fence {
			t.Fatalf("invalid takeover: %#v %#v", fresh, newLease)
		}
		old.State = EvolutionJobCompleted
		if err := s.SaveLeasedEvolutionJob(ctx, lease, old); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale write: %v", err)
		}
		if err := s.UpdateEvolutionJob(ctx, old); err == nil {
			t.Fatal("unleased legacy update bypassed fencing")
		}
		fresh.State = EvolutionJobCompleted
		if err := s.SaveLeasedEvolutionJob(ctx, newLease, fresh); err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseEvolutionJob(ctx, lease, "old failure", false, 0, 3); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale failure: %v", err)
		}
		got, err := s.GetEvolutionJob(ctx, fresh.ID)
		if err != nil || got.State != EvolutionJobCompleted {
			t.Fatalf("success overwritten: %#v %v", got, err)
		}
	})
	t.Run("retry_checkpoint_backoff_and_limit", func(t *testing.T) {
		s := makeStore(t)
		seedQueueJob(t, s)
		ctx := context.Background()
		j, l, err := s.ClaimEvolutionJob(ctx, "retry", 3*time.Second, 2)
		if err != nil {
			t.Fatal(err)
		}
		j.Stages[0].State = EvolutionStageCompleted
		j.Stages[0].RawOutput = json.RawMessage(`{"checkpoint":true}`)
		if err := s.SaveLeasedEvolutionJob(ctx, l, j); err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseEvolutionJob(ctx, l, "transient", true, 150*time.Millisecond, 2); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.ClaimEvolutionJob(ctx, "early", 3*time.Second, 2); !errors.Is(err, ErrNotFound) {
			t.Fatalf("backoff ignored: %v", err)
		}
		time.Sleep(190 * time.Millisecond)
		j, l, err = s.ClaimEvolutionJob(ctx, "retry", 3*time.Second, 2)
		if err != nil {
			t.Fatal(err)
		}
		var checkpoint struct {
			Checkpoint bool `json:"checkpoint"`
		}
		decodeErr := json.Unmarshal(j.Stages[0].RawOutput, &checkpoint)
		if j.Attempt != 2 || j.Stages[0].State != EvolutionStageCompleted || decodeErr != nil || !checkpoint.Checkpoint {
			t.Fatalf("checkpoint lost: %#v", j)
		}
		if err := s.ReleaseEvolutionJob(ctx, l, "exhausted", true, 0, 2); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetEvolutionJob(ctx, j.ID)
		if got.State != EvolutionJobFailed {
			t.Fatalf("attempt limit ignored: %#v", got)
		}
		if _, _, err := s.ClaimEvolutionJob(ctx, "again", time.Second, 2); !errors.Is(err, ErrNotFound) {
			t.Fatalf("terminal job reclaimed: %v", err)
		}
	})
	t.Run("cancellation_ownership_and_commit_race", func(t *testing.T) {
		s := makeStore(t)
		ctx := context.Background()
		j := seedQueueJob(t, s)
		if _, err := s.CancelEvolutionJob(ctx, "other-owner", j.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("tenant isolation: %v", err)
		}
		got, err := s.CancelEvolutionJob(ctx, "", j.ID)
		if err != nil || got.State != EvolutionJobCancelled {
			t.Fatalf("queued cancel: %#v %v", got, err)
		}
		seedQueueJob(t, s)
		j, l, err := s.ClaimEvolutionJob(ctx, "cancel", 3*time.Second, 3)
		if err != nil {
			t.Fatal(err)
		}
		got, err = s.CancelEvolutionJob(ctx, "", j.ID)
		if err != nil || !got.CancelRequested || got.State != EvolutionJobRunning {
			t.Fatalf("running cancel: %#v %v", got, err)
		}
		requested, err := s.HeartbeatEvolutionJob(ctx, l, 3*time.Second)
		if err != nil || !requested {
			t.Fatalf("heartbeat failed to observe cancellation: %v %v", requested, err)
		}
		j.State = EvolutionJobCompleted
		if err := s.SaveLeasedEvolutionJob(ctx, l, j); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("completion raced past cancel: %v", err)
		}
		if err := s.ReleaseEvolutionJob(ctx, l, "context cancelled", false, 0, 3); err != nil {
			t.Fatal(err)
		}
		got, err = s.CancelEvolutionJob(ctx, "", j.ID)
		if err != nil || got.State != EvolutionJobCancelled {
			t.Fatalf("cancel ack: %#v %v", got, err)
		}
	})
	t.Run("crashed_final_attempt_is_reaped", func(t *testing.T) {
		s := makeStore(t)
		seedQueueJob(t, s)
		ctx := context.Background()
		j, l, err := s.ClaimEvolutionJob(ctx, "crashed", 150*time.Millisecond, 1)
		if err != nil {
			t.Fatal(err)
		}
		j.Stages[0].State = EvolutionStageRunning
		if err = s.SaveLeasedEvolutionJob(ctx, l, j); err != nil {
			t.Fatal(err)
		}
		time.Sleep(190 * time.Millisecond)
		if _, _, err := s.ClaimEvolutionJob(ctx, "reaper", time.Second, 1); !errors.Is(err, ErrNotFound) {
			t.Fatalf("attempt cap bypassed: %v", err)
		}
		got, _ := s.GetEvolutionJob(ctx, j.ID)
		if got.State != EvolutionJobFailed {
			t.Fatalf("dead job stuck running: %#v", got)
		}
		if got.Stages[0].State != EvolutionStageFailed || got.Stages[0].FinishedAt == nil {
			t.Fatal("reaped job retained a running stage")
		}
		if _, err := s.HeartbeatEvolutionJob(ctx, l, time.Second); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("dead worker revived: %v", err)
		}
	})
}
