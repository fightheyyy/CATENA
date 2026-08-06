package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConversationIngestIsVisibleOnlyIdempotentAndOwnerScoped(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{GatewaySecret: testGatewaySecret})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	request := ConversationBatchRequest{
		Schema: conversationBatchSchema,
		Messages: []ConversationMessage{
			conversationFixture("msg-user-1", "conv-001", 1, now, "user", "你好，小八"),
			conversationFixture("msg-agent-1", "conv-001", 2, now.Add(time.Second), "assistant", "你好，我在。"),
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/ingest/conversations"
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, signedPlatformRequest(t, http.MethodPost, path, "conversation-project-a", "xiaobaos", body))
	if created.Code != http.StatusCreated {
		t.Fatalf("Conversation create returned %d: %s", created.Code, created.Body.String())
	}
	var receipt ConversationIngestReceipt
	if err := json.Unmarshal(created.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Schema != conversationReceiptSchema || receipt.Created != 2 || receipt.Duplicates != 0 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}

	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, signedPlatformRequest(t, http.MethodPost, path, "conversation-project-a", "xiaobaos", body))
	if retry.Code != http.StatusOK {
		t.Fatalf("Conversation retry returned %d: %s", retry.Code, retry.Body.String())
	}
	if err := json.Unmarshal(retry.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Created != 0 || receipt.Duplicates != 2 {
		t.Fatalf("unexpected duplicate receipt: %#v", receipt)
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, signedPlatformRequest(t, http.MethodGet, "/v1/conversations", "conversation-project-a", "viewer", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("Conversation list returned %d: %s", list.Code, list.Body.String())
	}
	var listResponse struct {
		Conversations []ConversationSummary `json:"conversations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResponse); err != nil {
		t.Fatal(err)
	}
	if len(listResponse.Conversations) != 1 || listResponse.Conversations[0].MessageCount != 2 ||
		listResponse.Conversations[0].Title != "你好，小八" {
		t.Fatalf("unexpected Conversation list: %#v", listResponse.Conversations)
	}

	detailPath := "/v1/conversations/conv-001?agent_id=xiaoba-local"
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, signedPlatformRequest(t, http.MethodGet, detailPath, "conversation-project-a", "viewer", nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("Conversation detail returned %d: %s", detail.Code, detail.Body.String())
	}
	var document ConversationDocument
	if err := json.Unmarshal(detail.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Schema != conversationReadSchema || len(document.Messages) != 2 ||
		document.Messages[0].Role != "user" || document.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected Conversation detail: %#v", document)
	}

	crossProject := httptest.NewRecorder()
	handler.ServeHTTP(crossProject, signedPlatformRequest(t, http.MethodGet, detailPath, "conversation-project-b", "viewer", nil))
	if crossProject.Code != http.StatusNotFound {
		t.Fatalf("cross-project Conversation detail returned %d: %s", crossProject.Code, crossProject.Body.String())
	}

	mutated := request
	mutated.Messages = append([]ConversationMessage(nil), request.Messages...)
	mutated.Messages[0].Content = []ConversationContentPart{{Type: "text", Text: "mutated"}}
	mutatedBody, _ := json.Marshal(mutated)
	conflict := httptest.NewRecorder()
	handler.ServeHTTP(conflict, signedPlatformRequest(t, http.MethodPost, path, "conversation-project-a", "xiaobaos", mutatedBody))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("mutated message retry returned %d: %s", conflict.Code, conflict.Body.String())
	}
}

func TestConversationIngestRejectsHiddenOrNonXiaoBaMessagesAtomically(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{GatewaySecret: testGatewaySecret})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	valid := conversationFixture("msg-valid", "conv-atomic", 1, now, "user", "visible")
	hidden := conversationFixture("msg-hidden", "conv-atomic", 2, now.Add(time.Second), "assistant", "hidden")
	hidden.Role = "system"
	request := ConversationBatchRequest{Schema: conversationBatchSchema, Messages: []ConversationMessage{valid, hidden}}
	body, _ := json.Marshal(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedPlatformRequest(
		t, http.MethodPost, "/v1/ingest/conversations", "conversation-project-atomic", "xiaobaos", body,
	))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("hidden role returned %d: %s", response.Code, response.Body.String())
	}
	owner := platformProjectUser("conversation-project-atomic", now).ID
	if _, err := store.ListConversationMessagesByOwner(t.Context(), owner, "xiaoba-local", "conv-atomic", 20); err != ErrNotFound {
		t.Fatalf("invalid batch was partially persisted: %v", err)
	}

	nonXiaoBa := valid
	nonXiaoBa.Runtime = "codex"
	request.Messages = []ConversationMessage{nonXiaoBa}
	body, _ = json.Marshal(request)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, signedPlatformRequest(
		t, http.MethodPost, "/v1/ingest/conversations", "conversation-project-atomic", "codex", body,
	))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("non-XiaoBa Conversation returned %d: %s", response.Code, response.Body.String())
	}
}

func TestConversationIngestAcceptsOwnerPersonalAPIToken(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC().Truncate(time.Microsecond)
	user, err := store.UpsertUser(context.Background(), User{
		ID: "conversation-pat-owner", GitHubID: 91337, Login: "conversation-owner",
		DisplayName: "Conversation Owner", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := "barena_pat_conversation_test"
	if err := store.CreateAPIToken(context.Background(), APIToken{
		ID: "pat-conversation", TokenHash: sessionTokenHash(token), UserID: user.ID,
		Name: "XiaoBaOS Conversation", CreatedAt: now,
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
	request := ConversationBatchRequest{
		Schema: conversationBatchSchema,
		Messages: []ConversationMessage{
			conversationFixture("msg_pat_1", "conv_pat", 1, now, "user", "帮我整理今天的工作"),
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/ingest/conversations",
		strings.NewReader(string(body)),
	)
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	if response.Code != http.StatusCreated {
		t.Fatalf("PAT Conversation create returned %d: %s", response.Code, response.Body.String())
	}
	messages, err := store.ListConversationMessagesByOwner(
		context.Background(), user.ID, "xiaoba-local", "conv_pat", 20,
	)
	if err != nil || len(messages) != 1 || messages[0].Content[0].Text != "帮我整理今天的工作" {
		t.Fatalf("PAT Conversation owner mismatch: messages=%#v err=%v", messages, err)
	}
}

func conversationFixture(
	messageID string,
	conversationID string,
	sequence int64,
	occurredAt time.Time,
	role string,
	text string,
) ConversationMessage {
	deliveryStatus := "received"
	if role == "assistant" {
		deliveryStatus = "delivered"
	}
	return ConversationMessage{
		Schema:         conversationMessageSchema,
		MessageID:      messageID,
		ConversationID: conversationID,
		Sequence:       sequence,
		OccurredAt:     occurredAt,
		Runtime:        "xiaobaos",
		AgentID:        "xiaoba-local",
		AgentName:      "XiaoBaOS",
		Surface:        "pet",
		Role:           role,
		Content:        []ConversationContentPart{{Type: "text", Text: text}},
		Delivery:       ConversationDelivery{Status: deliveryStatus},
		TraceID:        "00112233445566778899aabbccddeeff",
	}
}
