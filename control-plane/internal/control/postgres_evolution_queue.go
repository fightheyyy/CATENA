package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *PostgresStore) ClaimEvolutionJob(ctx context.Context, worker string, ttl time.Duration, maxAttempts int) (EvolutionJob, EvolutionLease, error) {
	if !validEvolutionClaim(worker, ttl, maxAttempts) {
		return EvolutionJob{}, EvolutionLease{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EvolutionJob{}, EvolutionLease{}, err
	}
	defer tx.Rollback()
	// Reap a bounded set without waiting on another worker's row locks. Jobs at
	// their attempt limit must not remain running forever after a process dies.
	_, err = tx.ExecContext(ctx, `WITH expired AS (
 SELECT job_id FROM spiral_evolution_jobs WHERE state IN ('queued','running')
 AND (lease_until IS NULL OR lease_until<=clock_timestamp())
 AND (cancel_requested OR attempt >= $1)
 ORDER BY created_at,job_id FOR UPDATE SKIP LOCKED LIMIT 32
)
UPDATE spiral_evolution_jobs j SET
 state=CASE WHEN cancel_requested THEN 'cancelled' ELSE 'failed' END,
 worker_id='',lease_until=NULL,fencing_token=fencing_token+1,updated_at=clock_timestamp(),
 document=document || jsonb_build_object('state',CASE WHEN cancel_requested THEN 'cancelled' ELSE 'failed' END,
   'error',CASE WHEN cancel_requested THEN '' ELSE 'job attempt limit reached' END,'updated_at',clock_timestamp(),
   'stages',COALESCE((SELECT jsonb_agg(CASE WHEN stage->>'state'='running'
     THEN stage || jsonb_build_object('state','failed','finished_at',clock_timestamp(),
       'error',CASE WHEN j.cancel_requested THEN '' ELSE 'job attempt limit reached' END)
     ELSE stage END ORDER BY ordinal)
     FROM jsonb_array_elements(COALESCE(j.document->'stages','[]'::jsonb)) WITH ORDINALITY AS x(stage,ordinal)),'[]'::jsonb))
FROM expired e WHERE j.job_id=e.job_id`, maxAttempts)
	if err != nil {
		return EvolutionJob{}, EvolutionLease{}, err
	}
	var j EvolutionJob
	var l EvolutionLease
	var doc []byte
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
 SELECT job_id FROM spiral_evolution_jobs WHERE state IN ('queued','running')
 AND NOT cancel_requested AND attempt < $3
 AND ((state='queued' AND next_attempt_at<=clock_timestamp()) OR
      (state='running' AND (lease_until IS NULL OR lease_until<=clock_timestamp())))
 ORDER BY next_attempt_at,created_at,job_id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE spiral_evolution_jobs j SET worker_id=$1,fencing_token=fencing_token+1,
 lease_until=clock_timestamp()+($2::bigint * interval '1 millisecond'),attempt=attempt+1,
 state='running',updated_at=clock_timestamp(),
 document=document || jsonb_build_object('state','running','attempt',attempt+1,'next_attempt_at',NULL,'updated_at',clock_timestamp())
FROM candidate c WHERE j.job_id=c.job_id
RETURNING COALESCE(j.owner_user_id,''),j.idempotency_key,j.request_fingerprint,j.document,
 j.job_id,j.worker_id,j.fencing_token,j.lease_until`, worker, ttl.Milliseconds(), maxAttempts).Scan(
		&j.OwnerUserID, &j.IdempotencyKey, &j.RequestFingerprint, &doc, &l.JobID, &l.WorkerID, &l.Fence, &l.Until)
	if errors.Is(err, sql.ErrNoRows) {
		if e := tx.Commit(); e != nil {
			return EvolutionJob{}, EvolutionLease{}, e
		}
		return EvolutionJob{}, EvolutionLease{}, ErrNotFound
	}
	if err != nil {
		return EvolutionJob{}, EvolutionLease{}, err
	}
	if err = json.Unmarshal(doc, &j); err != nil {
		return EvolutionJob{}, EvolutionLease{}, err
	}
	if err = tx.Commit(); err != nil {
		return EvolutionJob{}, EvolutionLease{}, err
	}
	return j, l, nil
}

func (s *PostgresStore) HeartbeatEvolutionJob(ctx context.Context, l EvolutionLease, ttl time.Duration) (bool, error) {
	if ttl < time.Millisecond || ttl > 10*time.Minute {
		return false, ErrConflict
	}
	var cancelled bool
	err := s.db.QueryRowContext(ctx, `UPDATE spiral_evolution_jobs SET lease_until=clock_timestamp()+($4::bigint * interval '1 millisecond')
WHERE job_id=$1 AND worker_id=$2 AND fencing_token=$3 AND fencing_token>0
AND state='running' AND lease_until>clock_timestamp() RETURNING cancel_requested`, l.JobID, l.WorkerID, l.Fence, ttl.Milliseconds()).Scan(&cancelled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrLeaseLost
	}
	return cancelled, err
}

func (s *PostgresStore) SaveLeasedEvolutionJob(ctx context.Context, l EvolutionLease, j EvolutionJob) error {
	if j.ID != l.JobID || (j.State != EvolutionJobRunning && j.State != EvolutionJobCompleted) {
		return ErrConflict
	}
	j.UpdatedAt = time.Now().UTC()
	j.Error = ""
	j.NextAttemptAt = nil
	j.CancelRequested = false
	doc, err := json.Marshal(j)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE spiral_evolution_jobs SET state=$4,current_stage=$5,document=$6,updated_at=clock_timestamp(),
 worker_id=CASE WHEN $4='completed' THEN '' ELSE worker_id END,
 lease_until=CASE WHEN $4='completed' THEN NULL ELSE lease_until END
WHERE job_id=$1 AND worker_id=$2 AND fencing_token=$3 AND fencing_token>0
AND state='running' AND lease_until>clock_timestamp() AND NOT cancel_requested
AND COALESCE(owner_user_id,'')=$7 AND COALESCE(source_run_id,'')=$8
AND source_trace_id=$9 AND source_agent_id=$10 AND idempotency_key=$11 AND request_fingerprint=$12 AND attempt=$13`,
		l.JobID, l.WorkerID, l.Fence, j.State, j.CurrentStage, doc, j.OwnerUserID, j.SourceRunID, j.SourceTraceID, j.SourceAgentID, j.IdempotencyKey, j.RequestFingerprint, j.Attempt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrLeaseLost
	}
	return err
}

func (s *PostgresStore) ReleaseEvolutionJob(ctx context.Context, l EvolutionLease, detail string, retry bool, delay time.Duration, maxAttempts int) error {
	if delay < 0 || maxAttempts < 1 {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var doc []byte
	var cancel bool
	var attempts int
	var now time.Time
	err = tx.QueryRowContext(ctx, `SELECT document,cancel_requested,attempt,clock_timestamp() FROM spiral_evolution_jobs
WHERE job_id=$1 AND worker_id=$2 AND fencing_token=$3 AND fencing_token>0 AND state='running'
AND lease_until>clock_timestamp() FOR UPDATE`, l.JobID, l.WorkerID, l.Fence).Scan(&doc, &cancel, &attempts, &now)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	var j EvolutionJob
	if err = json.Unmarshal(doc, &j); err != nil {
		return err
	}
	j.CancelRequested = cancel
	j.Attempt = attempts
	releaseEvolutionState(&j, detail, retry, delay, maxAttempts, now)
	doc, err = json.Marshal(j)
	if err != nil {
		return err
	}
	next := now
	if j.NextAttemptAt != nil {
		next = *j.NextAttemptAt
	}
	_, err = tx.ExecContext(ctx, `UPDATE spiral_evolution_jobs SET state=$2,document=$3,updated_at=$4,
worker_id='',lease_until=NULL,next_attempt_at=$5 WHERE job_id=$1`, l.JobID, j.State, doc, now, next)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) CancelEvolutionJob(ctx context.Context, owner, id string) (EvolutionJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EvolutionJob{}, err
	}
	defer tx.Rollback()
	var j EvolutionJob
	var doc []byte
	var immediate bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document,
 state='queued' OR lease_until IS NULL OR lease_until<=clock_timestamp()
FROM spiral_evolution_jobs WHERE job_id=$1 AND COALESCE(owner_user_id,'')=$2 FOR UPDATE`, id, owner).Scan(
		&j.OwnerUserID, &j.IdempotencyKey, &j.RequestFingerprint, &doc, &immediate)
	if errors.Is(err, sql.ErrNoRows) {
		return EvolutionJob{}, ErrNotFound
	}
	if err != nil {
		return EvolutionJob{}, err
	}
	if err = json.Unmarshal(doc, &j); err != nil {
		return EvolutionJob{}, err
	}
	if j.State.Terminal() {
		return j, tx.Commit()
	}
	j.CancelRequested = true
	j.UpdatedAt = time.Now().UTC()
	if immediate {
		releaseEvolutionState(&j, "", false, 0, 1, j.UpdatedAt)
	}
	doc, err = json.Marshal(j)
	if err != nil {
		return EvolutionJob{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE spiral_evolution_jobs SET cancel_requested=true,state=$2,document=$3,updated_at=clock_timestamp(),
 fencing_token=fencing_token+CASE WHEN $4 THEN 1 ELSE 0 END,
 worker_id=CASE WHEN $4 THEN '' ELSE worker_id END,lease_until=CASE WHEN $4 THEN NULL ELSE lease_until END
WHERE job_id=$1`, id, j.State, doc, immediate)
	if err != nil {
		return EvolutionJob{}, err
	}
	return j, tx.Commit()
}

func (s *PostgresStore) EvolutionQueueStats(ctx context.Context, owner *string) (EvolutionQueueStats, error) {
	var v EvolutionQueueStats
	var oldest sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE state='queued'),count(*) FILTER (WHERE state='running'),
count(*) FILTER (WHERE state='running' AND (lease_until IS NULL OR lease_until<=clock_timestamp())),
count(*) FILTER (WHERE state IN ('queued','running') AND cancel_requested),
count(*) FILTER (WHERE state='completed'),count(*) FILTER (WHERE state='failed'),count(*) FILTER (WHERE state='cancelled'),
COALESCE(sum(GREATEST(attempt-1,0)),0),min(created_at) FILTER (WHERE state='queued')
FROM spiral_evolution_jobs WHERE $1::text IS NULL OR COALESCE(owner_user_id,'')=$1`, owner).Scan(&v.Queued, &v.Running, &v.Expired, &v.CancelRequested, &v.Completed, &v.Failed, &v.Cancelled, &v.RetryAttempts, &oldest)
	if oldest.Valid {
		v.OldestQueuedAt = &oldest.Time
	}
	return v, err
}
