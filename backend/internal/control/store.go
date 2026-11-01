package control

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrLeaseLost = errors.New("job lease lost")
	ErrQueueFull = errors.New("analysis queue capacity reached")
)

type Store interface {
	ListArtifactsByOwner(context.Context, string, string, int) ([]Artifact, error)
	IngestConversationMessages(context.Context, string, []ConversationMessage, time.Time) (int, int, error)
	ListConversationSummariesByOwner(context.Context, string, string, int) ([]ConversationSummary, error)
	ListConversationMessagesByOwner(context.Context, string, string, string, int) ([]ConversationMessage, error)
	CreateAnalysisJob(context.Context, AnalysisJob) (AnalysisJob, bool, error)
	UpdateAnalysisJob(context.Context, AnalysisJob) error
	GetAnalysisJob(context.Context, string) (AnalysisJob, error)
	ListAnalysisJobs(context.Context, int) ([]AnalysisJob, error)
	ListAnalysisJobsByOwner(context.Context, string, int) ([]AnalysisJob, error)
	DeleteAnalysisJob(context.Context, string, string) error
	ClaimAnalysisJob(context.Context, string, time.Duration, int) (AnalysisJob, AnalysisLease, error)
	HeartbeatAnalysisJob(context.Context, AnalysisLease, time.Duration) (bool, error)
	SaveLeasedAnalysisJob(context.Context, AnalysisLease, AnalysisJob) error
	ReleaseAnalysisJob(context.Context, AnalysisLease, string, bool, time.Duration, int) error
	CancelAnalysisJob(context.Context, string, string) (AnalysisJob, error)
	AnalysisQueueStats(context.Context, *string) (AnalysisQueueStats, error)
	UpsertUser(context.Context, User) (User, error)
	GetUserBySessionHash(context.Context, string, time.Time) (User, error)
	CreateSession(context.Context, Session) error
	DeleteSession(context.Context, string) error
	CreateAPIToken(context.Context, APIToken) error
	CreateAgentWithAPIToken(context.Context, RegisteredAgent, APIToken) error
	ListRegisteredAgentsByOwner(context.Context, string) ([]RegisteredAgent, error)
	GetRegisteredAgentByOwner(context.Context, string, string) (RegisteredAgent, error)
	ObserveRegisteredAgent(context.Context, string, string, string, time.Time) error
	ListAPITokensByUser(context.Context, string) ([]APIToken, error)
	GetAPITokenByUser(context.Context, string, string) (APIToken, error)
	GetAPITokenByHash(context.Context, string) (APIToken, error)
	GetUserByAPITokenHash(context.Context, string) (User, error)
	DeleteAPIToken(context.Context, string, string) error
	UpsertAnalysisModelConfig(context.Context, AnalysisModelConfig) (AnalysisModelConfig, error)
	GetAnalysisModelConfigByOwner(context.Context, string) (AnalysisModelConfig, error)
	DeleteAnalysisModelConfigByOwner(context.Context, string) error
	UpsertMemoryTask(context.Context, MemoryTaskRecord) error
	GetMemoryTaskByOwner(context.Context, string, string) (MemoryTaskRecord, error)
	ListMemoryTasksByOwner(context.Context, string, int) ([]MemoryTaskRecord, error)
	EnsureAgentProfile(context.Context, AgentProfile) (AgentProfile, error)
	GetAgentProfileByOwner(context.Context, string) (AgentProfile, error)
	UpdateAgentProfile(context.Context, AgentProfile) (AgentProfile, error)
	ListPublicAgentProfiles(context.Context, int) ([]ProfileRecord, error)
	GetPublicAgentProfile(context.Context, string) (ProfileRecord, error)
	Ping(context.Context) error
	Close()
}

type MemoryStore struct {
	mu                   sync.RWMutex
	users                map[string]User
	userByGitHubID       map[int64]string
	sessions             map[string]Session
	apiTokens            map[string]APIToken
	registeredAgents     map[string]RegisteredAgent
	profiles             map[string]AgentProfile
	analysisJobs         map[string]AnalysisJob
	analysisLeases       map[string]AnalysisLease
	conversationMessages map[string]ConversationMessage
	analysisModelConfigs map[string]AnalysisModelConfig
	memoryTasks          map[string]MemoryTaskRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:                make(map[string]User),
		userByGitHubID:       make(map[int64]string),
		sessions:             make(map[string]Session),
		apiTokens:            make(map[string]APIToken),
		registeredAgents:     make(map[string]RegisteredAgent),
		profiles:             make(map[string]AgentProfile),
		analysisJobs:         make(map[string]AnalysisJob),
		analysisLeases:       make(map[string]AnalysisLease),
		conversationMessages: make(map[string]ConversationMessage),
		analysisModelConfigs: make(map[string]AnalysisModelConfig),
		memoryTasks:          make(map[string]MemoryTaskRecord),
	}
}

func (s *MemoryStore) UpsertAnalysisModelConfig(
	_ context.Context,
	config AnalysisModelConfig,
) (AnalysisModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(config.OwnerUserID) == "" || config.EncryptedAPIKey == "" {
		return AnalysisModelConfig{}, ErrConflict
	}
	s.analysisModelConfigs[config.OwnerUserID] = config
	return config, nil
}

func (s *MemoryStore) GetAnalysisModelConfigByOwner(
	_ context.Context,
	ownerUserID string,
) (AnalysisModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config, ok := s.analysisModelConfigs[ownerUserID]
	if !ok {
		return AnalysisModelConfig{}, ErrNotFound
	}
	return config, nil
}

func (s *MemoryStore) DeleteAnalysisModelConfigByOwner(
	_ context.Context,
	ownerUserID string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.analysisModelConfigs[ownerUserID]; !ok {
		return ErrNotFound
	}
	delete(s.analysisModelConfigs, ownerUserID)
	return nil
}

func (s *MemoryStore) CreateAnalysisJob(
	_ context.Context,
	job AnalysisJob,
) (AnalysisJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validAnalysisJobSource(job) {
		return AnalysisJob{}, false, ErrConflict
	}

	for _, existing := range s.analysisJobs {
		if existing.OwnerUserID == job.OwnerUserID &&
			analysisJobSourceKey(existing) == analysisJobSourceKey(job) &&
			existing.IdempotencyKey == job.IdempotencyKey {
			if existing.RequestFingerprint != job.RequestFingerprint {
				return AnalysisJob{}, false, ErrConflict
			}
			return cloneAnalysisJob(existing), false, nil
		}
	}
	active, owned := 0, 0
	for _, existing := range s.analysisJobs {
		if !existing.State.Terminal() {
			active++
			if existing.OwnerUserID == job.OwnerUserID {
				owned++
			}
		}
	}
	if active >= analysisQueueCapacity || owned >= analysisOwnerQueueCapacity {
		return AnalysisJob{}, false, ErrQueueFull
	}
	if _, exists := s.analysisJobs[job.ID]; exists {
		return AnalysisJob{}, false, ErrConflict
	}
	s.analysisJobs[job.ID] = cloneAnalysisJob(job)
	return cloneAnalysisJob(job), true, nil
}

func (s *MemoryStore) UpdateAnalysisJob(_ context.Context, job AnalysisJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, exists := s.analysisJobs[job.ID]
	if !exists {
		return ErrNotFound
	}
	if s.analysisLeases[job.ID].Fence != 0 {
		return ErrLeaseLost
	}
	if existing.OwnerUserID != job.OwnerUserID ||
		existing.SourceTraceID != job.SourceTraceID ||
		existing.SourceAgentID != job.SourceAgentID ||
		!equalStringSlices(existing.SourceTraceIDs, job.SourceTraceIDs) ||
		existing.IdempotencyKey != job.IdempotencyKey ||
		existing.RequestFingerprint != job.RequestFingerprint {
		return ErrConflict
	}
	s.analysisJobs[job.ID] = cloneAnalysisJob(job)
	return nil
}

func (s *MemoryStore) GetAnalysisJob(_ context.Context, id string) (AnalysisJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, exists := s.analysisJobs[id]
	if !exists {
		return AnalysisJob{}, ErrNotFound
	}
	return cloneAnalysisJob(job), nil
}

func (s *MemoryStore) ListAnalysisJobs(
	_ context.Context,
	limit int,
) ([]AnalysisJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]AnalysisJob, 0, len(s.analysisJobs))
	for _, job := range s.analysisJobs {
		result = append(result, cloneAnalysisJob(job))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListAnalysisJobsByOwner(
	_ context.Context,
	ownerUserID string,
	limit int,
) ([]AnalysisJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]AnalysisJob, 0)
	for _, job := range s.analysisJobs {
		if job.OwnerUserID == ownerUserID {
			result = append(result, cloneAnalysisJob(job))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) DeleteAnalysisJob(
	_ context.Context,
	ownerUserID string,
	id string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.analysisJobs[id]
	if !exists || job.OwnerUserID != ownerUserID {
		return ErrNotFound
	}
	if !job.State.Terminal() {
		return ErrConflict
	}
	delete(s.analysisJobs, id)
	return nil
}

func cloneAnalysisJob(input AnalysisJob) AnalysisJob {
	ownerUserID := input.OwnerUserID
	idempotencyKey := input.IdempotencyKey
	requestFingerprint := input.RequestFingerprint
	encoded, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	var output AnalysisJob
	if err := json.Unmarshal(encoded, &output); err != nil {
		panic(err)
	}
	output.OwnerUserID = ownerUserID
	output.IdempotencyKey = idempotencyKey
	output.RequestFingerprint = requestFingerprint
	return output
}

func (s *MemoryStore) UpsertUser(_ context.Context, user User) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID := s.userByGitHubID[user.GitHubID]; existingID != "" {
		existing := s.users[existingID]
		existing.Login = user.Login
		existing.DisplayName = user.DisplayName
		existing.AvatarURL = user.AvatarURL
		existing.UpdatedAt = user.UpdatedAt
		s.users[existingID] = existing
		return existing, nil
	}
	if user.ID == "" || user.GitHubID < 1 {
		return User{}, ErrConflict
	}
	s.users[user.ID] = user
	s.userByGitHubID[user.GitHubID] = user.ID
	return user, nil
}

func (s *MemoryStore) GetUserBySessionHash(_ context.Context, tokenHash string, now time.Time) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, exists := s.sessions[tokenHash]
	if !exists || !session.ExpiresAt.After(now) {
		return User{}, ErrNotFound
	}
	user, exists := s.users[session.UserID]
	if !exists {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (s *MemoryStore) CreateSession(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[session.TokenHash]; exists {
		return ErrConflict
	}
	if _, exists := s.users[session.UserID]; !exists {
		return ErrNotFound
	}
	s.sessions[session.TokenHash] = session
	return nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, tokenHash)
	return nil
}

func (s *MemoryStore) CreateAPIToken(_ context.Context, token APIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[token.UserID]; !exists {
		return ErrNotFound
	}
	if token.ID == "" || token.TokenHash == "" {
		return ErrConflict
	}
	for _, existing := range s.apiTokens {
		if existing.ID == token.ID || existing.TokenHash == token.TokenHash ||
			(token.AgentID != "" && existing.AgentID == token.AgentID) {
			return ErrConflict
		}
	}
	if token.AgentID != "" {
		agent, exists := s.registeredAgents[token.AgentID]
		if !exists || agent.OwnerUserID != token.UserID {
			return ErrNotFound
		}
	}
	s.apiTokens[token.TokenHash] = token
	return nil
}

func (s *MemoryStore) CreateAgentWithAPIToken(
	_ context.Context,
	agent RegisteredAgent,
	token APIToken,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[agent.OwnerUserID]; !exists || agent.ID == "" ||
		token.AgentID != agent.ID || token.UserID != agent.OwnerUserID || token.TokenHash == "" {
		return ErrConflict
	}
	if _, exists := s.registeredAgents[agent.ID]; exists {
		return ErrConflict
	}
	for _, existing := range s.apiTokens {
		if existing.ID == token.ID || existing.TokenHash == token.TokenHash ||
			(existing.AgentID != "" && existing.AgentID == agent.ID) {
			return ErrConflict
		}
	}
	s.registeredAgents[agent.ID] = agent
	s.apiTokens[token.TokenHash] = token
	return nil
}

func (s *MemoryStore) ListRegisteredAgentsByOwner(
	_ context.Context,
	ownerUserID string,
) ([]RegisteredAgent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]RegisteredAgent, 0)
	for _, agent := range s.registeredAgents {
		if agent.OwnerUserID == ownerUserID {
			result = append(result, agent)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *MemoryStore) GetRegisteredAgentByOwner(
	_ context.Context,
	ownerUserID string,
	agentID string,
) (RegisteredAgent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, exists := s.registeredAgents[agentID]
	if !exists || agent.OwnerUserID != ownerUserID {
		return RegisteredAgent{}, ErrNotFound
	}
	return agent, nil
}

func (s *MemoryStore) ObserveRegisteredAgent(
	_ context.Context,
	ownerUserID string,
	agentID string,
	runtimeKind string,
	seenAt time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	agent, exists := s.registeredAgents[agentID]
	if !exists || agent.OwnerUserID != ownerUserID {
		return ErrNotFound
	}
	if shouldReplaceRuntimeKind(agent.RuntimeKind, runtimeKind) {
		agent.RuntimeKind = runtimeKind
	}
	if seenAt.After(agent.LastSeenAt) {
		agent.LastSeenAt = seenAt
	}
	agent.UpdatedAt = seenAt
	s.registeredAgents[agentID] = agent
	return nil
}

func (s *MemoryStore) ListAPITokensByUser(
	_ context.Context,
	userID string,
) ([]APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]APIToken, 0)
	for _, token := range s.apiTokens {
		if token.UserID == userID {
			result = append(result, token)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (s *MemoryStore) GetAPITokenByUser(
	_ context.Context,
	userID string,
	tokenID string,
) (APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, token := range s.apiTokens {
		if token.ID == tokenID && token.UserID == userID {
			return token, nil
		}
	}
	return APIToken{}, ErrNotFound
}

func (s *MemoryStore) GetAPITokenByHash(
	_ context.Context,
	tokenHash string,
) (APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, exists := s.apiTokens[tokenHash]
	if !exists {
		return APIToken{}, ErrNotFound
	}
	return token, nil
}

func (s *MemoryStore) GetUserByAPITokenHash(
	_ context.Context,
	tokenHash string,
) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, exists := s.apiTokens[tokenHash]
	if !exists {
		return User{}, ErrNotFound
	}
	user, exists := s.users[token.UserID]
	if !exists {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (s *MemoryStore) DeleteAPIToken(
	_ context.Context,
	userID string,
	tokenID string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, token := range s.apiTokens {
		if token.ID == tokenID && token.UserID == userID {
			delete(s.apiTokens, hash)
			return nil
		}
	}
	return ErrNotFound
}

func (s *MemoryStore) EnsureAgentProfile(_ context.Context, profile AgentProfile) (AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.profiles[profile.OwnerUserID]; ok {
		return existing, nil
	}
	for _, existing := range s.profiles {
		if existing.Slug == profile.Slug {
			return AgentProfile{}, ErrConflict
		}
	}
	s.profiles[profile.OwnerUserID] = profile
	return profile, nil
}

func (s *MemoryStore) GetAgentProfileByOwner(_ context.Context, ownerUserID string) (AgentProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile, ok := s.profiles[ownerUserID]
	if !ok {
		return AgentProfile{}, ErrNotFound
	}
	return profile, nil
}

func (s *MemoryStore) UpdateAgentProfile(_ context.Context, profile AgentProfile) (AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.profiles[profile.OwnerUserID]
	if !ok {
		return AgentProfile{}, ErrNotFound
	}
	profile.Slug = existing.Slug
	profile.CreatedAt = existing.CreatedAt
	s.profiles[profile.OwnerUserID] = profile
	return profile, nil
}

func (s *MemoryStore) ListPublicAgentProfiles(_ context.Context, limit int) ([]ProfileRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ProfileRecord, 0)
	for ownerID, profile := range s.profiles {
		if !profile.IsPublic {
			continue
		}
		result = append(result, ProfileRecord{Profile: profile, User: s.users[ownerID]})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Profile.UpdatedAt.After(result[j].Profile.UpdatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) GetPublicAgentProfile(_ context.Context, slug string) (ProfileRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ownerID, profile := range s.profiles {
		if profile.IsPublic && profile.Slug == slug {
			return ProfileRecord{Profile: profile, User: s.users[ownerID]}, nil
		}
	}
	return ProfileRecord{}, ErrNotFound
}

func (s *MemoryStore) Ping(context.Context) error { return nil }
func (s *MemoryStore) Close()                     {}
