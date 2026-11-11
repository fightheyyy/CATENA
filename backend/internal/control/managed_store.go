package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const managedLock = 1943287104

type managedAgent struct {
	Skills         []harnessSkill `json:"skills"`
	Knowledge      []string       `json:"knowledge_refs"`
	Profile        string         `json:"profile_kind"`
	ID             string         `json:"agent_id"`
	Tenant         string         `json:"tenant_id"`
	Version        int            `json:"version"`
	Runtime        string         `json:"runtime_type"`
	RuntimeVersion string         `json:"runtime_version"`
	ModelRef       string         `json:"model_config_ref"`
	Instructions   string         `json:"instructions"`
	ToolPolicy     string         `json:"tool_policy"`
	CPU            string         `json:"cpu"`
	Memory         string         `json:"memory"`
	Timeout        int            `json:"timeout_seconds"`
}
type managedRun struct {
	ID       string     `json:"run_id"`
	Sandbox  string     `json:"sandbox_id"`
	Session  string     `json:"session_id"`
	Tenant   string     `json:"tenant_id"`
	State    string     `json:"state"`
	Input    string     `json:"input"`
	Attempt  int        `json:"attempt"`
	Worker   string     `json:"worker_id"`
	Fence    int64      `json:"fencing_token"`
	Timeout  int        `json:"execution_timeout"`
	Trace    string     `json:"trace_id"`
	Result   string     `json:"result"`
	Error    string     `json:"error"`
	Created  time.Time  `json:"created_at"`
	Claimed  *time.Time `json:"claimed_at"`
	Started  *time.Time `json:"execution_started_at"`
	Finished *time.Time `json:"finished_at"`
}
type managedBox struct {
	Revision   int64  `json:"revision"`
	ID         string `json:"sandbox_id"`
	Provider   string `json:"provider_sandbox_id"`
	Tenant     string `json:"tenant_id"`
	Session    string `json:"session_id"`
	State      string `json:"state"`
	Generation int64  `json:"generation"`
	CPU        string `json:"cpu"`
	Memory     string `json:"memory"`
}
type managedLease struct {
	Ephemeral  bool
	Run        managedRun
	Agent      managedAgent
	Box        managedBox
	Token      string
	Checkpoint []byte
}

func managedJSON(row *sql.Row, out any) error {
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(raw, out)
}
func managedRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var v json.RawMessage
		if err = rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func managedTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, managedLock); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
func managedCAS(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (s *PostgresStore) submitManaged(ctx context.Context, tenant, session, input, key string) (managedRun, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return managedRun{}, false, err
	}
	defer tx.Rollback()
	var limit int
	if err = tx.QueryRowContext(ctx, `SELECT max_queued FROM tenant_quotas WHERE tenant_id=$1 FOR UPDATE`, tenant).Scan(&limit); err != nil {
		return managedRun{}, false, ErrNotFound
	}
	var run managedRun
	err = managedJSON(tx.QueryRowContext(ctx, `SELECT to_jsonb(r)-'credential_hash' FROM agent_runs r WHERE tenant_id=$1 AND session_id=$2 AND idempotency_key=$3 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=r.session_id AND s.workflow_job_id IS NULL)`, tenant, session, key), &run)
	if err == nil {
		if run.Input != input {
			return run, false, ErrConflict
		}
		return run, false, tx.Commit()
	}
	if !errors.Is(err, ErrNotFound) {
		return run, false, err
	}
	var timeout, queued int
	if err = tx.QueryRowContext(ctx, `SELECT a.timeout_seconds FROM agent_sessions s JOIN managed_agents a USING(agent_id) WHERE s.tenant_id=$1 AND s.session_id=$2 AND s.workflow_job_id IS NULL AND s.state='READY' FOR SHARE OF s`, tenant, session).Scan(&timeout); err != nil {
		return run, false, ErrNotFound
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM agent_runs r JOIN agent_sessions s USING(session_id) WHERE r.tenant_id=$1 AND r.state='QUEUED' AND s.workflow_job_id IS NULL`, tenant).Scan(&queued); err != nil {
		return run, false, err
	}
	if queued >= limit {
		return run, false, ErrQueueFull
	}
	token, err := randomURLToken(16)
	if err != nil {
		return run, false, err
	}
	trace := sha256.Sum256([]byte(token))
	traceID := hex.EncodeToString(trace[:16])
	err = managedJSON(tx.QueryRowContext(ctx, `INSERT INTO agent_runs(run_id,session_id,tenant_id,input,execution_timeout,idempotency_key,trace_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING to_jsonb(agent_runs)-'credential_hash'`, newID("run"), session, tenant, input, timeout, key, traceID), &run)
	if err != nil {
		return run, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,fencing_token,kind,data) VALUES($1,0,'queued','{}')`, run.ID); err != nil {
		return run, false, err
	}
	return run, true, tx.Commit()
}

func (s *PostgresStore) claimManaged(ctx context.Context, worker string, ttl time.Duration, capacity int) (*managedLease, error) {
	return s.claimHarness(ctx, worker, ttl, capacity, false)
}

func (s *PostgresStore) claimHarness(ctx context.Context, worker string, ttl time.Duration, capacity int, ephemeral bool) (*managedLease, error) {
	tx, err := managedTx(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	l := &managedLease{Ephemeral: ephemeral}
	// Tenant rotation is durable, and the partial unique index independently enforces session exclusion.
	err = managedJSON(tx.QueryRowContext(ctx, `SELECT to_jsonb(r)-'credential_hash' FROM agent_runs r
 JOIN agent_sessions s USING(session_id) JOIN tenant_quotas q ON q.tenant_id=r.tenant_id
 WHERE (s.workflow_job_id IS NOT NULL)=$2
 AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=s.session_id)
 AND NOT EXISTS(SELECT 1 FROM sandbox_operations o JOIN managed_sandboxes b ON b.sandbox_id=o.sandbox_id WHERE b.session_id=r.session_id AND o.kind<>'create' AND o.state IN ('PENDING','UNKNOWN')) AND r.state='QUEUED' AND r.next_attempt_at<=now() AND s.state='READY'
 AND NOT EXISTS(SELECT 1 FROM agent_runs x WHERE x.session_id=r.session_id AND x.state IN ('RUNNING','STOPPING'))
 AND ($2 OR (SELECT count(*) FROM agent_runs x JOIN agent_sessions sx USING(session_id) WHERE x.tenant_id=r.tenant_id AND sx.workflow_job_id IS NULL AND x.state IN ('RUNNING','STOPPING'))<q.max_running)
 AND (EXISTS(SELECT 1 FROM managed_sandboxes b WHERE b.session_id=r.session_id AND b.state='IDLE') OR
 (NOT EXISTS(SELECT 1 FROM managed_sandboxes b WHERE b.session_id=r.session_id AND b.state<>'TERMINATED')
 AND (SELECT count(*) FROM resource_reservations z JOIN managed_sandboxes b USING(sandbox_id) JOIN agent_sessions bs USING(session_id) WHERE z.released_at IS NULL AND (bs.workflow_job_id IS NOT NULL)=$2)<$1
 AND ($2 OR (SELECT count(*) FROM resource_reservations z JOIN managed_sandboxes b USING(sandbox_id) JOIN agent_sessions bs USING(session_id) WHERE z.tenant_id=r.tenant_id AND bs.workflow_job_id IS NULL AND z.released_at IS NULL)<q.max_sandboxes)))
 ORDER BY q.last_scheduled_at,r.created_at,r.run_id FOR UPDATE OF r SKIP LOCKED LIMIT 1`, capacity, ephemeral), &l.Run)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = managedJSON(tx.QueryRowContext(ctx, `SELECT to_jsonb(a) FROM managed_agents a JOIN agent_sessions s USING(agent_id) WHERE s.session_id=$1`, l.Run.Session), &l.Agent); err != nil {
		return nil, err
	}
	err = managedJSON(tx.QueryRowContext(ctx, `SELECT to_jsonb(b) FROM managed_sandboxes b WHERE session_id=$1 AND state='IDLE' FOR UPDATE`, l.Run.Session), &l.Box)
	if errors.Is(err, ErrNotFound) {
		l.Box = managedBox{ID: newID("sandbox"), Tenant: l.Run.Tenant, Session: l.Run.Session, State: "PENDING", Generation: 1, CPU: l.Agent.CPU, Memory: l.Agent.Memory}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(generation),0)+1 FROM managed_sandboxes WHERE session_id=$1`, l.Run.Session).Scan(&l.Box.Generation); err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO managed_sandboxes(sandbox_id,tenant_id,session_id,state,generation,cpu,memory,expires_at) VALUES($1,$2,$3,'PENDING',$6,$4,$5,now()+interval '1 hour')`, l.Box.ID, l.Box.Tenant, l.Box.Session, l.Box.CPU, l.Box.Memory, l.Box.Generation)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO resource_reservations(sandbox_id,tenant_id) VALUES($1,$2)`, l.Box.ID, l.Box.Tenant)
		}
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='BUSY',revision=revision+1,last_activity_at=now() WHERE sandbox_id=$1`, l.Box.ID)
	}
	if err != nil {
		return nil, err
	}
	l.Token, err = randomURLToken(32)
	if err != nil {
		return nil, err
	}
	err = managedJSON(tx.QueryRowContext(ctx, `UPDATE agent_runs SET state='RUNNING',sandbox_id=$5,claimed_at=now(),worker_id=$2,fencing_token=fencing_token+1,attempt=attempt+1,lease_until=now()+$3*interval '1 millisecond',credential_hash=$4,updated_at=now() WHERE run_id=$1 RETURNING to_jsonb(agent_runs)-'credential_hash'`, l.Run.ID, worker, ttl.Milliseconds(), sessionTokenHash(l.Token), l.Box.ID), &l.Run)
	if err != nil {
		return nil, err
	}
	if !ephemeral {
		if _, err = tx.ExecContext(ctx, `UPDATE tenant_quotas SET last_scheduled_at=now() WHERE tenant_id=$1`, l.Run.Tenant); err != nil {
			return nil, err
		}
	}
	var checkpoint []byte
	var checkpointHash string
	err = tx.QueryRowContext(ctx, `SELECT c.snapshot,c.sha256 FROM managed_checkpoints c JOIN agent_sessions s ON s.checkpoint_ref=c.checkpoint_id WHERE s.session_id=$1`, l.Run.Session).Scan(&checkpoint, &checkpointHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if len(checkpoint) > 0 {
		sum := sha256.Sum256(checkpoint)
		if hex.EncodeToString(sum[:]) != checkpointHash {
			return nil, errors.New("checkpoint integrity verification failed")
		}
	}
	l.Checkpoint = checkpoint
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return l, nil
}
func (s *PostgresStore) heartbeatManaged(ctx context.Context, l *managedLease, ttl time.Duration) error {
	return managedCAS(s.db.ExecContext(ctx, `UPDATE agent_runs SET lease_until=now()+$4*interval '1 millisecond',updated_at=now() WHERE run_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=agent_runs.session_id) AND (execution_started_at IS NULL OR execution_started_at+execution_timeout*interval '1 second'>now())`, l.Run.ID, l.Run.Worker, l.Run.Fence, ttl.Milliseconds()))
}
func (s *PostgresStore) managedEvent(ctx context.Context, l *managedLease, kind string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if len(raw) > 64*1024 {
		return fmt.Errorf("managed event too large")
	}
	return managedCAS(s.db.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,fencing_token,kind,data)
 SELECT run_id,fencing_token,$4,$5 FROM agent_runs WHERE run_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=agent_runs.session_id)
 AND (SELECT count(*) FROM agent_run_events WHERE run_id=$1)<10000`, l.Run.ID, l.Run.Worker, l.Run.Fence, kind, raw))
}
func (s *PostgresStore) finishManaged(ctx context.Context, l *managedLease, state, result, detail string, snapshot []byte) error {
	tx, err := managedTx(ctx, s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !l.Ephemeral {
		result = bounded(redactEvidenceText(result), 128*1024)
	}
	err = managedCAS(tx.ExecContext(ctx, `UPDATE agent_runs SET state=$4,result=$5,error=$6,finished_at=now(),updated_at=now(),credential_hash=NULL,lease_until=NULL WHERE run_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=agent_runs.session_id) AND (execution_started_at IS NULL OR command_exited_at IS NOT NULL)`, l.Run.ID, l.Run.Worker, l.Run.Fence, state, result, bounded(redactEvidenceText(detail), 1000)))
	if err != nil {
		return err
	}
	if len(snapshot) > 0 && len(snapshot) <= 16*1024*1024 {
		id := newID("checkpoint")
		hash := sha256.Sum256(snapshot)
		_, err = tx.ExecContext(ctx, `INSERT INTO managed_checkpoints(checkpoint_id,session_id,run_id,agent_id,agent_version,sha256,size_bytes,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, l.Run.Session, l.Run.ID, l.Agent.ID, l.Agent.Version, hex.EncodeToString(hash[:]), len(snapshot), snapshot)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET checkpoint_ref=$2,updated_at=now() WHERE session_id=$1`, l.Run.Session, id)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET result_ref=$2 WHERE run_id=$1`, l.Run.ID, id)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state=CASE WHEN $2 THEN 'TERMINATING' ELSE 'IDLE' END,revision=revision+1,last_activity_at=now() WHERE sandbox_id=$1 AND state IN ('BUSY','READY')`, l.Box.ID, l.Ephemeral)
	if err != nil {
		return err
	}
	if l.Ephemeral {
		if _, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET state='DELETING',updated_at=now() WHERE session_id=$1`, l.Run.Session); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,fencing_token,kind,data) VALUES($1,$2,'finished',jsonb_build_object('state',$3::text))`, l.Run.ID, l.Run.Fence, state)
	if err != nil {
		return err
	}
	return tx.Commit()
}
