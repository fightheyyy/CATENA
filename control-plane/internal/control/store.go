package control

import (
	"bytes"
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
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Store interface {
	CreateRun(context.Context, Run) error
	AdoptScenarioRun(context.Context, Run, []EngineEvent) (Run, bool, error)
	GetRun(context.Context, string) (Run, error)
	ListRuns(context.Context, int) ([]Run, error)
	ListRunsByOwner(context.Context, string, int) ([]Run, error)
	UpdateRun(context.Context, Run) error
	AppendEvent(context.Context, EngineEvent) error
	ListEventsAfter(context.Context, string, int64, int) ([]EngineEvent, error)
	RunHasTrace(context.Context, string, string) (bool, error)
	CreateEvolutionJob(context.Context, EvolutionJob) (EvolutionJob, bool, error)
	UpdateEvolutionJob(context.Context, EvolutionJob) error
	GetEvolutionJob(context.Context, string) (EvolutionJob, error)
	ListEvolutionJobs(context.Context, int) ([]EvolutionJob, error)
	ListEvolutionJobsByOwner(context.Context, string, int) ([]EvolutionJob, error)
	CreateIssue(context.Context, Issue) error
	GetIssue(context.Context, string) (Issue, error)
	ListIssues(context.Context, int) ([]Issue, error)
	ListIssuesByOwner(context.Context, string, int) ([]Issue, error)
	PromoteIssue(context.Context, string, Case, time.Time) (Case, bool, error)
	GetCase(context.Context, string) (Case, error)
	ListCases(context.Context, int) ([]Case, error)
	ListCasesByOwner(context.Context, string, int) ([]Case, error)
	CreateReplay(context.Context, Run, HarnessVersion) (Run, HarnessVersion, bool, error)
	GetHarnessVersionByRun(context.Context, string) (HarnessVersion, error)
	FinalizeReplay(context.Context, string, ReplayFact, time.Time) (Evaluation, Release, bool, error)
	GetEvaluation(context.Context, string) (Evaluation, error)
	ListEvaluations(context.Context, int) ([]Evaluation, error)
	ListEvaluationsByOwner(context.Context, string, int) ([]Evaluation, error)
	GetRelease(context.Context, string) (Release, error)
	ListReleases(context.Context, int) ([]Release, error)
	ListReleasesByOwner(context.Context, string, int) ([]Release, error)
	UpsertUser(context.Context, User) (User, error)
	GetUserBySessionHash(context.Context, string, time.Time) (User, error)
	CreateSession(context.Context, Session) error
	DeleteSession(context.Context, string) error
	CreateAPIToken(context.Context, APIToken) error
	ListAPITokensByUser(context.Context, string) ([]APIToken, error)
	GetUserByAPITokenHash(context.Context, string) (User, error)
	DeleteAPIToken(context.Context, string, string) error
	EnsureAgentProfile(context.Context, AgentProfile) (AgentProfile, error)
	GetAgentProfileByOwner(context.Context, string) (AgentProfile, error)
	UpdateAgentProfile(context.Context, AgentProfile) (AgentProfile, error)
	ListPublicAgentProfiles(context.Context, int) ([]ProfileRecord, error)
	GetPublicAgentProfile(context.Context, string) (ProfileRecord, error)
	InterruptActiveRuns(context.Context) (int64, error)
	Ping(context.Context) error
	Close()
}

type MemoryStore struct {
	mu              sync.RWMutex
	runs            map[string]Run
	events          map[string][]EngineEvent
	users           map[string]User
	userByGitHubID  map[int64]string
	sessions        map[string]Session
	apiTokens       map[string]APIToken
	profiles        map[string]AgentProfile
	issues          map[string]Issue
	cases           map[string]Case
	harnessVersions map[string]HarnessVersion
	evaluations     map[string]Evaluation
	releases        map[string]Release
	evolutionJobs   map[string]EvolutionJob
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		runs:            make(map[string]Run),
		events:          make(map[string][]EngineEvent),
		users:           make(map[string]User),
		userByGitHubID:  make(map[int64]string),
		sessions:        make(map[string]Session),
		apiTokens:       make(map[string]APIToken),
		profiles:        make(map[string]AgentProfile),
		issues:          make(map[string]Issue),
		cases:           make(map[string]Case),
		harnessVersions: make(map[string]HarnessVersion),
		evaluations:     make(map[string]Evaluation),
		releases:        make(map[string]Release),
		evolutionJobs:   make(map[string]EvolutionJob),
	}
}

func (s *MemoryStore) CreateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return ErrConflict
	}
	if run.Origin == "" {
		run.Origin = OriginLocal
	}
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryStore) AdoptScenarioRun(
	_ context.Context,
	run Run,
	events []EngineEvent,
) (Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.runs[run.ID]; exists {
		if !sameAdoptedRun(existing, run) || !sameEventSlice(s.events[run.ID], events) {
			return Run{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if run.Origin != OriginPlatform || !run.State.Terminal() || len(events) == 0 {
		return Run{}, false, ErrConflict
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) || event.Validate(run) != nil {
			return Run{}, false, ErrConflict
		}
	}
	s.runs[run.ID] = run
	s.events[run.ID] = append([]EngineEvent(nil), events...)
	return run, true, nil
}

func (s *MemoryStore) GetRun(_ context.Context, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, exists := s.runs[id]
	if !exists {
		return Run{}, ErrNotFound
	}
	return run, nil
}

func (s *MemoryStore) ListRuns(_ context.Context, limit int) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runs := make([]Run, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (s *MemoryStore) ListRunsByOwner(_ context.Context, ownerUserID string, limit int) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runs := make([]Run, 0)
	for _, run := range s.runs {
		if run.OwnerUserID == ownerUserID {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (s *MemoryStore) UpdateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; !exists {
		return ErrNotFound
	}
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryStore) AppendEvent(_ context.Context, event EngineEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.events[event.RunID]
	for _, existing := range events {
		if existing.EventID == event.EventID || existing.Sequence == event.Sequence {
			if sameEvent(existing, event) {
				return nil
			}
			return ErrConflict
		}
	}
	if event.Sequence != int64(len(events)+1) {
		return ErrConflict
	}
	s.events[event.RunID] = append(events, event)
	return nil
}

func (s *MemoryStore) ListEventsAfter(_ context.Context, runID string, after int64, limit int) ([]EngineEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, exists := s.runs[runID]; !exists {
		return nil, ErrNotFound
	}
	result := make([]EngineEvent, 0)
	for _, event := range s.events[runID] {
		if event.Sequence > after {
			result = append(result, event)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) RunHasTrace(_ context.Context, runID string, traceID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, exists := s.runs[runID]; !exists {
		return false, ErrNotFound
	}
	for _, event := range s.events[runID] {
		if event.TraceID == traceID {
			return true, nil
		}
	}
	return false, nil
}

func (s *MemoryStore) CreateEvolutionJob(
	_ context.Context,
	job EvolutionJob,
) (EvolutionJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[job.SourceRunID]
	if !exists {
		return EvolutionJob{}, false, ErrNotFound
	}
	if run.OwnerUserID != job.OwnerUserID || !run.State.Terminal() {
		return EvolutionJob{}, false, ErrConflict
	}
	traceFound := false
	for _, event := range s.events[job.SourceRunID] {
		if event.TraceID == job.SourceTraceID {
			traceFound = true
			break
		}
	}
	if !traceFound {
		return EvolutionJob{}, false, ErrConflict
	}
	for _, existing := range s.evolutionJobs {
		if existing.OwnerUserID == job.OwnerUserID &&
			existing.SourceRunID == job.SourceRunID &&
			existing.IdempotencyKey == job.IdempotencyKey {
			if existing.RequestFingerprint != job.RequestFingerprint {
				return EvolutionJob{}, false, ErrConflict
			}
			return cloneEvolutionJob(existing), false, nil
		}
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
	if existing.OwnerUserID != job.OwnerUserID ||
		existing.SourceRunID != job.SourceRunID ||
		existing.SourceTraceID != job.SourceTraceID ||
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

func (s *MemoryStore) CreateIssue(_ context.Context, issue Issue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.issues[issue.ID]; exists {
		return ErrConflict
	}
	run, exists := s.runs[issue.SourceRunID]
	if !exists {
		return ErrNotFound
	}
	if run.OwnerUserID != issue.OwnerUserID {
		return ErrConflict
	}
	s.issues[issue.ID] = issue
	return nil
}

func (s *MemoryStore) GetIssue(_ context.Context, id string) (Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	issue, exists := s.issues[id]
	if !exists {
		return Issue{}, ErrNotFound
	}
	return issue, nil
}

func (s *MemoryStore) ListIssues(_ context.Context, limit int) ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Issue, 0, len(s.issues))
	for _, issue := range s.issues {
		result = append(result, issue)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListIssuesByOwner(
	_ context.Context,
	ownerUserID string,
	limit int,
) ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Issue, 0)
	for _, issue := range s.issues {
		if issue.OwnerUserID == ownerUserID {
			result = append(result, issue)
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

func (s *MemoryStore) PromoteIssue(
	_ context.Context,
	issueID string,
	promoted Case,
	updatedAt time.Time,
) (Case, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	issue, exists := s.issues[issueID]
	if !exists {
		return Case{}, false, ErrNotFound
	}
	if issue.Status == IssuePromoted {
		existing, ok := s.cases[issue.PromotedCaseID]
		if !ok {
			return Case{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if issue.Status != IssueOpen ||
		promoted.SourceIssueID != issue.ID ||
		promoted.SourceRunID != issue.SourceRunID ||
		promoted.SourceTraceID != issue.SourceTraceID ||
		promoted.OwnerUserID != issue.OwnerUserID {
		return Case{}, false, ErrConflict
	}
	if _, exists := s.cases[promoted.ID]; exists {
		return Case{}, false, ErrConflict
	}
	s.cases[promoted.ID] = promoted
	issue.Status = IssuePromoted
	issue.PromotedCaseID = promoted.ID
	issue.UpdatedAt = updatedAt
	s.issues[issue.ID] = issue
	return promoted, true, nil
}

func (s *MemoryStore) GetCase(_ context.Context, id string) (Case, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.cases[id]
	if !exists {
		return Case{}, ErrNotFound
	}
	return value, nil
}

func (s *MemoryStore) ListCases(_ context.Context, limit int) ([]Case, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Case, 0, len(s.cases))
	for _, value := range s.cases {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListCasesByOwner(
	_ context.Context,
	ownerUserID string,
	limit int,
) ([]Case, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Case, 0)
	for _, value := range s.cases {
		if value.OwnerUserID == ownerUserID {
			result = append(result, value)
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

func (s *MemoryStore) CreateReplay(
	_ context.Context,
	run Run,
	harness HarnessVersion,
) (Run, HarnessVersion, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sourceCase, exists := s.cases[harness.CaseID]
	if !exists {
		return Run{}, HarnessVersion{}, false, ErrNotFound
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
	for _, existing := range s.harnessVersions {
		if existing.OwnerUserID == harness.OwnerUserID &&
			existing.CaseID == harness.CaseID &&
			existing.IdempotencyKey == harness.IdempotencyKey {
			existingRun, ok := s.runs[existing.RunID]
			if !ok {
				return Run{}, HarnessVersion{}, false, ErrConflict
			}
			return existingRun, existing, false, nil
		}
	}
	if _, exists := s.runs[run.ID]; exists {
		return Run{}, HarnessVersion{}, false, ErrConflict
	}
	if _, exists := s.harnessVersions[harness.ID]; exists {
		return Run{}, HarnessVersion{}, false, ErrConflict
	}
	s.runs[run.ID] = run
	s.harnessVersions[harness.ID] = harness
	return run, harness, true, nil
}

func (s *MemoryStore) GetHarnessVersionByRun(
	_ context.Context,
	runID string,
) (HarnessVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.harnessVersions {
		if value.RunID == runID {
			return value, nil
		}
	}
	return HarnessVersion{}, ErrNotFound
}

func (s *MemoryStore) FinalizeReplay(
	_ context.Context,
	runID string,
	fact ReplayFact,
	now time.Time,
) (Evaluation, Release, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.evaluations {
		if existing.RunID == runID {
			for _, release := range s.releases {
				if release.EvaluationID == existing.ID {
					if !replayRecordsMatchFact(existing, release, fact) {
						return Evaluation{}, Release{}, false, ErrConflict
					}
					return existing, release, false, nil
				}
			}
			return Evaluation{}, Release{}, false, ErrConflict
		}
	}
	run, exists := s.runs[runID]
	if !exists {
		return Evaluation{}, Release{}, false, ErrNotFound
	}
	if run.Operation != OperationReplay ||
		run.Origin != OriginLocal ||
		(run.State != StateQueued && run.State != StateRunning) ||
		!validReplayFact(fact) {
		return Evaluation{}, Release{}, false, ErrConflict
	}
	var harness HarnessVersion
	for _, value := range s.harnessVersions {
		if value.RunID == runID {
			harness = value
			break
		}
	}
	if harness.ID == "" {
		return Evaluation{}, Release{}, false, ErrNotFound
	}
	sourceCase, exists := s.cases[harness.CaseID]
	if !exists {
		return Evaluation{}, Release{}, false, ErrConflict
	}
	events := s.events[runID]
	if len(events) == 0 {
		return Evaluation{}, Release{}, false, ErrConflict
	}
	terminal := events[len(events)-1]
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
	s.evaluations[evaluation.ID] = evaluation
	s.releases[release.ID] = release
	run.State = StateCompleted
	run.CurrentPhase = "complete"
	run.CurrentActor = "engine"
	run.UpdatedAt = now
	s.runs[run.ID] = run
	return evaluation, release, true, nil
}

func (s *MemoryStore) GetEvaluation(_ context.Context, id string) (Evaluation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.evaluations[id]
	if !exists {
		return Evaluation{}, ErrNotFound
	}
	return value, nil
}

func (s *MemoryStore) ListEvaluations(_ context.Context, limit int) ([]Evaluation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Evaluation, 0, len(s.evaluations))
	for _, value := range s.evaluations {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListEvaluationsByOwner(
	_ context.Context,
	ownerUserID string,
	limit int,
) ([]Evaluation, error) {
	values, err := s.ListEvaluations(context.Background(), 0)
	if err != nil {
		return nil, err
	}
	result := make([]Evaluation, 0)
	for _, value := range values {
		if value.OwnerUserID == ownerUserID {
			result = append(result, value)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) GetRelease(_ context.Context, id string) (Release, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.releases[id]
	if !exists {
		return Release{}, ErrNotFound
	}
	return value, nil
}

func (s *MemoryStore) ListReleases(_ context.Context, limit int) ([]Release, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Release, 0, len(s.releases))
	for _, value := range s.releases {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListReleasesByOwner(
	_ context.Context,
	ownerUserID string,
	limit int,
) ([]Release, error) {
	values, err := s.ListReleases(context.Background(), 0)
	if err != nil {
		return nil, err
	}
	result := make([]Release, 0)
	for _, value := range values {
		if value.OwnerUserID == ownerUserID {
			result = append(result, value)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
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
		if existing.ID == token.ID || existing.TokenHash == token.TokenHash {
			return ErrConflict
		}
	}
	s.apiTokens[token.TokenHash] = token
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

func (s *MemoryStore) InterruptActiveRuns(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for id, run := range s.runs {
		if run.State == StateQueued || run.State == StateRunning {
			run.State = StateInterrupted
			run.UpdatedAt = time.Now().UTC()
			s.runs[id] = run
			count++
		}
	}
	return count, nil
}

func (s *MemoryStore) Ping(context.Context) error { return nil }
func (s *MemoryStore) Close()                     {}

func sameEvent(left, right EngineEvent) bool {
	return left.Schema == right.Schema &&
		left.EventID == right.EventID &&
		left.RunID == right.RunID &&
		left.Sequence == right.Sequence &&
		left.Timestamp.Equal(right.Timestamp) &&
		left.Operation == right.Operation &&
		left.Kind == right.Kind &&
		left.Phase == right.Phase &&
		left.Actor == right.Actor &&
		left.AttemptID == right.AttemptID &&
		left.TraceID == right.TraceID &&
		bytes.Equal(left.Payload, right.Payload)
}

func sameEventSlice(left, right []EngineEvent) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameEvent(left[index], right[index]) {
			return false
		}
	}
	return true
}

func sameAdoptedRun(left, right Run) bool {
	return left.ID == right.ID &&
		left.RequestID == right.RequestID &&
		left.OwnerUserID == right.OwnerUserID &&
		left.Origin == right.Origin &&
		left.Operation == right.Operation &&
		left.State == right.State &&
		left.CurrentPhase == right.CurrentPhase &&
		left.CurrentActor == right.CurrentActor &&
		jsonEquivalent(left.Input, right.Input) &&
		jsonEquivalent(left.Runtime, right.Runtime) &&
		left.CancelRequested == right.CancelRequested &&
		left.Error == right.Error &&
		left.CreatedAt.Equal(right.CreatedAt) &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}

func replayRecordsMatchFact(
	evaluation Evaluation,
	release Release,
	fact ReplayFact,
) bool {
	return evaluation.TerminalEventID == fact.TerminalEventID &&
		evaluation.ReplayTraceID == fact.ReplayTraceID &&
		evaluation.PackageStatus == fact.PackageStatus &&
		evaluation.ResultStatus == fact.ResultStatus &&
		evaluation.Decision == fact.Decision &&
		evaluation.Summary == fact.Summary &&
		evaluation.ResultRef == fact.ResultRef &&
		release.TerminalEventID == fact.TerminalEventID &&
		release.ReplayTraceID == fact.ReplayTraceID &&
		release.Decision == fact.Decision &&
		release.Summary == fact.Summary
}

func replayEventMatchesFact(event EngineEvent, fact ReplayFact) bool {
	var payload struct {
		Status       string          `json:"status"`
		ResultStatus string          `json:"result_status"`
		Decision     ReleaseDecision `json:"decision"`
		Summary      string          `json:"summary"`
		ResultRef    string          `json:"result_ref"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil {
		return false
	}
	return payload.Status == fact.PackageStatus &&
		payload.ResultStatus == fact.ResultStatus &&
		payload.Decision == fact.Decision &&
		payload.Summary == fact.Summary &&
		payload.ResultRef == fact.ResultRef
}

func validReplayFact(fact ReplayFact) bool {
	return fact.TerminalEventID != "" &&
		validOTelTraceID(fact.ReplayTraceID) &&
		fact.PackageStatus == "complete" &&
		strings.TrimSpace(fact.ResultStatus) != "" &&
		len(fact.ResultStatus) <= 128 &&
		fact.Decision.Valid() &&
		strings.TrimSpace(fact.Summary) != "" &&
		len(fact.Summary) <= 12000 &&
		fact.ResultRef != ""
}

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
