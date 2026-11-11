ALTER TABLE managed_agents ADD COLUMN profile_kind TEXT NOT NULL DEFAULT 'managed'
  CHECK (profile_kind IN ('managed','analyze','improve'));
ALTER TABLE agent_sessions ADD COLUMN workflow_job_id TEXT;
ALTER TABLE agent_sessions ADD COLUMN workflow_worker_id TEXT;
ALTER TABLE agent_sessions ADD COLUMN workflow_fence BIGINT;
ALTER TABLE agent_sessions ADD CONSTRAINT harness_workflow_lease
  CHECK ((workflow_job_id IS NULL AND workflow_worker_id IS NULL AND workflow_fence IS NULL)
    OR (workflow_job_id IS NOT NULL AND workflow_worker_id IS NOT NULL AND workflow_fence IS NOT NULL));
CREATE UNIQUE INDEX harness_workflow_session ON agent_sessions(workflow_job_id) WHERE workflow_job_id IS NOT NULL;
CREATE INDEX harness_active_credentials ON agent_runs(credential_hash) WHERE credential_hash IS NOT NULL;
CREATE VIEW active_harness_sessions AS
SELECT s.session_id FROM agent_sessions s
WHERE s.workflow_job_id IS NULL OR EXISTS (
  SELECT 1 FROM spiral_evolution_jobs j WHERE j.job_id=s.workflow_job_id
    AND j.worker_id=s.workflow_worker_id AND j.fencing_token=s.workflow_fence
    AND j.state='running' AND NOT j.cancel_requested AND j.lease_until>clock_timestamp()
);
