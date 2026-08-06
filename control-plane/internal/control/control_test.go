package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMemoryStoreEngineEventIdempotency(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	run := Run{
		ID:        "run-test",
		RequestID: "request-test",
		Operation: OperationExplore,
		State:     StateRunning,
		Input:     json.RawMessage(`{}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	event := EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   "run-test.1",
		RunID:     run.ID,
		Sequence:  1,
		Timestamp: now,
		Operation: run.Operation,
		Kind:      "progress",
		Phase:     "probe",
		Actor:     "engine",
		Payload:   json.RawMessage(`{"status":"started"}`),
	}
	if err := store.AppendEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("exact duplicate should be idempotent: %v", err)
	}
	event.Payload = json.RawMessage(`{"status":"different"}`)
	if err := store.AppendEvent(context.Background(), event); err != ErrConflict {
		t.Fatalf("mutated duplicate should conflict, got %v", err)
	}
}

func TestHTTPRunLifecycleAndSSEReconnect(t *testing.T) {
	root := t.TempDir()
	worker := writeFakeWorker(t, root)
	store := NewMemoryStore()
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  worker,
		RunsRoot:    filepath.Join(root, "runs"),
		KillGrace:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(store, runner))
	defer server.Close()

	indexResponse, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	indexBody, _ := io.ReadAll(indexResponse.Body)
	indexResponse.Body.Close()
	indexHTML := string(indexBody)
	if indexResponse.StatusCode != http.StatusOK ||
		!strings.Contains(indexHTML, "Barena") ||
		!strings.Contains(indexHTML, `id="root"`) ||
		!strings.Contains(indexHTML, "Agent evaluation workbench") ||
		!strings.Contains(indexHTML, "/assets/") {
		t.Fatalf("embedded Web surface is unavailable: %d %s", indexResponse.StatusCode, indexBody)
	}
	bundleResponse, err := http.Get(server.URL + "/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	bundleBody, _ := io.ReadAll(bundleResponse.Body)
	bundleResponse.Body.Close()
	bundle := string(bundleBody)
	if bundleResponse.StatusCode != http.StatusOK ||
		!strings.Contains(bundle, "Explore an Agent") ||
		!strings.Contains(bundle, "User simulation → Target Agent → Inspector → Reviewer") ||
		!strings.Contains(bundle, "barena-results.log") {
		t.Fatalf("embedded Web bundle is unavailable: %d", bundleResponse.StatusCode)
	}

	run := createTestRun(t, server.URL, `{"scenario":{"scenario_id":"go-api"}}`)
	waitForState(t, store, run.ID, StateCompleted)
	events, err := store.ListEventsAfter(context.Background(), run.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("unexpected events: %#v", events)
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/runs/"+run.ID+"/events", nil)
	request.Header.Set("Last-Event-ID", events[0].EventID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(body), events[0].EventID) {
		t.Fatalf("SSE replayed an acknowledged event: %s", body)
	}
	if !strings.Contains(string(body), events[1].EventID) {
		t.Fatalf("SSE did not resume with the next event: %s", body)
	}

	runRoot := filepath.Join(root, "runs", run.ID)
	if status, err := readPackageStatus(filepath.Join(root, "runs"), run.ID); err != nil || status != "complete" {
		t.Fatalf("Run package was not verified: status=%q err=%v", status, err)
	}
	resultPath := filepath.Join(runRoot, "explore-result.json")
	if err := os.WriteFile(resultPath, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPackageStatus(filepath.Join(root, "runs"), run.ID); err == nil {
		t.Fatal("tampered Run package should fail verification")
	}
}

func TestHTTPCancelIsIdempotentAndTerminal(t *testing.T) {
	root := t.TempDir()
	worker := writeFakeWorker(t, root)
	store := NewMemoryStore()
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  worker,
		RunsRoot:    filepath.Join(root, "runs"),
		KillGrace:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(store, runner))
	defer server.Close()
	run := createTestRun(t, server.URL, `{"sleep":true}`)
	waitForState(t, store, run.ID, StateRunning)

	for range 2 {
		response, err := http.Post(server.URL+"/v1/runs/"+run.ID+"/cancel", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("cancel returned %d", response.StatusCode)
		}
	}
	waitForState(t, store, run.ID, StateCancelled)
}

func TestGitHubAuthOwnerIsolationAndCommunityPublication(t *testing.T) {
	var challenge string
	var redirectURI string
	identityProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			if r.URL.Query().Get("scope") != "read:user" ||
				r.URL.Query().Get("code_challenge_method") != "S256" {
				t.Fatalf("unexpected authorize query: %s", r.URL.RawQuery)
			}
			challenge = r.URL.Query().Get("code_challenge")
			redirectURI = r.URL.Query().Get("redirect_uri")
			target, _ := url.Parse(redirectURI)
			query := target.Query()
			query.Set("code", "test-code")
			query.Set("state", r.URL.Query().Get("state"))
			target.RawQuery = query.Encode()
			http.Redirect(w, r, target.String(), http.StatusFound)
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			verifier := r.Form.Get("code_verifier")
			sum := sha256.Sum256([]byte(verifier))
			actualChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
			if r.Form.Get("code") != "test-code" ||
				actualChallenge != challenge ||
				r.Form.Get("redirect_uri") != redirectURI {
				t.Fatalf("token exchange did not preserve PKCE and redirect binding")
			}
			writeJSON(w, http.StatusOK, map[string]string{
				"access_token": "temporary-token",
				"token_type":   "bearer",
			})
		case "/user":
			if r.Header.Get("Authorization") != "Bearer temporary-token" {
				t.Fatalf("unexpected authorization header")
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"id":         4242,
				"login":      "octocat",
				"name":       "Octo Cat",
				"avatar_url": "https://avatars.githubusercontent.com/u/4242",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer identityProvider.Close()

	root := t.TempDir()
	worker := writeFakeWorker(t, root)
	store := NewMemoryStore()
	runner, err := NewRunnerManager(store, RunnerConfig{
		NodeCommand: "node",
		WorkerPath:  worker,
		RunsRoot:    filepath.Join(root, "runs"),
		KillGrace:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	serverURL := "http://" + server.Listener.Addr().String()
	handler, err := NewHTTPHandlerWithConfig(store, runner, AuthConfig{
		GitHubClientID:        "client-id",
		GitHubClientSecret:    "client-secret",
		APITokenEncryptionKey: testGatewaySecret,
		RedirectURL:           serverURL + "/api/auth/callback/github",
		AuthorizeURL:          identityProvider.URL + "/authorize",
		TokenURL:              identityProvider.URL + "/token",
		UserAPIURL:            identityProvider.URL + "/user",
		SessionTTL:            time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.Get(server.URL + "/v1/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OAuth flow ended with %d", response.StatusCode)
	}

	var sessionPayload struct {
		Mode          string `json:"mode"`
		Authenticated bool   `json:"authenticated"`
		User          User   `json:"user"`
	}
	decodeTestJSON(t, client, http.MethodGet, server.URL+"/v1/auth/session", nil, &sessionPayload)
	if sessionPayload.Mode != "github" ||
		!sessionPayload.Authenticated ||
		sessionPayload.User.Login != "octocat" {
		t.Fatalf("unexpected session payload: %#v", sessionPayload)
	}

	var tokenPayload struct {
		APIToken APIToken `json:"api_token"`
		Token    string   `json:"token"`
	}
	decodeTestJSON(
		t,
		client,
		http.MethodPost,
		server.URL+"/v1/me/api-tokens",
		bytes.NewBufferString(`{"name":"MacBook Runner"}`),
		&tokenPayload,
	)
	if tokenPayload.APIToken.Name != "MacBook Runner" ||
		!strings.HasPrefix(tokenPayload.Token, "barena_pat_") {
		t.Fatalf("unexpected API token response: %#v", tokenPayload)
	}
	var tokenList struct {
		APITokens []APIToken `json:"api_tokens"`
	}
	decodeTestJSON(
		t,
		client,
		http.MethodGet,
		server.URL+"/v1/me/api-tokens",
		nil,
		&tokenList,
	)
	tokenListBytes, _ := json.Marshal(tokenList)
	if strings.Contains(string(tokenListBytes), tokenPayload.Token) ||
		strings.Contains(string(tokenListBytes), "token_hash") {
		t.Fatalf("API token listing exposed a secret: %s", tokenListBytes)
	}
	if len(tokenList.APITokens) != 1 ||
		!tokenList.APITokens[0].Recoverable ||
		!strings.HasSuffix(tokenList.APITokens[0].MaskedToken, tokenPayload.Token[len(tokenPayload.Token)-4:]) {
		t.Fatalf("unexpected recoverable token row: %#v", tokenList.APITokens)
	}
	var revealPayload struct {
		Token string `json:"token"`
	}
	decodeTestJSON(
		t,
		client,
		http.MethodPost,
		server.URL+"/v1/me/api-tokens/"+tokenPayload.APIToken.ID+"/reveal",
		nil,
		&revealPayload,
	)
	if revealPayload.Token != tokenPayload.Token {
		t.Fatal("owner reveal did not recover the created API token")
	}

	legacyToken := "barena_pat_legacy_hash_only"
	if err := store.CreateAPIToken(context.Background(), APIToken{
		ID:        "pat-legacy",
		TokenHash: sessionTokenHash(legacyToken),
		UserID:    sessionPayload.User.ID,
		Name:      "Legacy Runner",
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	legacyRevealRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/me/api-tokens/pat-legacy/reveal",
		nil,
	)
	legacyRevealResponse, err := client.Do(legacyRevealRequest)
	if err != nil {
		t.Fatal(err)
	}
	legacyRevealResponse.Body.Close()
	if legacyRevealResponse.StatusCode != http.StatusConflict {
		t.Fatalf("legacy reveal returned %d", legacyRevealResponse.StatusCode)
	}

	edgeCreateBody := bytes.NewBufferString(
		`{"operation":"explore","input":{"scenario":{"scenario_id":"edge-owned"}}}`,
	)
	edgeCreateResponse := bearerRequest(
		t,
		http.MethodPost,
		server.URL+"/v1/ingest/runs",
		tokenPayload.Token,
		edgeCreateBody,
	)
	if edgeCreateResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(edgeCreateResponse.Body)
		edgeCreateResponse.Body.Close()
		t.Fatalf("edge create returned %d: %s", edgeCreateResponse.StatusCode, body)
	}
	var edgeRun Run
	if err := json.NewDecoder(edgeCreateResponse.Body).Decode(&edgeRun); err != nil {
		t.Fatal(err)
	}
	edgeCreateResponse.Body.Close()
	if edgeRun.Origin != OriginEdge || edgeRun.State != StateRunning {
		t.Fatalf("unexpected edge Run: %#v", edgeRun)
	}
	emptyFinishResponse := bearerRequest(
		t,
		http.MethodPost,
		server.URL+"/v1/ingest/runs/"+edgeRun.ID+"/finish",
		tokenPayload.Token,
		bytes.NewBufferString(`{"state":"completed"}`),
	)
	emptyFinishResponse.Body.Close()
	if emptyFinishResponse.StatusCode != http.StatusConflict {
		t.Fatalf(
			"evidence-free edge completion returned %d",
			emptyFinishResponse.StatusCode,
		)
	}

	edgeEvent := func(sequence int64, phase string) []byte {
		t.Helper()
		value, err := json.Marshal(EngineEvent{
			Schema:    "barena.engine_event.v1",
			EventID:   edgeRun.ID + "." + strconv.FormatInt(sequence, 10),
			RunID:     edgeRun.ID,
			Sequence:  sequence,
			Timestamp: time.Now().UTC(),
			Operation: OperationExplore,
			Kind:      "terminal",
			Phase:     phase,
			Actor:     "user-simulator",
			TraceID:   "edge-trace",
			Payload:   json.RawMessage(`{"status":"observed"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	gapResponse := bearerRequest(
		t,
		http.MethodPost,
		server.URL+"/v1/ingest/runs/"+edgeRun.ID+"/events",
		tokenPayload.Token,
		bytes.NewReader(edgeEvent(2, "target")),
	)
	gapResponse.Body.Close()
	if gapResponse.StatusCode != http.StatusConflict {
		t.Fatalf("out-of-order Event returned %d", gapResponse.StatusCode)
	}
	firstEdgeEvent := edgeEvent(1, "complete")
	for range 2 {
		eventResponse := bearerRequest(
			t,
			http.MethodPost,
			server.URL+"/v1/ingest/runs/"+edgeRun.ID+"/events",
			tokenPayload.Token,
			bytes.NewReader(firstEdgeEvent),
		)
		eventResponse.Body.Close()
		if eventResponse.StatusCode != http.StatusNoContent {
			t.Fatalf("edge Event returned %d", eventResponse.StatusCode)
		}
	}
	finishResponse := bearerRequest(
		t,
		http.MethodPost,
		server.URL+"/v1/ingest/runs/"+edgeRun.ID+"/finish",
		tokenPayload.Token,
		bytes.NewBufferString(`{"state":"completed"}`),
	)
	finishResponse.Body.Close()
	if finishResponse.StatusCode != http.StatusOK {
		t.Fatalf("edge finish returned %d", finishResponse.StatusCode)
	}
	retainedEdgeRun, err := store.GetRun(context.Background(), edgeRun.ID)
	if err != nil ||
		retainedEdgeRun.OwnerUserID != sessionPayload.User.ID ||
		retainedEdgeRun.State != StateCompleted ||
		retainedEdgeRun.Origin != OriginEdge {
		t.Fatalf("edge Run was not retained correctly: %#v err=%v", retainedEdgeRun, err)
	}

	run := createTestRunWithClient(
		t,
		client,
		server.URL,
		`{"scenario":{"scenario_id":"private-owned","target":{"role":"base"}}}`,
	)
	waitForState(t, store, run.ID, StateCompleted)
	storedRun, err := store.GetRun(context.Background(), run.ID)
	if err != nil || storedRun.OwnerUserID != sessionPayload.User.ID {
		t.Fatalf("Run ownership was not persisted: %#v err=%v", storedRun, err)
	}

	foreignUser, err := store.UpsertUser(context.Background(), User{
		ID:          "usr-foreign",
		GitHubID:    5252,
		Login:       "foreign",
		DisplayName: "Foreign",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignPlaintext := "barena_pat_foreign_owner_secret"
	foreignEncrypted, err := encryptAPIToken(
		foreignPlaintext,
		"pat-foreign",
		testGatewaySecret,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAPIToken(context.Background(), APIToken{
		ID:             "pat-foreign",
		TokenHash:      sessionTokenHash(foreignPlaintext),
		EncryptedToken: foreignEncrypted,
		UserID:         foreignUser.ID,
		Name:           "Foreign Runner",
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	foreignRevealRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/me/api-tokens/pat-foreign/reveal",
		nil,
	)
	foreignRevealResponse, err := client.Do(foreignRevealRequest)
	if err != nil {
		t.Fatal(err)
	}
	foreignRevealResponse.Body.Close()
	if foreignRevealResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-owner token reveal returned %d", foreignRevealResponse.StatusCode)
	}
	foreignRun := Run{
		ID:          "run-foreign",
		RequestID:   "req-foreign",
		OwnerUserID: foreignUser.ID,
		Operation:   OperationExplore,
		State:       StateCompleted,
		Input:       json.RawMessage(`{"scenario":{"target":{"role":"secret-role"}}}`),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.CreateRun(context.Background(), foreignRun); err != nil {
		t.Fatal(err)
	}
	foreignResponse, err := client.Get(server.URL + "/v1/runs/" + foreignRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreignResponse.Body.Close()
	if foreignResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign Run returned %d", foreignResponse.StatusCode)
	}

	evidenceRun := Run{
		ID:          "run-community-evidence",
		RequestID:   "req-community-evidence",
		OwnerUserID: sessionPayload.User.ID,
		Operation:   OperationExplore,
		State:       StateCompleted,
		Input: json.RawMessage(
			`{"scenario":{"objective":"private prompt","target":{"role":"engineer-cat","skill":"test-generation"}}}`,
		),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.CreateRun(context.Background(), evidenceRun); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(context.Background(), EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   evidenceRun.ID + ".1",
		RunID:     evidenceRun.ID,
		Sequence:  1,
		Timestamp: time.Now().UTC(),
		Operation: OperationExplore,
		Kind:      "progress",
		Phase:     "complete",
		Actor:     "barena",
		TraceID:   "private-trace-id",
		Payload: json.RawMessage(
			`{"status":"completed","verdict":"pass","evidence":{"otlp_spans":7},"private":"do-not-publish"}`,
		),
	}); err != nil {
		t.Fatal(err)
	}

	updateBody := bytes.NewBufferString(
		`{"display_name":"Octo 的工程小八","bio":"从可验证运行中学习。","is_public":true}`,
	)
	var profilePayload map[string]any
	decodeTestJSON(
		t,
		client,
		http.MethodPut,
		server.URL+"/v1/me/profile",
		updateBody,
		&profilePayload,
	)
	var communityPayload map[string]any
	decodeTestJSON(
		t,
		client,
		http.MethodGet,
		server.URL+"/v1/community/profiles",
		nil,
		&communityPayload,
	)
	publicBytes, _ := json.Marshal(communityPayload)
	publicText := string(publicBytes)
	for _, secret := range []string{
		"private prompt",
		"private-trace-id",
		"do-not-publish",
		"secret-role",
	} {
		if strings.Contains(publicText, secret) {
			t.Fatalf("community response leaked private evidence %q: %s", secret, publicText)
		}
	}
	for _, expected := range []string{
		"engineer-cat",
		"test-generation",
		"verified",
	} {
		if !strings.Contains(publicText, expected) {
			t.Fatalf("community response omitted %q: %s", expected, publicText)
		}
	}

	deleteRequest, _ := http.NewRequest(
		http.MethodDelete,
		server.URL+"/v1/me/api-tokens/"+tokenPayload.APIToken.ID,
		nil,
	)
	deleteResponse, err := client.Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	deleteResponse.Body.Close()
	if deleteResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("token revocation returned %d", deleteResponse.StatusCode)
	}
	revokedResponse := bearerRequest(
		t,
		http.MethodGet,
		server.URL+"/v1/runs/"+edgeRun.ID,
		tokenPayload.Token,
		nil,
	)
	revokedResponse.Body.Close()
	if revokedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked API token returned %d", revokedResponse.StatusCode)
	}

	logoutRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/auth/logout",
		nil,
	)
	logoutResponse, err := client.Do(logoutRequest)
	if err != nil {
		t.Fatal(err)
	}
	logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("logout returned %d", logoutResponse.StatusCode)
	}
	privateResponse, err := client.Get(server.URL + "/v1/runs")
	if err != nil {
		t.Fatal(err)
	}
	privateResponse.Body.Close()
	if privateResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logged-out private API returned %d", privateResponse.StatusCode)
	}
}

func createTestRun(t *testing.T, serverURL, input string) Run {
	t.Helper()
	body := `{"operation":"explore","input":` + input + `}`
	response, err := http.Post(serverURL+"/v1/runs", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		bytes, _ := io.ReadAll(response.Body)
		t.Fatalf("create returned %d: %s", response.StatusCode, bytes)
	}
	var run Run
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	return run
}

func createTestRunWithClient(
	t *testing.T,
	client *http.Client,
	serverURL string,
	input string,
) Run {
	t.Helper()
	body := `{"operation":"explore","input":` + input + `}`
	request, _ := http.NewRequest(
		http.MethodPost,
		serverURL+"/v1/runs",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		bytes, _ := io.ReadAll(response.Body)
		t.Fatalf("create returned %d: %s", response.StatusCode, bytes)
	}
	var run Run
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	return run
}

func decodeTestJSON(
	t *testing.T,
	client *http.Client,
	method string,
	target string,
	body io.Reader,
	destination any,
) {
	t.Helper()
	request, _ := http.NewRequest(method, target, body)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s returned %d: %s", method, target, response.StatusCode, responseBody)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func bearerRequest(
	t *testing.T,
	method string,
	target string,
	token string,
	body io.Reader,
) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func waitForState(t *testing.T, store Store, runID string, expected RunState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := store.GetRun(context.Background(), runID)
		if err == nil && run.State == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, _ := store.GetRun(context.Background(), runID)
	t.Fatalf("Run did not reach %s; current=%s error=%s", expected, run.State, run.Error)
}

func writeFakeWorker(t *testing.T, root string) string {
	t.Helper()
	worker := filepath.Join(root, "fake-worker.js")
	source := `
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
let source = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", chunk => source += chunk);
process.stdin.on("end", () => {
  const request = JSON.parse(source);
  const root = path.join(request.runs_root, request.run_id);
  fs.mkdirSync(root);
  const events = [];
  const emit = (sequence, phase, actor, kind = "progress") => {
    const event = {
      schema: "barena.engine_event.v1",
      event_id: request.run_id + "." + sequence,
      run_id: request.run_id,
      sequence,
      timestamp: new Date().toISOString(),
      operation: request.operation,
      kind,
      phase,
      actor,
      trace_id: sequence === 1 ? "trace-" + request.run_id : undefined,
      payload: {status: kind === "terminal" ? "complete" : "started"}
    };
    events.push(event);
    fs.writeFileSync(path.join(root, "events.ndjson"), events.map(JSON.stringify).join("\n") + "\n");
    process.stdout.write(JSON.stringify(event) + "\n");
  };
  emit(1, "target", "target");
  if (request.input.sleep) {
    process.on("SIGINT", () => process.exit(0));
    setInterval(() => {}, 1000);
    return;
  }
  const resultRef = "explore-result.json";
  fs.writeFileSync(path.join(root, resultRef), JSON.stringify({run_id: request.run_id, status: "pass"}) + "\n");
  emit(2, "complete", "engine", "terminal");
  const files = [resultRef, "events.ndjson"].map(ref => {
    const bytes = fs.readFileSync(path.join(root, ref));
    return {
      ref,
      kind: ref === resultRef ? "result" : "events",
      media_type: ref.endsWith(".json") ? "application/json" : "application/x-ndjson",
      size: bytes.length,
      sha256: crypto.createHash("sha256").update(bytes).digest("hex")
    };
  });
  fs.writeFileSync(path.join(root, "run-package.json"), JSON.stringify({
    schema: "barena.run_package.v1",
    run_id: request.run_id,
    status: "complete",
    result_ref: resultRef,
    files
  }) + "\n");
});`
	if err := os.WriteFile(worker, []byte(source), 0o755); err != nil {
		t.Fatal(err)
	}
	return worker
}
