package control

import (
	"context"
	"errors"
	"sort"
	"time"
)

const evolutionQueueCapacity = 1000
const evolutionOwnerQueueCapacity = 100

// A fencing token increases on every acquisition. Until is informational:
// stores check their authoritative clock, never a caller-supplied timestamp.
type EvolutionLease struct {
	JobID    string
	WorkerID string
	Fence    int64
	Until    time.Time
}

type EvolutionQueueStats struct {
	Queued          int64      `json:"queued"`
	Running         int64      `json:"running"`
	Expired         int64      `json:"expired_leases"`
	CancelRequested int64      `json:"cancel_requested"`
	Completed       int64      `json:"completed"`
	Failed          int64      `json:"failed"`
	Cancelled       int64      `json:"cancelled"`
	RetryAttempts   int64      `json:"retry_attempts"`
	OldestQueuedAt  *time.Time `json:"oldest_queued_at,omitempty"`
}

func validEvolutionClaim(worker string, ttl time.Duration, maxAttempts int) bool {
	return worker != "" && len(worker) <= 160 && ttl >= time.Millisecond && ttl <= 10*time.Minute && maxAttempts > 0 && maxAttempts <= 20
}

func (s *MemoryStore) ClaimEvolutionJob(ctx context.Context, worker string, ttl time.Duration, maxAttempts int) (EvolutionJob, EvolutionLease, error) {
	if err := ctx.Err(); err != nil {
		return EvolutionJob{}, EvolutionLease{}, err
	}
	if !validEvolutionClaim(worker, ttl, maxAttempts) {
		return EvolutionJob{}, EvolutionLease{}, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var available []EvolutionJob
	for id, j := range s.evolutionJobs {
		l := s.evolutionLeases[id]
		if j.State.Terminal() || (j.State == EvolutionJobRunning && l.Until.After(now)) || (j.NextAttemptAt != nil && j.NextAttemptAt.After(now)) {
			continue
		}
		if j.CancelRequested || j.Attempt >= maxAttempts {
			releaseEvolutionState(&j, "job attempt limit reached", false, 0, maxAttempts, now)
			s.evolutionJobs[id] = cloneEvolutionJob(j)
			l.Fence++
			l.WorkerID = ""
			l.Until = time.Time{}
			s.evolutionLeases[id] = l
			continue
		}
		available = append(available, j)
	}
	if len(available) == 0 {
		return EvolutionJob{}, EvolutionLease{}, ErrNotFound
	}
	sort.Slice(available, func(i, j int) bool {
		if available[i].CreatedAt.Equal(available[j].CreatedAt) {
			return available[i].ID < available[j].ID
		}
		return available[i].CreatedAt.Before(available[j].CreatedAt)
	})
	j := available[0]
	j.Attempt++
	j.State = EvolutionJobRunning
	j.UpdatedAt = now
	j.NextAttemptAt = nil
	l := s.evolutionLeases[j.ID]
	l.JobID = j.ID
	l.WorkerID = worker
	l.Fence++
	l.Until = now.Add(ttl)
	s.evolutionLeases[j.ID] = l
	s.evolutionJobs[j.ID] = cloneEvolutionJob(j)
	return cloneEvolutionJob(j), l, nil
}

func (s *MemoryStore) ownsEvolutionLease(l EvolutionLease) bool {
	current := s.evolutionLeases[l.JobID]
	return current.Fence == l.Fence && l.Fence > 0 && current.WorkerID == l.WorkerID && current.Until.After(time.Now()) && s.evolutionJobs[l.JobID].State == EvolutionJobRunning
}

func (s *MemoryStore) HeartbeatEvolutionJob(ctx context.Context, l EvolutionLease, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if ttl < time.Millisecond || ttl > 10*time.Minute {
		return false, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ownsEvolutionLease(l) {
		return false, ErrLeaseLost
	}
	current := s.evolutionLeases[l.JobID]
	current.Until = time.Now().UTC().Add(ttl)
	s.evolutionLeases[l.JobID] = current
	return s.evolutionJobs[l.JobID].CancelRequested, nil
}

func sameEvolutionIdentity(a, b EvolutionJob) bool {
	return a.ID == b.ID && a.OwnerUserID == b.OwnerUserID && a.SourceTraceID == b.SourceTraceID && a.SourceAgentID == b.SourceAgentID && a.IdempotencyKey == b.IdempotencyKey && a.RequestFingerprint == b.RequestFingerprint && equalStringSlices(a.SourceTraceIDs, b.SourceTraceIDs)
}

func (s *MemoryStore) SaveLeasedEvolutionJob(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
	if err := validJobArtifacts(j); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.evolutionJobs[l.JobID]
	if !s.ownsEvolutionLease(l) || old.CancelRequested {
		return ErrLeaseLost
	}
	if !sameEvolutionIdentity(old, j) || j.Attempt != old.Attempt || (j.State != EvolutionJobRunning && j.State != EvolutionJobCompleted) {
		return ErrConflict
	}
	j.CancelRequested = false
	j.NextAttemptAt = nil
	j.UpdatedAt = time.Now().UTC()
	j.Error = ""
	s.evolutionJobs[j.ID] = cloneEvolutionJob(j)
	if j.State.Terminal() {
		current := s.evolutionLeases[j.ID]
		current.WorkerID = ""
		current.Until = time.Time{}
		s.evolutionLeases[j.ID] = current
	}
	return nil
}

func releaseEvolutionState(j *EvolutionJob, detail string, retry bool, delay time.Duration, maxAttempts int, now time.Time) {
	j.UpdatedAt = now
	j.Error = bounded(redactMemoryText(detail), 1000)
	j.NextAttemptAt = nil
	switch {
	case j.CancelRequested:
		j.State = EvolutionJobCancelled
		j.Error = ""
	case retry && j.Attempt < maxAttempts:
		j.State = EvolutionJobQueued
		t := now.Add(delay)
		j.NextAttemptAt = &t
	default:
		j.State = EvolutionJobFailed
	}
	for i := range j.Stages {
		if j.Stages[i].State != EvolutionStageRunning {
			continue
		}
		j.Stages[i].Error = j.Error
		if j.State == EvolutionJobQueued {
			j.Stages[i].State = EvolutionStageQueued
			j.Stages[i].StartedAt = nil
			j.Stages[i].FinishedAt = nil
		} else {
			j.Stages[i].State = EvolutionStageFailed
			j.Stages[i].FinishedAt = &now
		}
	}
}

func (s *MemoryStore) ReleaseEvolutionJob(ctx context.Context, l EvolutionLease, detail string, retry bool, delay time.Duration, maxAttempts int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay < 0 || maxAttempts < 1 {
		return ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ownsEvolutionLease(l) {
		return ErrLeaseLost
	}
	j := s.evolutionJobs[l.JobID]
	releaseEvolutionState(&j, detail, retry, delay, maxAttempts, time.Now().UTC())
	s.evolutionJobs[j.ID] = cloneEvolutionJob(j)
	current := s.evolutionLeases[j.ID]
	current.WorkerID = ""
	current.Until = time.Time{}
	s.evolutionLeases[j.ID] = current
	return nil
}

func (s *MemoryStore) CancelEvolutionJob(ctx context.Context, owner, id string) (EvolutionJob, error) {
	if err := ctx.Err(); err != nil {
		return EvolutionJob{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.evolutionJobs[id]
	if !ok || j.OwnerUserID != owner {
		return EvolutionJob{}, ErrNotFound
	}
	if j.State.Terminal() {
		return cloneEvolutionJob(j), nil
	}
	j.CancelRequested = true
	j.UpdatedAt = time.Now().UTC()
	l := s.evolutionLeases[id]
	if j.State == EvolutionJobQueued || !l.Until.After(time.Now()) {
		releaseEvolutionState(&j, "", false, 0, 1, j.UpdatedAt)
		l.Fence++
		l.WorkerID = ""
		l.Until = time.Time{}
		s.evolutionLeases[id] = l
	}
	s.evolutionJobs[id] = cloneEvolutionJob(j)
	return cloneEvolutionJob(j), nil
}

func (s *MemoryStore) EvolutionQueueStats(ctx context.Context, owner *string) (EvolutionQueueStats, error) {
	if err := ctx.Err(); err != nil {
		return EvolutionQueueStats{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var stats EvolutionQueueStats
	for id, j := range s.evolutionJobs {
		if owner != nil && j.OwnerUserID != *owner {
			continue
		}
		if j.Attempt > 1 {
			stats.RetryAttempts += int64(j.Attempt - 1)
		}
		switch j.State {
		case EvolutionJobQueued:
			stats.Queued++
			if stats.OldestQueuedAt == nil || j.CreatedAt.Before(*stats.OldestQueuedAt) {
				v := j.CreatedAt
				stats.OldestQueuedAt = &v
			}
		case EvolutionJobRunning:
			stats.Running++
			if !s.evolutionLeases[id].Until.After(time.Now()) {
				stats.Expired++
			}
		case EvolutionJobCompleted:
			stats.Completed++
		case EvolutionJobFailed:
			stats.Failed++
		case EvolutionJobCancelled:
			stats.Cancelled++
		}
		if j.CancelRequested && !j.State.Terminal() {
			stats.CancelRequested++
		}
	}
	return stats, nil
}

var errEvolutionTemporary = errors.New("temporary engine failure")
