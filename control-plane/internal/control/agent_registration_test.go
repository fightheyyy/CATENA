package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateRegisteredAgentCreatesStableIdentityAndBoundKey(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	user, err := store.UpsertUser(context.Background(), User{
		ID: "user-agent-owner", GitHubID: 501, Login: "owner", DisplayName: "Owner",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionPlaintext := "session-agent-owner"
	if err := store.CreateSession(context.Background(), Session{
		TokenHash: sessionTokenHash(sessionPlaintext), UserID: user.ID,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	server := &HTTPServer{store: store, auth: testAgentAuthConfig()}
	request := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(`{"display_name":"大狗"}`))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionPlaintext})
	recorder := httptest.NewRecorder()

	server.createRegisteredAgent(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Agent    RegisteredAgent `json:"agent"`
		APIToken APIToken        `json:"api_token"`
		Token    string          `json:"token"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Agent.ID == "" || response.Agent.DisplayName != "大狗" ||
		response.APIToken.AgentID != response.Agent.ID ||
		!strings.HasPrefix(response.Token, "catena_agent_") {
		t.Fatalf("unexpected Agent response: %#v", response)
	}
	stored, err := store.GetAPITokenByHash(context.Background(), sessionTokenHash(response.Token))
	if err != nil || stored.AgentID != response.Agent.ID || stored.UserID != user.ID {
		t.Fatalf("stored key binding = %#v, %v", stored, err)
	}
}

func TestLocalModeCreatesAgentAndAcceptsItsBoundKey(t *testing.T) {
	store := NewMemoryStore()
	traces := &recordingTraceStore{}
	server := &HTTPServer{
		store:  store,
		traces: traces,
		auth: AuthConfig{
			APITokenEncryptionKey: "local-test-agent-token-encryption-key",
		}.normalized(),
	}
	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/agents",
		strings.NewReader(`{"display_name":"Local Codex"}`),
	)
	createRecorder := httptest.NewRecorder()

	server.createRegisteredAgent(createRecorder, createRequest)

	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Agent RegisteredAgent `json:"agent"`
		Token string          `json:"token"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, "catena_agent_") {
		t.Fatalf("unexpected local Agent response: %#v", created)
	}
	storedAgent, err := store.GetRegisteredAgentByOwner(context.Background(), "local", created.Agent.ID)
	if err != nil || storedAgent.OwnerUserID != "local" {
		t.Fatalf("stored local Agent = %#v, %v", storedAgent, err)
	}

	body := mustProtoMarshal(t, testOTLPRequest(false))
	ingestRequest := httptest.NewRequest(http.MethodPost, "/v1/otlp/v1/traces", bytes.NewReader(body))
	ingestRequest.Header.Set("Content-Type", "application/x-protobuf")
	ingestRequest.Header.Set("Authorization", "Bearer "+created.Token)
	ingestRecorder := httptest.NewRecorder()

	server.ingestOTLPTraces(ingestRecorder, ingestRequest)

	if ingestRecorder.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, body = %s", ingestRecorder.Code, ingestRecorder.Body.String())
	}
	if traces.ownerID != "local" || len(traces.spans) != 1 || traces.spans[0].AgentID != created.Agent.ID {
		t.Fatalf("local Trace attribution = owner %q spans %#v", traces.ownerID, traces.spans)
	}
}

func TestGetRegisteredAgentReturnsCheapConnectionStatus(t *testing.T) {
	store, user, agent, _ := registeredAgentFixture(t, "大狗")
	now := time.Now().UTC()
	sessionPlaintext := "session-agent-status"
	if err := store.CreateSession(context.Background(), Session{
		TokenHash: sessionTokenHash(sessionPlaintext), UserID: user.ID,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	server := &HTTPServer{store: store, auth: testAgentAuthConfig()}
	request := httptest.NewRequest(http.MethodGet, "/v1/agents/"+agent.ID, nil)
	request.SetPathValue("agent_id", agent.ID)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionPlaintext})
	recorder := httptest.NewRecorder()

	server.getRegisteredAgent(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Agent      RegisteredAgent `json:"agent"`
		Connected  bool            `json:"connected"`
		Credential *APIToken       `json:"credential"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Agent.ID != agent.ID || response.Connected || response.Credential == nil ||
		response.Credential.AgentID != agent.ID {
		t.Fatalf("unexpected connection status: %#v", response)
	}
}

func TestBoundAgentKeyOwnsOTLPIdentityAndDetectsRuntime(t *testing.T) {
	store, user, agent, plaintext := registeredAgentFixture(t, "大狗")
	traces := &recordingTraceStore{}
	server := &HTTPServer{store: store, traces: traces, auth: testAgentAuthConfig()}
	body := mustProtoMarshal(t, testOTLPRequest(false))
	request := httptest.NewRequest(http.MethodPost, "/v1/otlp/v1/traces", nil)
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Authorization", "Bearer "+plaintext)
	recorder := httptest.NewRecorder()

	server.ingestOTLPTraces(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if traces.ownerID != user.ID || len(traces.spans) != 1 || traces.spans[0].AgentID != agent.ID {
		t.Fatalf("Trace attribution = owner %q spans %#v", traces.ownerID, traces.spans)
	}
	observed, err := store.GetRegisteredAgentByOwner(context.Background(), user.ID, agent.ID)
	if err != nil || observed.RuntimeKind != "codex" || observed.LastSeenAt.IsZero() {
		t.Fatalf("observed Agent = %#v, %v", observed, err)
	}
}

func TestBoundAgentKeyOverridesConversationPayloadIdentity(t *testing.T) {
	store, user, agent, plaintext := registeredAgentFixture(t, "大狗")
	now := time.Now().UTC().Truncate(time.Microsecond)
	message := conversationFixture("msg_bound_1", "conv_bound", 1, now, "user", "你好")
	message.AgentID = ""
	message.AgentName = "client-controlled-name"
	payload, err := json.Marshal(ConversationBatchRequest{
		Schema: conversationBatchSchema, Messages: []ConversationMessage{message},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &HTTPServer{store: store, auth: testAgentAuthConfig()}
	request := httptest.NewRequest(http.MethodPost, "/v1/ingest/conversations", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+plaintext)
	recorder := httptest.NewRecorder()

	server.ingestConversations(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	stored, err := store.ListConversationMessagesByOwner(
		context.Background(), user.ID, agent.ID, "conv_bound", 10,
	)
	if err != nil || len(stored) != 1 || stored[0].AgentName != "大狗" {
		t.Fatalf("Conversation attribution = %#v, %v", stored, err)
	}
	observed, err := store.GetRegisteredAgentByOwner(context.Background(), user.ID, agent.ID)
	if err != nil || observed.RuntimeKind != "xiaobaos" {
		t.Fatalf("observed runtime = %#v, %v", observed, err)
	}
}

func TestDetectOTLPRuntimeUsesEvidenceAndFallsBackToGenericOTel(t *testing.T) {
	tests := []struct {
		service string
		scope   string
		runtime string
		system  string
		want    string
	}{
		{service: "xiaobaos", want: "xiaobaos"},
		{service: "barena-dsh-target", runtime: "dsh", system: "deepseek-harness", want: "dsh"},
		{service: "barena-dsh-target", runtime: "dsh", want: "otel"},
		{service: "deepseek-harness", system: "deepseek-harness", want: "otel"},
		{service: "catena-runtime-codex", runtime: "codex", want: "codex"},
		{service: "catena-runtime-claude-code", runtime: "claude-code", want: "claude_code"},
		{service: "codex-app-server", want: "otel"},
		{service: "claude-code", want: "otel"},
		{service: "hermes-codex", want: "otel"},
		{service: "openclaw-claude", want: "otel"},
		{service: "custom-agent", scope: "custom.instrumentation", want: "otel"},
	}
	for _, test := range tests {
		got := detectOTLPRuntime([]TraceSpan{{
			ServiceName: test.service, ScopeName: test.scope, ResourceAttributes: map[string]any{
				"service.name":  test.service,
				"agent.runtime": test.runtime,
				"gen_ai.system": test.system,
			},
		}})
		if got != test.want {
			t.Errorf("detectOTLPRuntime(%q, %q) = %q, want %q", test.service, test.scope, got, test.want)
		}
	}
}

func registeredAgentFixture(t *testing.T, displayName string) (*MemoryStore, User, RegisteredAgent, string) {
	t.Helper()
	store := NewMemoryStore()
	now := time.Now().UTC()
	user, err := store.UpsertUser(context.Background(), User{
		ID: "user-bound-agent", GitHubID: 502, Login: "bound", DisplayName: "Bound",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	agent := RegisteredAgent{
		ID: "agent_stable", OwnerUserID: user.ID, DisplayName: displayName,
		CreatedAt: now, UpdatedAt: now,
	}
	plaintext := "catena_agent_fixture"
	token := APIToken{
		ID: "agent_key_fixture", TokenHash: sessionTokenHash(plaintext), UserID: user.ID,
		AgentID: agent.ID, Name: displayName, CreatedAt: now,
	}
	if err := store.CreateAgentWithAPIToken(context.Background(), agent, token); err != nil {
		t.Fatal(err)
	}
	return store, user, agent, plaintext
}

func testAgentAuthConfig() AuthConfig {
	return (AuthConfig{
		GitHubClientID: "client", GitHubClientSecret: "secret",
		RedirectURL:           "https://catena.example/v1/auth/github/callback",
		APITokenEncryptionKey: testGatewaySecret,
	}).normalized()
}
