package control

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
	CreateEvolutionJob(context.Context, EvolutionJob) (EvolutionJob, bool, error)
	UpdateEvolutionJob(context.Context, EvolutionJob) error
	GetEvolutionJob(context.Context, string) (EvolutionJob, error)
	ListEvolutionJobs(context.Context, int) ([]EvolutionJob, error)
	ListEvolutionJobsByOwner(context.Context, string, int) ([]EvolutionJob, error)
	DeleteEvolutionJob(context.Context, string, string) error
	ClaimEvolutionJob(context.Context, string, time.Duration, int) (EvolutionJob, EvolutionLease, error)
	HeartbeatEvolutionJob(context.Context, EvolutionLease, time.Duration) (bool, error)
	SaveLeasedEvolutionJob(context.Context, EvolutionLease, EvolutionJob) error
	ReleaseEvolutionJob(context.Context, EvolutionLease, string, bool, time.Duration, int) error
	CancelEvolutionJob(context.Context, string, string) (EvolutionJob, error)
	EvolutionQueueStats(context.Context, *string) (EvolutionQueueStats, error)
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
	UpsertEvolutionModelConfig(context.Context, EvolutionModelConfig) (EvolutionModelConfig, error)
	GetEvolutionModelConfigByOwner(context.Context, string) (EvolutionModelConfig, error)
	DeleteEvolutionModelConfigByOwner(context.Context, string) error
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
	mu                    sync.RWMutex
	users                 map[string]User
	userByGitHubID        map[int64]string
	sessions              map[string]Session
	apiTokens             map[string]APIToken
	registeredAgents      map[string]RegisteredAgent
	profiles              map[string]AgentProfile
	evolutionJobs         map[string]EvolutionJob
	evolutionLeases       map[string]EvolutionLease
	conversationMessages  map[string]ConversationMessage
	evolutionModelConfigs map[string]EvolutionModelConfig
	memoryTasks           map[string]MemoryTaskRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:                 make(map[string]User),
		userByGitHubID:        make(map[int64]string),
		sessions:              make(map[string]Session),
		apiTokens:             make(map[string]APIToken),
		registeredAgents:      make(map[string]RegisteredAgent),
		profiles:              make(map[string]AgentProfile),
		evolutionJobs:         make(map[string]EvolutionJob),
		evolutionLeases:       make(map[string]EvolutionLease),
		conversationMessages:  make(map[string]ConversationMessage),
		evolutionModelConfigs: make(map[string]EvolutionModelConfig),
		memoryTasks:           make(map[string]MemoryTaskRecord),
	}
}

func (s *MemoryStore) UpsertEvolutionModelConfig(
	_ context.Context,
	config EvolutionModelConfig,
) (EvolutionModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(config.OwnerUserID) == "" || config.EncryptedAPIKey == "" {
		return EvolutionModelConfig{}, ErrConflict
	}
	s.evolutionModelConfigs[config.OwnerUserID] = config
	return config, nil
}

func (s *MemoryStore) GetEvolutionModelConfigByOwner(
	_ context.Context,
	ownerUserID string,
) (EvolutionModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config, ok := s.evolutionModelConfigs[ownerUserID]
	if !ok {
		return EvolutionModelConfig{}, ErrNotFound
	}
	return config, nil
}

func (s *MemoryStore) DeleteEvolutionModelConfigByOwner(
	_ context.Context,
	ownerUserID string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.evolutionModelConfigs[ownerUserID]; !ok {
		return ErrNotFound
	}
	delete(s.evolutionModelConfigs, ownerUserID)
	return nil
}

func (s *MemoryStore) CreateEvolutionJob(
	_ context.Context,
	job EvolutionJob,
) (EvolutionJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validEvolutionJobSource(job) {
		return EvolutionJob{}, false, ErrConflict
	}

	for _, existing := range s.evolutionJobs {
		if existing.OwnerUserID == job.OwnerUserID &&
			evolutionJobSourceKey(existing) == evolutionJobSourceKey(job) &&
			existing.IdempotencyKey == job.IdempotencyKey {
			if existing.RequestFingerprint != job.RequestFingerprint {
				return EvolutionJob{}, false, ErrConflict
			}
			return cloneEvolutionJob(existing), false, nil
		}
	}
	active, owned := 0, 0
	for _, existing := range s.evolutionJobs {
		if !existing.State.Terminal() {
			active++
			if existing.OwnerUserID == job.OwnerUserID {
				owned++
			}
		}
	}
	if active >= evolutionQueueCapacity || owned >= evolutionOwnerQueueCapacity {
		return EvolutionJob{}, false, ErrQueueFull
	}
	if _, exists := s.evolutionJobs[job.ID]; exists {
		return EvolutionJob{}, false, ErrConflict
	}
	s.evolutionJobs[job.ID] = cloneEvolutionJob(job)
	return cloneEvolutionJob(job), true, nil
}

func (s *MemoryStore) UpdateEvolutionJob(_ context.Context, job EvolutionJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, exists := s.evolutionJobs[job.ID]
	if !exists {
		return ErrNotFound
	}
	if s.evolutionLeases[job.ID].Fence != 0 {
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
	s.evolutionJobs[job.ID] = cloneEvolutionJob(job)
	return nil
}

func (s *MemoryStore) GetEvolutionJob(_ context.Context, id string) (EvolutionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, exists := s.evolutionJobs[id]
	if !exists {
		return EvolutionJob{}, ErrNotFound
	}
	return cloneEvolutionJob(job), nil
}

func (s *MemoryStore) ListEvolutionJobs(
	_ context.Context,
	limit int,
) ([]EvolutionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]EvolutionJob, 0, len(s.evolutionJobs))
	for _, job := range s.evolutionJobs {
		result = append(result, cloneEvolutionJob(job))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListEvolutionJobsByOwner(
	_ context.Context,
	ownerUserID string,
	limit int,
) ([]EvolutionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]EvolutionJob, 0)
	for _, job := range s.evolutionJobs {
		if job.OwnerUserID == ownerUserID {
			result = append(result, cloneEvolutionJob(job))
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

func (s *MemoryStore) DeleteEvolutionJob(
	_ context.Context,
	ownerUserID string,
	id string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.evolutionJobs[id]
	if !exists || job.OwnerUserID != ownerUserID {
		return ErrNotFound
	}
	if !job.State.Terminal() {
		return ErrConflict
	}
	delete(s.evolutionJobs, id)
	return nil
}

func cloneEvolutionJob(input EvolutionJob) EvolutionJob {
	ownerUserID := input.OwnerUserID
	idempotencyKey := input.IdempotencyKey
	requestFingerprint := input.RequestFingerprint
	encoded, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	var output EvolutionJob
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

func jsonEquivalent(left, right json.RawMessage) bool {
	if len(left) == 0 {
		left = json.RawMessage(`{}`)
	}
	if len(right) == 0 {
		right = json.RawMessage(`{}`)
	}
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil &&
		json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}
