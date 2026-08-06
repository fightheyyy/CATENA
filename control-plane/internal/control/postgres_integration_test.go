package control

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPostgresConcurrentProjectIdentityUpsert(t *testing.T) {
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

	identity := platformProjectUser(newID("project-pg-upsert"), time.Now().UTC())
	const callers = 12
	start := make(chan struct{})
	results := make(chan User, callers)
	errors := make(chan error, callers)
	var waitGroup sync.WaitGroup
	for range callers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			persisted, err := store.UpsertUser(ctx, identity)
			if err != nil {
				errors <- err
				return
			}
			results <- persisted
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Errorf("concurrent identity upsert failed: %v", err)
	}
	for persisted := range results {
		if persisted.ID != identity.ID || persisted.GitHubID != identity.GitHubID {
			t.Errorf("unexpected persisted identity: %#v", persisted)
		}
	}
}

func TestPostgresAPITokenRecoverySurvivesStoreRestart(t *testing.T) {
	databaseURL := os.Getenv("BARENA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("BARENA_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	store, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	user, err := store.UpsertUser(ctx, User{
		ID:          newID("usr-token-pg"),
		GitHubID:    now.UnixNano(),
		Login:       newID("token-pg"),
		DisplayName: "Token persistence test",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	const plaintext = "barena_pat_postgres_restart_secret"
	tokenID := newID("pat-pg")
	encrypted, err := encryptAPIToken(plaintext, tokenID, testGatewaySecret)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.CreateAPIToken(ctx, APIToken{
		ID:             tokenID,
		TokenHash:      sessionTokenHash(plaintext),
		EncryptedToken: encrypted,
		UserID:         user.ID,
		Name:           "Restart-safe Runner",
		CreatedAt:      now,
	}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()

	reopened, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	retained, err := reopened.GetAPITokenByUser(ctx, user.ID, tokenID)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := decryptAPIToken(
		retained.EncryptedToken,
		retained.ID,
		testGatewaySecret,
	)
	if err != nil || recovered != plaintext {
		t.Fatalf("unexpected retained token %q err=%v", recovered, err)
	}
	authenticated, err := reopened.GetUserByAPITokenHash(ctx, sessionTokenHash(plaintext))
	if err != nil || authenticated.ID != user.ID {
		t.Fatalf("hash authentication changed after restart: user=%#v err=%v", authenticated, err)
	}
	if err := reopened.DeleteAPIToken(ctx, user.ID, tokenID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTraceIssuePromotion(t *testing.T) {
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
		ID:        newID("run-pg-test"),
		RequestID: newID("request-pg-test"),
		Origin:    OriginEdge,
		Operation: OperationExplore,
		State:     StateCompleted,
		Input:     json.RawMessage(`{"scenario":{"objective":"postgres trace to case"}}`),
		Runtime:   json.RawMessage(`{"name":"xiaobaos"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	traceID := newID("trace-pg-test")
	event := EngineEvent{
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
	}
	if err := store.AppendEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	retained, err := store.RunHasTrace(ctx, run.ID, traceID)
	if err != nil || !retained {
		t.Fatalf("Trace correlation was not retained: retained=%v err=%v", retained, err)
	}

	issue := Issue{
		ID:            newID("issue-pg-test"),
		SourceRunID:   run.ID,
		SourceTraceID: traceID,
		Title:         "PostgreSQL evidence issue",
		Summary:       "Exercise the durable Trace-to-Case transaction.",
		Severity:      SeverityHigh,
		Status:        IssueOpen,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.CreateIssue(ctx, issue); err != nil {
		t.Fatal(err)
	}
	promoted := Case{
		Schema:          "barena.case.v1",
		ID:              newID("case-pg-test"),
		Revision:        1,
		SourceIssueID:   issue.ID,
		SourceRunID:     run.ID,
		SourceTraceID:   traceID,
		Title:           issue.Title,
		Operation:       run.Operation,
		Input:           run.Input,
		Runtime:         run.Runtime,
		ReplayPrompt:    "Preserve source evidence",
		SuccessCriteria: "The durable Case preserves source evidence.",
		Verifier: json.RawMessage(
			`{"kind":"artifact_assertions","artifacts":[{"path":"result.txt","exists":true}]}`,
		),
		CreatedAt: now,
	}
	first, created, err := store.PromoteIssue(ctx, issue.ID, promoted, now)
	if err != nil || !created {
		t.Fatalf("promotion failed: created=%v err=%v", created, err)
	}
	second, created, err := store.PromoteIssue(
		ctx,
		issue.ID,
		Case{ID: newID("must-not-create")},
		now.Add(time.Second),
	)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf(
			"promotion was not idempotent: first=%s second=%s created=%v err=%v",
			first.ID,
			second.ID,
			created,
			err,
		)
	}
	storedIssue, err := store.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedIssue.Status != IssuePromoted ||
		storedIssue.PromotedCaseID != promoted.ID {
		t.Fatalf("unexpected promoted Issue: %#v", storedIssue)
	}

	replayRun := Run{
		ID:        newID("run-pg-replay"),
		RequestID: newID("request-pg-replay"),
		Origin:    OriginLocal,
		Operation: OperationReplay,
		State:     StateQueued,
		Input:     json.RawMessage(`{"platform_case":{},"case_base_dir":"/tmp"}`),
		Runtime:   run.Runtime,
		CreatedAt: now,
		UpdatedAt: now,
	}
	harness := HarnessVersion{
		ID:             newID("harness-pg-test"),
		CaseID:         promoted.ID,
		RunID:          replayRun.ID,
		SourceRunID:    run.ID,
		SourceTraceID:  traceID,
		IdempotencyKey: newID("idempotency-pg-test"),
		Runtime:        run.Runtime,
		CreatedAt:      now,
	}
	storedRun, storedHarness, created, err := store.CreateReplay(
		ctx,
		replayRun,
		harness,
	)
	if err != nil || !created || storedRun.ID != replayRun.ID ||
		storedHarness.ID != harness.ID {
		t.Fatalf(
			"CreateReplay failed: run=%#v harness=%#v created=%v err=%v",
			storedRun,
			storedHarness,
			created,
			err,
		)
	}
	retryRun := replayRun
	retryRun.ID = newID("must-not-create-run")
	retryRun.RequestID = newID("must-not-create-request")
	retryHarness := harness
	retryHarness.ID = newID("must-not-create-harness")
	retryHarness.RunID = retryRun.ID
	duplicateRun, duplicateHarness, created, err := store.CreateReplay(
		ctx,
		retryRun,
		retryHarness,
	)
	if err != nil || created ||
		duplicateRun.ID != replayRun.ID ||
		duplicateHarness.ID != harness.ID {
		t.Fatalf(
			"CreateReplay retry was not idempotent: run=%#v harness=%#v created=%v err=%v",
			duplicateRun,
			duplicateHarness,
			created,
			err,
		)
	}
	replayRun.State = StateRunning
	replayRun.UpdatedAt = now.Add(time.Second)
	if err := store.UpdateRun(ctx, replayRun); err != nil {
		t.Fatal(err)
	}
	replayTraceID := "22222222222222222222222222222222"
	terminal := EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   replayRun.ID + ".1",
		RunID:     replayRun.ID,
		Sequence:  1,
		Timestamp: now.Add(time.Second),
		Operation: OperationReplay,
		Kind:      "terminal",
		Phase:     "complete",
		Actor:     "engine",
		TraceID:   replayTraceID,
		Payload: json.RawMessage(
			`{"status":"complete","result_status":"pass","decision":"cleared","summary":"verified","result_ref":"result.json"}`,
		),
	}
	if err := store.AppendEvent(ctx, terminal); err != nil {
		t.Fatal(err)
	}
	fact := ReplayFact{
		TerminalEventID: terminal.EventID,
		ReplayTraceID:   replayTraceID,
		PackageStatus:   "complete",
		ResultStatus:    "pass",
		Decision:        DecisionCleared,
		Summary:         "verified",
		ResultRef:       "result.json",
	}
	evaluation, release, created, err := store.FinalizeReplay(
		ctx,
		replayRun.ID,
		fact,
		now.Add(2*time.Second),
	)
	if err != nil || !created ||
		evaluation.CaseID != promoted.ID ||
		evaluation.SourceTraceID != traceID ||
		release.Decision != DecisionCleared {
		t.Fatalf(
			"FinalizeReplay failed: evaluation=%#v release=%#v created=%v err=%v",
			evaluation,
			release,
			created,
			err,
		)
	}
	duplicateEvaluation, duplicateRelease, created, err := store.FinalizeReplay(
		ctx,
		replayRun.ID,
		fact,
		now.Add(3*time.Second),
	)
	if err != nil || created ||
		duplicateEvaluation.ID != evaluation.ID ||
		duplicateRelease.ID != release.ID {
		t.Fatalf(
			"FinalizeReplay retry was not idempotent: evaluation=%#v release=%#v created=%v err=%v",
			duplicateEvaluation,
			duplicateRelease,
			created,
			err,
		)
	}
}
