-- Keep operation ownership and export progress durable across controller failures.
ALTER TABLE managed_sandboxes ADD COLUMN revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sandbox_operations ALTER COLUMN sandbox_id DROP NOT NULL;
ALTER TABLE sandbox_operations DROP CONSTRAINT sandbox_operations_state_check;
ALTER TABLE sandbox_operations ADD CONSTRAINT sandbox_operations_state_check CHECK(state IN ('PENDING','SUCCEEDED','UNKNOWN','ABANDONED'));
ALTER TABLE sandbox_operations ADD COLUMN worker_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sandbox_operations ADD COLUMN fencing_token BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sandbox_operations ADD COLUMN lease_until TIMESTAMPTZ;
ALTER TABLE sandbox_operations ADD COLUMN expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sandbox_operations ADD COLUMN provider_ids JSONB NOT NULL DEFAULT '[]';
CREATE INDEX managed_operation_recovery ON sandbox_operations(sandbox_id,updated_at) WHERE state IN ('PENDING','UNKNOWN') AND kind<>'create';
ALTER TABLE agent_runs ADD COLUMN command_exited_at TIMESTAMPTZ;
ALTER TABLE agent_runs ADD COLUMN trace_base_exported BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE agent_runs ADD COLUMN trace_event_cursor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN trace_event_upper BIGINT;
ALTER TABLE agent_runs ADD COLUMN trace_tool_starts JSONB NOT NULL DEFAULT '{}';
-- Replay only truncated historical exports; original telemetry remains intact.
UPDATE agent_runs r SET trace_exported=false WHERE trace_exported AND (SELECT count(*) FROM agent_run_events e WHERE e.run_id=r.run_id)>200;
-- Cancellation/lease loss is nonterminal until the old instance is isolated.
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_state_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_state_check CHECK(state IN ('QUEUED','RUNNING','STOPPING','COMPLETED','FAILED','CANCELLED','NEEDS_REVIEW'));
DROP INDEX managed_session_mutex;
CREATE UNIQUE INDEX managed_session_mutex ON agent_runs(session_id) WHERE state IN ('RUNNING','STOPPING');
ALTER TABLE agent_runs ADD COLUMN isolation_confirmed_at TIMESTAMPTZ;
