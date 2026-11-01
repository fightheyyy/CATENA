package control

import (
	"context"
	"errors"
	"sort"
	"time"
)

const analysisQueueCapacity = 1000
const analysisOwnerQueueCapacity = 100

// A fencing token increases on every acquisition. Until is informational:
// stores check their authoritative clock, never a caller-supplied timestamp.
type AnalysisLease struct {
	JobID    string
	WorkerID string
	Fence    int64
	Until    time.Time
}

type AnalysisQueueStats struct {
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

func validAnalysisClaim(worker string, ttl time.Duration, maxAttempts int) bool {
	return worker != "" && len(worker) <= 160 && ttl >= time.Millisecond && ttl <= 10*time.Minute && maxAttempts > 0 && maxAttempts <= 20
}

func (s *MemoryStore) ClaimAnalysisJob(ctx context.Context, worker string, ttl time.Duration, maxAttempts int) (AnalysisJob, AnalysisLease, error) {
	if err := ctx.Err(); err != nil {
		return AnalysisJob{}, AnalysisLease{}, err
	}
	if !validAnalysisClaim(worker, ttl, maxAttempts) {
		return AnalysisJob{}, AnalysisLease{}, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var available []AnalysisJob
	for id, j := range s.analysisJobs {
		l := s.analysisLeases[id]
		if j.State.Terminal() || (j.State == AnalysisJobRunning && l.Until.After(now)) || (j.NextAttemptAt != nil && j.NextAttemptAt.After(now)) {
			continue
		}
		if j.CancelRequested || j.Attempt >= maxAttempts {
			releaseAnalysisState(&j, "job attempt limit reached", false, 0, maxAttempts, now)
			s.analysisJobs[id] = cloneAnalysisJob(j)
			l.Fence++
			l.WorkerID = ""
			l.Until = time.Time{}
			s.analysisLeases[id] = l
			continue
		}
		available = append(available, j)
	}
	if len(available) == 0 {
		return AnalysisJob{}, AnalysisLease{}, ErrNotFound
	}
	sort.Slice(available, func(i, j int) bool {
		if available[i].CreatedAt.Equal(available[j].CreatedAt) {
			return available[i].ID < available[j].ID
		}
		return available[i].CreatedAt.Before(available[j].CreatedAt)
	})
	j := available[0]
	j.Attempt++
	j.State = AnalysisJobRunning
	j.UpdatedAt = now
	j.NextAttemptAt = nil
	l := s.analysisLeases[j.ID]
	l.JobID = j.ID
	l.WorkerID = worker
	l.Fence++
	l.Until = now.Add(ttl)
	s.analysisLeases[j.ID] = l
	s.analysisJobs[j.ID] = cloneAnalysisJob(j)
	return cloneAnalysisJob(j), l, nil
}

func (s *MemoryStore) ownsAnalysisLease(l AnalysisLease) bool {
	current := s.analysisLeases[l.JobID]
	return current.Fence == l.Fence && l.Fence > 0 && current.WorkerID == l.WorkerID && current.Until.After(time.Now()) && s.analysisJobs[l.JobID].State == AnalysisJobRunning
}

func (s *MemoryStore) HeartbeatAnalysisJob(ctx context.Context, l AnalysisLease, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if ttl < time.Millisecond || ttl > 10*time.Minute {
		return false, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ownsAnalysisLease(l) {
		return false, ErrLeaseLost
	}
	current := s.analysisLeases[l.JobID]
	current.Until = time.Now().UTC().Add(ttl)
	s.analysisLeases[l.JobID] = current
	return s.analysisJobs[l.JobID].CancelRequested, nil
}

func sameAnalysisIdentity(a, b AnalysisJob) bool {
	return a.ID == b.ID && a.OwnerUserID == b.OwnerUserID && a.SourceTraceID == b.SourceTraceID && a.SourceAgentID == b.SourceAgentID && a.IdempotencyKey == b.IdempotencyKey && a.RequestFingerprint == b.RequestFingerprint && equalStringSlices(a.SourceTraceIDs, b.SourceTraceIDs)
}

func (s *MemoryStore) SaveLeasedAnalysisJob(ctx context.Context, l AnalysisLease, j AnalysisJob) error {
	if err := validJobArtifacts(j); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.analysisJobs[l.JobID]
	if !s.ownsAnalysisLease(l) || old.CancelRequested {
		return ErrLeaseLost
	}
	if !sameAnalysisIdentity(old, j) || j.Attempt != old.Attempt || (j.State != AnalysisJobRunning && j.State != AnalysisJobCompleted) {
		return ErrConflict
	}
	j.CancelRequested = false
	j.NextAttemptAt = nil
	j.UpdatedAt = time.Now().UTC()
	j.Error = ""
	s.analysisJobs[j.ID] = cloneAnalysisJob(j)
	if j.State.Terminal() {
		current := s.analysisLeases[j.ID]
		current.WorkerID = ""
		current.Until = time.Time{}
		s.analysisLeases[j.ID] = current
	}
	return nil
}

func releaseAnalysisState(j *AnalysisJob, detail string, retry bool, delay time.Duration, maxAttempts int, now time.Time) {
	j.UpdatedAt = now
	j.Error = bounded(redactMemoryText(detail), 1000)
	j.NextAttemptAt = nil
	switch {
	case j.CancelRequested:
		j.State = AnalysisJobCancelled
		j.Error = ""
	case retry && j.Attempt < maxAttempts:
		j.State = AnalysisJobQueued
		t := now.Add(delay)
		j.NextAttemptAt = &t
	default:
		j.State = AnalysisJobFailed
	}
	for i := range j.Stages {
		if j.Stages[i].State != AnalysisStageRunning {
			continue
		}
		j.Stages[i].Error = j.Error
		if j.State == AnalysisJobQueued {
			j.Stages[i].State = AnalysisStageQueued
			j.Stages[i].StartedAt = nil
			j.Stages[i].FinishedAt = nil
		} else {
			j.Stages[i].State = AnalysisStageFailed
			j.Stages[i].FinishedAt = &now
		}
	}
}

func (s *MemoryStore) ReleaseAnalysisJob(ctx context.Context, l AnalysisLease, detail string, retry bool, delay time.Duration, maxAttempts int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay < 0 || maxAttempts < 1 {
		return ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ownsAnalysisLease(l) {
		return ErrLeaseLost
	}
	j := s.analysisJobs[l.JobID]
	releaseAnalysisState(&j, detail, retry, delay, maxAttempts, time.Now().UTC())
	s.analysisJobs[j.ID] = cloneAnalysisJob(j)
	current := s.analysisLeases[j.ID]
	current.WorkerID = ""
	current.Until = time.Time{}
	s.analysisLeases[j.ID] = current
	return nil
}

func (s *MemoryStore) CancelAnalysisJob(ctx context.Context, owner, id string) (AnalysisJob, error) {
	if err := ctx.Err(); err != nil {
		return AnalysisJob{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.analysisJobs[id]
	if !ok || j.OwnerUserID != owner {
		return AnalysisJob{}, ErrNotFound
	}
	if j.State.Terminal() {
		return cloneAnalysisJob(j), nil
	}
	j.CancelRequested = true
	j.UpdatedAt = time.Now().UTC()
	l := s.analysisLeases[id]
	if j.State == AnalysisJobQueued || !l.Until.After(time.Now()) {
		releaseAnalysisState(&j, "", false, 0, 1, j.UpdatedAt)
		l.Fence++
		l.WorkerID = ""
		l.Until = time.Time{}
		s.analysisLeases[id] = l
	}
	s.analysisJobs[id] = cloneAnalysisJob(j)
	return cloneAnalysisJob(j), nil
}

func (s *MemoryStore) AnalysisQueueStats(ctx context.Context, owner *string) (AnalysisQueueStats, error) {
	if err := ctx.Err(); err != nil {
		return AnalysisQueueStats{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var stats AnalysisQueueStats
	for id, j := range s.analysisJobs {
		if owner != nil && j.OwnerUserID != *owner {
			continue
		}
		if j.Attempt > 1 {
			stats.RetryAttempts += int64(j.Attempt - 1)
		}
		switch j.State {
		case AnalysisJobQueued:
			stats.Queued++
			if stats.OldestQueuedAt == nil || j.CreatedAt.Before(*stats.OldestQueuedAt) {
				v := j.CreatedAt
				stats.OldestQueuedAt = &v
			}
		case AnalysisJobRunning:
			stats.Running++
			if !s.analysisLeases[id].Until.After(time.Now()) {
				stats.Expired++
			}
		case AnalysisJobCompleted:
			stats.Completed++
		case AnalysisJobFailed:
			stats.Failed++
		case AnalysisJobCancelled:
			stats.Cancelled++
		}
		if j.CancelRequested && !j.State.Terminal() {
			stats.CancelRequested++
		}
	}
	return stats, nil
}

var errAnalysisTemporary = errors.New("temporary engine failure")
