package control

import (
	"context"
	"database/sql"
	"embed"
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
	// Bound connection growth when HTTP authentication and workers compete.
	// Pool waits obey each operation's context; never open one connection per request.
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
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

//go:embed migrations/*.sql
var migrationFiles embed.FS

func (s *PostgresStore) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1943287103)`); err != nil {
		return err
	}
	core, _ := migrationFiles.ReadFile("migrations/001_core.sql")
	if _, err = tx.ExecContext(ctx, string(core)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS catena_schema_migrations(version INTEGER PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	var applied bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catena_schema_migrations WHERE version=2)`).Scan(&applied); err != nil {
		return err
	}
	if !applied {
		sql, _ := migrationFiles.ReadFile("migrations/002_v2.sql")
		if _, err = tx.ExecContext(ctx, string(sql)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catena_schema_migrations(version) VALUES(2)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PostgresStore) CreateAnalysisJob(
	ctx context.Context,
	job AnalysisJob,
) (AnalysisJob, bool, error) {
	if !validAnalysisJobSource(job) {
		return AnalysisJob{}, false, ErrConflict
	}

	document, err := json.Marshal(job)
	if err != nil {
		return AnalysisJob{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AnalysisJob{}, false, err
	}
	defer tx.Rollback()
	// Serialize only admission, not execution. Idempotent retries remain valid
	// even when the queue is full. The fixed lock key is shared by all replicas.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1943287102)`); err != nil {
		return AnalysisJob{}, false, err
	}
	existing, lookupErr := scanAnalysisJob(tx.QueryRowContext(ctx, `SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs WHERE COALESCE(owner_user_id,'')=$1 AND source_trace_id=$2 AND source_agent_id=$3 AND idempotency_key=$4`,
		job.OwnerUserID, job.SourceTraceID, job.SourceAgentID, job.IdempotencyKey))
	if lookupErr == nil {
		if existing.RequestFingerprint != job.RequestFingerprint {
			return AnalysisJob{}, false, ErrConflict
		}
		return existing, false, tx.Commit()
	}
	if !errors.Is(lookupErr, ErrNotFound) {
		return AnalysisJob{}, false, lookupErr
	}
	var active, owned int
	if err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE COALESCE(owner_user_id,'')=$1) FROM spiral_evolution_jobs WHERE state IN ('queued','running')`, job.OwnerUserID).Scan(&active, &owned); err != nil {
		return AnalysisJob{}, false, err
	}
	if active >= analysisQueueCapacity || owned >= analysisOwnerQueueCapacity {
		return AnalysisJob{}, false, ErrQueueFull
	}
	created, err := scanAnalysisJob(tx.QueryRowContext(ctx, `
INSERT INTO spiral_evolution_jobs
  (job_id,owner_user_id,source_run_id,source_trace_id,source_agent_id,idempotency_key,
   request_fingerprint,state,current_stage,document,created_at,updated_at)
SELECT $1,NULLIF($2,''),NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12
WHERE ($3='' AND $4<>'' AND $5='' AND $13='trace') OR ($3='' AND $4='' AND $5<>'' AND $13 IN ('agent_trace_set','conversation'))
ON CONFLICT DO NOTHING
RETURNING COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document`,
		job.ID, job.OwnerUserID, "", job.SourceTraceID, job.SourceAgentID,
		job.IdempotencyKey, job.RequestFingerprint, job.State,
		job.CurrentStage, document, job.CreatedAt, job.UpdatedAt, job.SourceKind))
	if err == nil {
		return created, true, tx.Commit()
	}
	if !errors.Is(err, ErrNotFound) {
		return AnalysisJob{}, false, err
	}
	existing, lookupErr = scanAnalysisJob(tx.QueryRowContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs
WHERE COALESCE(owner_user_id,'')=$1 AND source_trace_id=$2 AND source_agent_id=$3 AND idempotency_key=$4`,
		job.OwnerUserID, job.SourceTraceID, job.SourceAgentID, job.IdempotencyKey))
	if lookupErr == nil {
		if existing.RequestFingerprint != job.RequestFingerprint {
			return AnalysisJob{}, false, ErrConflict
		}
		return existing, false, tx.Commit()
	}
	if !errors.Is(lookupErr, ErrNotFound) {
		return AnalysisJob{}, false, lookupErr
	}
	return AnalysisJob{}, false, ErrConflict
}

func (s *PostgresStore) UpdateAnalysisJob(ctx context.Context, job AnalysisJob) error {
	document, err := json.Marshal(job)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE spiral_evolution_jobs
SET state=$8,current_stage=$9,document=$10,updated_at=$11
WHERE job_id=$1
  AND fencing_token=0
  AND COALESCE(owner_user_id,'')=$2
	  AND COALESCE(source_run_id,'')=$3
  AND source_trace_id=$4
  AND source_agent_id=$5
  AND idempotency_key=$6
  AND request_fingerprint=$7`,
		job.ID, job.OwnerUserID, "", job.SourceTraceID, job.SourceAgentID,
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

func (s *PostgresStore) GetAnalysisJob(
	ctx context.Context,
	id string,
) (AnalysisJob, error) {
	return scanAnalysisJob(s.db.QueryRowContext(ctx, `
SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document
FROM spiral_evolution_jobs WHERE job_id=$1`, id))
}

func (s *PostgresStore) ListAnalysisJobs(
	ctx context.Context,
	limit int,
) ([]AnalysisJob, error) {
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
	return scanAnalysisJobs(rows)
}

func (s *PostgresStore) ListAnalysisJobsByOwner(
	ctx context.Context,
	ownerUserID string,
	limit int,
) ([]AnalysisJob, error) {
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
	return scanAnalysisJobs(rows)
}

func (s *PostgresStore) DeleteAnalysisJob(
	ctx context.Context,
	ownerUserID string,
	id string,
) error {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM spiral_evolution_jobs
WHERE job_id=$1
  AND COALESCE(owner_user_id,'')=$2
  AND state IN ('completed','failed')`, id, ownerUserID)
	if err != nil {
		return mapStoreError(err)
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	job, lookupErr := s.GetAnalysisJob(ctx, id)
	if lookupErr != nil || job.OwnerUserID != ownerUserID {
		return ErrNotFound
	}
	if !job.State.Terminal() {
		return ErrConflict
	}
	return ErrNotFound
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

func (s *PostgresStore) UpsertAnalysisModelConfig(
	ctx context.Context,
	config AnalysisModelConfig,
) (AnalysisModelConfig, error) {
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
	return scanAnalysisModelConfig(row)
}

func (s *PostgresStore) GetAnalysisModelConfigByOwner(
	ctx context.Context,
	ownerUserID string,
) (AnalysisModelConfig, error) {
	return scanAnalysisModelConfig(s.db.QueryRowContext(ctx, `
SELECT owner_user_id,provider,base_url,model,encrypted_api_key,updated_at
FROM catena_evolution_model_configs WHERE owner_user_id=$1`, ownerUserID))
}

func (s *PostgresStore) DeleteAnalysisModelConfigByOwner(
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

func scanAnalysisModelConfig(row rowScanner) (AnalysisModelConfig, error) {
	var config AnalysisModelConfig
	err := row.Scan(
		&config.OwnerUserID,
		&config.Provider,
		&config.BaseURL,
		&config.Model,
		&config.EncryptedAPIKey,
		&config.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AnalysisModelConfig{}, ErrNotFound
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

func (s *PostgresStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *PostgresStore) Close()                         { _ = s.db.Close() }

type rowScanner interface {
	Scan(...any) error
}

func scanAnalysisJob(row rowScanner) (AnalysisJob, error) {
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
			return AnalysisJob{}, ErrNotFound
		}
		return AnalysisJob{}, err
	}
	var job AnalysisJob
	if err := json.Unmarshal(document, &job); err != nil {
		return AnalysisJob{}, err
	}
	job.OwnerUserID = ownerUserID
	job.IdempotencyKey = idempotencyKey
	job.RequestFingerprint = requestFingerprint
	return job, nil
}

func scanAnalysisJobs(rows *sql.Rows) ([]AnalysisJob, error) {
	result := make([]AnalysisJob, 0)
	for rows.Next() {
		job, err := scanAnalysisJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

const evaluationSelect = `
SELECT evaluation_id,COALESCE(owner_user_id,''),harness_version_id,case_id,run_id,
       source_run_id,source_trace_id,replay_trace_id,terminal_event_id,
       package_status,result_status,decision,summary,result_ref,created_at
FROM barena_evaluations
`

const releaseSelect = `
SELECT release_id,COALESCE(owner_user_id,''),harness_version_id,evaluation_id,
       case_id,run_id,source_run_id,source_trace_id,replay_trace_id,
       terminal_event_id,decision,summary,created_at
FROM barena_releases
`

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
