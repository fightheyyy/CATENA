package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// MemoryTaskRecord is Catena's durable projection of an asynchronous GauzMem
// extraction. The provider remains responsible for execution; Catena owns the
// user-visible source identity and last known task state.
type MemoryTaskRecord struct {
	MemoryTaskStatus
	OwnerUserID             string    `json:"-"`
	SourceConversationID    string    `json:"source_conversation_id"`
	SourceConversationTitle string    `json:"source_conversation_title,omitempty"`
	AgentID                 string    `json:"agent_id"`
	AgentName               string    `json:"agent_name,omitempty"`
	CreatedAtTime           time.Time `json:"-"`
	UpdatedAtTime           time.Time `json:"-"`
}

func newMemoryTaskRecord(
	ownerUserID string,
	document ConversationDocument,
	receipt MemoryIngestReceipt,
	now time.Time,
) MemoryTaskRecord {
	status := strings.TrimSpace(receipt.Status)
	if !validMemoryTaskState(status) {
		status = "pending"
	}
	return MemoryTaskRecord{
		MemoryTaskStatus: MemoryTaskStatus{
			TaskID:         receipt.TaskID,
			Status:         status,
			Progress:       memoryTaskInitialProgress(status),
			Message:        receipt.Message,
			ConversationID: receipt.ConversationID,
			CreatedAt:      now.UTC().Format(time.RFC3339Nano),
			UpdatedAt:      now.UTC().Format(time.RFC3339Nano),
			Steps:          []MemoryTaskStep{},
		},
		OwnerUserID:             ownerUserID,
		SourceConversationID:    document.Summary.ConversationID,
		SourceConversationTitle: bounded(document.Summary.Title, 500),
		AgentID:                 document.Summary.AgentID,
		AgentName:               bounded(document.Summary.AgentName, 160),
		CreatedAtTime:           now.UTC(),
		UpdatedAtTime:           now.UTC(),
	}
}

func memoryTaskInitialProgress(status string) float64 {
	if status == "completed" {
		return 1
	}
	return 0
}

func (r MemoryTaskRecord) valid() bool {
	return validConversationIdentifier(r.TaskID, 160) &&
		strings.TrimSpace(r.OwnerUserID) != "" &&
		validConversationText(r.SourceConversationID, 512) &&
		validConversationIdentifier(r.AgentID, 160) &&
		validMemoryTaskState(r.Status)
}

func mergeMemoryTaskStatus(record MemoryTaskRecord, status MemoryTaskStatus, now time.Time) MemoryTaskRecord {
	if record.Status == "completed" || record.Status == "failed" {
		return record
	}
	record.MemoryTaskStatus = status
	record.OwnerUserID = strings.TrimSpace(record.OwnerUserID)
	record.UpdatedAtTime = now.UTC()
	record.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	if record.CreatedAt == "" {
		record.CreatedAt = record.CreatedAtTime.UTC().Format(time.RFC3339Nano)
	}
	if record.Steps == nil {
		record.Steps = []MemoryTaskStep{}
	}
	return record
}

func cloneMemoryTaskRecord(input MemoryTaskRecord) MemoryTaskRecord {
	result := input
	result.Steps = append([]MemoryTaskStep(nil), input.Steps...)
	return result
}

func (s *MemoryStore) UpsertMemoryTask(_ context.Context, record MemoryTaskRecord) error {
	if !record.valid() {
		return ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.memoryTasks[record.TaskID]; ok {
		if existing.OwnerUserID != record.OwnerUserID {
			return ErrConflict
		}
		if (existing.Status == "completed" || existing.Status == "failed") &&
			(record.Status == "pending" || record.Status == "processing") {
			return ErrConflict
		}
		if record.UpdatedAtTime.Before(existing.UpdatedAtTime) {
			return ErrConflict
		}
	}
	s.memoryTasks[record.TaskID] = cloneMemoryTaskRecord(record)
	return nil
}

func (s *MemoryStore) GetMemoryTaskByOwner(_ context.Context, ownerUserID string, taskID string) (MemoryTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.memoryTasks[taskID]
	if !ok || record.OwnerUserID != ownerUserID {
		return MemoryTaskRecord{}, ErrNotFound
	}
	return cloneMemoryTaskRecord(record), nil
}

func (s *MemoryStore) ListMemoryTasksByOwner(_ context.Context, ownerUserID string, limit int) ([]MemoryTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]MemoryTaskRecord, 0)
	for _, record := range s.memoryTasks {
		if record.OwnerUserID == ownerUserID {
			result = append(result, cloneMemoryTaskRecord(record))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAtTime.After(result[j].UpdatedAtTime) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *PostgresStore) UpsertMemoryTask(ctx context.Context, record MemoryTaskRecord) error {
	if !record.valid() {
		return ErrConflict
	}
	document, err := json.Marshal(record)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO catena_memory_tasks
  (task_id,owner_user_id,source_conversation_id,agent_id,status,document,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (task_id) DO UPDATE SET
  status=EXCLUDED.status,
  document=EXCLUDED.document,
  updated_at=EXCLUDED.updated_at
WHERE catena_memory_tasks.owner_user_id=EXCLUDED.owner_user_id
  AND EXCLUDED.updated_at >= catena_memory_tasks.updated_at
  AND NOT (
    catena_memory_tasks.status IN ('completed','failed')
    AND EXCLUDED.status IN ('pending','processing')
  )`,
		record.TaskID, record.OwnerUserID, record.SourceConversationID, record.AgentID,
		record.Status, document, record.CreatedAtTime, record.UpdatedAtTime)
	if err != nil {
		return mapStoreError(err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *PostgresStore) GetMemoryTaskByOwner(ctx context.Context, ownerUserID string, taskID string) (MemoryTaskRecord, error) {
	return scanMemoryTaskRecord(s.db.QueryRowContext(ctx, `
SELECT owner_user_id,created_at,updated_at,document
FROM catena_memory_tasks WHERE owner_user_id=$1 AND task_id=$2`, ownerUserID, taskID))
}

func (s *PostgresStore) ListMemoryTasksByOwner(ctx context.Context, ownerUserID string, limit int) ([]MemoryTaskRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT owner_user_id,created_at,updated_at,document
FROM catena_memory_tasks WHERE owner_user_id=$1
ORDER BY updated_at DESC LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MemoryTaskRecord, 0)
	for rows.Next() {
		record, scanErr := scanMemoryTaskRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func scanMemoryTaskRecord(row rowScanner) (MemoryTaskRecord, error) {
	var ownerUserID string
	var createdAt time.Time
	var updatedAt time.Time
	var document json.RawMessage
	if err := row.Scan(&ownerUserID, &createdAt, &updatedAt, &document); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MemoryTaskRecord{}, ErrNotFound
		}
		return MemoryTaskRecord{}, err
	}
	var record MemoryTaskRecord
	if err := json.Unmarshal(document, &record); err != nil {
		return MemoryTaskRecord{}, err
	}
	record.OwnerUserID = ownerUserID
	record.CreatedAtTime = createdAt.UTC()
	record.UpdatedAtTime = updatedAt.UTC()
	if record.CreatedAt == "" {
		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	}
	if record.Steps == nil {
		record.Steps = []MemoryTaskStep{}
	}
	return record, nil
}
