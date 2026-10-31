package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
	backend, capabilities := "openviking", []string{"semantic", "filesystem"}
	if provider, ok := s.memory.(interface{ MemoryProvider() (string, []string) }); ok {
		backend, capabilities = provider.MemoryProvider()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       status,
		"backend":      backend,
		"capabilities": capabilities,
	})
}

func (s *HTTPServer) createMemoryNote(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	backend, ok := s.memory.(MemoryNoteBackend)
	if !ok {
		writeProblem(w, http.StatusServiceUnavailable, "Memory file writing is not available")
		return
	}
	var note MemoryNoteRequest
	if err := decodeJSON(w, r, &note); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	note.Title, note.Content = strings.TrimSpace(note.Title), strings.TrimSpace(note.Content)
	if note.Title == "" || len(note.Title) > 200 || strings.ContainsAny(note.Title, "\r\n") || note.Content == "" || len(note.Content) > 32000 {
		writeProblem(w, http.StatusBadRequest, "title (1-200 bytes) and content (1-32000 bytes) are required")
		return
	}
	record, err := backend.CreateNote(r.Context(), traceOwnerID(user), note)
	if err != nil {
		slog.Warn("memory note write failed")
		writeProblem(w, http.StatusBadGateway, "Memory note could not be indexed")
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *HTTPServer) listMemories(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Memory provider is not configured")
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
		slog.Warn("Memory provider memory listing failed")
		writeProblem(w, http.StatusBadGateway, "Memory provider memory listing failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	if recordErr == nil && (record.Status == "completed" || record.Status == "failed") {
		writeJSON(w, http.StatusOK, record)
		return
	}
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
		slog.Warn("Memory provider task status failed", "task_id", taskID)
		writeProblem(w, http.StatusBadGateway, "Memory task status is unavailable")
		return
	}
	if recordErr == nil {
		record = mergeMemoryTaskStatus(record, result, time.Now().UTC())
		if persistErr := s.store.UpsertMemoryTask(ctx, record); persistErr != nil {
			slog.Warn("memory task status persistence failed", "task_id", taskID)
		}
		writeJSON(w, http.StatusOK, record)
		return
	}
	// Compatibility for tasks created before Catena persisted its own task
	// ledger. Memory provider still enforces the owner-derived project boundary.
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) searchMemories(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.memory == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Memory provider is not configured")
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
		slog.Warn("Memory provider recall failed")
		writeProblem(w, http.StatusBadGateway, "Memory provider recall failed")
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
		slog.Warn("Memory provider fact graph failed", "fact_id", factID)
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
