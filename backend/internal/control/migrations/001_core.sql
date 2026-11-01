CREATE TABLE IF NOT EXISTS barena_users (
  user_id TEXT PRIMARY KEY,
  github_id BIGINT NOT NULL UNIQUE,
  github_login TEXT NOT NULL,
  display_name TEXT NOT NULL,
  avatar_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS barena_sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES barena_users(user_id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS barena_sessions_expires_at_idx
  ON barena_sessions (expires_at);
CREATE TABLE IF NOT EXISTS catena_agents (
  agent_id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES barena_users(user_id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  runtime_kind TEXT NOT NULL DEFAULT '',
  last_seen_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS catena_agents_owner_created_at_idx
  ON catena_agents (owner_user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS barena_api_tokens (
  token_id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES barena_users(user_id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE barena_api_tokens
  ADD COLUMN IF NOT EXISTS encrypted_token TEXT NOT NULL DEFAULT '';
ALTER TABLE barena_api_tokens
  ADD COLUMN IF NOT EXISTS agent_id TEXT REFERENCES catena_agents(agent_id) ON DELETE CASCADE;
CREATE UNIQUE INDEX IF NOT EXISTS barena_api_tokens_agent_id_idx
  ON barena_api_tokens (agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS barena_api_tokens_user_created_at_idx
  ON barena_api_tokens (user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS catena_evolution_model_configs (
  owner_user_id TEXT PRIMARY KEY REFERENCES barena_users(user_id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  base_url TEXT NOT NULL,
  model TEXT NOT NULL,
  encrypted_api_key TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS catena_memory_tasks (
  task_id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL,
  source_conversation_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','processing','completed','failed')),
  document JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS catena_memory_tasks_owner_updated_idx
  ON catena_memory_tasks (owner_user_id, updated_at DESC);
CREATE TABLE IF NOT EXISTS barena_agent_profiles (
  owner_user_id TEXT PRIMARY KEY REFERENCES barena_users(user_id) ON DELETE CASCADE,
  slug TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  bio TEXT NOT NULL DEFAULT '',
  is_public BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS catena_conversation_messages (
  owner_user_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  sequence BIGINT NOT NULL CHECK (sequence > 0),
  occurred_at TIMESTAMPTZ NOT NULL,
  received_at TIMESTAMPTZ NOT NULL,
  runtime TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  agent_name TEXT NOT NULL DEFAULT '',
  surface TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user','assistant')),
  role_name TEXT NOT NULL DEFAULT '',
  content JSONB NOT NULL,
  delivery JSONB NOT NULL,
  trace_id TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL,
  PRIMARY KEY (owner_user_id, message_id),
  UNIQUE (owner_user_id, agent_id, conversation_id, sequence)
);
CREATE INDEX IF NOT EXISTS catena_conversation_messages_owner_updated_idx
  ON catena_conversation_messages
  (owner_user_id, occurred_at DESC, conversation_id);
CREATE INDEX IF NOT EXISTS catena_conversation_messages_owner_agent_idx
  ON catena_conversation_messages
  (owner_user_id, agent_id, occurred_at DESC);
CREATE TABLE IF NOT EXISTS spiral_evolution_jobs (
  job_id TEXT PRIMARY KEY,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE SET NULL,
  source_run_id TEXT,
  source_trace_id TEXT NOT NULL,
  source_agent_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('queued','running','completed','failed','cancelled')),
  current_stage TEXT NOT NULL DEFAULT '',
  document JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE spiral_evolution_jobs
  ALTER COLUMN source_run_id DROP NOT NULL;
ALTER TABLE spiral_evolution_jobs
  ADD COLUMN IF NOT EXISTS source_agent_id TEXT NOT NULL DEFAULT '';
ALTER TABLE spiral_evolution_jobs
  ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS fencing_token BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS cancel_requested BOOLEAN NOT NULL DEFAULT false;
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='spiral_evolution_jobs'::regclass
    AND conname='spiral_evolution_jobs_state_check' AND pg_get_constraintdef(oid) NOT LIKE '%cancelled%') THEN
    ALTER TABLE spiral_evolution_jobs DROP CONSTRAINT spiral_evolution_jobs_state_check;
    ALTER TABLE spiral_evolution_jobs ADD CONSTRAINT spiral_evolution_jobs_state_check
      CHECK (state IN ('queued','running','completed','failed','cancelled'));
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS spiral_evolution_jobs_runnable_idx
  ON spiral_evolution_jobs (next_attempt_at,created_at,job_id) WHERE state IN ('queued','running');
CREATE UNIQUE INDEX IF NOT EXISTS spiral_evolution_jobs_request_key_v2_idx
  ON spiral_evolution_jobs
  (COALESCE(owner_user_id,''), source_trace_id, source_agent_id, idempotency_key);
CREATE INDEX IF NOT EXISTS spiral_evolution_jobs_owner_created_at_idx
  ON spiral_evolution_jobs (owner_user_id, created_at DESC);
