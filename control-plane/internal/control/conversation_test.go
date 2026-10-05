package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"unicode/utf8"
)

func TestConversationIngestEndpointIsRetired(t *testing.T) {
    handler, err := NewHTTPHandlerWithConfig(NewMemoryStore(), nil, AuthConfig{GatewaySecret: testGatewaySecret})
    if err != nil {
        t.Fatal(err)
    }
    response := httptest.NewRecorder()
    handler.ServeHTTP(response, signedPlatformRequest(t, http.MethodPost, "/v1/ingest/conversations", "legacy-project", "xiaobaos", []byte(`{}`)))
    if response.Code != http.StatusNotFound {
        t.Fatalf("retired Conversation ingest returned %d: %s", response.Code, response.Body.String())
    }
}

func TestHistoricalConversationRemainsReadable(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{GatewaySecret: testGatewaySecret})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	owner := platformProjectUser("legacy-project", now).ID
	message := conversationFixture("legacy-message", "legacy-conversation", 1, now, "user", "历史消息")
	if _, _, err := store.IngestConversationMessages(context.Background(), owner, []ConversationMessage{message}, now); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedPlatformRequest(t, http.MethodGet, "/v1/conversations", "legacy-project", "viewer", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("historical Conversation list returned %d: %s", response.Code, response.Body.String())
	}
	var result struct { Conversations []ConversationSummary `json:"conversations"` }
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Conversations) != 1 || result.Conversations[0].MessageCount != 1 {
		t.Fatalf("historical Conversation missing: %#v", result.Conversations)
	}
}

func TestConversationPreviewPreservesUTF8AtRuneBoundary(t *testing.T) {
	preview := conversationTextPreview([]ConversationContentPart{{
		Type: "text",
		Text: "帮我检查今天的部署异常，先说明你看到了什么，再给出下一步。",
	}}, 12)
	if preview != "帮我检查今天的部署异常，" {
		t.Fatalf("preview = %q", preview)
	}
	if !utf8.ValidString(preview) {
		t.Fatalf("preview is invalid UTF-8: %q", preview)
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
