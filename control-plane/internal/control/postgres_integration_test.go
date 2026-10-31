package control

import (
	"context"
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

func TestPostgresRegisteredAgentAndBoundKeyAreAtomic(t *testing.T) {
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
	user, err := store.UpsertUser(ctx, User{
		ID: newID("usr-agent-pg"), GitHubID: now.UnixNano(), Login: newID("agent-pg"),
		DisplayName: "Agent owner", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	agent := RegisteredAgent{
		ID: newID("agent-pg"), OwnerUserID: user.ID, DisplayName: "大狗",
		CreatedAt: now, UpdatedAt: now,
	}
	plaintext := "catena_agent_" + newID("pg")
	token := APIToken{
		ID: newID("agent-key-pg"), TokenHash: sessionTokenHash(plaintext),
		UserID: user.ID, AgentID: agent.ID, Name: agent.DisplayName, CreatedAt: now,
	}
	if err := store.CreateAgentWithAPIToken(ctx, agent, token); err != nil {
		t.Fatal(err)
	}
	bound, err := store.GetAPITokenByHash(ctx, token.TokenHash)
	if err != nil || bound.AgentID != agent.ID {
		t.Fatalf("bound key = %#v, %v", bound, err)
	}
	// PostgreSQL timestamps have microsecond precision, not Go nanoseconds.
	seenAt := now.Add(time.Minute).Truncate(time.Microsecond)
	if err := store.ObserveRegisteredAgent(ctx, user.ID, agent.ID, "codex", seenAt); err != nil {
		t.Fatal(err)
	}
	observed, err := store.GetRegisteredAgentByOwner(ctx, user.ID, agent.ID)
	if err != nil || observed.RuntimeKind != "codex" || !observed.LastSeenAt.Equal(seenAt) {
		t.Fatalf("observed Agent = %#v, %v", observed, err)
	}
	if err := store.DeleteAPIToken(ctx, user.ID, token.ID); err != nil {
		t.Fatal(err)
	}
}
