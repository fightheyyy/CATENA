package control

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestPostgresConversationPersistenceIsOwnerScopedOrderedIdempotentAndAtomic(t *testing.T) {
	databaseURL := os.Getenv("BARENA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("BARENA_TEST_DATABASE_URL is not configured")
	}

	ctx := t.Context()
	store, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	ownerA := newID("conversation-owner-a")
	ownerB := newID("conversation-owner-b")
	agentID := newID("xiaoba-pg-agent")
	conversationID := newID("conversation-pg")
	userMessageID := newID("conversation-pg-user")
	assistantMessageID := newID("conversation-pg-assistant")

	userMessage := postgresConversationFixture(
		userMessageID, conversationID, agentID, 1, now, "user", "Owner A request",
	)
	assistantMessage := postgresConversationFixture(
		assistantMessageID, conversationID, agentID, 2, now.Add(time.Second),
		"assistant", "Owner A response",
	)
	// Deliberately ingest out of sequence so the read verifies durable ordering,
	// rather than accidentally preserving the input slice order.
	batch := []ConversationMessage{assistantMessage, userMessage}
	receivedAt := now.Add(2 * time.Second)

	created, duplicates, err := store.IngestConversationMessages(
		ctx, ownerA, batch, receivedAt,
	)
	if err != nil || created != 2 || duplicates != 0 {
		t.Fatalf(
			"PostgreSQL Conversation create failed: created=%d duplicates=%d err=%v",
			created, duplicates, err,
		)
	}

	storedA, err := store.ListConversationMessagesByOwner(
		ctx, ownerA, agentID, conversationID, 20,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedA) != 2 || storedA[0].MessageID != userMessageID ||
		storedA[0].Sequence != 1 || storedA[1].MessageID != assistantMessageID ||
		storedA[1].Sequence != 2 {
		t.Fatalf("Conversation messages did not retain sequence order: %#v", storedA)
	}
	if !storedA[0].OccurredAt.Equal(userMessage.OccurredAt) ||
		!storedA[1].OccurredAt.Equal(assistantMessage.OccurredAt) ||
		!storedA[0].ReceivedAt.Equal(receivedAt) ||
		!storedA[1].ReceivedAt.Equal(receivedAt) {
		t.Fatalf("Conversation timestamps changed across persistence: %#v", storedA)
	}

	if _, err := store.ListConversationMessagesByOwner(
		ctx, ownerB, agentID, conversationID, 20,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign owner read should be not found, got %v", err)
	}

	// ReceivedAt is server-owned and excluded from the content fingerprint, so
	// an exact source retry remains idempotent even when it arrives later.
	created, duplicates, err = store.IngestConversationMessages(
		ctx, ownerA, batch, receivedAt.Add(time.Hour),
	)
	if err != nil || created != 0 || duplicates != 2 {
		t.Fatalf(
			"exact Conversation retry was not idempotent: created=%d duplicates=%d err=%v",
			created, duplicates, err,
		)
	}

	mutated := cloneConversationMessage(userMessage)
	mutated.Content = []ConversationContentPart{{Type: "text", Text: "mutated source content"}}
	if created, duplicates, err = store.IngestConversationMessages(
		ctx, ownerA, []ConversationMessage{mutated}, receivedAt.Add(2*time.Hour),
	); !errors.Is(err, ErrConflict) || created != 0 || duplicates != 0 {
		t.Fatalf(
			"mutated message_id should conflict: created=%d duplicates=%d err=%v",
			created, duplicates, err,
		)
	}

	// The first row is valid and would consume sequence 3. The second row has a
	// fresh message_id but reuses sequence 2. The sequence conflict must roll the
	// whole transaction back, including the preceding valid insert.
	sequenceThree := postgresConversationFixture(
		newID("conversation-pg-sequence-three"), conversationID, agentID, 3,
		now.Add(3*time.Second), "user", "must be rolled back",
	)
	sequenceCollision := postgresConversationFixture(
		newID("conversation-pg-sequence-collision"), conversationID, agentID, 2,
		now.Add(4*time.Second), "assistant", "conflicting sequence",
	)
	if created, duplicates, err = store.IngestConversationMessages(
		ctx, ownerA, []ConversationMessage{sequenceThree, sequenceCollision},
		receivedAt.Add(3*time.Hour),
	); !errors.Is(err, ErrConflict) || created != 0 || duplicates != 0 {
		t.Fatalf(
			"duplicate Conversation sequence should conflict atomically: created=%d duplicates=%d err=%v",
			created, duplicates, err,
		)
	}

	storedAfterConflict, err := store.ListConversationMessagesByOwner(
		ctx, ownerA, agentID, conversationID, 20,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedAfterConflict) != 2 ||
		storedAfterConflict[0].Content[0].Text != "Owner A request" ||
		storedAfterConflict[1].Content[0].Text != "Owner A response" {
		t.Fatalf("conflicting batch partially changed persisted Conversation: %#v", storedAfterConflict)
	}

	// The same source identifiers are independent in another owner namespace.
	ownerBUser := cloneConversationMessage(userMessage)
	ownerBUser.Content = []ConversationContentPart{{Type: "text", Text: "Owner B request"}}
	ownerBAssistant := cloneConversationMessage(assistantMessage)
	ownerBAssistant.Content = []ConversationContentPart{{Type: "text", Text: "Owner B response"}}
	created, duplicates, err = store.IngestConversationMessages(
		ctx, ownerB, []ConversationMessage{ownerBUser, ownerBAssistant},
		receivedAt.Add(4*time.Hour),
	)
	if err != nil || created != 2 || duplicates != 0 {
		t.Fatalf(
			"second owner could not reuse source identities: created=%d duplicates=%d err=%v",
			created, duplicates, err,
		)
	}
	storedB, err := store.ListConversationMessagesByOwner(
		ctx, ownerB, agentID, conversationID, 20,
	)
	if err != nil || len(storedB) != 2 || storedB[0].Content[0].Text != "Owner B request" {
		t.Fatalf("second owner Conversation was not isolated: messages=%#v err=%v", storedB, err)
	}
}

func postgresConversationFixture(
	messageID string,
	conversationID string,
	agentID string,
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
		AgentID:        agentID,
		AgentName:      "XiaoBaOS",
		Surface:        "pet",
		Role:           role,
		Content:        []ConversationContentPart{{Type: "text", Text: text}},
		Delivery:       ConversationDelivery{Status: deliveryStatus},
		TraceID:        "00112233445566778899aabbccddeeff",
	}
}
