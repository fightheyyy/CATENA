package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings" // MemoryBackend is the private Catena-to-memory-provider boundary. Implementations do
	// not authenticate end users; Catena resolves the owner before calling it.
)

type MemoryBackend interface {
	Ping(context.Context) error
	Task(context.Context, string, string) (MemoryTaskStatus, error)
	List(context.Context, string, int) (MemoryList, error)
	Search(context.Context, string, MemorySearchRequest) (MemoryRecallBundle, error)
	Graph(context.Context, string, int64) (MemoryFactGraph, error)
}

type MemoryIngestReceipt struct {
	TraceID              string `json:"trace_id,omitempty"`
	SourceConversationID string `json:"source_conversation_id,omitempty"`
	ConversationID       int64  `json:"conversation_id"`
	TaskID               string `json:"task_id"`
	Status               string `json:"status"`
	Indexed              bool   `json:"indexed"`
	Message              string `json:"message,omitempty"`
}

type MemoryTaskStep struct {
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	StartedAt       string  `json:"started_at,omitempty"`
	FinishedAt      string  `json:"finished_at,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Error           string  `json:"error,omitempty"`
}

type MemoryTaskStatus struct {
	Result         json.RawMessage  `json:"result,omitempty"`
	TaskID         string           `json:"task_id"`
	Status         string           `json:"status"`
	CurrentStep    string           `json:"current_step,omitempty"`
	Progress       float64          `json:"progress"`
	Message        string           `json:"message,omitempty"`
	Error          string           `json:"error,omitempty"`
	ConversationID int64            `json:"conversation_id,omitempty"`
	CreatedAt      string           `json:"created_at,omitempty"`
	UpdatedAt      string           `json:"updated_at,omitempty"`
	Steps          []MemoryTaskStep `json:"steps"`
}

type MemorySearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (r MemorySearchRequest) normalized() (MemorySearchRequest, error) {
	r.Query = strings.TrimSpace(r.Query)
	if r.Query == "" || len(r.Query) > 4000 {
		return MemorySearchRequest{}, errors.New("query must contain from 1 to 4000 characters")
	}
	if r.TopK == 0 {
		r.TopK = 8
	}
	if r.TopK < 1 || r.TopK > 50 {
		return MemorySearchRequest{}, errors.New("top_k must be from 1 to 50")
	}
	return r, nil
}

type MemoryRecallItem struct {
	ID       string         `json:"id"`
	Content  string         `json:"content"`
	Title    string         `json:"title,omitempty"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type MemoryRecord struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	CreatedAt string         `json:"created_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type MemoryList struct {
	Memories []MemoryRecord `json:"memories"`
	Total    int            `json:"total"`
}

type MemoryRecallBundle struct {
	Success           bool               `json:"success"`
	Query             string             `json:"query"`
	Facts             []MemoryRecallItem `json:"facts"`
	Conversations     []MemoryRecallItem `json:"conversations"`
	Topics            []MemoryRecallItem `json:"topics"`
	ShortTermMemory   json.RawMessage    `json:"short_term_memory,omitempty"`
	RecentTurns       json.RawMessage    `json:"recent_turns,omitempty"`
	GraphExpansion    json.RawMessage    `json:"graph_expansion,omitempty"`
	TemporalExpansion json.RawMessage    `json:"temporal_expansion,omitempty"`
	SearchTimeMS      float64            `json:"search_time_ms,omitempty"`
}

type MemoryGraphEntity struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type MemoryGraphRelation struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Origin     string  `json:"origin,omitempty"`
}

// MemoryFactGraph is the public, tenant-safe view of one GauzMem fact
// neighborhood. It deliberately excludes GauzMem's private project_id.
type MemoryFactGraph struct {
	FactID         int64                 `json:"fact_id"`
	Content        string                `json:"content"`
	Entities       []MemoryGraphEntity   `json:"entities"`
	Relations      []MemoryGraphRelation `json:"relations"`
	TotalEntities  int                   `json:"total_entities"`
	TotalRelations int                   `json:"total_relations"`
}

func validMemoryTaskState(value string) bool {
	switch value {
	case "pending", "processing", "completed", "failed":
		return true
	default:
		return false
	}
}

type MemoryNoteRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type MemoryNoteBackend interface {
	CreateNote(context.Context, string, MemoryNoteRequest) (MemoryRecord, error)
}
