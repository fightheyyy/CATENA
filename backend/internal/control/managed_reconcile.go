package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type managedRemote struct {
	ID      string    `json:"provider_id"`
	Sandbox string    `json:"sandbox_id"`
	State   string    `json:"state"`
	Created time.Time `json:"created_at"`
}

func (p *managedPlatform) reconcile(ctx context.Context) error {
	// Serialize controllers across processes, including external lifecycle calls.
	tx, err := managedTx(ctx, p.store.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT run_id,session_id,execution_started_at IS NOT NULL,cancel_requested,attempt FROM agent_runs WHERE state='RUNNING' AND (lease_until<=now() OR cancel_requested OR execution_started_at+execution_timeout*interval '1 second'<=now()) FOR UPDATE`)
	if err != nil {
		return err
	}
	type expired struct {
		id, session     string
		started, cancel bool
		attempt         int
	}
	expiredRuns := []expired{}
	for rows.Next() {
		var x expired
		if err = rows.Scan(&x.id, &x.session, &x.started, &x.cancel, &x.attempt); err != nil {
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
		state := "QUEUED"
		detail := "lease expired before execution; retry after sandbox quarantine"
		if x.started {
			state = "NEEDS_REVIEW"
			detail = "execution outcome uncertain; old sandbox quarantined; no automatic replay"
		}
		if x.attempt >= 3 && !x.started {
			state = "FAILED"
		}
		if x.cancel {
			state = "CANCELLED"
			detail = "cancelled; in-flight tool effects may have occurred"
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state=$2,error=$3,fencing_token=fencing_token+1,worker_id='',credential_hash=NULL,lease_until=NULL,next_attempt_at=now()+power(2,LEAST(attempt,5))*interval '1 second',updated_at=now(),finished_at=CASE WHEN $2='QUEUED' THEN NULL ELSE now() END WHERE run_id=$1`, x.id, state, detail)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATING',error=$2 WHERE session_id=$1 AND state<>'TERMINATED'`, x.session, detail)
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
	// Recovery decisions are committed before network calls; revoked capabilities stay revoked on provider failure.
	tx, err = managedTx(ctx, p.store.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var remotes []managedRemote
	if err = p.call(ctx, "list", map[string]any{"namespace": p.config.Namespace}, &remotes); err != nil {
		return err
	}
	byBox := map[string][]managedRemote{}
	for _, remote := range remotes {
		byBox[remote.Sandbox] = append(byBox[remote.Sandbox], remote)
	}
	rows, err = tx.QueryContext(ctx, `SELECT to_jsonb(b),s.state,EXISTS(SELECT 1 FROM agent_runs r WHERE r.session_id=b.session_id AND r.state='RUNNING' AND r.lease_until>now()),b.last_activity_at,b.expires_at FROM managed_sandboxes b JOIN agent_sessions s USING(session_id) WHERE b.state<>'TERMINATED' FOR UPDATE OF b`)
	if err != nil {
		return err
	}
	type binding struct {
		box           managedBox
		sessionState  string
		active        bool
		last, expires time.Time
	}
	bindings := []binding{}
	known := map[string]bool{}
	for rows.Next() {
		var raw []byte
		var b binding
		if err = rows.Scan(&raw, &b.sessionState, &b.active, &b.last, &b.expires); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &b.box); err != nil {
			rows.Close()
			return err
		}
		bindings = append(bindings, b)
		known[b.box.ID] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, b := range bindings {
		box := b.box
		found := byBox[box.ID]
		if b.active {
			continue
		}
		terminate := box.State == "TERMINATING" || b.sessionState == "DELETING" || b.sessionState == "NEEDS_REVIEW" || time.Now().After(b.expires) || time.Since(b.last) > p.config.Idle
		if len(found) == 1 && found[0].State != "Running" && found[0].State != "RUNNING" && found[0].State != "Paused" && found[0].State != "PAUSED" {
			terminate = true
		}
		if len(found) > 1 {
			terminate = true
		}
		var createRecorded bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sandbox_operations WHERE sandbox_id=$1 AND kind='create')`, box.ID).Scan(&createRecorded); err != nil {
			return err
		}
		if createRecorded && box.Provider == "" && (box.State == "CREATING" || box.State == "UNKNOWN" || box.State == "TERMINATING") && len(found) == 0 && time.Now().Before(b.expires.Add(time.Minute)) {
			// shortcut: provider lacks create-idempotency keys, retain reservation until max TTL + grace resolves unknown create outcomes.
			_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATING',error='create outcome unknown; capacity retained until provider TTL plus grace' WHERE sandbox_id=$1`, box.ID)
			if err != nil {
				return err
			}
			continue
		}
		if len(found) == 0 {
			terminate = true
		}
		action, targetBox := "", ""
		if !terminate && b.sessionState == "PAUSING" {
			action, targetBox = "pause", "PAUSED"
		}
		if !terminate && b.sessionState == "READY" && box.State == "PAUSED" {
			action, targetBox = "resume", "IDLE"
		}
		if action != "" {
			opID := box.ID + "-" + action
			_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_operations(operation_id,sandbox_id,kind,state,attempts) VALUES($1,$2,$3,'PENDING',1) ON CONFLICT(operation_id) DO UPDATE SET state='PENDING',attempts=sandbox_operations.attempts+1,updated_at=now()`, opID, box.ID, action)
			if err != nil {
				return err
			}
			already := action == "pause" && (found[0].State == "Paused" || found[0].State == "PAUSED") || action == "resume" && (found[0].State == "Running" || found[0].State == "RUNNING")
			if !already {
				var out map[string]any
				if e := p.call(ctx, action, map[string]any{"provider_id": found[0].ID}, &out); e != nil {
					_, err = tx.ExecContext(ctx, `UPDATE sandbox_operations SET state='UNKNOWN',error=$2 WHERE operation_id=$1`, opID, e.Error())
					if err != nil {
						return err
					}
					continue
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state=$2,last_activity_at=now() WHERE sandbox_id=$1`, box.ID, targetBox)
			if err != nil {
				return err
			}
			if action == "pause" {
				_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET state='PAUSED',updated_at=now() WHERE session_id=$1`, box.Session)
				if err != nil {
					return err
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE sandbox_operations SET state='SUCCEEDED',error='',updated_at=now() WHERE operation_id=$1`, opID)
			if err != nil {
				return err
			}
			continue
		}

		if !terminate {
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_operations(operation_id,sandbox_id,kind,state,attempts) VALUES($1,$2,'terminate','PENDING',1) ON CONFLICT(operation_id) DO UPDATE SET attempts=sandbox_operations.attempts+1,updated_at=now()`, box.ID+"-terminate", box.ID)
		if err != nil {
			return err
		}
		failed := false
		for _, remote := range found {
			var out map[string]any
			if e := p.call(ctx, "terminate", map[string]any{"provider_id": remote.ID}, &out); e != nil {
				failed = true
				_, err = tx.ExecContext(ctx, `UPDATE sandbox_operations SET state='UNKNOWN',error=$2 WHERE operation_id=$1`, box.ID+"-terminate", e.Error())
				if err != nil {
					return err
				}
			}
		}
		if failed {
			_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATING',error='provider termination unconfirmed; retrying' WHERE sandbox_id=$1`, box.ID)
			if err != nil {
				return err
			}
			continue
		}
		_, err = tx.ExecContext(ctx, `UPDATE managed_sandboxes SET state='TERMINATED',last_activity_at=now(),error='' WHERE sandbox_id=$1`, box.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE resource_reservations SET released_at=now() WHERE sandbox_id=$1 AND released_at IS NULL`, box.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE sandbox_operations SET state='SUCCEEDED',error='',updated_at=now() WHERE operation_id=$1`, box.ID+"-terminate")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET sandbox_binding=NULL,state=CASE WHEN state='DELETING' THEN 'DELETED' WHEN state='PAUSING' THEN 'PAUSED' ELSE state END,updated_at=now() WHERE session_id=$1`, box.Session)
		if err != nil {
			return err
		}
	}
	// Only our persisted namespace is considered; never touch unrelated provider resources.
	for _, remote := range remotes {
		if !known[remote.Sandbox] && time.Since(remote.Created) > time.Minute {
			var out map[string]any
			if e := p.call(ctx, "terminate", map[string]any{"provider_id": remote.ID}, &out); e != nil {
				return fmt.Errorf("orphan cleanup: %w", e)
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_sessions SET state=CASE WHEN state='DELETING' THEN 'DELETED' ELSE 'PAUSED' END,updated_at=now() WHERE state IN ('DELETING','PAUSING') AND NOT EXISTS(SELECT 1 FROM managed_sandboxes b WHERE b.session_id=agent_sessions.session_id AND b.state<>'TERMINATED')`)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return p.exportManagedTraces(ctx)
}
func (p *managedPlatform) exportManagedTraces(ctx context.Context) error {
	if p.server.traceSink == nil {
		return nil
	}
	rows, err := p.store.db.QueryContext(ctx, `SELECT to_jsonb(r)-'credential_hash',s.agent_id,COALESCE(r.sandbox_id,'') FROM agent_runs r JOIN agent_sessions s USING(session_id) WHERE r.state IN ('COMPLETED','FAILED','CANCELLED','NEEDS_REVIEW') AND NOT r.trace_exported ORDER BY r.created_at LIMIT 20`)
	if err != nil {
		return err
	}
	type item struct {
		r          managedRun
		agent, box string
	}
	items := []item{}
	for rows.Next() {
		var v item
		var raw []byte
		if err = rows.Scan(&raw, &v.agent, &v.box); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &v.r); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		end := time.Now()
		if v.r.Finished != nil {
			end = *v.r.Finished
		}
		status := int32(1)
		if v.r.State != "COMPLETED" {
			status = 2
		}
		span := TraceSpan{AgentID: v.agent, TraceID: v.r.Trace, SpanID: v.r.Trace[:16], Name: "managed.agent.run", ServiceName: "catena-managed", ScopeName: "catena-managed", StartTime: v.r.Created, EndTime: end, StatusCode: status, StatusMessage: v.r.Error, Input: bounded(redactEvidenceText(v.r.Input), 8192), Output: bounded(redactEvidenceText(v.r.Result), 8192), Attributes: map[string]any{"catena.session.id": v.r.Session, "catena.run.id": v.r.ID, "catena.sandbox.id": v.box, "catena.attempt": v.r.Attempt, "catena.run.state": v.r.State}, ResourceAttributes: map[string]any{"service.name": "catena-managed", "agent.runtime": "codex", "catena.agent.id": v.agent, "catena.owner.id": v.r.Tenant}}
		if v.r.Claimed != nil {
			span.Attributes["catena.queue.wait_ms"] = v.r.Claimed.Sub(v.r.Created).Milliseconds()
		}
		if v.r.Started != nil && v.r.Finished != nil {
			span.Attributes["catena.execution.duration_ms"] = v.r.Finished.Sub(*v.r.Started).Milliseconds()
		}
		events, err := managedRows(ctx, p.store.db, `SELECT jsonb_build_object('kind',kind,'time',created_at,'data',data) FROM agent_run_events WHERE run_id=$1 ORDER BY event_id LIMIT 200`, v.r.ID)
		if err != nil {
			return err
		}
		for _, raw := range events {
			var ev struct {
				Kind string         `json:"kind"`
				Time time.Time      `json:"time"`
				Data map[string]any `json:"data"`
			}
			if json.Unmarshal(raw, &ev) == nil {
				span.Events = append(span.Events, TraceEvent{Name: ev.Kind, Time: ev.Time, Attributes: ev.Data})
			}
		}
		spans := []TraceSpan{span}
		modelRows, e := p.store.db.QueryContext(ctx, `SELECT request_id,created_at,status,latency_ms,input_tokens,output_tokens FROM managed_model_requests WHERE run_id=$1 ORDER BY request_id`, v.r.ID)
		if e != nil {
			return e
		}
		for modelRows.Next() {
			var id int64
			var at time.Time
			var status, latency, input, output sql.NullInt64
			if e = modelRows.Scan(&id, &at, &status, &latency, &input, &output); e != nil {
				modelRows.Close()
				return e
			}
			attrs := map[string]any{"catena.run.id": v.r.ID, "catena.session.id": v.r.Session, "catena.sandbox.id": v.box, "catena.attempt": v.r.Attempt}
			if input.Valid {
				attrs["gen_ai.usage.input_tokens"] = input.Int64
			}
			if output.Valid {
				attrs["gen_ai.usage.output_tokens"] = output.Int64
			}
			if status.Valid {
				attrs["http.response.status_code"] = status.Int64
			}
			code := int32(1)
			if !status.Valid || status.Int64 >= 400 {
				code = 2
			}
			spans = append(spans, TraceSpan{AgentID: v.agent, TraceID: v.r.Trace, SpanID: sessionTokenHash(fmt.Sprintf("%s/model/%d", v.r.ID, id))[:16], ParentSpanID: span.SpanID, Name: "managed.llm.request", ServiceName: span.ServiceName, ScopeName: span.ScopeName, StartTime: at, EndTime: at.Add(time.Duration(latency.Int64) * time.Millisecond), StatusCode: code, Attributes: attrs, ResourceAttributes: span.ResourceAttributes})
		}
		e = modelRows.Err()
		modelRows.Close()
		if e != nil {
			return e
		}
		toolStarts := map[string]time.Time{}
		for _, ev := range span.Events {
			data := ev.Attributes
			if legacy, ok := data["event"].(string); ok {
				_ = json.Unmarshal([]byte(legacy), &data)
			}
			item, ok := data["item"].(map[string]any)
			if !ok || item["type"] != "command_execution" {
				continue
			}
			id, _ := item["id"].(string)
			if data["type"] == "item.started" {
				toolStarts[id] = ev.Time
				continue
			}
			if data["type"] != "item.completed" {
				continue
			}
			at, ok := toolStarts[id]
			if !ok {
				at = ev.Time
			}
			code := int32(1)
			if item["status"] == "failed" {
				code = 2
			}
			command, _ := item["command"].(string)
			output, _ := item["aggregated_output"].(string)
			spans = append(spans, TraceSpan{AgentID: v.agent, TraceID: v.r.Trace, SpanID: sessionTokenHash(v.r.ID + "/tool/" + id)[:16], ParentSpanID: span.SpanID, Name: "managed.tool.shell", ServiceName: span.ServiceName, ScopeName: span.ScopeName, StartTime: at, EndTime: ev.Time, StatusCode: code, Input: bounded(redactEvidenceText(command), 4096), Output: bounded(redactEvidenceText(output), 8192), Attributes: map[string]any{"catena.run.id": v.r.ID, "catena.session.id": v.r.Session, "catena.sandbox.id": v.box, "tool.exit_code": item["exit_code"]}, ResourceAttributes: span.ResourceAttributes})
		}
		if err = p.server.traceSink.InsertSpans(ctx, v.r.Tenant, spans); err != nil {
			return err
		}
		_, err = p.store.db.ExecContext(ctx, `UPDATE agent_runs SET trace_exported=true WHERE run_id=$1`, v.r.ID)
		if err != nil {
			return err
		}
	}
	return nil
}
