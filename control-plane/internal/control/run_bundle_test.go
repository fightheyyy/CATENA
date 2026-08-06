package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunBundleIngestIsAtomicIdempotentAndOwnerScoped(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	traceID := "00112233445566778899aabbccddeeff"
	nativeTraceID := "11223344556677889900aabbccddeeff"
	runID := "local-explore-run-001"
	terminalFact := json.RawMessage(`{"schema":"barena.explore_terminal_fact.v1","status":"pass","summary":"retained local Barena fact","reviewer":{"scores":{"correctness":1}},"evidence":{"root_trace_id":"00112233445566778899aabbccddeeff"}}`)
	request := runBundleRequestFixture(t, runID, traceID, now, terminalFact)
	request.Run.Input = json.RawMessage(`{"primary_trace_id":"00112233445566778899aabbccddeeff","trace_ids":["00112233445566778899aabbccddeeff","11223344556677889900aabbccddeeff"],"evidence":{"native_trace_ids":["11223344556677889900aabbccddeeff"]}}`)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/ingest/run-bundles"
	firstRequest := signedPlatformRequest(t, http.MethodPost, path, "bundle-project-a", "local-barena", body)
	firstRequest.Header.Set("Idempotency-Key", "barena:"+runID+":explore")
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, firstRequest)
	if first.Code != http.StatusCreated {
		t.Fatalf("Run Bundle create returned %d: %s", first.Code, first.Body.String())
	}
	var created RunBundle
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Schema != runBundleSchema || created.Run.ID != runID ||
		created.Run.Origin != OriginEdge || !created.Run.State.Terminal() ||
		created.TerminalFactSchema != "barena.explore_terminal_fact.v1" ||
		len(created.TraceIDs) != 2 || created.TraceIDs[0] != traceID ||
		created.TraceIDs[1] != nativeTraceID {
		t.Fatalf("unexpected persisted Run Bundle: %#v", created)
	}
	storedRun, err := store.GetRun(context.Background(), runID)
	if err != nil || storedRun.OwnerUserID == "" {
		t.Fatalf("Run Bundle Run was not stored atomically: run=%#v err=%v", storedRun, err)
	}
	storedEvents, err := store.ListEventsAfter(context.Background(), runID, 0, 100)
	if err != nil || len(storedEvents) != 2 || storedEvents[1].Kind != "terminal" {
		t.Fatalf("Run Bundle Events were not stored atomically: events=%#v err=%v", storedEvents, err)
	}
	if retained, err := store.RunHasTrace(context.Background(), runID, nativeTraceID); err != nil || !retained {
		t.Fatalf("Run input Trace association was not retained: retained=%v err=%v", retained, err)
	}

	retryRequest := signedPlatformRequest(t, http.MethodPost, path, "bundle-project-a", "local-barena", body)
	retryRequest.Header.Set("Idempotency-Key", "barena:"+runID+":explore")
	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, retryRequest)
	if retry.Code != http.StatusOK {
		t.Fatalf("Run Bundle retry returned %d: %s", retry.Code, retry.Body.String())
	}
	var duplicate RunBundle
	if err := json.Unmarshal(retry.Body.Bytes(), &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != created.ID {
		t.Fatalf("Run Bundle retry created another aggregate: %s != %s", duplicate.ID, created.ID)
	}

	getPath := "/v1/run-bundles/" + created.ID
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, signedPlatformRequest(
		t, http.MethodGet, getPath, "bundle-project-a", "viewer-a", nil,
	))
	if get.Code != http.StatusOK {
		t.Fatalf("Run Bundle GET returned %d: %s", get.Code, get.Body.String())
	}
	crossProject := httptest.NewRecorder()
	handler.ServeHTTP(crossProject, signedPlatformRequest(
		t, http.MethodGet, getPath, "bundle-project-b", "viewer-b", nil,
	))
	if crossProject.Code != http.StatusNotFound {
		t.Fatalf("cross-project Run Bundle GET returned %d: %s", crossProject.Code, crossProject.Body.String())
	}

	mutatedFact := json.RawMessage(`{"schema":"barena.explore_terminal_fact.v1","status":"fail","summary":"mutated"}`)
	mutatedRequest := runBundleRequestFixture(t, runID, traceID, now, mutatedFact)
	mutatedBody, err := json.Marshal(mutatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	mutatedHTTP := signedPlatformRequest(t, http.MethodPost, path, "bundle-project-a", "local-barena", mutatedBody)
	mutatedHTTP.Header.Set("Idempotency-Key", "barena:"+runID+":explore")
	mutated := httptest.NewRecorder()
	handler.ServeHTTP(mutated, mutatedHTTP)
	if mutated.Code != http.StatusConflict {
		t.Fatalf("mutated idempotent Run Bundle returned %d: %s", mutated.Code, mutated.Body.String())
	}
}

func TestRunBundleRejectsOversizedTerminalFactBeforePersistence(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	terminalFact := json.RawMessage(`{"schema":"barena.explore_terminal_fact.v1","padding":"` +
		strings.Repeat("x", maxRunBundleTerminalFact) + `"}`)
	request := runBundleRequestFixture(
		t,
		"oversized-terminal-run",
		"abcdefabcdefabcdefabcdefabcdefab",
		now,
		terminalFact,
	)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := signedPlatformRequest(
		t, http.MethodPost, "/v1/ingest/run-bundles", "oversized-project", "local-barena", body,
	)
	httpRequest.Header.Set("Idempotency-Key", "barena:oversized-terminal-run:explore")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized terminal fact returned %d: %s", response.Code, response.Body.String())
	}
	if _, err := store.GetRun(context.Background(), "oversized-terminal-run"); err != ErrNotFound {
		t.Fatalf("oversized Run Bundle was partially persisted: %v", err)
	}
}

func TestRunBundleAcceptsOwnerPAT(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC().Truncate(time.Microsecond)
	user, err := store.UpsertUser(context.Background(), User{
		ID: "run-bundle-pat-owner", GitHubID: 84123, Login: "bundle-owner",
		DisplayName: "Bundle Owner", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := "barena_pat_run_bundle_test"
	if err := store.CreateAPIToken(context.Background(), APIToken{
		ID: "pat-run-bundle", TokenHash: sessionTokenHash(token), UserID: user.ID,
		Name: "Run Bundle test", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GitHubClientID: "client", GitHubClientSecret: "secret",
		APITokenEncryptionKey: testGatewaySecret,
		RedirectURL:           "https://catena.example/v1/auth/github/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	runID := "pat-ingested-run"
	request := runBundleRequestFixture(
		t, runID, "1234567890abcdef1234567890abcdef", now,
		json.RawMessage(`{"schema":"barena.explore_terminal_fact.v1","status":"pass"}`),
	)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/ingest/run-bundles", strings.NewReader(string(body)))
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Idempotency-Key", "barena:"+runID+":explore")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	if response.Code != http.StatusCreated {
		t.Fatalf("PAT Run Bundle create returned %d: %s", response.Code, response.Body.String())
	}
	stored, err := store.GetRun(context.Background(), runID)
	if err != nil || stored.OwnerUserID != user.ID {
		t.Fatalf("PAT Run Bundle owner mismatch: run=%#v err=%v", stored, err)
	}
}

func TestRunBundleAdvisoryLockKeyIsPostgresTextSafe(t *testing.T) {
	bundle := RunBundle{ID: runBundleID("owner", "retry-key")}
	lockKey := runBundleAdvisoryLockKey(bundle)
	if lockKey != bundle.ID || strings.ContainsRune(lockKey, '\x00') {
		t.Fatalf("advisory lock key must be the deterministic PostgreSQL-safe Bundle ID: %q", lockKey)
	}
}

func TestRunBundleTerminalDigestIgnoresJSONFormatting(t *testing.T) {
	compact, err := runBundleTerminalDigest(json.RawMessage(`{"schema":"barena.explore_terminal_fact.v1","status":"fail"}`))
	if err != nil {
		t.Fatal(err)
	}
	pretty, err := runBundleTerminalDigest(json.RawMessage("{\n  \"schema\": \"barena.explore_terminal_fact.v1\",\n  \"status\": \"fail\"\n}"))
	if err != nil {
		t.Fatal(err)
	}
	if compact != pretty {
		t.Fatal("terminal fact digest must survive persisted pretty-print formatting")
	}
}

func TestRunBundleRejectsTerminalFactHashBeforePersistence(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	request := runBundleRequestFixture(
		t,
		"invalid-hash-run",
		"ffeeddccbbaa99887766554433221100",
		now,
		json.RawMessage(`{"schema":"barena.explore_terminal_fact.v1","status":"pass"}`),
	)
	request.TerminalFactSHA256 = hex.EncodeToString(make([]byte, sha256.Size))
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := signedPlatformRequest(
		t, http.MethodPost, "/v1/ingest/run-bundles", "invalid-hash-project", "local-barena", body,
	)
	httpRequest.Header.Set("Idempotency-Key", "barena:invalid-hash-run:explore")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid terminal hash returned %d: %s", response.Code, response.Body.String())
	}
	if _, err := store.GetRun(context.Background(), "invalid-hash-run"); err != ErrNotFound {
		t.Fatalf("invalid Run Bundle was partially persisted: %v", err)
	}
}

func runBundleRequestFixture(
	t *testing.T,
	runID string,
	traceID string,
	now time.Time,
	terminalFact json.RawMessage,
) CreateRunBundleRequest {
	t.Helper()
	digest, err := runBundleTerminalDigest(terminalFact)
	if err != nil {
		t.Fatal(err)
	}
	return CreateRunBundleRequest{
		Schema: runBundleSchema,
		Run: RunBundleRun{
			RunID: runID, Operation: OperationExplore, State: StateCompleted,
			Input:     json.RawMessage(`{"scenario":{"objective":"exercise arbitrary Agent"}}`),
			Runtime:   json.RawMessage(`{"runtime":"external"}`),
			CreatedAt: now, UpdatedAt: now.Add(time.Second),
		},
		Events: []EngineEvent{
			{
				Schema: "barena.engine_event.v1", EventID: runID + ".1", RunID: runID,
				Sequence: 1, Timestamp: now, Operation: OperationExplore,
				Kind: "progress", Phase: "target", Actor: "runner", TraceID: traceID,
				Payload: json.RawMessage(`{"status":"completed"}`),
			},
			{
				Schema: "barena.engine_event.v1", EventID: runID + ".2", RunID: runID,
				Sequence: 2, Timestamp: now.Add(time.Second), Operation: OperationExplore,
				Kind: "terminal", Phase: "complete", Actor: "runner", TraceID: traceID,
				Payload: terminalFact,
			},
		},
		TerminalFactSHA256: hex.EncodeToString(digest[:]),
	}
}
