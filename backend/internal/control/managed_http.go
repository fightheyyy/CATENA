package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *HTTPServer) managedOwner(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return "", false
	}
	if s.managed == nil {
		writeProblem(w, 503, "managed execution is disabled; configure CATENA_MANAGED_RUNNER_URL")
		return "", false
	}
	w.Header().Set("Cache-Control", "no-store")
	return traceOwnerID(user), true
}
func (s *HTTPServer) registerManaged(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/managed-agents", s.managedAgents)
	mux.HandleFunc("GET /v1/managed-agents", s.managedAgents)
	mux.HandleFunc("POST /v1/agent-sessions", s.managedSessions)
	mux.HandleFunc("GET /v1/agent-sessions", s.managedSessions)
	mux.HandleFunc("GET /v1/agent-sessions/{session_id}", s.managedSessions)
	mux.HandleFunc("POST /v1/agent-sessions/{session_id}/runs", s.managedSubmit)
	mux.HandleFunc("GET /v1/agent-runs/{run_id}", s.managedRunDetail)
	mux.HandleFunc("POST /v1/agent-runs/{run_id}/cancel", s.managedCancel)
	mux.HandleFunc("GET /v1/agent-runs/{run_id}/events", s.managedEvents)
	mux.HandleFunc("GET /v1/agent-runs/{run_id}/artifact", s.managedArtifact)
	mux.HandleFunc("POST /v1/agent-sessions/{session_id}/pause", s.managedSessionAction)
	mux.HandleFunc("POST /v1/agent-sessions/{session_id}/resume", s.managedSessionAction)
	mux.HandleFunc("DELETE /v1/agent-sessions/{session_id}", s.managedSessionAction)
	mux.HandleFunc("GET /v1/sandboxes", s.managedResources)
	mux.HandleFunc("GET /v1/me/resources", s.managedResources)
	mux.HandleFunc("POST /internal/managed/authorize", s.managedAuthorize)
	mux.HandleFunc("POST /internal/managed/model/responses", s.managedModel)
}
func (s *HTTPServer) managedAgents(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	db := s.managed.store.db
	if r.Method == "GET" {
		v, err := managedRows(r.Context(), db, `SELECT to_jsonb(a) FROM managed_agents a WHERE tenant_id=$1 AND profile_kind='managed' ORDER BY created_at DESC LIMIT 100`, owner)
		managedReply(w, 200, map[string]any{"agents": v}, err)
		return
	}
	var in struct {
		AgentID        string         `json:"agent_id"`
		Instructions   string         `json:"instructions"`
		Timeout        int            `json:"timeout_seconds"`
		ToolPolicy     string         `json:"tool_policy"`
		Runtime        string         `json:"runtime_type"`
		RuntimeVersion string         `json:"runtime_version"`
		Skills         []harnessSkill `json:"skills"`
		Knowledge      []string       `json:"knowledge_refs"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if in.Timeout == 0 {
		in.Timeout = 600
	}
	if in.Runtime == "" {
		in.Runtime = "codex"
	}
	if in.RuntimeVersion == "" {
		in.RuntimeVersion = "0.160.0"
	}
	if in.ToolPolicy == "" {
		in.ToolPolicy = "sandbox-write"
	}
	if len(in.Instructions) > 32768 || in.Timeout < 10 || in.Timeout > 1800 || in.Runtime != "codex" || in.RuntimeVersion != "0.160.0" || (in.ToolPolicy != "sandbox-write" && in.ToolPolicy != "read-only") || !validManagedProfile(in.Skills, in.Knowledge) {
		writeProblem(w, 400, "invalid Managed Profile: check Codex version, tool policy, timeout, Skill limits and workspace references")
		return
	}
	// A managed definition references an existing observed Agent; old registration APIs retain their schema.
	if _, err := s.store.GetRegisteredAgentByOwner(r.Context(), owner, in.AgentID); err != nil {
		writeProblem(w, 404, "register an Agent first")
		return
	}
	if _, err := s.analysisModelCredentials(r.Context(), owner); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		managedReply(w, 0, nil, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO tenant_quotas(tenant_id) VALUES($1) ON CONFLICT DO NOTHING`, owner)
	var agent managedAgent
	if in.Skills == nil {
		in.Skills = []harnessSkill{}
	}
	if in.Knowledge == nil {
		in.Knowledge = []string{}
	}
	skills, _ := json.Marshal(in.Skills)
	knowledge, _ := json.Marshal(in.Knowledge)
	if err == nil {
		err = managedJSON(tx.QueryRowContext(r.Context(), `INSERT INTO managed_agents(agent_id,tenant_id,runtime_type,runtime_version,model_config_ref,instructions,tool_policy,cpu,memory,timeout_seconds,skills,knowledge_refs)
 SELECT $1,$2,'codex','0.160.0',$2,$3,$5,cpu,memory,$4,$6,$7 FROM tenant_quotas WHERE tenant_id=$2 RETURNING to_jsonb(managed_agents)`, in.AgentID, owner, in.Instructions, in.Timeout, in.ToolPolicy, skills, knowledge), &agent)
	}
	if err == nil {
		err = tx.Commit()
	}
	managedReply(w, 201, agent, mapStoreError(err))
}
func managedReply(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		writeProblem(w, statusFor(err), "managed operation failed: "+bounded(redactEvidenceText(err.Error()), 400))
		return
	}
	writeJSON(w, status, value)
}
func (s *HTTPServer) managedSessions(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	db := s.managed.store.db
	ctx := r.Context()
	if r.Method == "POST" {
		var in struct {
			Agent string `json:"agent_id"`
		}
		if err := decodeJSON(w, r, &in); err != nil {
			writeProblem(w, 400, err.Error())
			return
		}
		var out json.RawMessage
		err := managedJSONRaw(db.QueryRowContext(ctx, `INSERT INTO agent_sessions(session_id,tenant_id,agent_id) SELECT $1,$2,agent_id FROM managed_agents WHERE agent_id=$3 AND tenant_id=$2 AND profile_kind='managed' RETURNING to_jsonb(agent_sessions)-ARRAY['workflow_job_id','workflow_worker_id','workflow_fence']`, newID("session"), owner, in.Agent), &out)
		managedReply(w, 201, out, err)
		return
	}
	if id := r.PathValue("session_id"); id != "" {
		var out json.RawMessage
		err := managedJSONRaw(db.QueryRowContext(ctx, `SELECT to_jsonb(s)-ARRAY['workflow_job_id','workflow_worker_id','workflow_fence'] FROM agent_sessions s WHERE tenant_id=$1 AND session_id=$2 AND workflow_job_id IS NULL`, owner, id), &out)
		if err != nil {
			managedReply(w, 0, nil, err)
			return
		}
		runs, err := managedRows(ctx, db, `SELECT to_jsonb(r)-'credential_hash' FROM agent_runs r WHERE tenant_id=$1 AND session_id=$2 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=r.session_id AND s.workflow_job_id IS NULL) ORDER BY created_at DESC LIMIT 100`, owner, id)
		managedReply(w, 200, map[string]any{"session": out, "runs": runs}, err)
		return
	}
	v, err := managedRows(ctx, db, `SELECT to_jsonb(s)-ARRAY['workflow_job_id','workflow_worker_id','workflow_fence'] FROM agent_sessions s WHERE tenant_id=$1 AND workflow_job_id IS NULL AND state<>'DELETED' ORDER BY created_at DESC LIMIT 100`, owner)
	managedReply(w, 200, map[string]any{"sessions": v}, err)
}
func managedJSONRaw(row *sql.Row, out *json.RawMessage) error {
	err := row.Scan(out)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (s *HTTPServer) managedSubmit(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Input string `json:"input"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(strings.TrimSpace(in.Input)) == 0 || len(in.Input) > 65536 || len(key) == 0 || len(key) > 200 {
		writeProblem(w, 400, "input 1..65536 bytes and Idempotency-Key 1..200 bytes required")
		return
	}
	run, created, err := s.managed.store.submitManaged(r.Context(), owner, r.PathValue("session_id"), in.Input, key)
	status := 200
	if created {
		status = 202
	}
	managedReply(w, status, run, err)
}
func (s *HTTPServer) managedRunDetail(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	var out json.RawMessage
	err := managedJSONRaw(s.managed.store.db.QueryRowContext(r.Context(), `SELECT to_jsonb(r)-'credential_hash' FROM agent_runs r WHERE tenant_id=$1 AND run_id=$2 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=r.session_id AND s.workflow_job_id IS NULL)`, owner, r.PathValue("run_id")), &out)
	managedReply(w, 200, out, err)
}
func (s *HTTPServer) managedCancel(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	var out json.RawMessage
	// Running cancellation retains ownership until reconciliation has isolated its environment.
	err := managedJSONRaw(s.managed.store.db.QueryRowContext(r.Context(), `UPDATE agent_runs SET cancel_requested=CASE WHEN state IN ('QUEUED','RUNNING','STOPPING') THEN true ELSE cancel_requested END,credential_hash=NULL,finished_at=CASE WHEN state='QUEUED' THEN now() ELSE finished_at END,state=CASE WHEN state='QUEUED' THEN 'CANCELLED' ELSE state END,updated_at=now() WHERE tenant_id=$1 AND run_id=$2 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=agent_runs.session_id AND s.workflow_job_id IS NULL) RETURNING to_jsonb(agent_runs)-'credential_hash'`, owner, r.PathValue("run_id")), &out)
	managedReply(w, 200, out, err)
}
func (s *HTTPServer) managedSessionAction(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	target := "PAUSING"
	if strings.HasSuffix(r.URL.Path, "/resume") {
		target = "READY"
	}
	if r.Method == "DELETE" {
		target = "DELETING"
	}
	tx, err := managedTx(r.Context(), s.managed.store.db)
	if err != nil {
		managedReply(w, 0, nil, err)
		return
	}
	defer tx.Rollback()
	var active bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE session_id=$1 AND tenant_id=$2 AND state IN ('RUNNING','STOPPING') AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=agent_runs.session_id AND s.workflow_job_id IS NULL))`, r.PathValue("session_id"), owner).Scan(&active)
	if err == nil && active {
		writeProblem(w, 409, "cancel the active run and wait for resource cleanup first")
		return
	}
	var out json.RawMessage
	if err == nil {
		err = managedJSONRaw(tx.QueryRowContext(r.Context(), `UPDATE agent_sessions SET state=CASE WHEN state='DELETED' THEN state ELSE $3 END,updated_at=now() WHERE tenant_id=$1 AND session_id=$2 AND workflow_job_id IS NULL AND ($3='DELETING' OR state NOT IN ('DELETED','DELETING','NEEDS_REVIEW')) RETURNING to_jsonb(agent_sessions)-ARRAY['workflow_job_id','workflow_worker_id','workflow_fence']`, owner, r.PathValue("session_id"), target), &out)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE agent_runs SET state='CANCELLED',cancel_requested=true WHERE session_id=$1 AND state='QUEUED' AND $2='DELETING'`, r.PathValue("session_id"), target)
	}
	if err == nil {
		err = tx.Commit()
	}
	managedReply(w, 202, out, err)
}
func (s *HTTPServer) managedResources(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	db := s.managed.store.db
	boxes, err := managedRows(r.Context(), db, `SELECT to_jsonb(b) FROM managed_sandboxes b WHERE tenant_id=$1 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=b.session_id AND s.workflow_job_id IS NULL) ORDER BY (state='TERMINATED'),created_at DESC LIMIT 100`, owner)
	if err != nil {
		managedReply(w, 0, nil, err)
		return
	}
	var quota json.RawMessage
	err = managedJSONRaw(db.QueryRowContext(r.Context(), `SELECT to_jsonb(q)||jsonb_build_object('reserved_sandboxes',(SELECT count(*) FROM resource_reservations z JOIN managed_sandboxes b USING(sandbox_id) JOIN agent_sessions s USING(session_id) WHERE z.tenant_id=$1 AND s.workflow_job_id IS NULL AND z.released_at IS NULL),'running',(SELECT count(*) FROM agent_runs r JOIN agent_sessions s USING(session_id) WHERE r.tenant_id=$1 AND s.workflow_job_id IS NULL AND r.state IN ('RUNNING','STOPPING')),'queued',(SELECT count(*) FROM agent_runs r JOIN agent_sessions s USING(session_id) WHERE r.tenant_id=$1 AND s.workflow_job_id IS NULL AND r.state='QUEUED'),'global_capacity',$2::int,'metrics_supported',false,'network_isolation','trusted-local-only') FROM tenant_quotas q WHERE tenant_id=$1`, owner, s.managed.config.Capacity), &quota)
	if errors.Is(err, ErrNotFound) {
		quota = json.RawMessage(`null`)
		err = nil
	}
	managedReply(w, 200, map[string]any{"sandboxes": boxes, "quota": quota}, err)
}
func (s *HTTPServer) managedEvents(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = r.URL.Query().Get("after")
	}
	if cursor == "" {
		cursor = "0"
	}
	after, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || after < 0 {
		writeProblem(w, 400, "invalid event cursor")
		return
	}
	var exists bool
	err = s.managed.store.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE tenant_id=$1 AND run_id=$2 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=agent_runs.session_id AND s.workflow_job_id IS NULL))`, owner, r.PathValue("run_id")).Scan(&exists)
	if err != nil {
		managedReply(w, 0, nil, err)
		return
	}
	if !exists {
		writeProblem(w, 404, "run not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, 500, "streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// Connections rotate after 25s; EventSource reconnects using its durable cursor.
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	for {
		rows, e := s.managed.store.db.QueryContext(ctx, `SELECT event_id,kind,data FROM agent_run_events WHERE run_id=$1 AND event_id>$2 ORDER BY event_id LIMIT 200`, r.PathValue("run_id"), after)
		if e != nil {
			return
		}
		for rows.Next() {
			var id int64
			var kind string
			var data []byte
			if e = rows.Scan(&id, &kind, &data); e != nil {
				break
			}
			if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, kind, data); e != nil {
				break
			}
			after = id
		}
		e = errors.Join(e, rows.Err())
		rows.Close()
		if e != nil {
			return
		}
		fmt.Fprint(w, ": keepalive\n\n")
		flusher.Flush()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *HTTPServer) managedArtifact(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.managedOwner(w, r)
	if !ok {
		return
	}
	var snapshot []byte
	var hash string
	err := s.managed.store.db.QueryRowContext(r.Context(), `SELECT c.snapshot,c.sha256 FROM managed_checkpoints c JOIN agent_runs r ON r.result_ref=c.checkpoint_id WHERE r.tenant_id=$1 AND r.run_id=$2 AND EXISTS(SELECT 1 FROM agent_sessions s WHERE s.session_id=r.session_id AND s.workflow_job_id IS NULL)`, owner, r.PathValue("run_id")).Scan(&snapshot, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		writeProblem(w, 404, "checkpoint not found")
		return
	}
	if err != nil {
		managedReply(w, 0, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="workspace.tar.gz"`)
	w.Header().Set("X-Content-SHA256", hash)
	w.Write(snapshot)
}

// ModelGateway exposes only the capability-authenticated model endpoint to sandboxes.
// Local-mode user APIs must never be reachable through this listener.
func (h *ManagedHTTPHandler) ModelGateway() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || (r.URL.Path != "/internal/managed/model/responses" && r.URL.Path != "/internal/harness/tool") {
			http.NotFound(w, r)
			return
		}
		h.Handler.ServeHTTP(w, r)
	})
}
