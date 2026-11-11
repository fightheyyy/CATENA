package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type managedRemote struct {
	ID      string    `json:"provider_id"`
	Sandbox string    `json:"sandbox_id"`
	State   string    `json:"state"`
	Created time.Time `json:"created_at"`
}

func (p *managedPlatform) recoverManagedRuns(ctx context.Context) error {
	// Only durable recovery decisions are made under this short database lock.
	tx, err := managedTx(ctx, p.store.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT run_id,session_id,execution_started_at IS NOT NULL,cancel_requested FROM agent_runs WHERE state='RUNNING' AND (lease_until<=now() OR cancel_requested OR execution_started_at+execution_timeout*interval '1 second'<=now()) FOR UPDATE`)
	if err != nil {
		return err
	}
	type expired struct {
		id, session     string
		started, cancel bool
	}
	expiredRuns := []expired{}
	for rows.Next() {
		var x expired
		if err = rows.Scan(&x.id, &x.session, &x.started, &x.cancel); err != nil {
			rows.Close()
			return err
		}
		expiredRuns = append(expiredRuns, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, x := range expiredRuns {
		detail := "lease expired before execution; retry after sandbox quarantine"
		if x.started {
			detail = "execution outcome uncertain; old sandbox quarantined; no automatic replay"
		}
		if x.cancel {
			detail = "cancelled; in-flight tool effects may have occurred"
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state='STOPPING',error=CASE WHEN error='' THEN $2 ELSE left(error || '; ' || $2,1000) END,fencing_token=fencing_token+1,worker_id='',credential_hash=NULL,lease_until=NULL,next_attempt_at=now()+power(2,LEAST(attempt,5))*interval '1 second',updated_at=now(),finished_at=NULL WHERE run_id=$1`, x.id, detail)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATING',revision=revision+1,error=$2 WHERE session_id=$1 AND state<>'TERMINATED'`, x.session, detail)
		if err != nil {
			return err
		}
		if x.started {
			_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET state='NEEDS_REVIEW',updated_at=now() WHERE session_id=$1`, x.session)
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,fencing_token,kind,data) SELECT run_id,fencing_token,'recovery',jsonb_build_object('state',state,'reason',error) FROM agent_runs WHERE run_id=$1`, x.id)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

type managedBinding struct {
	Box            managedBox
	SessionState   string
	Active         bool
	CreateRecorded bool
}
type managedOperation struct {
	ID        string   `json:"operation_id"`
	Box       string   `json:"sandbox_id"`
	Kind      string   `json:"kind"`
	State     string   `json:"state"`
	Worker    string   `json:"worker_id"`
	Fence     int64    `json:"fencing_token"`
	LeaseLive bool     `json:"lease_live"`
	Revision  int64    `json:"expected_revision"`
	Targets   []string `json:"provider_ids"`
}

func (p *managedPlatform) providerTimeout() time.Duration {
	if p.config.ProviderTimeout > 0 {
		return p.config.ProviderTimeout
	}
	return 10 * time.Second
}
func (p *managedPlatform) operationLease() time.Duration {
	if p.config.OperationLease > 0 {
		return p.config.OperationLease
	}
	return 30 * time.Second
}
func (p *managedPlatform) providerCall(ctx context.Context, action string, data, out any) error {
	callCtx, stop := context.WithTimeout(ctx, p.providerTimeout())
	defer stop()
	return p.call(callCtx, action, data, out)
}
func (p *managedPlatform) managedBindings(ctx context.Context) ([]managedBinding, error) {
	rows, err := p.store.db.QueryContext(ctx, `SELECT to_jsonb(b),s.state,EXISTS(SELECT 1 FROM agent_runs r WHERE r.session_id=b.session_id AND r.state='RUNNING'),EXISTS(SELECT 1 FROM sandbox_operations o WHERE o.sandbox_id=b.sandbox_id AND kind='create') FROM managed_sandboxes b JOIN agent_sessions s USING(session_id) WHERE b.state<>'TERMINATED'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []managedBinding{}
	for rows.Next() {
		var b managedBinding
		var raw []byte
		if err = rows.Scan(&raw, &b.SessionState, &b.Active, &b.CreateRecorded); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &b.Box); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (p *managedPlatform) reconcile(ctx context.Context) error {
	if err := p.recoverManagedRuns(ctx); err != nil {
		return err
	}
	bindings, err := p.managedBindings(ctx)
	if err != nil {
		return err
	}
	var remotes []managedRemote
	// Provider I/O never runs while holding a transaction or connection lock.
	if err = p.providerCall(ctx, "list", map[string]any{"namespace": p.config.Namespace}, &remotes); err != nil {
		return err
	}
	byBox := map[string][]managedRemote{}
	for _, r := range remotes {
		byBox[r.Sandbox] = append(byBox[r.Sandbox], r)
	}
	var problems []error
	for _, b := range bindings {
		op, e := p.planManagedOperation(ctx, b, byBox[b.Box.ID])
		if e != nil {
			problems = append(problems, e)
			continue
		}
		if op != nil {
			if e = p.performManagedOperation(ctx, *op); e != nil {
				problems = append(problems, e)
			}
		}
	}
	for _, r := range remotes {
		op, e := p.planManagedOrphan(ctx, r)
		if e != nil {
			problems = append(problems, e)
			continue
		}
		if op != nil {
			if e = p.performManagedOperation(ctx, *op); e != nil {
				problems = append(problems, e)
			}
		}
	}
	// This statement has no external side effects and cannot race a live binding.
	tx, e := managedTx(ctx, p.store.db)
	if e != nil {
		return errors.Join(append(problems, e)...)
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `UPDATE agent_sessions SET state=CASE WHEN state='DELETING' THEN 'DELETED' ELSE 'PAUSED' END,updated_at=now() WHERE state IN ('DELETING','PAUSING') AND NOT EXISTS(SELECT 1 FROM managed_sandboxes b WHERE b.session_id=agent_sessions.session_id AND b.state<>'TERMINATED')`)
	if e == nil {
		e = tx.Commit()
	}
	return errors.Join(append(problems, e)...)
}
func (p *managedPlatform) planManagedOperation(ctx context.Context, b managedBinding, found []managedRemote) (*managedOperation, error) {
	if b.Active {
		return nil, nil
	}
	tx, err := managedTx(ctx, p.store.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var revision int64
	var sessionState string
	var active, expired, idle, createMayArrive bool
	err = tx.QueryRowContext(ctx, `SELECT b.revision,s.state,EXISTS(SELECT 1 FROM agent_runs r WHERE r.session_id=b.session_id AND r.state='RUNNING'),b.expires_at<=now(),b.last_activity_at+$2*interval '1 millisecond'<=now(),b.expires_at+interval '1 minute'>now() FROM managed_sandboxes b JOIN agent_sessions s USING(session_id) WHERE b.sandbox_id=$1 AND b.state<>'TERMINATED' FOR UPDATE OF b`, b.Box.ID, p.config.Idle.Milliseconds()).Scan(&revision, &sessionState, &active, &expired, &idle, &createMayArrive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revision != b.Box.Revision || sessionState != b.SessionState || active {
		return nil, nil
	}
	var old managedOperation
	err = managedJSON(tx.QueryRowContext(ctx, `SELECT to_jsonb(o)||jsonb_build_object('lease_live',COALESCE(lease_until>now(),false)) FROM sandbox_operations o WHERE sandbox_id=$1 AND kind<>'create' AND state IN ('PENDING','UNKNOWN') ORDER BY updated_at LIMIT 1 FOR UPDATE`, b.Box.ID), &old)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err == nil && old.LeaseLive {
		return nil, nil
	}
	kind := ""
	if err == nil {
		if old.Kind == "terminate" {
			kind = "terminate"
		} else {
			// A timed-out pause/resume can still arrive late. Never reuse this instance.
			if _, err = tx.ExecContext(ctx, `UPDATE sandbox_operations SET state='ABANDONED',error='operation outcome uncertain; quarantine instance',lease_until=NULL WHERE operation_id=$1`, old.ID); err != nil {
				return nil, err
			}
			kind = "terminate"
		}
	}
	terminate := kind == "terminate" || b.Box.State == "TERMINATING" || sessionState == "DELETING" || sessionState == "NEEDS_REVIEW" || expired || idle || len(found) == 0 || len(found) > 1
	if len(found) == 1 && found[0].State != "Running" && found[0].State != "RUNNING" && found[0].State != "Paused" && found[0].State != "PAUSED" {
		terminate = true
	}
	if b.CreateRecorded && b.Box.Provider == "" && len(found) == 0 && len(old.Targets) == 0 && createMayArrive {
		_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATING',error='create outcome unknown; capacity retained until provider TTL plus grace',revision=revision+1 WHERE sandbox_id=$1`, b.Box.ID)
		if err == nil {
			err = tx.Commit()
		}
		return nil, err
	}
	if terminate {
		kind = "terminate"
	} else if sessionState == "PAUSING" {
		kind = "pause"
	} else if sessionState == "READY" && (b.Box.State == "PAUSED" || found[0].State == "Paused" || found[0].State == "PAUSED") {
		kind = "resume"
	}
	if kind == "" {
		return nil, nil
	}
	targets := []string{}
	seen := map[string]bool{}
	for _, id := range old.Targets {
		if !seen[id] {
			targets = append(targets, id)
			seen[id] = true
		}
	}
	for _, r := range found {
		if !seen[r.ID] {
			targets = append(targets, r.ID)
			seen[r.ID] = true
		}
	}
	if len(targets) == 0 && b.Box.Provider != "" {
		targets = append(targets, b.Box.Provider)
	}
	raw, err := json.Marshal(targets)
	if err != nil {
		return nil, err
	}
	opID := newID("operation")
	if old.ID != "" && old.Kind == "terminate" {
		opID = old.ID
	}
	_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET revision=revision+1,state=CASE WHEN $2='terminate' THEN 'TERMINATING' ELSE state END WHERE sandbox_id=$1`, b.Box.ID, kind)
	if err != nil {
		return nil, err
	}
	var op managedOperation
	err = managedJSON(tx.QueryRowContext(ctx, `INSERT INTO sandbox_operations(operation_id,sandbox_id,kind,state,attempts,worker_id,fencing_token,lease_until,expected_revision,provider_ids) VALUES($1,$2,$3,'PENDING',1,$4,1,now()+$5*interval '1 millisecond',$6,$7) ON CONFLICT(operation_id) DO UPDATE SET state='PENDING',attempts=sandbox_operations.attempts+1,worker_id=$4,fencing_token=sandbox_operations.fencing_token+1,lease_until=now()+$5*interval '1 millisecond',expected_revision=$6,provider_ids=$7,updated_at=now() RETURNING to_jsonb(sandbox_operations)`, opID, b.Box.ID, kind, newID("reconciler"), p.operationLease().Milliseconds(), revision+1, raw), &op)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}
func (p *managedPlatform) planManagedOrphan(ctx context.Context, r managedRemote) (*managedOperation, error) {
	tx, err := managedTx(ctx, p.store.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var eligible bool
	if err = tx.QueryRowContext(ctx, `SELECT $3::timestamptz<=now()-interval '1 minute' AND NOT EXISTS(SELECT 1 FROM managed_sandboxes WHERE sandbox_id=$1 OR provider_sandbox_id=$2)`, r.Sandbox, r.ID, r.Created).Scan(&eligible); err != nil {
		return nil, err
	}
	if !eligible {
		return nil, nil
	}
	raw, _ := json.Marshal([]string{r.ID})
	var op managedOperation
	err = managedJSON(tx.QueryRowContext(ctx, `INSERT INTO sandbox_operations(operation_id,kind,state,attempts,worker_id,fencing_token,lease_until,provider_ids) VALUES($1,'terminate','PENDING',1,$2,1,now()+$3*interval '1 millisecond',$4) ON CONFLICT(operation_id) DO UPDATE SET state='PENDING',attempts=sandbox_operations.attempts+1,worker_id=$2,fencing_token=sandbox_operations.fencing_token+1,lease_until=now()+$3*interval '1 millisecond',updated_at=now() WHERE sandbox_operations.lease_until IS NULL OR sandbox_operations.lease_until<=now() RETURNING to_jsonb(sandbox_operations)`, "orphan-"+r.ID, newID("reconciler"), p.operationLease().Milliseconds(), raw), &op)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}
func (p *managedPlatform) performManagedOperation(ctx context.Context, op managedOperation) error {
	for _, id := range op.Targets {
		var out map[string]any
		if err := p.providerCall(ctx, op.Kind, map[string]any{"provider_id": id}, &out); err != nil {
			recordCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer stop()
			// Unknown results keep the reservation and block session reuse.
			e := managedCAS(p.store.db.ExecContext(recordCtx, `UPDATE sandbox_operations SET state='UNKNOWN',error=$4,worker_id='',lease_until=NULL,updated_at=now() WHERE operation_id=$1 AND worker_id=$2 AND fencing_token=$3 AND lease_until>now() AND state='PENDING'`, op.ID, op.Worker, op.Fence, bounded(err.Error(), 1000)))
			return errors.Join(err, e)
		}
	}
	return p.finalizeManagedOperation(ctx, op)
}
func (p *managedPlatform) finalizeManagedOperation(ctx context.Context, op managedOperation) error {
	tx, err := managedTx(ctx, p.store.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sandbox_operations WHERE operation_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='PENDING' AND lease_until>now())`, op.ID, op.Worker, op.Fence).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrLeaseLost
	}
	if op.Box != "" {
		state := "IDLE"
		if op.Kind == "pause" {
			state = "PAUSED"
		}
		if op.Kind == "terminate" {
			state = "TERMINATED"
		}
		err = managedCAS(tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state=$3,revision=revision+1,error='',last_activity_at=now() WHERE sandbox_id=$1 AND revision=$2 AND NOT EXISTS(SELECT 1 FROM agent_runs WHERE session_id=managed_sandboxes.session_id AND state='RUNNING')`, op.Box, op.Revision, state))
		if err != nil {
			return err
		}
		if op.Kind == "terminate" {
			_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state=CASE WHEN cancel_requested THEN 'CANCELLED' WHEN execution_started_at IS NOT NULL THEN 'NEEDS_REVIEW' WHEN attempt>=3 THEN 'FAILED' ELSE 'QUEUED' END,finished_at=CASE WHEN cancel_requested OR execution_started_at IS NOT NULL OR attempt>=3 THEN now() ELSE NULL END,isolation_confirmed_at=now(),updated_at=now() WHERE sandbox_id=$1 AND state='STOPPING'`, op.Box)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,fencing_token,kind,data) SELECT run_id,fencing_token,'execution.isolated',jsonb_build_object('state',state,'sandbox_id',sandbox_id) FROM agent_runs WHERE sandbox_id=$1 AND isolation_confirmed_at=now()`, op.Box)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE resource_reservations SET released_at=now() WHERE sandbox_id=$1 AND released_at IS NULL`, op.Box); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET sandbox_binding=NULL,state=CASE WHEN state='DELETING' THEN 'DELETED' WHEN state='PAUSING' THEN 'PAUSED' ELSE state END,updated_at=now() WHERE session_id=(SELECT session_id FROM managed_sandboxes WHERE sandbox_id=$1)`, op.Box)
		} else if op.Kind == "pause" {
			_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET state='PAUSED',updated_at=now() WHERE state='PAUSING' AND session_id=(SELECT session_id FROM managed_sandboxes WHERE sandbox_id=$1)`, op.Box)
		}
		if err != nil {
			return err
		}
	}
	if err = managedCAS(tx.ExecContext(ctx, `UPDATE sandbox_operations SET state='SUCCEEDED',error='',worker_id='',lease_until=NULL,updated_at=now() WHERE operation_id=$1 AND worker_id=$2 AND fencing_token=$3 AND lease_until>now() AND state='PENDING'`, op.ID, op.Worker, op.Fence)); err != nil {
		return err
	}
	return tx.Commit()
}
