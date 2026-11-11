package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const managedTracePageSize = 100

type managedTraceProgress struct {
	Base          bool
	Cursor, Upper int64
	Starts        map[string]time.Time
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
		Run        managedRun
		Agent, Box string
	}
	items := []item{}
	for rows.Next() {
		var v item
		var raw []byte
		if err = rows.Scan(&raw, &v.Agent, &v.Box); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &v.Run); err != nil {
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
		if err = p.exportManagedTrace(ctx, v.Run, v.Agent, v.Box); err != nil {
			return err
		}
	}
	return nil
}
func (p *managedPlatform) exportManagedTrace(ctx context.Context, run managedRun, agent, box string) error {
	var progress managedTraceProgress
	var starts []byte
	err := p.store.db.QueryRowContext(ctx, `UPDATE agent_runs SET trace_event_upper=COALESCE(trace_event_upper,(SELECT COALESCE(max(event_id),0) FROM agent_run_events WHERE run_id=$1)) WHERE run_id=$1 AND NOT trace_exported RETURNING trace_base_exported,trace_event_cursor,trace_event_upper,trace_tool_starts`, run.ID).Scan(&progress.Base, &progress.Cursor, &progress.Upper, &starts)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(starts, &progress.Starts); err != nil {
		return err
	}
	if progress.Starts == nil {
		progress.Starts = map[string]time.Time{}
	}
	end := time.Now()
	if run.Finished != nil {
		end = *run.Finished
	}
	status := int32(1)
	if run.State != "COMPLETED" {
		status = 2
	}
	attrs := map[string]any{"catena.run.id": run.ID, "catena.session.id": run.Session, "catena.sandbox.id": box, "catena.attempt": run.Attempt, "catena.run.state": run.State, "catena.events.paged": true}
	if run.Claimed != nil {
		attrs["catena.queue.wait_ms"] = run.Claimed.Sub(run.Created).Milliseconds()
	}
	if run.Started != nil && run.Finished != nil {
		attrs["catena.execution.duration_ms"] = run.Finished.Sub(*run.Started).Milliseconds()
	}
	var count int64
	if err = p.store.db.QueryRowContext(ctx, `SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND event_id<=$2`, run.ID, progress.Upper).Scan(&count); err != nil {
		return err
	}
	attrs["catena.events.count"] = count
	root := TraceSpan{AgentID: agent, TraceID: run.Trace, SpanID: run.Trace[:16], Name: "managed.agent.run", ServiceName: "catena-managed", ScopeName: "catena-managed", StartTime: run.Created, EndTime: end, StatusCode: status, StatusMessage: run.Error, Input: bounded(redactEvidenceText(run.Input), 8192), Output: bounded(redactEvidenceText(run.Result), 8192), Attributes: attrs, ResourceAttributes: map[string]any{"service.name": "catena-managed", "agent.runtime": "codex", "catena.agent.id": agent, "catena.owner.id": run.Tenant}}
	if !progress.Base {
		spans, err := p.managedModelSpans(ctx, run, agent, box, root)
		if err != nil {
			return err
		}
		if err = p.server.traceSink.InsertSpans(ctx, run.Tenant, spans); err != nil {
			return err
		}
		if _, err = p.store.db.ExecContext(ctx, `UPDATE agent_runs SET trace_base_exported=true WHERE run_id=$1`, run.ID); err != nil {
			return err
		}
	}
	for progress.Cursor < progress.Upper {
		rows, err := p.store.db.QueryContext(ctx, `SELECT event_id,kind,created_at,data FROM agent_run_events WHERE run_id=$1 AND event_id>$2 AND event_id<=$3 ORDER BY event_id LIMIT $4`, run.ID, progress.Cursor, progress.Upper, managedTracePageSize)
		if err != nil {
			return err
		}
		page := root
		page.SpanID = sessionTokenHash(fmt.Sprintf("%s/events/%d", run.ID, progress.Cursor))[:16]
		page.ParentSpanID = root.SpanID
		page.Name = "managed.run.events"
		page.Input = ""
		page.Output = ""
		page.Events = nil
		page.Attributes = map[string]any{"catena.run.id": run.ID, "catena.session.id": run.Session, "catena.sandbox.id": box, "catena.events.after": progress.Cursor}
		last := progress.Cursor
		tools := []TraceSpan{}
		for rows.Next() {
			var id int64
			var kind string
			var at time.Time
			var raw []byte
			if err = rows.Scan(&id, &kind, &at, &raw); err != nil {
				rows.Close()
				return err
			}
			var data map[string]any
			if err = json.Unmarshal(raw, &data); err != nil {
				rows.Close()
				return err
			}
			if data == nil {
				data = map[string]any{}
			}
			if tool := managedToolSpan(run, box, root, kind, at, data, progress.Starts); tool != nil {
				tools = append(tools, *tool)
			}
			data["catena.event.id"] = id
			page.Events = append(page.Events, TraceEvent{Name: kind, Time: at, Attributes: data})
			last = id
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if last == progress.Cursor {
			return fmt.Errorf("missing events below frozen export cursor for %s", run.ID)
		}
		page.Attributes["catena.events.last"] = last
		page.Attributes["catena.events.count"] = len(page.Events)
		if err = p.server.traceSink.InsertSpans(ctx, run.Tenant, append([]TraceSpan{page}, tools...)); err != nil {
			return err
		}
		raw, err := json.Marshal(progress.Starts)
		if err != nil {
			return err
		}
		// The cursor and cross-page tool starts advance only after Collector acknowledgement.
		if err = managedCAS(p.store.db.ExecContext(ctx, `UPDATE agent_runs SET trace_event_cursor=$3,trace_tool_starts=$4 WHERE run_id=$1 AND trace_event_cursor=$2 AND NOT trace_exported`, run.ID, progress.Cursor, last, raw)); err != nil {
			return err
		}
		progress.Cursor = last
	}
	return managedCAS(p.store.db.ExecContext(ctx, `UPDATE agent_runs SET trace_exported=true WHERE run_id=$1 AND trace_base_exported AND trace_event_cursor>=trace_event_upper AND NOT trace_exported`, run.ID))
}
func managedToolSpan(run managedRun, box string, root TraceSpan, kind string, at time.Time, data map[string]any, starts map[string]time.Time) *TraceSpan {
	if kind != "runtime" {
		return nil
	}
	if legacy, ok := data["event"].(string); ok {
		if json.Unmarshal([]byte(legacy), &data) != nil {
			return nil
		}
	}
	item, ok := data["item"].(map[string]any)
	if !ok || item["type"] != "command_execution" {
		return nil
	}
	id, _ := item["id"].(string)
	if id == "" {
		return nil
	}
	if data["type"] == "item.started" {
		starts[id] = at
		return nil
	}
	if data["type"] != "item.completed" {
		return nil
	}
	start, ok := starts[id]
	if !ok {
		start = at
	}
	delete(starts, id)
	command, _ := item["command"].(string)
	output, _ := item["aggregated_output"].(string)
	span := root
	span.SpanID = sessionTokenHash(run.ID + "/tool/" + id)[:16]
	span.ParentSpanID = root.SpanID
	span.Name = "managed.tool.shell"
	span.StartTime = start
	span.EndTime = at
	span.StatusCode = 1
	span.StatusMessage = ""
	span.Events = nil
	if item["status"] == "failed" {
		span.StatusCode = 2
	}
	span.Input = bounded(redactEvidenceText(command), 4096)
	span.Output = bounded(redactEvidenceText(output), 8192)
	span.Attributes = map[string]any{"catena.run.id": run.ID, "catena.session.id": run.Session, "catena.sandbox.id": box, "tool.exit_code": item["exit_code"]}
	return &span
}
func (p *managedPlatform) managedModelSpans(ctx context.Context, run managedRun, agent, box string, root TraceSpan) ([]TraceSpan, error) {
	spans := []TraceSpan{root}
	modelRows, e := p.store.db.QueryContext(ctx, `SELECT request_id,created_at,status,latency_ms,input_tokens,output_tokens FROM managed_model_requests WHERE run_id=$1 ORDER BY request_id`, run.ID)
	if e != nil {
		return nil, e
	}
	for modelRows.Next() {
		var id int64
		var at time.Time
		var status, latency, input, output sql.NullInt64
		if e = modelRows.Scan(&id, &at, &status, &latency, &input, &output); e != nil {
			modelRows.Close()
			return nil, e
		}
		attrs := map[string]any{"catena.run.id": run.ID, "catena.session.id": run.Session, "catena.sandbox.id": box, "catena.attempt": run.Attempt}
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
		spans = append(spans, TraceSpan{AgentID: agent, TraceID: run.Trace, SpanID: sessionTokenHash(fmt.Sprintf("%s/model/%d", run.ID, id))[:16], ParentSpanID: root.SpanID, Name: "managed.llm.request", ServiceName: root.ServiceName, ScopeName: root.ScopeName, StartTime: at, EndTime: at.Add(time.Duration(latency.Int64) * time.Millisecond), StatusCode: code, Attributes: attrs, ResourceAttributes: root.ResourceAttributes})
	}
	e = modelRows.Err()
	modelRows.Close()
	if e != nil {
		return nil, e
	}
	return spans, nil
}
