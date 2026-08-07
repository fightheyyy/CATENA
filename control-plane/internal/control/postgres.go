package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgresStore{db: db}
	if err := store.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
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
CREATE TABLE IF NOT EXISTS barena_agent_profiles (
  owner_user_id TEXT PRIMARY KEY REFERENCES barena_users(user_id) ON DELETE CASCADE,
  slug TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  bio TEXT NOT NULL DEFAULT '',
  is_public BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS barena_runs (
  run_id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL UNIQUE,
  operation TEXT NOT NULL,
  state TEXT NOT NULL,
  current_phase TEXT NOT NULL DEFAULT '',
  current_actor TEXT NOT NULL DEFAULT '',
  input JSONB NOT NULL,
  runtime JSONB,
  cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE barena_runs
  ADD COLUMN IF NOT EXISTS owner_user_id TEXT
  REFERENCES barena_users(user_id) ON DELETE SET NULL;
ALTER TABLE barena_runs
  ADD COLUMN IF NOT EXISTS execution_origin TEXT NOT NULL DEFAULT 'local';
CREATE INDEX IF NOT EXISTS barena_runs_created_at_idx
  ON barena_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS barena_runs_owner_created_at_idx
  ON barena_runs (owner_user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS barena_engine_events (
  run_id TEXT NOT NULL REFERENCES barena_runs(run_id) ON DELETE CASCADE,
  sequence BIGINT NOT NULL,
  event_id TEXT NOT NULL UNIQUE,
  timestamp TIMESTAMPTZ NOT NULL,
  operation TEXT NOT NULL,
  kind TEXT NOT NULL,
  phase TEXT NOT NULL,
  actor TEXT NOT NULL,
  attempt_id TEXT NOT NULL DEFAULT '',
  trace_id TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL,
  PRIMARY KEY (run_id, sequence)
);
CREATE TABLE IF NOT EXISTS barena_run_bundles (
  run_bundle_id TEXT PRIMARY KEY,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE SET NULL,
  run_id TEXT NOT NULL UNIQUE REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  terminal_fact_sha256 TEXT NOT NULL,
  terminal_fact BYTEA NOT NULL,
  document JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS barena_run_bundles_owner_key_idx
  ON barena_run_bundles (COALESCE(owner_user_id,''), idempotency_key);
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
  source_run_id TEXT REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_trace_id TEXT NOT NULL,
  source_agent_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('queued','running','completed','failed')),
  current_stage TEXT NOT NULL DEFAULT '',
  document JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE spiral_evolution_jobs
  ALTER COLUMN source_run_id DROP NOT NULL;
ALTER TABLE spiral_evolution_jobs
  ADD COLUMN IF NOT EXISTS source_agent_id TEXT NOT NULL DEFAULT '';
DROP INDEX IF EXISTS spiral_evolution_jobs_request_key_idx;
CREATE UNIQUE INDEX IF NOT EXISTS spiral_evolution_jobs_request_key_idx
  ON spiral_evolution_jobs
  (COALESCE(owner_user_id,''), source_trace_id, source_agent_id, idempotency_key);
CREATE INDEX IF NOT EXISTS spiral_evolution_jobs_owner_created_at_idx
  ON spiral_evolution_jobs (owner_user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS barena_issues (
  issue_id TEXT PRIMARY KEY,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE SET NULL,
  source_run_id TEXT NOT NULL REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_trace_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  severity TEXT NOT NULL,
  status TEXT NOT NULL,
  promoted_case_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS barena_issues_owner_created_at_idx
  ON barena_issues (owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS barena_issues_source_run_idx
  ON barena_issues (source_run_id);
CREATE TABLE IF NOT EXISTS barena_cases (
  case_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE SET NULL,
  source_issue_id TEXT NOT NULL UNIQUE
    REFERENCES barena_issues(issue_id) ON DELETE RESTRICT,
  source_run_id TEXT NOT NULL REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_trace_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  operation TEXT NOT NULL,
  input JSONB NOT NULL,
  runtime JSONB,
  success_criteria TEXT NOT NULL,
  verifier JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS barena_cases_owner_created_at_idx
  ON barena_cases (owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS barena_cases_source_run_idx
  ON barena_cases (source_run_id);
ALTER TABLE barena_cases
  ADD COLUMN IF NOT EXISTS replay_prompt TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS barena_harness_versions (
  harness_version_id TEXT PRIMARY KEY,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE RESTRICT,
  case_id TEXT NOT NULL REFERENCES barena_cases(case_id) ON DELETE RESTRICT,
  run_id TEXT NOT NULL UNIQUE REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_run_id TEXT NOT NULL REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_trace_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  runtime JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS barena_harness_versions_replay_key_idx
  ON barena_harness_versions
  (COALESCE(owner_user_id,''), case_id, idempotency_key);
CREATE INDEX IF NOT EXISTS barena_harness_versions_owner_created_at_idx
  ON barena_harness_versions (owner_user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS barena_evaluations (
  evaluation_id TEXT PRIMARY KEY,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE RESTRICT,
  harness_version_id TEXT NOT NULL
    REFERENCES barena_harness_versions(harness_version_id) ON DELETE RESTRICT,
  case_id TEXT NOT NULL REFERENCES barena_cases(case_id) ON DELETE RESTRICT,
  run_id TEXT NOT NULL UNIQUE REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_run_id TEXT NOT NULL REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_trace_id TEXT NOT NULL DEFAULT '',
  replay_trace_id TEXT NOT NULL DEFAULT '',
  terminal_event_id TEXT NOT NULL UNIQUE
    REFERENCES barena_engine_events(event_id) ON DELETE RESTRICT,
  package_status TEXT NOT NULL,
  result_status TEXT NOT NULL DEFAULT '',
  decision TEXT NOT NULL CHECK (decision IN ('cleared','held','rejected')),
  summary TEXT NOT NULL DEFAULT '',
  result_ref TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS barena_evaluations_owner_created_at_idx
  ON barena_evaluations (owner_user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS barena_releases (
  release_id TEXT PRIMARY KEY,
  owner_user_id TEXT REFERENCES barena_users(user_id) ON DELETE RESTRICT,
  harness_version_id TEXT NOT NULL
    REFERENCES barena_harness_versions(harness_version_id) ON DELETE RESTRICT,
  evaluation_id TEXT NOT NULL UNIQUE
    REFERENCES barena_evaluations(evaluation_id) ON DELETE RESTRICT,
  case_id TEXT NOT NULL REFERENCES barena_cases(case_id) ON DELETE RESTRICT,
  run_id TEXT NOT NULL UNIQUE REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_run_id TEXT NOT NULL REFERENCES barena_runs(run_id) ON DELETE RESTRICT,
  source_trace_id TEXT NOT NULL DEFAULT '',
  replay_trace_id TEXT NOT NULL DEFAULT '',
  terminal_event_id TEXT NOT NULL
    REFERENCES barena_engine_events(event_id) ON DELETE RESTRICT,
  decision TEXT NOT NULL CHECK (decision IN ('cleared','held','rejected')),
  summary TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS barena_releases_owner_created_at_idx
  ON barena_releases (owner_user_id, created_at DESC);
`)
	return err
}

func (s *PostgresStore) CreateRun(ctx context.Context, run Run) error {
	if run.Origin == "" {
		run.Origin = OriginLocal
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO barena_runs
  (run_id, request_id, owner_user_id, execution_origin, operation, state,
   current_phase, current_actor, input, runtime, cancel_requested, error,
   created_at, updated_at)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		run.ID, run.RequestID, run.OwnerUserID, run.Origin, run.Operation, run.State,
		run.CurrentPhase, run.CurrentActor, run.Input, nullableJSON(run.Runtime),
		run.CancelRequested, run.Error, run.CreatedAt, run.UpdatedAt)
	return mapStoreError(err)
}

func (s *PostgresStore) AdoptScenarioRun(
	ctx context.Context,
	run Run,
	events []EngineEvent,
) (Run, bool, error) {
	if run.Origin != OriginPlatform || !run.State.Terminal() || len(events) == 0 {
		return Run{}, false, ErrConflict
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) || event.Validate(run) != nil {
			return Run{}, false, ErrConflict
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, false, err
	}
	defer tx.Rollback()
	existing, err := getRunTx(ctx, tx, run.ID, true)
	if err == nil {
		existingEvents, listErr := listEventsTx(ctx, tx, run.ID)
		if listErr != nil {
			return Run{}, false, listErr
		}
		if !sameAdoptedRun(existing, run) || !sameEventSlice(existingEvents, events) {
			return Run{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Run{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_runs
  (run_id,request_id,owner_user_id,execution_origin,operation,state,current_phase,
   current_actor,input,runtime,cancel_requested,error,created_at,updated_at)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		run.ID, run.RequestID, run.OwnerUserID, run.Origin, run.Operation, run.State,
		run.CurrentPhase, run.CurrentActor, run.Input, nullableJSON(run.Runtime),
		run.CancelRequested, run.Error, run.CreatedAt, run.UpdatedAt); err != nil {
		return Run{}, false, mapStoreError(err)
	}
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_engine_events
  (run_id,sequence,event_id,timestamp,operation,kind,phase,actor,attempt_id,trace_id,payload)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			event.RunID, event.Sequence, event.EventID, event.Timestamp,
			event.Operation, event.Kind, event.Phase, event.Actor,
			event.AttemptID, event.TraceID, event.Payload); err != nil {
			return Run{}, false, mapStoreError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, err
	}
	return run, true, nil
}

func (s *PostgresStore) CreateRunBundle(
	ctx context.Context,
	bundle RunBundle,
) (RunBundle, bool, error) {
	if err := validateStoredRunBundle(bundle); err != nil {
		return RunBundle{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RunBundle{}, false, err
	}
	defer tx.Rollback()
	// PostgreSQL text values cannot contain NUL bytes. The Bundle ID is already
	// a deterministic SHA-256-derived identity for owner + idempotency key, so
	// it is both collision-resistant and safe to pass through the text protocol.
	lockKey := runBundleAdvisoryLockKey(bundle)
	if _, err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		lockKey,
	); err != nil {
		return RunBundle{}, false, err
	}
	existing, err := scanRunBundle(tx.QueryRowContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,
       terminal_fact,document
FROM barena_run_bundles
WHERE COALESCE(owner_user_id,'')=$1 AND idempotency_key=$2`,
		bundle.OwnerUserID, bundle.IdempotencyKey))
	if err == nil {
		if existing.RequestFingerprint != bundle.RequestFingerprint {
			return RunBundle{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return RunBundle{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_runs
  (run_id,request_id,owner_user_id,execution_origin,operation,state,current_phase,
   current_actor,input,runtime,cancel_requested,error,created_at,updated_at)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		bundle.Run.ID, bundle.Run.RequestID, bundle.Run.OwnerUserID, bundle.Run.Origin,
		bundle.Run.Operation, bundle.Run.State, bundle.Run.CurrentPhase,
		bundle.Run.CurrentActor, bundle.Run.Input, nullableJSON(bundle.Run.Runtime),
		bundle.Run.CancelRequested, bundle.Run.Error, bundle.Run.CreatedAt,
		bundle.Run.UpdatedAt); err != nil {
		return RunBundle{}, false, mapStoreError(err)
	}
	for _, event := range bundle.Events {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_engine_events
  (run_id,sequence,event_id,timestamp,operation,kind,phase,actor,attempt_id,trace_id,payload)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			event.RunID, event.Sequence, event.EventID, event.Timestamp,
			event.Operation, event.Kind, event.Phase, event.Actor,
			event.AttemptID, event.TraceID, event.Payload); err != nil {
			return RunBundle{}, false, mapStoreError(err)
		}
	}
	document, err := json.Marshal(bundle)
	if err != nil {
		return RunBundle{}, false, err
	}
	terminalFact := bundle.Events[len(bundle.Events)-1].Payload
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_run_bundles
  (run_bundle_id,owner_user_id,run_id,idempotency_key,request_fingerprint,
   terminal_fact_sha256,terminal_fact,document,created_at)
VALUES ($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9)`,
		bundle.ID, bundle.OwnerUserID, bundle.Run.ID, bundle.IdempotencyKey,
		bundle.RequestFingerprint, bundle.TerminalFactSHA256, []byte(terminalFact),
		document, bundle.CreatedAt); err != nil {
		return RunBundle{}, false, mapStoreError(err)
	}
	if err := tx.Commit(); err != nil {
		return RunBundle{}, false, err
	}
	return bundle, true, nil
}

func (s *PostgresStore) GetRunBundle(ctx context.Context, id string) (RunBundle, error) {
	return scanRunBundle(s.db.QueryRowContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,
       terminal_fact,document
FROM barena_run_bundles WHERE run_bundle_id=$1`, id))
}

func (s *PostgresStore) GetRun(ctx context.Context, id string) (Run, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT run_id, request_id, COALESCE(owner_user_id,''), execution_origin,
       operation, state, current_phase, current_actor, input,
       COALESCE(runtime, '{}'::jsonb), cancel_requested, error, created_at, updated_at
FROM barena_runs WHERE run_id=$1`, id)
	return scanRun(row)
}

func (s *PostgresStore) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT run_id, request_id, COALESCE(owner_user_id,''), execution_origin,
       operation, state, current_phase, current_actor, input,
       COALESCE(runtime, '{}'::jsonb), cancel_requested, error, created_at, updated_at
FROM barena_runs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *PostgresStore) ListRunsByOwner(ctx context.Context, ownerUserID string, limit int) ([]Run, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT run_id, request_id, COALESCE(owner_user_id,''), execution_origin,
       operation, state, current_phase, current_actor, input,
       COALESCE(runtime, '{}'::jsonb), cancel_requested, error, created_at, updated_at
FROM barena_runs
WHERE owner_user_id=$1
ORDER BY created_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateRun(ctx context.Context, run Run) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE barena_runs
SET state=$2,current_phase=$3,current_actor=$4,cancel_requested=$5,error=$6,updated_at=$7
WHERE run_id=$1`,
		run.ID, run.State, run.CurrentPhase, run.CurrentActor,
		run.CancelRequested, run.Error, run.UpdatedAt)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) AppendEvent(ctx context.Context, event EngineEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var runID string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT run_id FROM barena_runs WHERE run_id=$1 FOR UPDATE`,
		event.RunID,
	).Scan(&runID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var existing EngineEvent
	existing.Schema = "barena.engine_event.v1"
	err = tx.QueryRowContext(ctx, `
SELECT event_id,run_id,sequence,timestamp,operation,kind,phase,actor,attempt_id,trace_id,payload
FROM barena_engine_events
WHERE run_id=$1 AND (event_id=$2 OR sequence=$3)
LIMIT 1`, event.RunID, event.EventID, event.Sequence).Scan(
		&existing.EventID, &existing.RunID, &existing.Sequence,
		&existing.Timestamp, &existing.Operation, &existing.Kind,
		&existing.Phase, &existing.Actor, &existing.AttemptID,
		&existing.TraceID, &existing.Payload,
	)
	if err == nil {
		if sameEvent(existing, event) {
			return nil
		}
		return ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var lastSequence int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(sequence),0)
FROM barena_engine_events
WHERE run_id=$1`, event.RunID).Scan(&lastSequence); err != nil {
		return err
	}
	if event.Sequence != lastSequence+1 {
		return ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_engine_events
  (run_id,sequence,event_id,timestamp,operation,kind,phase,actor,attempt_id,trace_id,payload)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		event.RunID, event.Sequence, event.EventID, event.Timestamp,
		event.Operation, event.Kind, event.Phase, event.Actor,
		event.AttemptID, event.TraceID, event.Payload); err != nil {
		return mapStoreError(err)
	}
	return tx.Commit()
}

func (s *PostgresStore) ListEventsAfter(ctx context.Context, runID string, after int64, limit int) ([]EngineEvent, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT event_id,run_id,sequence,timestamp,operation,kind,phase,actor,attempt_id,trace_id,payload
FROM barena_engine_events
WHERE run_id=$1 AND sequence>$2
ORDER BY sequence ASC LIMIT $3`, runID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]EngineEvent, 0)
	for rows.Next() {
		var event EngineEvent
		event.Schema = "barena.engine_event.v1"
		if err := rows.Scan(
			&event.EventID, &event.RunID, &event.Sequence, &event.Timestamp,
			&event.Operation, &event.Kind, &event.Phase, &event.Actor,
			&event.AttemptID, &event.TraceID, &event.Payload,
		); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *PostgresStore) RunHasTrace(
	ctx context.Context,
	runID string,
	traceID string,
) (bool, error) {
	var input json.RawMessage
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT input FROM barena_runs WHERE run_id=$1`,
		runID,
	).Scan(&input); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM barena_engine_events
  WHERE run_id=$1 AND trace_id=$2
)`, runID, traceID).Scan(&exists); err != nil {
		return false, err
	}
	return exists || runInputHasTrace(input, traceID), nil
}

func (s *PostgresStore) CreateEvolutionJob(
	ctx context.Context,
	job EvolutionJob,
) (EvolutionJob, bool, error) {
	if !validEvolutionJobSource(job) {
		return EvolutionJob{}, false, ErrConflict
	}
	if job.SourceRunID != "" {
		run, err := s.GetRun(ctx, job.SourceRunID)
		if err != nil {
			return EvolutionJob{}, false, err
		}
		if run.OwnerUserID != job.OwnerUserID || !run.State.Terminal() {
			return EvolutionJob{}, false, ErrConflict
		}
		retained, err := s.RunHasTrace(ctx, job.SourceRunID, job.SourceTraceID)
		if err != nil {
			return EvolutionJob{}, false, err
		}
		if !retained {
			return EvolutionJob{}, false, ErrConflict
		}
	}
	document, err := json.Marshal(job)
	if err != nil {
		return EvolutionJob{}, false, err
	}
	created, err := scanEvolutionJob(s.db.QueryRowContext(ctx, `
INSERT INTO spiral_evolution_jobs
  (job_id,owner_user_id,source_run_id,source_trace_id,source_agent_id,idempotency_key,
   request_fingerprint,state,current_stage,document,created_at,updated_at)
SELECT $1,NULLIF($2,''),NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12
WHERE ($3='' AND $4<>'' AND $5='' AND $13='trace') OR EXISTS (
  SELECT 1 FROM barena_runs source
  WHERE source.run_id=$3
    AND $13='run_trace'
    AND COALESCE(source.owner_user_id,'')=$2
    AND source.state IN ('completed','interrupted','cancelled','failed')
) OR ($3='' AND $4='' AND $5<>'' AND $13='agent_trace_set')
ON CONFLICT DO NOTHING
RETURNING COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document`,
		job.ID, job.OwnerUserID, job.SourceRunID, job.SourceTraceID, job.SourceAgentID,
		job.IdempotencyKey, job.RequestFingerprint, job.State,
		job.CurrentStage, document, job.CreatedAt, job.UpdatedAt, job.SourceKind))
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return EvolutionJob{}, false, err
	}
	existing, lookupErr := scanEvolutionJob(s.db.QueryRowContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs
WHERE COALESCE(owner_user_id,'')=$1 AND source_trace_id=$2 AND source_agent_id=$3 AND idempotency_key=$4`,
		job.OwnerUserID, job.SourceTraceID, job.SourceAgentID, job.IdempotencyKey))
	if lookupErr == nil {
		if existing.RequestFingerprint != job.RequestFingerprint {
			return EvolutionJob{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if !errors.Is(lookupErr, ErrNotFound) {
		return EvolutionJob{}, false, lookupErr
	}
	if job.SourceRunID != "" {
		if _, runErr := s.GetRun(ctx, job.SourceRunID); runErr != nil {
			return EvolutionJob{}, false, runErr
		}
	}
	return EvolutionJob{}, false, ErrConflict
}

func (s *PostgresStore) UpdateEvolutionJob(ctx context.Context, job EvolutionJob) error {
	document, err := json.Marshal(job)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE spiral_evolution_jobs
SET state=$8,current_stage=$9,document=$10,updated_at=$11
WHERE job_id=$1
  AND COALESCE(owner_user_id,'')=$2
	  AND COALESCE(source_run_id,'')=$3
  AND source_trace_id=$4
  AND source_agent_id=$5
  AND idempotency_key=$6
  AND request_fingerprint=$7`,
		job.ID, job.OwnerUserID, job.SourceRunID, job.SourceTraceID, job.SourceAgentID,
		job.IdempotencyKey, job.RequestFingerprint, job.State,
		job.CurrentStage, document, job.UpdatedAt)
	if err != nil {
		return mapStoreError(err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *PostgresStore) GetEvolutionJob(
	ctx context.Context,
	id string,
) (EvolutionJob, error) {
	return scanEvolutionJob(s.db.QueryRowContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs WHERE job_id=$1`, id))
}

func (s *PostgresStore) ListEvolutionJobs(
	ctx context.Context,
	limit int,
) ([]EvolutionJob, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvolutionJobs(rows)
}

func (s *PostgresStore) ListEvolutionJobsByOwner(
	ctx context.Context,
	ownerUserID string,
	limit int,
) ([]EvolutionJob, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs
WHERE owner_user_id=$1
ORDER BY created_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvolutionJobs(rows)
}

func (s *PostgresStore) CreateIssue(ctx context.Context, issue Issue) error {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO barena_issues
  (issue_id,owner_user_id,source_run_id,source_trace_id,title,summary,severity,
   status,promoted_case_id,created_at,updated_at)
SELECT $1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9,$10,$11
FROM barena_runs
WHERE run_id=$3 AND COALESCE(owner_user_id,'')=$2`,
		issue.ID, issue.OwnerUserID, issue.SourceRunID, issue.SourceTraceID,
		issue.Title, issue.Summary, issue.Severity, issue.Status,
		issue.PromotedCaseID, issue.CreatedAt, issue.UpdatedAt)
	if err != nil {
		return mapStoreError(err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) GetIssue(ctx context.Context, id string) (Issue, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT issue_id,COALESCE(owner_user_id,''),source_run_id,source_trace_id,title,
       summary,severity,status,promoted_case_id,created_at,updated_at
FROM barena_issues WHERE issue_id=$1`, id)
	return scanIssue(row)
}

func (s *PostgresStore) ListIssues(ctx context.Context, limit int) ([]Issue, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT issue_id,COALESCE(owner_user_id,''),source_run_id,source_trace_id,title,
       summary,severity,status,promoted_case_id,created_at,updated_at
FROM barena_issues ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIssues(rows)
}

func (s *PostgresStore) ListIssuesByOwner(
	ctx context.Context,
	ownerUserID string,
	limit int,
) ([]Issue, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT issue_id,COALESCE(owner_user_id,''),source_run_id,source_trace_id,title,
       summary,severity,status,promoted_case_id,created_at,updated_at
FROM barena_issues
WHERE owner_user_id=$1
ORDER BY created_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIssues(rows)
}

func (s *PostgresStore) PromoteIssue(
	ctx context.Context,
	issueID string,
	promoted Case,
	updatedAt time.Time,
) (Case, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Case{}, false, err
	}
	defer tx.Rollback()

	var issue Issue
	err = tx.QueryRowContext(ctx, `
SELECT issue_id,COALESCE(owner_user_id,''),source_run_id,source_trace_id,title,
       summary,severity,status,promoted_case_id,created_at,updated_at
FROM barena_issues WHERE issue_id=$1 FOR UPDATE`, issueID).Scan(
		&issue.ID, &issue.OwnerUserID, &issue.SourceRunID, &issue.SourceTraceID,
		&issue.Title, &issue.Summary, &issue.Severity, &issue.Status,
		&issue.PromotedCaseID, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Case{}, false, ErrNotFound
	}
	if err != nil {
		return Case{}, false, err
	}
	if issue.Status == IssuePromoted {
		existing, getErr := getCaseTx(ctx, tx, issue.PromotedCaseID)
		return existing, false, getErr
	}
	if issue.Status != IssueOpen ||
		promoted.SourceIssueID != issue.ID ||
		promoted.SourceRunID != issue.SourceRunID ||
		promoted.SourceTraceID != issue.SourceTraceID ||
		promoted.OwnerUserID != issue.OwnerUserID {
		return Case{}, false, ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `
	INSERT INTO barena_cases
	  (case_id,revision,owner_user_id,source_issue_id,source_run_id,source_trace_id,
	   title,operation,input,runtime,replay_prompt,success_criteria,verifier,created_at)
	VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		promoted.ID, promoted.Revision, promoted.OwnerUserID,
		promoted.SourceIssueID, promoted.SourceRunID, promoted.SourceTraceID,
		promoted.Title, promoted.Operation, promoted.Input,
		nullableJSON(promoted.Runtime), promoted.ReplayPrompt, promoted.SuccessCriteria,
		promoted.Verifier, promoted.CreatedAt); err != nil {
		return Case{}, false, mapStoreError(err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE barena_issues
SET status=$2,promoted_case_id=$3,updated_at=$4
WHERE issue_id=$1`,
		issueID, IssuePromoted, promoted.ID, updatedAt); err != nil {
		return Case{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Case{}, false, err
	}
	return promoted, true, nil
}

func (s *PostgresStore) GetCase(ctx context.Context, id string) (Case, error) {
	row := s.db.QueryRowContext(ctx, `
	SELECT case_id,revision,COALESCE(owner_user_id,''),source_issue_id,source_run_id,
	       source_trace_id,title,operation,input,COALESCE(runtime,'{}'::jsonb),
	       replay_prompt,success_criteria,verifier,created_at
FROM barena_cases WHERE case_id=$1`, id)
	return scanCase(row)
}

func (s *PostgresStore) ListCases(ctx context.Context, limit int) ([]Case, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
	SELECT case_id,revision,COALESCE(owner_user_id,''),source_issue_id,source_run_id,
	       source_trace_id,title,operation,input,COALESCE(runtime,'{}'::jsonb),
	       replay_prompt,success_criteria,verifier,created_at
FROM barena_cases ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCases(rows)
}

func (s *PostgresStore) ListCasesByOwner(
	ctx context.Context,
	ownerUserID string,
	limit int,
) ([]Case, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
	SELECT case_id,revision,COALESCE(owner_user_id,''),source_issue_id,source_run_id,
	       source_trace_id,title,operation,input,COALESCE(runtime,'{}'::jsonb),
	       replay_prompt,success_criteria,verifier,created_at
FROM barena_cases
WHERE owner_user_id=$1
ORDER BY created_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCases(rows)
}

func (s *PostgresStore) CreateReplay(
	ctx context.Context,
	run Run,
	harness HarnessVersion,
) (Run, HarnessVersion, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, HarnessVersion{}, false, err
	}
	defer tx.Rollback()
	sourceCase, err := getCaseForUpdateTx(ctx, tx, harness.CaseID)
	if err != nil {
		return Run{}, HarnessVersion{}, false, err
	}
	if run.OwnerUserID != sourceCase.OwnerUserID ||
		harness.OwnerUserID != sourceCase.OwnerUserID ||
		harness.RunID != run.ID ||
		harness.SourceRunID != sourceCase.SourceRunID ||
		harness.SourceTraceID != sourceCase.SourceTraceID ||
		!jsonEquivalent(run.Runtime, sourceCase.Runtime) ||
		!jsonEquivalent(harness.Runtime, sourceCase.Runtime) ||
		run.Operation != OperationReplay ||
		run.Origin != OriginLocal ||
		harness.IdempotencyKey == "" {
		return Run{}, HarnessVersion{}, false, ErrConflict
	}
	existing, err := scanHarnessVersion(tx.QueryRowContext(ctx, `
SELECT harness_version_id,COALESCE(owner_user_id,''),case_id,run_id,source_run_id,
       source_trace_id,idempotency_key,runtime,created_at
FROM barena_harness_versions
WHERE COALESCE(owner_user_id,'')=$1 AND case_id=$2 AND idempotency_key=$3`,
		harness.OwnerUserID, harness.CaseID, harness.IdempotencyKey))
	if err == nil {
		existingRun, getErr := getRunTx(ctx, tx, existing.RunID, false)
		return existingRun, existing, false, getErr
	}
	if !errors.Is(err, ErrNotFound) {
		return Run{}, HarnessVersion{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_runs
  (run_id,request_id,owner_user_id,execution_origin,operation,state,current_phase,
   current_actor,input,runtime,cancel_requested,error,created_at,updated_at)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		run.ID, run.RequestID, run.OwnerUserID, run.Origin, run.Operation, run.State,
		run.CurrentPhase, run.CurrentActor, run.Input, nullableJSON(run.Runtime),
		run.CancelRequested, run.Error, run.CreatedAt, run.UpdatedAt); err != nil {
		return Run{}, HarnessVersion{}, false, mapStoreError(err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_harness_versions
  (harness_version_id,owner_user_id,case_id,run_id,source_run_id,source_trace_id,
   idempotency_key,runtime,created_at)
VALUES ($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9)`,
		harness.ID, harness.OwnerUserID, harness.CaseID, harness.RunID,
		harness.SourceRunID, harness.SourceTraceID, harness.IdempotencyKey,
		jsonOrEmpty(harness.Runtime), harness.CreatedAt); err != nil {
		return Run{}, HarnessVersion{}, false, mapStoreError(err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, HarnessVersion{}, false, err
	}
	return run, harness, true, nil
}

func (s *PostgresStore) GetHarnessVersionByRun(
	ctx context.Context,
	runID string,
) (HarnessVersion, error) {
	return scanHarnessVersion(s.db.QueryRowContext(ctx, `
SELECT harness_version_id,COALESCE(owner_user_id,''),case_id,run_id,source_run_id,
       source_trace_id,idempotency_key,runtime,created_at
FROM barena_harness_versions WHERE run_id=$1`, runID))
}

func (s *PostgresStore) FinalizeReplay(
	ctx context.Context,
	runID string,
	fact ReplayFact,
	now time.Time,
) (Evaluation, Release, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Evaluation{}, Release{}, false, err
	}
	defer tx.Rollback()
	run, err := getRunTx(ctx, tx, runID, true)
	if err != nil {
		return Evaluation{}, Release{}, false, err
	}
	existing, err := getEvaluationByRunTx(ctx, tx, runID)
	if err == nil {
		release, releaseErr := getReleaseByEvaluationTx(ctx, tx, existing.ID)
		if releaseErr == nil && !replayRecordsMatchFact(existing, release, fact) {
			return Evaluation{}, Release{}, false, ErrConflict
		}
		return existing, release, false, releaseErr
	}
	if !errors.Is(err, ErrNotFound) {
		return Evaluation{}, Release{}, false, err
	}
	if run.Operation != OperationReplay ||
		run.Origin != OriginLocal ||
		(run.State != StateQueued && run.State != StateRunning) ||
		!validReplayFact(fact) {
		return Evaluation{}, Release{}, false, ErrConflict
	}
	harness, err := scanHarnessVersion(tx.QueryRowContext(ctx, `
SELECT harness_version_id,COALESCE(owner_user_id,''),case_id,run_id,source_run_id,
       source_trace_id,idempotency_key,runtime,created_at
FROM barena_harness_versions WHERE run_id=$1`, runID))
	if err != nil {
		return Evaluation{}, Release{}, false, err
	}
	sourceCase, err := getCaseTx(ctx, tx, harness.CaseID)
	if err != nil {
		return Evaluation{}, Release{}, false, err
	}
	var terminal EngineEvent
	terminal.Schema = "barena.engine_event.v1"
	err = tx.QueryRowContext(ctx, `
SELECT event_id,run_id,sequence,timestamp,operation,kind,phase,actor,attempt_id,
       trace_id,payload
FROM barena_engine_events WHERE run_id=$1
ORDER BY sequence DESC LIMIT 1`, runID).Scan(
		&terminal.EventID, &terminal.RunID, &terminal.Sequence, &terminal.Timestamp,
		&terminal.Operation, &terminal.Kind, &terminal.Phase, &terminal.Actor,
		&terminal.AttemptID, &terminal.TraceID, &terminal.Payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Evaluation{}, Release{}, false, ErrConflict
	}
	if err != nil {
		return Evaluation{}, Release{}, false, err
	}
	if terminal.EventID != fact.TerminalEventID ||
		terminal.Kind != "terminal" ||
		terminal.Phase != "complete" ||
		terminal.Actor != "engine" ||
		!validOTelTraceID(terminal.TraceID) ||
		terminal.TraceID != fact.ReplayTraceID ||
		!replayEventMatchesFact(terminal, fact) ||
		run.OwnerUserID != harness.OwnerUserID ||
		sourceCase.OwnerUserID != harness.OwnerUserID ||
		sourceCase.SourceRunID != harness.SourceRunID ||
		sourceCase.SourceTraceID != harness.SourceTraceID {
		return Evaluation{}, Release{}, false, ErrConflict
	}
	evaluation := Evaluation{
		ID:               "evaluation-" + runID,
		OwnerUserID:      harness.OwnerUserID,
		HarnessVersionID: harness.ID,
		CaseID:           harness.CaseID,
		RunID:            runID,
		SourceRunID:      harness.SourceRunID,
		SourceTraceID:    harness.SourceTraceID,
		ReplayTraceID:    fact.ReplayTraceID,
		TerminalEventID:  fact.TerminalEventID,
		PackageStatus:    fact.PackageStatus,
		ResultStatus:     fact.ResultStatus,
		Decision:         fact.Decision,
		Summary:          fact.Summary,
		ResultRef:        fact.ResultRef,
		CreatedAt:        now,
	}
	release := Release{
		ID:               "release-" + runID,
		OwnerUserID:      harness.OwnerUserID,
		HarnessVersionID: harness.ID,
		EvaluationID:     evaluation.ID,
		CaseID:           harness.CaseID,
		RunID:            runID,
		SourceRunID:      harness.SourceRunID,
		SourceTraceID:    harness.SourceTraceID,
		ReplayTraceID:    fact.ReplayTraceID,
		TerminalEventID:  fact.TerminalEventID,
		Decision:         fact.Decision,
		Summary:          fact.Summary,
		CreatedAt:        now,
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_evaluations
  (evaluation_id,owner_user_id,harness_version_id,case_id,run_id,source_run_id,
   source_trace_id,replay_trace_id,terminal_event_id,package_status,result_status,
   decision,summary,result_ref,created_at)
VALUES ($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		evaluation.ID, evaluation.OwnerUserID, evaluation.HarnessVersionID,
		evaluation.CaseID, evaluation.RunID, evaluation.SourceRunID,
		evaluation.SourceTraceID, evaluation.ReplayTraceID,
		evaluation.TerminalEventID, evaluation.PackageStatus,
		evaluation.ResultStatus, evaluation.Decision, evaluation.Summary,
		evaluation.ResultRef, evaluation.CreatedAt); err != nil {
		return Evaluation{}, Release{}, false, mapStoreError(err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO barena_releases
  (release_id,owner_user_id,harness_version_id,evaluation_id,case_id,run_id,
   source_run_id,source_trace_id,replay_trace_id,terminal_event_id,decision,
   summary,created_at)
VALUES ($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		release.ID, release.OwnerUserID, release.HarnessVersionID,
		release.EvaluationID, release.CaseID, release.RunID, release.SourceRunID,
		release.SourceTraceID, release.ReplayTraceID, release.TerminalEventID,
		release.Decision, release.Summary, release.CreatedAt); err != nil {
		return Evaluation{}, Release{}, false, mapStoreError(err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE barena_runs
SET state='completed',current_phase='complete',current_actor='engine',updated_at=$2
WHERE run_id=$1`, runID, now); err != nil {
		return Evaluation{}, Release{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Evaluation{}, Release{}, false, err
	}
	return evaluation, release, true, nil
}

func (s *PostgresStore) GetEvaluation(ctx context.Context, id string) (Evaluation, error) {
	return scanEvaluation(s.db.QueryRowContext(ctx, evaluationSelect+`
WHERE evaluation_id=$1`, id))
}

func (s *PostgresStore) ListEvaluations(ctx context.Context, limit int) ([]Evaluation, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, evaluationSelect+`
ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvaluations(rows)
}

func (s *PostgresStore) ListEvaluationsByOwner(
	ctx context.Context,
	ownerUserID string,
	limit int,
) ([]Evaluation, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, evaluationSelect+`
WHERE owner_user_id=$1 ORDER BY created_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvaluations(rows)
}

func (s *PostgresStore) GetRelease(ctx context.Context, id string) (Release, error) {
	return scanRelease(s.db.QueryRowContext(ctx, releaseSelect+`
WHERE release_id=$1`, id))
}

func (s *PostgresStore) ListReleases(ctx context.Context, limit int) ([]Release, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, releaseSelect+`
ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReleases(rows)
}

func (s *PostgresStore) ListReleasesByOwner(
	ctx context.Context,
	ownerUserID string,
	limit int,
) ([]Release, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, releaseSelect+`
WHERE owner_user_id=$1 ORDER BY created_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReleases(rows)
}

func (s *PostgresStore) UpsertUser(ctx context.Context, user User) (User, error) {
	// A fresh Platform project fans one browser batch out into several signed
	// control-plane requests. Every request resolves the same deterministic
	// project principal, so the first requests can race on both unique indexes
	// (user_id and github_id). PostgreSQL's ON CONFLICT target only arbitrates
	// github_id; without serialization, a concurrent insert can still lose on
	// the primary key and make one otherwise valid list call fail.
	//
	// Serialize only this external identity. The transaction-scoped advisory
	// lock is released automatically on commit/rollback and does not block
	// unrelated users or projects.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock($1)`,
		user.GitHubID,
	); err != nil {
		return User{}, err
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO barena_users
  (user_id,github_id,github_login,display_name,avatar_url,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (github_id) DO UPDATE SET
  github_login=EXCLUDED.github_login,
  display_name=EXCLUDED.display_name,
  avatar_url=EXCLUDED.avatar_url,
  updated_at=EXCLUDED.updated_at
RETURNING user_id,github_id,github_login,display_name,avatar_url,created_at,updated_at`,
		user.ID, user.GitHubID, user.Login, user.DisplayName, user.AvatarURL,
		user.CreatedAt, user.UpdatedAt)
	persisted, err := scanUser(row)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return persisted, nil
}

func (s *PostgresStore) GetUserBySessionHash(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT u.user_id,u.github_id,u.github_login,u.display_name,u.avatar_url,u.created_at,u.updated_at
FROM barena_sessions s
JOIN barena_users u ON u.user_id=s.user_id
WHERE s.token_hash=$1 AND s.expires_at>$2`, tokenHash, now)
	return scanUser(row)
}

func (s *PostgresStore) CreateSession(ctx context.Context, session Session) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO barena_sessions (token_hash,user_id,expires_at,created_at)
VALUES ($1,$2,$3,$4)`,
		session.TokenHash, session.UserID, session.ExpiresAt, session.CreatedAt)
	return mapStoreError(err)
}

func (s *PostgresStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM barena_sessions WHERE token_hash=$1`, tokenHash)
	return err
}

func (s *PostgresStore) CreateAPIToken(ctx context.Context, token APIToken) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO barena_api_tokens (token_id,token_hash,encrypted_token,user_id,agent_id,name,created_at)
VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7)`,
		token.ID, token.TokenHash, token.EncryptedToken, token.UserID, token.AgentID, token.Name, token.CreatedAt)
	return mapStoreError(err)
}

func (s *PostgresStore) UpsertEvolutionModelConfig(
	ctx context.Context,
	config EvolutionModelConfig,
) (EvolutionModelConfig, error) {
	row := s.db.QueryRowContext(ctx, `
INSERT INTO catena_evolution_model_configs
  (owner_user_id,provider,base_url,model,encrypted_api_key,updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (owner_user_id) DO UPDATE SET
  provider=EXCLUDED.provider,
  base_url=EXCLUDED.base_url,
  model=EXCLUDED.model,
  encrypted_api_key=EXCLUDED.encrypted_api_key,
  updated_at=EXCLUDED.updated_at
RETURNING owner_user_id,provider,base_url,model,encrypted_api_key,updated_at`,
		config.OwnerUserID, config.Provider, config.BaseURL, config.Model,
		config.EncryptedAPIKey, config.UpdatedAt)
	return scanEvolutionModelConfig(row)
}

func (s *PostgresStore) GetEvolutionModelConfigByOwner(
	ctx context.Context,
	ownerUserID string,
) (EvolutionModelConfig, error) {
	return scanEvolutionModelConfig(s.db.QueryRowContext(ctx, `
SELECT owner_user_id,provider,base_url,model,encrypted_api_key,updated_at
FROM catena_evolution_model_configs WHERE owner_user_id=$1`, ownerUserID))
}

func (s *PostgresStore) DeleteEvolutionModelConfigByOwner(
	ctx context.Context,
	ownerUserID string,
) error {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM catena_evolution_model_configs WHERE owner_user_id=$1`, ownerUserID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanEvolutionModelConfig(row rowScanner) (EvolutionModelConfig, error) {
	var config EvolutionModelConfig
	err := row.Scan(
		&config.OwnerUserID,
		&config.Provider,
		&config.BaseURL,
		&config.Model,
		&config.EncryptedAPIKey,
		&config.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EvolutionModelConfig{}, ErrNotFound
	}
	return config, err
}

func (s *PostgresStore) CreateAgentWithAPIToken(
	ctx context.Context,
	agent RegisteredAgent,
	token APIToken,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `
INSERT INTO catena_agents
  (agent_id,owner_user_id,display_name,runtime_kind,last_seen_at,created_at,updated_at)
VALUES ($1,$2,$3,$4,NULL,$5,$6)`,
		agent.ID, agent.OwnerUserID, agent.DisplayName, agent.RuntimeKind,
		agent.CreatedAt, agent.UpdatedAt); err != nil {
		return mapStoreError(err)
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO barena_api_tokens
  (token_id,token_hash,encrypted_token,user_id,agent_id,name,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		token.ID, token.TokenHash, token.EncryptedToken, token.UserID,
		token.AgentID, token.Name, token.CreatedAt); err != nil {
		return mapStoreError(err)
	}
	return tx.Commit()
}

func (s *PostgresStore) ListRegisteredAgentsByOwner(
	ctx context.Context,
	ownerUserID string,
) ([]RegisteredAgent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT agent_id,owner_user_id,display_name,runtime_kind,last_seen_at,created_at,updated_at
FROM catena_agents WHERE owner_user_id=$1 ORDER BY created_at DESC`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RegisteredAgent, 0)
	for rows.Next() {
		var agent RegisteredAgent
		var lastSeen sql.NullTime
		if err := rows.Scan(&agent.ID, &agent.OwnerUserID, &agent.DisplayName,
			&agent.RuntimeKind, &lastSeen, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			agent.LastSeenAt = lastSeen.Time
		}
		result = append(result, agent)
	}
	return result, rows.Err()
}

func (s *PostgresStore) GetRegisteredAgentByOwner(
	ctx context.Context,
	ownerUserID string,
	agentID string,
) (RegisteredAgent, error) {
	var agent RegisteredAgent
	var lastSeen sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT agent_id,owner_user_id,display_name,runtime_kind,last_seen_at,created_at,updated_at
FROM catena_agents WHERE owner_user_id=$1 AND agent_id=$2`, ownerUserID, agentID).Scan(
		&agent.ID, &agent.OwnerUserID, &agent.DisplayName, &agent.RuntimeKind,
		&lastSeen, &agent.CreatedAt, &agent.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RegisteredAgent{}, ErrNotFound
	}
	if lastSeen.Valid {
		agent.LastSeenAt = lastSeen.Time
	}
	return agent, err
}

func (s *PostgresStore) ObserveRegisteredAgent(
	ctx context.Context,
	ownerUserID string,
	agentID string,
	runtimeKind string,
	seenAt time.Time,
) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE catena_agents SET
  runtime_kind = CASE
    WHEN $3 = '' THEN runtime_kind
    WHEN runtime_kind = '' OR runtime_kind = 'otel' OR $3 <> 'otel' THEN $3
    ELSE runtime_kind
  END,
  last_seen_at = CASE WHEN last_seen_at IS NULL OR last_seen_at < $4 THEN $4 ELSE last_seen_at END,
  updated_at = $4
WHERE owner_user_id=$1 AND agent_id=$2`, ownerUserID, agentID, runtimeKind, seenAt)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListAPITokensByUser(
	ctx context.Context,
	userID string,
) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT token_id,token_hash,encrypted_token,user_id,COALESCE(agent_id,''),name,created_at
FROM barena_api_tokens
WHERE user_id=$1
ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]APIToken, 0)
	for rows.Next() {
		var token APIToken
		if err := rows.Scan(
			&token.ID,
			&token.TokenHash,
			&token.EncryptedToken,
			&token.UserID,
			&token.AgentID,
			&token.Name,
			&token.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, token)
	}
	return result, rows.Err()
}

func (s *PostgresStore) GetAPITokenByUser(
	ctx context.Context,
	userID string,
	tokenID string,
) (APIToken, error) {
	var token APIToken
	err := s.db.QueryRowContext(ctx, `
SELECT token_id,token_hash,encrypted_token,user_id,COALESCE(agent_id,''),name,created_at
FROM barena_api_tokens
WHERE token_id=$1 AND user_id=$2`, tokenID, userID).Scan(
		&token.ID,
		&token.TokenHash,
		&token.EncryptedToken,
		&token.UserID,
		&token.AgentID,
		&token.Name,
		&token.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, ErrNotFound
	}
	return token, err
}

func (s *PostgresStore) GetAPITokenByHash(
	ctx context.Context,
	tokenHash string,
) (APIToken, error) {
	var token APIToken
	err := s.db.QueryRowContext(ctx, `
SELECT token_id,token_hash,encrypted_token,user_id,COALESCE(agent_id,''),name,created_at
FROM barena_api_tokens WHERE token_hash=$1`, tokenHash).Scan(
		&token.ID, &token.TokenHash, &token.EncryptedToken, &token.UserID,
		&token.AgentID, &token.Name, &token.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, ErrNotFound
	}
	return token, err
}

func (s *PostgresStore) GetUserByAPITokenHash(
	ctx context.Context,
	tokenHash string,
) (User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT u.user_id,u.github_id,u.github_login,u.display_name,u.avatar_url,u.created_at,u.updated_at
FROM barena_api_tokens t
JOIN barena_users u ON u.user_id=t.user_id
WHERE t.token_hash=$1`, tokenHash)
	return scanUser(row)
}

func (s *PostgresStore) DeleteAPIToken(
	ctx context.Context,
	userID string,
	tokenID string,
) error {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM barena_api_tokens WHERE token_id=$1 AND user_id=$2`,
		tokenID, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) EnsureAgentProfile(
	ctx context.Context,
	profile AgentProfile,
) (AgentProfile, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO barena_agent_profiles
  (owner_user_id,slug,display_name,bio,is_public,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (owner_user_id) DO NOTHING`,
		profile.OwnerUserID, profile.Slug, profile.DisplayName, profile.Bio,
		profile.IsPublic, profile.CreatedAt, profile.UpdatedAt)
	if err != nil {
		return AgentProfile{}, mapStoreError(err)
	}
	return s.GetAgentProfileByOwner(ctx, profile.OwnerUserID)
}

func (s *PostgresStore) GetAgentProfileByOwner(
	ctx context.Context,
	ownerUserID string,
) (AgentProfile, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT owner_user_id,slug,display_name,bio,is_public,created_at,updated_at
FROM barena_agent_profiles WHERE owner_user_id=$1`, ownerUserID)
	return scanAgentProfile(row)
}

func (s *PostgresStore) UpdateAgentProfile(
	ctx context.Context,
	profile AgentProfile,
) (AgentProfile, error) {
	row := s.db.QueryRowContext(ctx, `
UPDATE barena_agent_profiles
SET display_name=$2,bio=$3,is_public=$4,updated_at=$5
WHERE owner_user_id=$1
RETURNING owner_user_id,slug,display_name,bio,is_public,created_at,updated_at`,
		profile.OwnerUserID, profile.DisplayName, profile.Bio,
		profile.IsPublic, profile.UpdatedAt)
	return scanAgentProfile(row)
}

func (s *PostgresStore) ListPublicAgentProfiles(
	ctx context.Context,
	limit int,
) ([]ProfileRecord, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT p.owner_user_id,p.slug,p.display_name,p.bio,p.is_public,p.created_at,p.updated_at,
       u.user_id,u.github_id,u.github_login,u.display_name,u.avatar_url,u.created_at,u.updated_at
FROM barena_agent_profiles p
JOIN barena_users u ON u.user_id=p.owner_user_id
WHERE p.is_public=TRUE
ORDER BY p.updated_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ProfileRecord, 0)
	for rows.Next() {
		record, err := scanProfileRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *PostgresStore) GetPublicAgentProfile(
	ctx context.Context,
	slug string,
) (ProfileRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT p.owner_user_id,p.slug,p.display_name,p.bio,p.is_public,p.created_at,p.updated_at,
       u.user_id,u.github_id,u.github_login,u.display_name,u.avatar_url,u.created_at,u.updated_at
FROM barena_agent_profiles p
JOIN barena_users u ON u.user_id=p.owner_user_id
WHERE p.slug=$1 AND p.is_public=TRUE`, slug)
	return scanProfileRecord(row)
}

func (s *PostgresStore) InterruptActiveRuns(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE barena_runs
SET state='interrupted',updated_at=$1
WHERE state IN ('queued','running')`, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *PostgresStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *PostgresStore) Close()                         { _ = s.db.Close() }

type rowScanner interface {
	Scan(...any) error
}

func scanRun(row rowScanner) (Run, error) {
	var run Run
	if err := row.Scan(
		&run.ID, &run.RequestID, &run.OwnerUserID, &run.Origin,
		&run.Operation, &run.State, &run.CurrentPhase, &run.CurrentActor,
		&run.Input, &run.Runtime, &run.CancelRequested, &run.Error,
		&run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	return run, nil
}

func scanIssue(row rowScanner) (Issue, error) {
	var issue Issue
	if err := row.Scan(
		&issue.ID, &issue.OwnerUserID, &issue.SourceRunID, &issue.SourceTraceID,
		&issue.Title, &issue.Summary, &issue.Severity, &issue.Status,
		&issue.PromotedCaseID, &issue.CreatedAt, &issue.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, err
	}
	return issue, nil
}

func scanIssues(rows *sql.Rows) ([]Issue, error) {
	result := make([]Issue, 0)
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, issue)
	}
	return result, rows.Err()
}

func scanCase(row rowScanner) (Case, error) {
	var value Case
	value.Schema = "barena.case.v1"
	if err := row.Scan(
		&value.ID, &value.Revision, &value.OwnerUserID,
		&value.SourceIssueID, &value.SourceRunID, &value.SourceTraceID,
		&value.Title, &value.Operation, &value.Input, &value.Runtime,
		&value.ReplayPrompt, &value.SuccessCriteria, &value.Verifier, &value.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Case{}, ErrNotFound
		}
		return Case{}, err
	}
	return value, nil
}

func scanCases(rows *sql.Rows) ([]Case, error) {
	result := make([]Case, 0)
	for rows.Next() {
		value, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanEvolutionJob(row rowScanner) (EvolutionJob, error) {
	var ownerUserID string
	var idempotencyKey string
	var requestFingerprint string
	var document json.RawMessage
	if err := row.Scan(
		&ownerUserID,
		&idempotencyKey,
		&requestFingerprint,
		&document,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EvolutionJob{}, ErrNotFound
		}
		return EvolutionJob{}, err
	}
	var job EvolutionJob
	if err := json.Unmarshal(document, &job); err != nil {
		return EvolutionJob{}, err
	}
	job.OwnerUserID = ownerUserID
	job.IdempotencyKey = idempotencyKey
	job.RequestFingerprint = requestFingerprint
	return job, nil
}

func scanRunBundle(row rowScanner) (RunBundle, error) {
	var ownerUserID string
	var idempotencyKey string
	var requestFingerprint string
	var terminalFact []byte
	var document json.RawMessage
	if err := row.Scan(
		&ownerUserID,
		&idempotencyKey,
		&requestFingerprint,
		&terminalFact,
		&document,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunBundle{}, ErrNotFound
		}
		return RunBundle{}, err
	}
	var bundle RunBundle
	if err := json.Unmarshal(document, &bundle); err != nil {
		return RunBundle{}, err
	}
	bundle.OwnerUserID = ownerUserID
	bundle.IdempotencyKey = idempotencyKey
	bundle.RequestFingerprint = requestFingerprint
	if len(bundle.Events) == 0 {
		return RunBundle{}, ErrConflict
	}
	bundle.Run.OwnerUserID = ownerUserID
	bundle.Events[len(bundle.Events)-1].Payload = cloneJSON(terminalFact)
	if err := validateStoredRunBundle(bundle); err != nil {
		return RunBundle{}, err
	}
	return bundle, nil
}

func scanEvolutionJobs(rows *sql.Rows) ([]EvolutionJob, error) {
	result := make([]EvolutionJob, 0)
	for rows.Next() {
		job, err := scanEvolutionJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

func getCaseTx(ctx context.Context, tx *sql.Tx, id string) (Case, error) {
	row := tx.QueryRowContext(ctx, `
	SELECT case_id,revision,COALESCE(owner_user_id,''),source_issue_id,source_run_id,
	       source_trace_id,title,operation,input,COALESCE(runtime,'{}'::jsonb),
	       replay_prompt,success_criteria,verifier,created_at
FROM barena_cases WHERE case_id=$1`, id)
	return scanCase(row)
}

func getCaseForUpdateTx(ctx context.Context, tx *sql.Tx, id string) (Case, error) {
	row := tx.QueryRowContext(ctx, `
SELECT case_id,revision,COALESCE(owner_user_id,''),source_issue_id,source_run_id,
       source_trace_id,title,operation,input,COALESCE(runtime,'{}'::jsonb),
       replay_prompt,success_criteria,verifier,created_at
FROM barena_cases WHERE case_id=$1 FOR UPDATE`, id)
	return scanCase(row)
}

func getRunTx(ctx context.Context, tx *sql.Tx, id string, forUpdate bool) (Run, error) {
	query := `
SELECT run_id,request_id,COALESCE(owner_user_id,''),execution_origin,operation,
       state,current_phase,current_actor,input,COALESCE(runtime,'{}'::jsonb),
       cancel_requested,error,created_at,updated_at
FROM barena_runs WHERE run_id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanRun(tx.QueryRowContext(ctx, query, id))
}

func listEventsTx(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
) ([]EngineEvent, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT event_id,run_id,sequence,timestamp,operation,kind,phase,actor,attempt_id,
       trace_id,payload
FROM barena_engine_events
WHERE run_id=$1
ORDER BY sequence ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]EngineEvent, 0)
	for rows.Next() {
		var event EngineEvent
		event.Schema = "barena.engine_event.v1"
		if err := rows.Scan(
			&event.EventID, &event.RunID, &event.Sequence, &event.Timestamp,
			&event.Operation, &event.Kind, &event.Phase, &event.Actor,
			&event.AttemptID, &event.TraceID, &event.Payload,
		); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func scanHarnessVersion(row rowScanner) (HarnessVersion, error) {
	var value HarnessVersion
	if err := row.Scan(
		&value.ID, &value.OwnerUserID, &value.CaseID, &value.RunID,
		&value.SourceRunID, &value.SourceTraceID, &value.IdempotencyKey,
		&value.Runtime, &value.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HarnessVersion{}, ErrNotFound
		}
		return HarnessVersion{}, err
	}
	return value, nil
}

const evaluationSelect = `
SELECT evaluation_id,COALESCE(owner_user_id,''),harness_version_id,case_id,run_id,
       source_run_id,source_trace_id,replay_trace_id,terminal_event_id,
       package_status,result_status,decision,summary,result_ref,created_at
FROM barena_evaluations
`

func scanEvaluation(row rowScanner) (Evaluation, error) {
	var value Evaluation
	if err := row.Scan(
		&value.ID, &value.OwnerUserID, &value.HarnessVersionID, &value.CaseID,
		&value.RunID, &value.SourceRunID, &value.SourceTraceID,
		&value.ReplayTraceID, &value.TerminalEventID, &value.PackageStatus,
		&value.ResultStatus, &value.Decision, &value.Summary, &value.ResultRef,
		&value.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Evaluation{}, ErrNotFound
		}
		return Evaluation{}, err
	}
	return value, nil
}

func scanEvaluations(rows *sql.Rows) ([]Evaluation, error) {
	result := make([]Evaluation, 0)
	for rows.Next() {
		value, err := scanEvaluation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func getEvaluationByRunTx(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
) (Evaluation, error) {
	return scanEvaluation(tx.QueryRowContext(ctx, evaluationSelect+`
WHERE run_id=$1`, runID))
}

const releaseSelect = `
SELECT release_id,COALESCE(owner_user_id,''),harness_version_id,evaluation_id,
       case_id,run_id,source_run_id,source_trace_id,replay_trace_id,
       terminal_event_id,decision,summary,created_at
FROM barena_releases
`

func scanRelease(row rowScanner) (Release, error) {
	var value Release
	if err := row.Scan(
		&value.ID, &value.OwnerUserID, &value.HarnessVersionID,
		&value.EvaluationID, &value.CaseID, &value.RunID, &value.SourceRunID,
		&value.SourceTraceID, &value.ReplayTraceID, &value.TerminalEventID,
		&value.Decision, &value.Summary, &value.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Release{}, ErrNotFound
		}
		return Release{}, err
	}
	return value, nil
}

func scanReleases(rows *sql.Rows) ([]Release, error) {
	result := make([]Release, 0)
	for rows.Next() {
		value, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func getReleaseByEvaluationTx(
	ctx context.Context,
	tx *sql.Tx,
	evaluationID string,
) (Release, error) {
	return scanRelease(tx.QueryRowContext(ctx, releaseSelect+`
WHERE evaluation_id=$1`, evaluationID))
}

func scanUser(row rowScanner) (User, error) {
	var user User
	if err := row.Scan(
		&user.ID, &user.GitHubID, &user.Login, &user.DisplayName,
		&user.AvatarURL, &user.CreatedAt, &user.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return user, nil
}

func scanAgentProfile(row rowScanner) (AgentProfile, error) {
	var profile AgentProfile
	if err := row.Scan(
		&profile.OwnerUserID, &profile.Slug, &profile.DisplayName,
		&profile.Bio, &profile.IsPublic, &profile.CreatedAt, &profile.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentProfile{}, ErrNotFound
		}
		return AgentProfile{}, err
	}
	return profile, nil
}

func scanProfileRecord(row rowScanner) (ProfileRecord, error) {
	var record ProfileRecord
	if err := row.Scan(
		&record.Profile.OwnerUserID, &record.Profile.Slug,
		&record.Profile.DisplayName, &record.Profile.Bio,
		&record.Profile.IsPublic, &record.Profile.CreatedAt,
		&record.Profile.UpdatedAt, &record.User.ID, &record.User.GitHubID,
		&record.User.Login, &record.User.DisplayName, &record.User.AvatarURL,
		&record.User.CreatedAt, &record.User.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProfileRecord{}, ErrNotFound
		}
		return ProfileRecord{}, err
	}
	return record, nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func jsonOrEmpty(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	// PostgreSQL integrity errors are deliberately collapsed at this boundary.
	if len(err.Error()) > 0 && (contains(err.Error(), "duplicate key") || contains(err.Error(), "unique constraint")) {
		return ErrConflict
	}
	return err
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
