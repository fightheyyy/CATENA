package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxMemoryTraceSpans    = 64
	maxMemoryEvidenceBytes = 64 * 1024
	maxMemoryFieldBytes    = 2 * 1024
	maxMemoryResponseBytes = 4 * 1024 * 1024
)

// MemoryBackend is the private Catena-to-GauzMem boundary. Implementations do
// not authenticate end users; Catena resolves the owner before calling it.
type MemoryBackend interface {
	Ping(context.Context) error
	IngestTrace(context.Context, string, TraceDetail) (MemoryIngestReceipt, error)
	IngestConversation(context.Context, string, ConversationDocument) (MemoryIngestReceipt, error)
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

type GauzMemoryClient struct {
	baseURL *url.URL
	client  *http.Client
	apiKey  string
}

func NewGauzMemoryClient(rawURL string, apiKeys ...string) (*GauzMemoryClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("CATENA_MEMORY_URL must be an absolute HTTP(S) URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	apiKey := ""
	if len(apiKeys) > 0 {
		apiKey = strings.TrimSpace(apiKeys[0])
	}
	return &GauzMemoryClient{
		baseURL: parsed,
		client:  &http.Client{Timeout: 90 * time.Second},
		apiKey:  apiKey,
	}, nil
}

func (c *GauzMemoryClient) Ping(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/health"), nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("GauzMem health request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GauzMem health returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *GauzMemoryClient) IngestTrace(
	ctx context.Context,
	ownerID string,
	trace TraceDetail,
) (MemoryIngestReceipt, error) {
	document := traceMemoryDocument(trace)
	payload := map[string]any{
		"text":         document,
		"project_id":   memoryProjectID(ownerID),
		"source_name":  "catena-trace:" + trace.Summary.TraceID,
		"content_type": "conversation",
		"replace":      true,
		"metadata": map[string]any{
			"source":       "catena_trace",
			"source_type":  "catena_trace",
			"source_id":    trace.Summary.TraceID,
			"trace_id":     trace.Summary.TraceID,
			"file_hash":    memoryTraceSourceHash(trace.Summary.TraceID),
			"chunk_index":  0,
			"root_name":    trace.Summary.RootName,
			"service_name": trace.Summary.ServiceName,
			"agent_id":     trace.Summary.ServiceName,
			"model":        trace.Summary.Model,
			"started_at":   trace.Summary.StartTime.UTC().Format(time.RFC3339Nano),
			"ended_at":     trace.Summary.EndTime.UTC().Format(time.RFC3339Nano),
			"span_count":   trace.Summary.SpanCount,
			"error_count":  trace.Summary.ErrorCount,
		},
	}
	var upstream struct {
		Success        bool   `json:"success"`
		ConversationID int64  `json:"conversation_id"`
		TaskID         string `json:"task_id"`
		Status         string `json:"status"`
		Indexed        bool   `json:"indexed"`
		Message        string `json:"message"`
	}
	if err := c.postJSON(ctx, "/api/v1/memories/extract", payload, &upstream); err != nil {
		return MemoryIngestReceipt{}, err
	}
	if !upstream.Success || upstream.ConversationID < 1 || upstream.TaskID == "" {
		return MemoryIngestReceipt{}, errors.New("GauzMem returned an incomplete ingestion receipt")
	}
	return MemoryIngestReceipt{
		TraceID:        trace.Summary.TraceID,
		ConversationID: upstream.ConversationID,
		TaskID:         upstream.TaskID,
		Status:         upstream.Status,
		Indexed:        upstream.Indexed,
		Message:        upstream.Message,
	}, nil
}

func (c *GauzMemoryClient) IngestConversation(
	ctx context.Context,
	ownerID string,
	document ConversationDocument,
) (MemoryIngestReceipt, error) {
	conversationID := document.Summary.ConversationID
	payload := map[string]any{
		"text":         conversationMemoryDocument(document),
		"project_id":   memoryProjectID(ownerID),
		"source_name":  "catena-conversation:" + conversationID,
		"content_type": "conversation",
		"replace":      true,
		"metadata": map[string]any{
			"source":          "catena_conversation",
			"source_type":     "catena_conversation",
			"source_id":       conversationID,
			"conversation_id": conversationID,
			"file_hash":       memoryConversationSourceHash(document.Summary.AgentID, conversationID),
			"chunk_index":     0,
			"runtime":         document.Summary.Runtime,
			"agent_id":        document.Summary.AgentID,
			"agent_name":      document.Summary.AgentName,
			"surface":         document.Summary.Surface,
			"started_at":      document.Summary.CreatedAt.UTC().Format(time.RFC3339Nano),
			"ended_at":        document.Summary.UpdatedAt.UTC().Format(time.RFC3339Nano),
			"message_count":   document.Summary.MessageCount,
		},
	}
	var upstream struct {
		Success        bool   `json:"success"`
		ConversationID int64  `json:"conversation_id"`
		TaskID         string `json:"task_id"`
		Status         string `json:"status"`
		Indexed        bool   `json:"indexed"`
		Message        string `json:"message"`
	}
	if err := c.postJSON(ctx, "/api/v1/memories/extract", payload, &upstream); err != nil {
		return MemoryIngestReceipt{}, err
	}
	if !upstream.Success || upstream.ConversationID < 1 || upstream.TaskID == "" {
		return MemoryIngestReceipt{}, errors.New("GauzMem returned an incomplete ingestion receipt")
	}
	return MemoryIngestReceipt{
		SourceConversationID: conversationID,
		ConversationID:       upstream.ConversationID,
		TaskID:               upstream.TaskID,
		Status:               upstream.Status,
		Indexed:              upstream.Indexed,
		Message:              upstream.Message,
	}, nil
}

func (c *GauzMemoryClient) Task(
	ctx context.Context,
	ownerID string,
	taskID string,
) (MemoryTaskStatus, error) {
	taskID = strings.TrimSpace(taskID)
	if !validConversationIdentifier(taskID, 160) {
		return MemoryTaskStatus{}, errors.New("task_id must be a valid identifier")
	}
	var upstream struct {
		TaskID         string  `json:"task_id"`
		ProjectID      string  `json:"project_id"`
		Status         string  `json:"status"`
		CurrentStep    string  `json:"current_step"`
		Progress       float64 `json:"progress"`
		Message        string  `json:"message"`
		Error          string  `json:"error"`
		ConversationID int64   `json:"conversation_id"`
		CreatedAt      string  `json:"created_at"`
		UpdatedAt      string  `json:"updated_at"`
		Steps          []struct {
			Name            string  `json:"step_name"`
			Status          string  `json:"status"`
			StartedAt       string  `json:"start_time"`
			FinishedAt      string  `json:"end_time"`
			DurationSeconds float64 `json:"duration_seconds"`
			Error           string  `json:"error"`
		} `json:"steps"`
	}
	if err := c.getJSON(ctx, "/api/v1/tasks/"+url.PathEscape(taskID), &upstream); err != nil {
		var upstreamErr *gauzHTTPError
		if errors.As(err, &upstreamErr) && upstreamErr.Status == http.StatusNotFound {
			return MemoryTaskStatus{}, ErrNotFound
		}
		return MemoryTaskStatus{}, err
	}
	if upstream.TaskID != taskID || upstream.ProjectID != memoryProjectID(ownerID) {
		return MemoryTaskStatus{}, ErrNotFound
	}
	if !validMemoryTaskState(upstream.Status) {
		return MemoryTaskStatus{}, errors.New("GauzMem returned an invalid task state")
	}
	progress := upstream.Progress
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}
	steps := upstream.Steps
	if len(steps) > 20 {
		steps = steps[:20]
	}
	result := MemoryTaskStatus{
		TaskID:         taskID,
		Status:         upstream.Status,
		CurrentStep:    bounded(redactMemoryText(upstream.CurrentStep), 160),
		Progress:       progress,
		Message:        bounded(redactMemoryText(upstream.Message), 500),
		Error:          bounded(redactMemoryText(upstream.Error), 500),
		ConversationID: upstream.ConversationID,
		CreatedAt:      bounded(upstream.CreatedAt, 80),
		UpdatedAt:      bounded(upstream.UpdatedAt, 80),
		Steps:          make([]MemoryTaskStep, 0, len(steps)),
	}
	for _, step := range steps {
		if !validMemoryTaskState(step.Status) {
			continue
		}
		result.Steps = append(result.Steps, MemoryTaskStep{
			Name:            bounded(redactMemoryText(step.Name), 160),
			Status:          step.Status,
			StartedAt:       bounded(step.StartedAt, 80),
			FinishedAt:      bounded(step.FinishedAt, 80),
			DurationSeconds: step.DurationSeconds,
			Error:           bounded(redactMemoryText(step.Error), 500),
		})
	}
	return result, nil
}

func validMemoryTaskState(value string) bool {
	switch value {
	case "pending", "processing", "completed", "failed":
		return true
	default:
		return false
	}
}

func (c *GauzMemoryClient) List(
	ctx context.Context,
	ownerID string,
	limit int,
) (MemoryList, error) {
	if limit == 0 {
		limit = 24
	}
	if limit < 1 || limit > 100 {
		return MemoryList{}, errors.New("limit must be from 1 to 100")
	}
	payload := map[string]any{
		"project_id": memoryProjectID(ownerID),
		"page":       1,
		"page_size":  limit,
	}
	var upstream struct {
		Memories []struct {
			ID        string         `json:"id"`
			Memory    string         `json:"memory"`
			CreatedAt string         `json:"created_at"`
			Metadata  map[string]any `json:"metadata"`
		} `json:"memories"`
		Total int `json:"total"`
	}
	if err := c.postJSON(ctx, "/api/v1/memories/get", payload, &upstream); err != nil {
		return MemoryList{}, err
	}
	result := MemoryList{Memories: make([]MemoryRecord, 0, len(upstream.Memories)), Total: upstream.Total}
	for _, item := range upstream.Memories {
		result.Memories = append(result.Memories, MemoryRecord{
			ID: item.ID, Content: item.Memory, CreatedAt: item.CreatedAt, Metadata: item.Metadata,
		})
	}
	return result, nil
}

func (c *GauzMemoryClient) Search(
	ctx context.Context,
	ownerID string,
	request MemorySearchRequest,
) (MemoryRecallBundle, error) {
	normalized, err := request.normalized()
	if err != nil {
		return MemoryRecallBundle{}, err
	}
	payload := map[string]any{
		"query":      normalized.Query,
		"project_id": memoryProjectID(ownerID),
		"top_k":      normalized.TopK,
		"use_bm25":   true,
		"expansions": map[string]any{
			"graph": map[string]any{
				"enabled":  true,
				"max_hops": 2,
			},
			"temporal": map[string]any{
				"enabled":      true,
				"mode":         "turn",
				"hop_distance": 1,
				"direction":    "both",
			},
		},
	}
	var result MemoryRecallBundle
	if err := c.postJSON(ctx, "/api/v1/memories/search/bundle", payload, &result); err != nil {
		return MemoryRecallBundle{}, err
	}
	result.Query = normalized.Query
	if result.Facts == nil {
		result.Facts = []MemoryRecallItem{}
	}
	if result.Conversations == nil {
		result.Conversations = []MemoryRecallItem{}
	}
	if result.Topics == nil {
		result.Topics = []MemoryRecallItem{}
	}
	return result, nil
}

func (c *GauzMemoryClient) Graph(
	ctx context.Context,
	ownerID string,
	factID int64,
) (MemoryFactGraph, error) {
	if factID < 1 {
		return MemoryFactGraph{}, errors.New("fact_id must be a positive integer")
	}
	query := url.Values{}
	query.Set("project_id", memoryProjectID(ownerID))
	path := "/api/v1/graph/facts/" + strconv.FormatInt(factID, 10) + "?" + query.Encode()
	var result MemoryFactGraph
	if err := c.getJSON(ctx, path, &result); err != nil {
		return MemoryFactGraph{}, err
	}
	if result.FactID != factID || strings.TrimSpace(result.Content) == "" {
		return MemoryFactGraph{}, errors.New("GauzMem returned an incomplete fact graph")
	}
	if result.Entities == nil {
		result.Entities = []MemoryGraphEntity{}
	}
	if result.Relations == nil {
		result.Relations = []MemoryGraphRelation{}
	}
	return result, nil
}

func (c *GauzMemoryClient) getJSON(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpointWithQuery(path), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	return c.doJSON(request, output)
}

func (c *GauzMemoryClient) postJSON(ctx context.Context, path string, input any, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	return c.doJSON(request, output)
}

func (c *GauzMemoryClient) doJSON(request *http.Request, output any) error {
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("GauzMem request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxMemoryResponseBytes+1))
	if err != nil {
		return fmt.Errorf("GauzMem response failed: %w", err)
	}
	if len(responseBody) > maxMemoryResponseBytes {
		return errors.New("GauzMem response exceeded 4 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(responseBody, &problem)
		if problem.Detail == "" {
			problem.Detail = http.StatusText(response.StatusCode)
		}
		return &gauzHTTPError{Status: response.StatusCode, Detail: bounded(problem.Detail, 500)}
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return errors.New("GauzMem returned invalid JSON")
	}
	return nil
}

type gauzHTTPError struct {
	Status int
	Detail string
}

func (e *gauzHTTPError) Error() string {
	return fmt.Sprintf("GauzMem returned HTTP %d: %s", e.Status, e.Detail)
}

func (c *GauzMemoryClient) endpointWithQuery(path string) string {
	parsed, err := url.Parse(path)
	if err != nil {
		return c.endpoint(path)
	}
	copy := *c.baseURL
	copy.Path = strings.TrimRight(copy.Path, "/") + parsed.Path
	copy.RawQuery = parsed.RawQuery
	return copy.String()
}

func (c *GauzMemoryClient) endpoint(path string) string {
	copy := *c.baseURL
	copy.Path = strings.TrimRight(copy.Path, "/") + path
	return copy.String()
}

func memoryProjectID(ownerID string) string {
	digest := sha256.Sum256([]byte("catena-memory:" + ownerID))
	return "catena_" + hex.EncodeToString(digest[:12])
}

func memoryTraceSourceHash(traceID string) string {
	digest := sha256.Sum256([]byte("catena-trace:" + traceID))
	return hex.EncodeToString(digest[:])
}

func memoryConversationSourceHash(agentID string, conversationID string) string {
	digest := sha256.Sum256([]byte("catena-conversation:" + agentID + ":" + conversationID))
	return hex.EncodeToString(digest[:])
}

var memorySecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:sk|ghp|github_pat|barena_pat)_[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(?i)(authorization|api[_-]?key|access[_-]?token|secret|password)(\s*[=:]\s*)[^\s,;"'}]+`),
}

func redactMemoryText(value string) string {
	result := strings.ToValidUTF8(value, "�")
	for _, pattern := range memorySecretPatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			if index := strings.IndexAny(match, "=:"); index >= 0 {
				return match[:index+1] + "[REDACTED]"
			}
			if strings.HasPrefix(strings.ToLower(match), "bearer ") {
				return "Bearer [REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return result
}

func traceMemoryDocument(trace TraceDetail) string {
	spans := append([]TraceSpan(nil), trace.Spans...)
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].StartTime.Equal(spans[j].StartTime) {
			return spans[i].SpanID < spans[j].SpanID
		}
		return spans[i].StartTime.Before(spans[j].StartTime)
	})
	if len(spans) > maxMemoryTraceSpans {
		spans = spans[:maxMemoryTraceSpans]
	}
	var builder strings.Builder
	writeMemoryLine := func(format string, values ...any) bool {
		line := fmt.Sprintf(format, values...)
		remaining := maxMemoryEvidenceBytes - builder.Len()
		if remaining <= 0 {
			return false
		}
		if len(line) > remaining {
			line = line[:remaining]
		}
		builder.WriteString(line)
		return len(line) < remaining
	}
	writeMemoryLine("Catena Trace Evidence\ntrace_id: %s\nroot: %s\nservice: %s\nmodel: %s\nstarted_at: %s\nended_at: %s\nspans: %d\nerrors: %d\n",
		trace.Summary.TraceID,
		redactMemoryText(trace.Summary.RootName),
		redactMemoryText(trace.Summary.ServiceName),
		redactMemoryText(trace.Summary.Model),
		trace.Summary.StartTime.UTC().Format(time.RFC3339Nano),
		trace.Summary.EndTime.UTC().Format(time.RFC3339Nano),
		trace.Summary.SpanCount,
		trace.Summary.ErrorCount,
	)
	for index, span := range spans {
		if !writeMemoryLine("\nSpan %d\nname: %s\nservice: %s\nstatus: %d %s\n",
			index+1,
			redactMemoryText(span.Name),
			redactMemoryText(span.ServiceName),
			span.StatusCode,
			redactMemoryText(span.StatusMessage),
		) {
			break
		}
		if span.Input != "" {
			input := bounded(redactMemoryText(span.Input), maxMemoryFieldBytes)
			if !writeMemoryLine("input: %s\n", input) {
				break
			}
		}
		if span.Output != "" {
			output := bounded(redactMemoryText(span.Output), maxMemoryFieldBytes)
			if !writeMemoryLine("output: %s\n", output) {
				break
			}
		}
	}
	return builder.String()
}

func conversationMemoryDocument(document ConversationDocument) string {
	messages := append([]ConversationMessage(nil), document.Messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Sequence == messages[j].Sequence {
			return messages[i].OccurredAt.Before(messages[j].OccurredAt)
		}
		return messages[i].Sequence < messages[j].Sequence
	})
	var builder strings.Builder
	writeMemoryLine := func(format string, values ...any) bool {
		line := fmt.Sprintf(format, values...)
		remaining := maxMemoryEvidenceBytes - builder.Len()
		if remaining <= 0 {
			return false
		}
		if len(line) > remaining {
			line = line[:remaining]
		}
		builder.WriteString(line)
		return len(line) < remaining
	}
	writeMemoryLine("Catena User-visible Conversation\nconversation_id: %s\nruntime: %s\nagent: %s\nsurface: %s\nstarted_at: %s\nended_at: %s\nmessages: %d\n",
		redactMemoryText(document.Summary.ConversationID),
		redactMemoryText(document.Summary.Runtime),
		redactMemoryText(document.Summary.AgentID),
		redactMemoryText(document.Summary.Surface),
		document.Summary.CreatedAt.UTC().Format(time.RFC3339Nano),
		document.Summary.UpdatedAt.UTC().Format(time.RFC3339Nano),
		document.Summary.MessageCount,
	)
	for index, message := range messages {
		if !writeMemoryLine("\nMessage %d\nrole: %s\nat: %s\n",
			index+1,
			redactMemoryText(message.Role),
			message.OccurredAt.UTC().Format(time.RFC3339Nano),
		) {
			break
		}
		for _, part := range message.Content {
			switch part.Type {
			case "text":
				if !writeMemoryLine("text: %s\n", bounded(redactMemoryText(part.Text), maxMemoryFieldBytes)) {
					return builder.String()
				}
			case "file":
				if !writeMemoryLine("file: %s (%s)\n", bounded(redactMemoryText(part.Name), 512), bounded(redactMemoryText(part.MIMEType), 160)) {
					return builder.String()
				}
			}
		}
	}
	return builder.String()
}

func (s *HTTPServer) memoryStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}
	status := "unavailable"
	if s.memory != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.memory.Ping(ctx); err == nil {
			status = "available"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       status,
		"backend":      "gauzmem",
		"capabilities": []string{"semantic", "graph", "temporal"},
	})
}

func (s *HTTPServer) listMemories(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "GauzMem is not configured")
		return
	}
	limit := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeProblem(w, http.StatusBadRequest, "limit must be from 1 to 100")
			return
		}
		limit = parsed
	}
	result, err := s.memory.List(r.Context(), traceOwnerID(user), limit)
	if err != nil {
		slog.Warn("GauzMem memory listing failed", "error", err)
		writeProblem(w, http.StatusBadGateway, "GauzMem memory listing failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) rememberTrace(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.traces == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Trace storage is not configured")
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "GauzMem is not configured")
		return
	}
	traceID := strings.ToLower(strings.TrimSpace(r.PathValue("trace_id")))
	if len(traceID) != 32 {
		writeProblem(w, http.StatusBadRequest, "trace_id must be 32 lowercase hexadecimal characters")
		return
	}
	if _, err := hex.DecodeString(traceID); err != nil {
		writeProblem(w, http.StatusBadRequest, "trace_id must be 32 lowercase hexadecimal characters")
		return
	}
	ownerID := traceOwnerID(user)
	trace, err := s.traces.GetTrace(r.Context(), ownerID, traceID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	receipt, err := s.memory.IngestTrace(r.Context(), ownerID, trace)
	if err != nil {
		slog.Warn("GauzMem trace ingestion failed", "trace_id", traceID, "error", err)
		writeProblem(w, http.StatusBadGateway, "GauzMem trace ingestion failed")
		return
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

func (s *HTTPServer) rememberConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Memory service is not configured")
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
	document := ConversationDocument{
		Schema:   conversationReadSchema,
		Summary:  summarizeConversation(messages),
		Messages: messages,
	}
	memoryOwnerID := traceOwnerID(user)
	receipt, err := s.memory.IngestConversation(r.Context(), memoryOwnerID, document)
	if err != nil {
		slog.Warn("memory conversation ingestion failed", "conversation_id", conversationID, "error", err)
		writeProblem(w, http.StatusBadGateway, "Memory conversation ingestion failed")
		return
	}
	record := newMemoryTaskRecord(memoryOwnerID, document, receipt, time.Now().UTC())
	if err := s.store.UpsertMemoryTask(r.Context(), record); err != nil {
		slog.Error("memory task persistence failed", "task_id", receipt.TaskID, "error", err)
		writeProblem(w, http.StatusInternalServerError, "Memory task could not be tracked")
		return
	}
	writeJSON(w, http.StatusAccepted, record)
}

func (s *HTTPServer) listMemoryTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeProblem(w, http.StatusBadRequest, "limit must be from 1 to 100")
			return
		}
		limit = parsed
	}
	tasks, err := s.store.ListMemoryTasksByOwner(r.Context(), traceOwnerID(user), limit)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if tasks == nil {
		tasks = []MemoryTaskRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *HTTPServer) memoryTaskStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Memory service is not configured")
		return
	}
	taskID := strings.TrimSpace(r.PathValue("task_id"))
	if !validConversationIdentifier(taskID, 160) {
		writeProblem(w, http.StatusBadRequest, "task_id must be a valid identifier")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	ownerID := traceOwnerID(user)
	record, recordErr := s.store.GetMemoryTaskByOwner(ctx, ownerID, taskID)
	result, err := s.memory.Task(ctx, ownerID, taskID)
	if errors.Is(err, ErrNotFound) {
		if recordErr == nil && (record.Status == "completed" || record.Status == "failed") {
			writeJSON(w, http.StatusOK, record)
			return
		}
		writeProblem(w, http.StatusNotFound, "Memory task not found or expired")
		return
	}
	if err != nil {
		slog.Warn("GauzMem task status failed", "task_id", taskID, "error", err)
		writeProblem(w, http.StatusBadGateway, "Memory task status is unavailable")
		return
	}
	if recordErr == nil {
		record = mergeMemoryTaskStatus(record, result, time.Now().UTC())
		if persistErr := s.store.UpsertMemoryTask(ctx, record); persistErr != nil {
			slog.Warn("memory task status persistence failed", "task_id", taskID, "error", persistErr)
		}
		writeJSON(w, http.StatusOK, record)
		return
	}
	// Compatibility for tasks created before Catena persisted its own task
	// ledger. GauzMem still enforces the owner-derived project boundary.
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) searchMemories(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "GauzMem is not configured")
		return
	}
	var request MemorySearchRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	normalized, err := request.normalized()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.memory.Search(r.Context(), traceOwnerID(user), normalized)
	if err != nil {
		slog.Warn("GauzMem recall failed", "error", err)
		writeProblem(w, http.StatusBadGateway, "GauzMem recall failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) memoryFactGraph(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Memory service is not configured")
		return
	}
	factID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("fact_id")), 10, 64)
	if err != nil || factID < 1 {
		writeProblem(w, http.StatusBadRequest, "fact_id must be a positive integer")
		return
	}
	ownerID := traceOwnerID(user)
	result, err := s.memory.Graph(r.Context(), ownerID, factID)
	if err != nil {
		slog.Warn("GauzMem fact graph failed", "fact_id", factID, "error", err)
		writeProblem(w, http.StatusBadGateway, "Memory graph is unavailable")
		return
	}
	result = s.augmentMemoryGraph(r.Context(), ownerID, result)
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) augmentMemoryGraph(ctx context.Context, ownerID string, graph MemoryFactGraph) MemoryFactGraph {
	for index := range graph.Relations {
		if graph.Relations[index].Origin == "" {
			graph.Relations[index].Origin = "semantic"
		}
	}
	memories, err := s.memory.List(ctx, ownerID, 100)
	if err != nil {
		return graph
	}
	selectedIndex := -1
	for index, memory := range memories.Memories {
		if memory.ID == strconv.FormatInt(graph.FactID, 10) {
			selectedIndex = index
			break
		}
	}
	if selectedIndex < 0 {
		return graph
	}
	selected := memories.Memories[selectedIndex]
	conversationID := memoryMetadataString(selected.Metadata, "conversation_id")
	agentID := memoryMetadataString(selected.Metadata, "agent_id")
	agentName := memoryMetadataString(selected.Metadata, "agent_name")
	if agentName == "" {
		agentName = agentID
	}
	if conversationID != "" && !memoryGraphHasEntity(graph.Entities, conversationID) {
		graph.Entities = append(graph.Entities, MemoryGraphEntity{
			Name: conversationID, Type: "conversation", Description: "Source Conversation",
		})
	}
	if conversationID != "" && !memoryGraphHasRelation(graph.Relations, graph.Content, conversationID, "SOURCE_CONVERSATION") {
		graph.Relations = append(graph.Relations, MemoryGraphRelation{
			Source: graph.Content, Target: conversationID, Type: "SOURCE_CONVERSATION",
			Confidence: 1, Origin: "provenance",
		})
	}
	if agentName != "" && !memoryGraphHasEntity(graph.Entities, agentName) {
		graph.Entities = append(graph.Entities, MemoryGraphEntity{
			Name: agentName, Type: "agent", Description: "Source Agent",
		})
	}
	if agentName != "" && !memoryGraphHasRelation(graph.Relations, graph.Content, agentName, "SOURCE_AGENT") {
		graph.Relations = append(graph.Relations, MemoryGraphRelation{
			Source: graph.Content, Target: agentName, Type: "SOURCE_AGENT",
			Confidence: 1, Origin: "provenance",
		})
	}
	semanticRelationCount := 0
	for _, relation := range graph.Relations {
		if relation.Origin == "semantic" {
			semanticRelationCount++
		}
	}
	if semanticRelationCount == 0 && conversationID != "" {
		added := 0
		for _, candidate := range memories.Memories {
			if candidate.ID == selected.ID ||
				memoryMetadataString(candidate.Metadata, "conversation_id") != conversationID ||
				strings.TrimSpace(candidate.Content) == "" {
				continue
			}
			graph.Relations = append(graph.Relations, MemoryGraphRelation{
				Source: graph.Content, Target: candidate.Content, Type: "SAME_CONVERSATION",
				Confidence: 1, Origin: "provenance",
			})
			added++
			if added == 4 {
				break
			}
		}
	}
	graph.TotalEntities = len(graph.Entities)
	graph.TotalRelations = len(graph.Relations)
	return graph
}

func memoryMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func memoryGraphHasEntity(entities []MemoryGraphEntity, name string) bool {
	for _, entity := range entities {
		if strings.EqualFold(strings.TrimSpace(entity.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func memoryGraphHasRelation(relations []MemoryGraphRelation, source string, target string, relationType string) bool {
	for _, relation := range relations {
		if strings.EqualFold(strings.TrimSpace(relation.Source), strings.TrimSpace(source)) &&
			strings.EqualFold(strings.TrimSpace(relation.Target), strings.TrimSpace(target)) &&
			strings.EqualFold(strings.TrimSpace(relation.Type), strings.TrimSpace(relationType)) {
			return true
		}
	}
	return false
}
