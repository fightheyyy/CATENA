package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	conversationBatchSchema   = "xiaoba.conversation_batch.v1"
	conversationMessageSchema = "xiaoba.conversation_message.v1"
	conversationReceiptSchema = "catena.conversation_ingest_receipt.v1"
	conversationReadSchema    = "catena.conversation.v1"
	maxConversationBatch      = 200
	maxConversationContent    = 64 * 1024
)

type ConversationContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Name     string `json:"name,omitempty"`
	Ref      string `json:"ref,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type ConversationDelivery struct {
	Status             string   `json:"status"`
	PlatformMessageIDs []string `json:"platform_message_ids,omitempty"`
}

type ConversationMessage struct {
	Schema         string                    `json:"schema"`
	MessageID      string                    `json:"message_id"`
	ConversationID string                    `json:"conversation_id"`
	Sequence       int64                     `json:"sequence"`
	OccurredAt     time.Time                 `json:"occurred_at"`
	Runtime        string                    `json:"runtime"`
	AgentID        string                    `json:"agent_id"`
	AgentName      string                    `json:"agent_name,omitempty"`
	Surface        string                    `json:"surface"`
	Role           string                    `json:"role"`
	RoleName       string                    `json:"role_name,omitempty"`
	Content        []ConversationContentPart `json:"content"`
	Delivery       ConversationDelivery      `json:"delivery"`
	TraceID        string                    `json:"trace_id,omitempty"`
	ReceivedAt     time.Time                 `json:"received_at,omitempty"`
	OwnerUserID    string                    `json:"-"`
}

type ConversationBatchRequest struct {
	Schema   string                `json:"schema"`
	Messages []ConversationMessage `json:"messages"`
}

type ConversationIngestReceipt struct {
	Schema     string `json:"schema"`
	Created    int    `json:"created"`
	Duplicates int    `json:"duplicates"`
}

type ConversationSummary struct {
	ConversationID   string    `json:"conversation_id"`
	AgentID          string    `json:"agent_id"`
	AgentName        string    `json:"agent_name,omitempty"`
	Runtime          string    `json:"runtime"`
	Surface          string    `json:"surface"`
	Title            string    `json:"title"`
	MessageCount     int       `json:"message_count"`
	UserMessageCount int       `json:"user_message_count"`
	LastPreview      string    `json:"last_visible_message_preview,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ConversationDocument struct {
	Schema   string                `json:"schema"`
	Summary  ConversationSummary   `json:"conversation"`
	Messages []ConversationMessage `json:"messages"`
}

func (r ConversationBatchRequest) Validate() error {
	if r.Schema != conversationBatchSchema {
		return errors.New("unsupported Conversation batch schema")
	}
	if len(r.Messages) == 0 || len(r.Messages) > maxConversationBatch {
		return fmt.Errorf("messages must contain between 1 and %d items", maxConversationBatch)
	}
	seen := make(map[string]ConversationMessage, len(r.Messages))
	for index, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("messages[%d]: %w", index, err)
		}
		if previous, ok := seen[message.MessageID]; ok {
			if !sameConversationMessage(previous, message) {
				return fmt.Errorf("messages[%d]: duplicate message_id has different content", index)
			}
			continue
		}
		seen[message.MessageID] = message
	}
	return nil
}

func (m ConversationMessage) Validate() error {
	if m.Schema != conversationMessageSchema {
		return errors.New("unsupported Conversation message schema")
	}
	if !validConversationIdentifier(m.MessageID, 160) {
		return errors.New("message_id is invalid")
	}
	if !validConversationText(m.ConversationID, 512) {
		return errors.New("conversation_id is invalid")
	}
	if m.Sequence < 1 {
		return errors.New("sequence must be positive")
	}
	if m.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if m.Runtime != "xiaobaos" {
		return errors.New("runtime must be xiaobaos")
	}
	if !validConversationIdentifier(m.AgentID, 160) {
		return errors.New("agent_id is invalid")
	}
	if !validConversationTextOptional(m.AgentName, 160) || !validConversationTextOptional(m.RoleName, 160) {
		return errors.New("agent_name or role_name is invalid")
	}
	switch m.Surface {
	case "cli", "feishu", "weixin", "pet":
	default:
		return errors.New("surface must be cli, feishu, weixin, or pet")
	}
	if m.Role != "user" && m.Role != "assistant" {
		return errors.New("role must be user or assistant")
	}
	expectedDelivery := "received"
	if m.Role == "assistant" {
		expectedDelivery = "delivered"
	}
	if m.Delivery.Status != expectedDelivery {
		return fmt.Errorf("delivery.status must be %s for role %s", expectedDelivery, m.Role)
	}
	if len(m.Content) == 0 || len(m.Content) > 16 {
		return errors.New("content must contain between 1 and 16 visible parts")
	}
	total := 0
	for index, part := range m.Content {
		switch part.Type {
		case "text":
			if part.Text == "" || part.Name != "" || part.Ref != "" || part.MIMEType != "" {
				return fmt.Errorf("content[%d] is not a valid text part", index)
			}
			total += len(part.Text)
		case "file":
			if !validConversationText(part.Name, 512) || !validConversationTextOptional(part.Ref, 2048) ||
				!validConversationTextOptional(part.MIMEType, 256) || part.Text != "" {
				return fmt.Errorf("content[%d] is not a valid file part", index)
			}
			total += len(part.Name) + len(part.Ref) + len(part.MIMEType)
		default:
			return fmt.Errorf("content[%d].type must be text or file", index)
		}
	}
	if total > maxConversationContent {
		return errors.New("visible message content is too large")
	}
	if len(m.Delivery.PlatformMessageIDs) > 16 {
		return errors.New("too many platform_message_ids")
	}
	for _, id := range m.Delivery.PlatformMessageIDs {
		if !validConversationText(id, 512) {
			return errors.New("platform_message_ids contains an invalid value")
		}
	}
	if m.TraceID != "" && !validTraceID(m.TraceID) {
		return errors.New("trace_id must be a 32-character lowercase hexadecimal OTel trace ID")
	}
	return nil
}

func validConversationIdentifier(value string, limit int) bool {
	if value == "" || len(value) > limit {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._:-", char) {
			continue
		}
		return false
	}
	return true
}

func validConversationText(value string, limit int) bool {
	return value != "" && validConversationTextOptional(value, limit)
}

func validConversationTextOptional(value string, limit int) bool {
	return len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

func validTraceID(value string) bool {
	if len(value) != 32 || value == strings.Repeat("0", 32) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func conversationMessageFingerprint(message ConversationMessage) string {
	message.OwnerUserID = ""
	message.ReceivedAt = time.Time{}
	encoded, _ := json.Marshal(message)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func sameConversationMessage(left, right ConversationMessage) bool {
	return conversationMessageFingerprint(left) == conversationMessageFingerprint(right)
}

func cloneConversationMessage(message ConversationMessage) ConversationMessage {
	message.Content = append([]ConversationContentPart(nil), message.Content...)
	message.Delivery.PlatformMessageIDs = append([]string(nil), message.Delivery.PlatformMessageIDs...)
	return message
}

func (s *HTTPServer) ingestConversations(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireConversationIngestPrincipal(w, r)
	if !ok {
		return
	}
	var request ConversationBatchRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	// A bound connection key is authoritative. Override client-controlled
	// identity before validation so new clients do not need to send agent_id.
	if principal.Agent != nil {
		for index := range request.Messages {
			request.Messages[index].AgentID = principal.Agent.ID
			request.Messages[index].AgentName = principal.Agent.DisplayName
		}
	}
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerUserID := ""
	if principal.User != nil {
		ownerUserID = principal.User.ID
	}
	created, duplicates, err := s.store.IngestConversationMessages(
		r.Context(), ownerUserID, request.Messages, time.Now().UTC(),
	)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if principal.Agent != nil {
		_ = s.store.ObserveRegisteredAgent(
			r.Context(), ownerUserID, principal.Agent.ID, "xiaobaos", time.Now().UTC(),
		)
	}
	status := http.StatusCreated
	if created == 0 {
		status = http.StatusOK
	}
	writeJSON(w, status, ConversationIngestReceipt{
		Schema: conversationReceiptSchema, Created: created, Duplicates: duplicates,
	})
}

func (s *HTTPServer) listConversations(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := queryLimit(r, 50, 200)
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	if agentID != "" && !validConversationIdentifier(agentID, 160) {
		writeProblem(w, http.StatusBadRequest, "agent_id is invalid")
		return
	}
	ownerUserID := ""
	if user != nil {
		ownerUserID = user.ID
	}
	summaries, err := s.store.ListConversationSummariesByOwner(r.Context(), ownerUserID, agentID, limit)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema":        "catena.conversation_list.v1",
		"conversations": summaries,
	})
}

func (s *HTTPServer) getConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	conversationID := strings.TrimSpace(r.PathValue("conversation_id"))
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	if !validConversationText(conversationID, 512) || !validConversationIdentifier(agentID, 160) {
		writeProblem(w, http.StatusBadRequest, "conversation_id and agent_id are required")
		return
	}
	ownerUserID := ""
	if user != nil {
		ownerUserID = user.ID
	}
	messages, err := s.store.ListConversationMessagesByOwner(
		r.Context(), ownerUserID, agentID, conversationID, 2000,
	)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if len(messages) == 0 {
		writeProblem(w, http.StatusNotFound, "Conversation not found")
		return
	}
	writeJSON(w, http.StatusOK, ConversationDocument{
		Schema:   conversationReadSchema,
		Summary:  summarizeConversation(messages),
		Messages: messages,
	})
}

func queryLimit(r *http.Request, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if err != nil || value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func summarizeConversation(messages []ConversationMessage) ConversationSummary {
	ordered := append([]ConversationMessage(nil), messages...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Sequence == ordered[j].Sequence {
			return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
		}
		return ordered[i].Sequence < ordered[j].Sequence
	})
	first := ordered[0]
	last := ordered[len(ordered)-1]
	title := ""
	userCount := 0
	for _, message := range ordered {
		if message.Role == "user" {
			userCount++
			if title == "" {
				title = conversationTextPreview(message.Content, 80)
			}
		}
	}
	if title == "" {
		title = fmt.Sprintf("%s · %s", first.Surface, boundedRunes(first.ConversationID, 12))
	}
	return ConversationSummary{
		ConversationID:   first.ConversationID,
		AgentID:          first.AgentID,
		AgentName:        first.AgentName,
		Runtime:          first.Runtime,
		Surface:          first.Surface,
		Title:            title,
		MessageCount:     len(ordered),
		UserMessageCount: userCount,
		LastPreview:      conversationTextPreview(last.Content, 120),
		CreatedAt:        first.OccurredAt,
		UpdatedAt:        last.OccurredAt,
	}
}

func conversationTextPreview(parts []ConversationContentPart, limit int) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			values = append(values, strings.TrimSpace(part.Text))
		}
		if part.Type == "file" {
			values = append(values, "["+part.Name+"]")
		}
	}
	return boundedRunes(strings.Join(values, " "), limit)
}

func boundedRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func (s *MemoryStore) IngestConversationMessages(
	_ context.Context,
	ownerUserID string,
	messages []ConversationMessage,
	receivedAt time.Time,
) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	created := 0
	duplicates := 0
	pending := make(map[string]ConversationMessage, len(messages))
	for _, incoming := range messages {
		incoming.OwnerUserID = ownerUserID
		incoming.ReceivedAt = receivedAt
		key := ownerUserID + "\x00" + incoming.MessageID
		if existing, exists := s.conversationMessages[key]; exists {
			if !sameConversationMessage(existing, incoming) {
				return 0, 0, ErrConflict
			}
			duplicates++
			continue
		}
		if existing, exists := pending[key]; exists {
			if !sameConversationMessage(existing, incoming) {
				return 0, 0, ErrConflict
			}
			duplicates++
			continue
		}
		for _, existing := range s.conversationMessages {
			if existing.OwnerUserID == ownerUserID && existing.AgentID == incoming.AgentID &&
				existing.ConversationID == incoming.ConversationID && existing.Sequence == incoming.Sequence {
				return 0, 0, ErrConflict
			}
		}
		for _, existing := range pending {
			if existing.OwnerUserID == ownerUserID && existing.AgentID == incoming.AgentID &&
				existing.ConversationID == incoming.ConversationID && existing.Sequence == incoming.Sequence {
				return 0, 0, ErrConflict
			}
		}
		pending[key] = cloneConversationMessage(incoming)
		created++
	}
	for key, message := range pending {
		s.conversationMessages[key] = message
	}
	return created, duplicates, nil
}

func (s *MemoryStore) ListConversationSummariesByOwner(
	_ context.Context,
	ownerUserID string,
	agentID string,
	limit int,
) ([]ConversationSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	groups := make(map[string][]ConversationMessage)
	for _, message := range s.conversationMessages {
		if message.OwnerUserID != ownerUserID || (agentID != "" && message.AgentID != agentID) {
			continue
		}
		key := message.AgentID + "\x00" + message.ConversationID
		groups[key] = append(groups[key], cloneConversationMessage(message))
	}
	summaries := make([]ConversationSummary, 0, len(groups))
	for _, group := range groups {
		summaries = append(summaries, summarizeConversation(group))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

func (s *MemoryStore) ListConversationMessagesByOwner(
	_ context.Context,
	ownerUserID string,
	agentID string,
	conversationID string,
	limit int,
) ([]ConversationMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := make([]ConversationMessage, 0)
	for _, message := range s.conversationMessages {
		if message.OwnerUserID == ownerUserID && message.AgentID == agentID &&
			message.ConversationID == conversationID {
			messages = append(messages, cloneConversationMessage(message))
		}
	}
	if len(messages) == 0 {
		return nil, ErrNotFound
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Sequence < messages[j].Sequence
	})
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return messages, nil
}

func (s *PostgresStore) IngestConversationMessages(
	ctx context.Context,
	ownerUserID string,
	messages []ConversationMessage,
	receivedAt time.Time,
) (int, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	created := 0
	duplicates := 0
	for _, incoming := range messages {
		incoming.OwnerUserID = ownerUserID
		incoming.ReceivedAt = receivedAt
		content, _ := json.Marshal(incoming.Content)
		delivery, _ := json.Marshal(incoming.Delivery)
		var insertedID string
		err := tx.QueryRowContext(ctx, `
INSERT INTO catena_conversation_messages
  (owner_user_id, message_id, conversation_id, sequence, occurred_at,
   received_at, runtime, agent_id, agent_name, surface, role, role_name,
   content, delivery, trace_id, fingerprint)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (owner_user_id, message_id) DO NOTHING
RETURNING message_id`,
			ownerUserID, incoming.MessageID, incoming.ConversationID, incoming.Sequence,
			incoming.OccurredAt, receivedAt, incoming.Runtime, incoming.AgentID,
			incoming.AgentName, incoming.Surface, incoming.Role, incoming.RoleName,
			content, delivery, incoming.TraceID, conversationMessageFingerprint(incoming),
		).Scan(&insertedID)
		if err == nil {
			created++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, mapStoreError(err)
		}
		var existingFingerprint string
		if err := tx.QueryRowContext(ctx, `
SELECT fingerprint FROM catena_conversation_messages
WHERE owner_user_id = $1 AND message_id = $2`, ownerUserID, incoming.MessageID,
		).Scan(&existingFingerprint); err != nil {
			return 0, 0, mapStoreError(err)
		}
		if existingFingerprint != conversationMessageFingerprint(incoming) {
			return 0, 0, ErrConflict
		}
		duplicates++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, mapStoreError(err)
	}
	return created, duplicates, nil
}

func (s *PostgresStore) ListConversationSummariesByOwner(
	ctx context.Context,
	ownerUserID string,
	agentID string,
	limit int,
) ([]ConversationSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT agent_id, conversation_id
FROM catena_conversation_messages
WHERE owner_user_id = $1 AND ($2 = '' OR agent_id = $2)
GROUP BY agent_id, conversation_id
ORDER BY MAX(occurred_at) DESC, conversation_id
LIMIT $3`, ownerUserID, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type identity struct{ agentID, conversationID string }
	identities := make([]identity, 0)
	for rows.Next() {
		var item identity
		if err := rows.Scan(&item.agentID, &item.conversationID); err != nil {
			return nil, err
		}
		identities = append(identities, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	summaries := make([]ConversationSummary, 0, len(identities))
	for _, item := range identities {
		messages, err := s.ListConversationMessagesByOwner(ctx, ownerUserID, item.agentID, item.conversationID, 2000)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summarizeConversation(messages))
	}
	return summaries, nil
}

func (s *PostgresStore) ListConversationMessagesByOwner(
	ctx context.Context,
	ownerUserID string,
	agentID string,
	conversationID string,
	limit int,
) ([]ConversationMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT message_id, conversation_id, sequence, occurred_at, received_at,
       runtime, agent_id, agent_name, surface, role, role_name,
       content, delivery, trace_id
FROM (
  SELECT * FROM catena_conversation_messages
  WHERE owner_user_id = $1 AND agent_id = $2 AND conversation_id = $3
  ORDER BY sequence DESC
  LIMIT $4
) recent
ORDER BY sequence ASC`, ownerUserID, agentID, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]ConversationMessage, 0)
	for rows.Next() {
		message := ConversationMessage{Schema: conversationMessageSchema, OwnerUserID: ownerUserID}
		var content, delivery []byte
		if err := rows.Scan(
			&message.MessageID, &message.ConversationID, &message.Sequence,
			&message.OccurredAt, &message.ReceivedAt, &message.Runtime, &message.AgentID,
			&message.AgentName, &message.Surface, &message.Role, &message.RoleName,
			&content, &delivery, &message.TraceID,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(content, &message.Content); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(delivery, &message.Delivery); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, ErrNotFound
	}
	return messages, nil
}
