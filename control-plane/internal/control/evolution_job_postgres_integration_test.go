package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPostgresEvolutionJobPersistenceAndIdempotency(t *testing.T) {
	databaseURL := os.Getenv("BARENA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("BARENA_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	store, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	run := Run{
		ID:        newID("evolution-pg-run"),
		RequestID: newID("evolution-pg-request"),
		Origin:    OriginPlatform,
		Operation: OperationExplore,
		State:     StateCompleted,
		Input:     json.RawMessage(`{"scenario":{"objective":"persistent evolution"}}`),
		Runtime:   json.RawMessage(`{"runtime":"xiaobaos"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	traceID := newID("evolution-pg-trace")
	if err := store.AppendEvent(ctx, EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   run.ID + ".1",
		RunID:     run.ID,
		Sequence:  1,
		Timestamp: now,
		Operation: run.Operation,
		Kind:      "terminal",
		Phase:     "complete",
		Actor:     "engine",
		TraceID:   traceID,
		Payload:   json.RawMessage(`{"status":"complete"}`),
	}); err != nil {
		t.Fatal(err)
	}
	job := EvolutionJob{
		Schema:             evolutionJobSchema,
		ID:                 newID("evolution-pg-job"),
		SourceKind:         EvolutionSourceRunTrace,
		SourceRunID:        run.ID,
		SourceTraceID:      traceID,
		Objective:          "Persist one candidate proposal.",
		IdempotencyKey:     newID("evolution-pg-key"),
		RequestFingerprint: newID("evolution-pg-fingerprint"),
		State:              EvolutionJobQueued,
		Stages:             cloneEvolutionStages(evolutionJobStages),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	createdJob, created, err := store.CreateEvolutionJob(ctx, job)
	if err != nil || !created || createdJob.ID != job.ID {
		t.Fatalf("CreateEvolutionJob failed: job=%#v created=%v err=%v", createdJob, created, err)
	}
	retry := job
	retry.ID = newID("must-not-create-evolution-job")
	duplicate, created, err := store.CreateEvolutionJob(ctx, retry)
	if err != nil || created || duplicate.ID != job.ID {
		t.Fatalf("Evolution Job retry was not idempotent: job=%#v created=%v err=%v", duplicate, created, err)
	}
	mutated := retry
	mutated.RequestFingerprint = newID("mutated-fingerprint")
	if _, _, err := store.CreateEvolutionJob(ctx, mutated); !errors.Is(err, ErrConflict) {
		t.Fatalf("mutated idempotency request should conflict, got %v", err)
	}

	job.State = EvolutionJobCompleted
	job.CurrentStage = "complete"
	job.Finding = &EvolutionFinding{
		Title: "Persistent finding", Summary: "Stored as one aggregate.",
		Severity: "medium", Evidence: []string{"trace retained"},
	}
	job.Candidate = &EvolutionCandidate{
		ID: newID("candidate"), Kind: EvolutionCandidateMemory,
		Title: "Remember constraint", Summary: "Draft only.",
		Content: json.RawMessage(`{"instruction":"remember"}`),
		Status:  evolutionCandidateStatus,
	}
	job.Review = &EvolutionReview{
		Verdict: "pass", Summary: "Proposal review only.",
		Scope: evolutionReviewScope, CandidateStatus: evolutionCandidateStatus,
	}
	job.UpdatedAt = now.Add(time.Second)
	if err := store.UpdateEvolutionJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetEvolutionJob(ctx, job.ID)
	if err != nil || stored.State != EvolutionJobCompleted ||
		stored.Candidate == nil || stored.Candidate.Status != evolutionCandidateStatus ||
		stored.OwnerUserID != job.OwnerUserID ||
		stored.IdempotencyKey != job.IdempotencyKey {
		t.Fatalf("unexpected persisted Evolution Job: job=%#v err=%v", stored, err)
	}
	listed, err := store.ListEvolutionJobs(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range listed {
		if value.ID == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("persisted Evolution Job %s was not listed", job.ID)
	}
	if err := store.DeleteEvolutionJob(ctx, job.OwnerUserID, job.ID); err != nil {
		t.Fatalf("DeleteEvolutionJob failed: %v", err)
	}
	if _, err := store.GetEvolutionJob(ctx, job.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted Evolution Job is still readable: %v", err)
	}
}

func TestPostgresTraceOnlyEvolutionJobHasNoSyntheticRun(t *testing.T) {
	databaseURL := os.Getenv("BARENA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("BARENA_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	store, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	traceID := fmt.Sprintf("%032x", now.UnixNano())
	pack := EvolutionEvidencePack{
		Schema: evolutionEvidenceSchema, SourceKind: EvolutionSourceTrace,
		SourceTraceID: traceID, Spans: []EvolutionEvidenceSpan{}, RunEvents: []EngineEvent{},
		Redacted: true, Boundary: EvolutionEvidenceBoundary{
			ReleaseAuthority: evolutionReleaseAuthority, CandidateStatus: evolutionCandidateStatus,
			ReviewScope: evolutionReviewScope,
		}, CreatedAt: now,
	}
	digestInput, err := evolutionEvidenceDigestInput(pack)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(digestInput)
	pack.SHA256 = hex.EncodeToString(digest[:])
	job := EvolutionJob{
		Schema: evolutionJobSchema, ID: newID("trace-only-pg-job"),
		SourceKind: EvolutionSourceTrace, SourceTraceID: traceID,
		IdempotencyKey:     newID("trace-only-pg-key"),
		RequestFingerprint: newID("trace-only-pg-fingerprint"),
		State:              EvolutionJobQueued, Stages: cloneEvolutionStages(evolutionJobStages),
		EvidencePack: &pack, Boundary: pack.Boundary, CreatedAt: now, UpdatedAt: now,
	}
	created, wasCreated, err := store.CreateEvolutionJob(ctx, job)
	if err != nil || !wasCreated || created.SourceRunID != "" || created.SourceKind != EvolutionSourceTrace {
		t.Fatalf("Trace-only Evolution Job create failed: job=%#v created=%v err=%v", created, wasCreated, err)
	}
	retry := job
	retry.ID = newID("must-not-create-trace-only-job")
	duplicate, wasCreated, err := store.CreateEvolutionJob(ctx, retry)
	if err != nil || wasCreated || duplicate.ID != job.ID || duplicate.SourceRunID != "" {
		t.Fatalf("Trace-only Evolution retry was not idempotent: job=%#v created=%v err=%v", duplicate, wasCreated, err)
	}
	stored, err := store.GetEvolutionJob(ctx, job.ID)
	if err != nil || stored.SourceRunID != "" || stored.EvidencePack == nil ||
		stored.EvidencePack.SHA256 != pack.SHA256 ||
		!validEvolutionEvidencePack(*stored.EvidencePack, stored) {
		t.Fatalf("Trace-only Evolution persistence invented a Run or lost evidence: job=%#v err=%v", stored, err)
	}
}
