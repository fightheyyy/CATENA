-- Additive managed execution model; existing analysis data and APIs are unchanged.
CREATE TABLE managed_agents (
 agent_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1,
 runtime_type TEXT NOT NULL CHECK(runtime_type='codex'), runtime_version TEXT NOT NULL CHECK(runtime_version='0.160.0'),
 model_config_ref TEXT NOT NULL, instructions TEXT NOT NULL,
 tool_policy TEXT NOT NULL CHECK(tool_policy='sandbox-write'),
 cpu TEXT NOT NULL DEFAULT '1', memory TEXT NOT NULL DEFAULT '1Gi', timeout_seconds INTEGER NOT NULL CHECK(timeout_seconds BETWEEN 10 AND 1800),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,agent_id)
);
CREATE TABLE tenant_quotas (
 tenant_id TEXT PRIMARY KEY, max_sandboxes INTEGER NOT NULL DEFAULT 2 CHECK(max_sandboxes BETWEEN 1 AND 8),
 max_running INTEGER NOT NULL DEFAULT 2 CHECK(max_running BETWEEN 1 AND 8),
 max_queued INTEGER NOT NULL DEFAULT 100 CHECK(max_queued BETWEEN 1 AND 1000),
 requests_per_minute INTEGER NOT NULL DEFAULT 60 CHECK(requests_per_minute>0),
 max_model_requests INTEGER NOT NULL DEFAULT 200 CHECK(max_model_requests>0),
 max_output_tokens INTEGER NOT NULL DEFAULT 4096 CHECK(max_output_tokens BETWEEN 1 AND 32768),
 cpu TEXT NOT NULL DEFAULT '1', memory TEXT NOT NULL DEFAULT '1Gi', last_scheduled_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch'
);
CREATE TABLE agent_sessions (
 session_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, agent_id TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'READY' CHECK(state IN ('READY','PAUSING','PAUSED','DELETING','DELETED','NEEDS_REVIEW')),
 sandbox_binding TEXT, checkpoint_ref TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,agent_id) REFERENCES managed_agents(tenant_id,agent_id), UNIQUE(tenant_id,session_id)
);
CREATE TABLE agent_runs (
 run_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, tenant_id TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'QUEUED' CHECK(state IN ('QUEUED','RUNNING','COMPLETED','FAILED','CANCELLED','NEEDS_REVIEW')),
 sandbox_id TEXT, input TEXT NOT NULL, attempt INTEGER NOT NULL DEFAULT 0, worker_id TEXT NOT NULL DEFAULT '', fencing_token BIGINT NOT NULL DEFAULT 0,
 lease_until TIMESTAMPTZ, claimed_at TIMESTAMPTZ, execution_started_at TIMESTAMPTZ, execution_timeout INTEGER NOT NULL,
 idempotency_key TEXT NOT NULL, trace_id TEXT NOT NULL, result_ref TEXT, result TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
 cancel_requested BOOLEAN NOT NULL DEFAULT false, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 credential_hash TEXT, model_requests INTEGER NOT NULL DEFAULT 0, input_tokens BIGINT NOT NULL DEFAULT 0, output_tokens BIGINT NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), finished_at TIMESTAMPTZ, trace_exported BOOLEAN NOT NULL DEFAULT false,
 FOREIGN KEY(tenant_id,session_id) REFERENCES agent_sessions(tenant_id,session_id), UNIQUE(tenant_id,session_id,idempotency_key)
);
CREATE UNIQUE INDEX managed_session_mutex ON agent_runs(session_id) WHERE state='RUNNING';
CREATE INDEX managed_run_queue ON agent_runs(next_attempt_at,created_at) WHERE state='QUEUED';
CREATE TABLE managed_sandboxes (
 sandbox_id TEXT PRIMARY KEY, provider_sandbox_id TEXT UNIQUE, tenant_id TEXT NOT NULL, session_id TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('PENDING','CREATING','READY','BUSY','IDLE','PAUSED','UNKNOWN','FAILED','TERMINATING','TERMINATED')),
 generation BIGINT NOT NULL, cpu TEXT NOT NULL, memory TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(), expires_at TIMESTAMPTZ NOT NULL,
 error TEXT NOT NULL DEFAULT '', FOREIGN KEY(tenant_id,session_id) REFERENCES agent_sessions(tenant_id,session_id)
);
CREATE UNIQUE INDEX managed_live_binding ON managed_sandboxes(session_id) WHERE state<>'TERMINATED';
CREATE TABLE resource_reservations (
 sandbox_id TEXT PRIMARY KEY REFERENCES managed_sandboxes(sandbox_id), tenant_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), released_at TIMESTAMPTZ
);
CREATE TABLE sandbox_operations (
 operation_id TEXT PRIMARY KEY, sandbox_id TEXT NOT NULL REFERENCES managed_sandboxes(sandbox_id), kind TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('PENDING','SUCCEEDED','UNKNOWN')), attempts INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '', updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE agent_run_events (
 event_id BIGSERIAL PRIMARY KEY, run_id TEXT NOT NULL REFERENCES agent_runs(run_id), fencing_token BIGINT NOT NULL,
 kind TEXT NOT NULL, data JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX managed_event_cursor ON agent_run_events(run_id,event_id);
CREATE TABLE managed_checkpoints (
 checkpoint_id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES agent_sessions(session_id), run_id TEXT NOT NULL REFERENCES agent_runs(run_id),
 agent_id TEXT NOT NULL, agent_version INTEGER NOT NULL, format_version INTEGER NOT NULL DEFAULT 1,
 sha256 TEXT NOT NULL, size_bytes INTEGER NOT NULL CHECK(size_bytes BETWEEN 1 AND 16777216), snapshot BYTEA NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE managed_model_requests (
 request_id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL, run_id TEXT NOT NULL REFERENCES agent_runs(run_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), status INTEGER, input_tokens BIGINT, output_tokens BIGINT, latency_ms BIGINT
);
CREATE TABLE managed_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);

ALTER TABLE agent_runs ADD CONSTRAINT managed_run_sandbox_fk FOREIGN KEY(sandbox_id) REFERENCES managed_sandboxes(sandbox_id);
