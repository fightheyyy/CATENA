package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type workflowAccess struct {
	ctx   context.Context
	lease AnalysisLease
	owner string
	kind  string
}

type workflowTurn struct {
	Prompt   string `json:"prompt"`
	Callback string `json:"callback"`
	Timeout  int    `json:"timeout_seconds"`
}

type workflowTool struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *HTTPServer) openWorkflow(ctx context.Context, lease AnalysisLease, owner, kind string) (string, func(), error) {
	token, err := randomURLToken(32)
	if err != nil {
		return "", nil, err
	}
	s.workflowAccess.Store(sessionTokenHash(token), workflowAccess{ctx, lease, owner, kind})
	return token, func() { s.workflowAccess.Delete(sessionTokenHash(token)) }, nil
}

func bearerCapability(r *http.Request) string {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(token) != 43 || strings.ContainsAny(token, " \t\r\n") {
		return ""
	}
	return token
}

func (s *HTTPServer) startWorkflowTurn(w http.ResponseWriter, r *http.Request) {
	token := bearerCapability(r)
	access, ok := s.workflowAccess.Load(sessionTokenHash(token))
	if s.managed == nil || token == "" || !ok {
		writeProblem(w, 403, "workflow execution unavailable")
		return
	}
	binding := access.(workflowAccess)
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(binding.ctx, cancel)
	defer stop()
	defer cancel()
	var request workflowTurn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*1024*1024)).Decode(&request) != nil ||
		len(request.Prompt) == 0 || len(request.Prompt) > 1_000_000 ||
		len(request.Callback) != 32 || !safeRuntimeID.MatchString(request.Callback) ||
		request.Timeout < 1 || request.Timeout > 1800 {
		writeProblem(w, 400, "invalid workflow turn")
		return
	}
	run, err := s.managed.store.submitWorkflowTurn(ctx, binding, request)
	if err != nil {
		writeProblem(w, 409, "workflow lease unavailable or turn already started")
		return
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = s.managed.store.cancelWorkflowTurn(cleanup, run)
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state, result string
		err = s.managed.store.db.QueryRowContext(ctx, `SELECT state,result FROM agent_runs WHERE run_id=$1`, run).Scan(&state, &result)
		if err != nil {
			writeProblem(w, 503, "workflow turn unavailable")
			return
		}
		switch state {
		case "COMPLETED":
			if !json.Valid([]byte(result)) {
				writeProblem(w, 502, "invalid workflow result")
				return
			}
			writeJSON(w, 200, json.RawMessage(result))
			return
		case "FAILED", "NEEDS_REVIEW", "CANCELLED":
			writeProblem(w, 409, "workflow execution did not complete")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *PostgresStore) submitWorkflowTurn(ctx context.Context, access workflowAccess, in workflowTurn) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Lock the business Job while creating its one execution binding. A new
	// Job lease must never re-execute a turn from a previous lease.
	var valid string
	err = tx.QueryRowContext(ctx, `SELECT job_id FROM spiral_evolution_jobs WHERE job_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='running' AND NOT cancel_requested AND lease_until>clock_timestamp() AND COALESCE(owner_user_id,'local')=$4 AND document->>'pipeline'=$5
	FOR UPDATE`, access.lease.JobID, access.lease.WorkerID, access.lease.Fence, access.owner, access.kind).Scan(&valid)
	if err != nil {
		return "", ErrLeaseLost
	}
	agentKey := sha256.Sum256([]byte(access.owner + "/" + access.kind))
	agent := "internal-" + hex.EncodeToString(agentKey[:16])
	run, session := newID("run"), newID("session")
	trace, err := randomURLToken(16)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(trace))
	if _, err = tx.ExecContext(ctx, `INSERT INTO tenant_quotas(tenant_id) VALUES($1) ON CONFLICT DO NOTHING`, access.owner); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO managed_agents(agent_id,tenant_id,runtime_type,runtime_version,model_config_ref,instructions,tool_policy,timeout_seconds,profile_kind) VALUES($1,$2,'codex','0.160.0',$2,'','read-only',300,$3) ON CONFLICT DO NOTHING`, agent, access.owner, access.kind); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_sessions(session_id,tenant_id,agent_id,workflow_job_id,workflow_worker_id,workflow_fence) VALUES($1,$2,$3,$4,$5,$6)`, session, access.owner, agent, access.lease.JobID, access.lease.WorkerID, access.lease.Fence); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_runs(run_id,session_id,tenant_id,input,execution_timeout,idempotency_key,trace_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, run, session, access.owner, in.Prompt, in.Timeout, in.Callback, hex.EncodeToString(hash[:16])); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,fencing_token,kind,data) VALUES($1,0,'queued','{}')`, run); err != nil {
		return "", err
	}
	return run, tx.Commit()
}

func (s *PostgresStore) cancelWorkflowTurn(ctx context.Context, run string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_runs SET cancel_requested=true,credential_hash=NULL,
	state=CASE WHEN state='QUEUED' THEN 'CANCELLED' ELSE state END,
	finished_at=CASE WHEN state='QUEUED' THEN now() ELSE finished_at END
	WHERE run_id=$1 AND state IN ('QUEUED','RUNNING','STOPPING')`, run)
	return err
}

func (s *HTTPServer) workflowTool(w http.ResponseWriter, r *http.Request) {
	token := bearerCapability(r)
	if token == "" || s.managed == nil || s.analysisRuntime == nil {
		writeProblem(w, 403, "workflow tool capability required")
		return
	}
	var in workflowTool
	if decodeJSON(w, r, &in) != nil || len(in.Arguments) == 0 {
		writeProblem(w, 400, "invalid tool request")
		return
	}
	var owner, job, callback, kind, run string
	err := s.managed.store.db.QueryRowContext(r.Context(), `SELECT r.tenant_id,s.workflow_job_id,r.idempotency_key,a.profile_kind,r.run_id
	FROM agent_runs r JOIN agent_sessions s USING(session_id) JOIN managed_agents a USING(agent_id)
	JOIN active_harness_sessions live USING(session_id)
	WHERE r.credential_hash=$1 AND r.state='RUNNING' AND NOT r.cancel_requested AND r.lease_until>now()
	AND r.execution_started_at+r.execution_timeout*interval '1 second'>now() AND s.workflow_job_id IS NOT NULL`, sessionTokenHash(token)).Scan(&owner, &job, &callback, &kind, &run)
	if err != nil || !workflowAllows(kind, in.Tool) {
		writeProblem(w, 403, "workflow tool unavailable")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// The Engine callback is scoped to this one registered turn. The Engine
	// still owns candidate and case checks and applies its native cancellation.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var valid bool
				e := s.managed.store.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs r JOIN active_harness_sessions live USING(session_id)
				WHERE run_id=$1 AND credential_hash=$2 AND state='RUNNING' AND NOT cancel_requested AND lease_until>now()
				AND execution_started_at+execution_timeout*interval '1 second'>now())`, run, sessionTokenHash(token)).Scan(&valid)
				if e != nil || !valid {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	var out json.RawMessage
	status, err := s.analysisRuntime.call(ctx, owner, http.MethodPost, "/internal/harness/tool/"+job+"/"+callback, in, &out)
	if err != nil || status != 200 || len(out) > 128*1024 {
		if errors.Is(err, context.Canceled) {
			return
		}
		writeProblem(w, 409, "trusted tool failed or lease ended")
		return
	}
	writeJSON(w, 200, out)
}

func workflowAllows(kind, tool string) bool {
	return kind == "analyze" && (tool == "list_traces" || tool == "get_trace") ||
		kind == "improve" && tool == "evaluate_candidate"
}
