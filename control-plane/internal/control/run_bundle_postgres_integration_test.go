package control

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestPostgresRunBundlePersistenceKeepsTerminalFactIntegrity(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Microsecond)
	runID := newID("postgres-run-bundle")
	terminalFact := json.RawMessage("{ \"schema\": \"barena.explore_terminal_fact.v1\", \"status\": \"pass\" }")
	request := runBundleRequestFixture(
		t,
		runID,
		"11223344556677889900aabbccddeeff",
		now,
		terminalFact,
	)
	bundle, err := newRunBundle(request, "", newID("postgres-run-bundle-key"), now)
	if err != nil {
		t.Fatal(err)
	}
	created, wasCreated, err := store.CreateRunBundle(ctx, bundle)
	if err != nil || !wasCreated || created.ID != bundle.ID {
		t.Fatalf("PostgreSQL Run Bundle create failed: bundle=%#v created=%v err=%v", created, wasCreated, err)
	}
	retry := bundle
	retry.CreatedAt = retry.CreatedAt.Add(time.Minute)
	duplicate, wasCreated, err := store.CreateRunBundle(ctx, retry)
	if err != nil || wasCreated || duplicate.ID != bundle.ID {
		t.Fatalf("PostgreSQL Run Bundle retry was not idempotent: bundle=%#v created=%v err=%v", duplicate, wasCreated, err)
	}
	stored, err := store.GetRunBundle(ctx, bundle.ID)
	if err != nil || len(stored.Events) == 0 {
		t.Fatalf("PostgreSQL Run Bundle GET failed: bundle=%#v err=%v", stored, err)
	}
	storedTerminal := stored.Events[len(stored.Events)-1].Payload
	if string(storedTerminal) != string(terminalFact) {
		t.Fatalf("terminal fact bytes changed across JSONB persistence: got=%q want=%q", storedTerminal, terminalFact)
	}
	mutated := bundle
	mutated.RequestFingerprint = newID("mutated-run-bundle-fingerprint")
	if _, _, err := store.CreateRunBundle(ctx, mutated); !errors.Is(err, ErrConflict) {
		t.Fatalf("mutated idempotent Run Bundle should conflict, got %v", err)
	}
}
