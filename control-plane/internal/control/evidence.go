package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	evolutionJobSchema             = "spiral.evolution_job.v1"
	evolutionCandidateStatus       = "draft/unverified"
	evolutionReviewScope           = "proposal_only"
	evolutionEvidenceSchema        = "catena.evolution_evidence_pack.v1"
	evolutionAgentEvidenceSchema   = "catena.agent_trace_set_evidence_pack.v1"
	evolutionReleaseAuthority      = "human"
	maxEvolutionEvidenceSpans      = 64
	maxEvolutionEvidenceEvents     = 32
	maxEvolutionEvidenceFieldBytes = 2 * 1024
	maxEvolutionEvidencePackBytes  = 256 * 1024
	maxAgentEvolutionTraceScan     = 100
	maxAgentEvolutionTraces        = 12
	maxAgentEvolutionEvidenceSpans = 64
)

func normalizedEvolutionOutputLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "zh", "zh-cn", "zh-hans":
		return "zh-CN", nil
	case "en", "en-us", "en-gb":
		return "en", nil
	default:
		return "", errors.New("output_language must be zh-CN or en")
	}
}

func selectAgentEvolutionTraces(values []TraceSummary, limit int) []TraceSummary {
	selected := append([]TraceSummary(nil), values...)
	sort.SliceStable(selected, func(i, j int) bool {
		leftHasError := selected[i].ErrorCount > 0
		rightHasError := selected[j].ErrorCount > 0
		if leftHasError != rightHasError {
			return leftHasError
		}
		if !selected[i].EndTime.Equal(selected[j].EndTime) {
			return selected[i].EndTime.After(selected[j].EndTime)
		}
		return selected[i].TraceID < selected[j].TraceID
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	return selected
}

func normalizedEvolutionTraceID(value string) (string, error) {
	traceID := strings.ToLower(strings.TrimSpace(value))
	if len(traceID) != 32 {
		return "", fmt.Errorf("trace_id must be 32 hexadecimal characters")
	}
	if _, err := hex.DecodeString(traceID); err != nil {
		return "", fmt.Errorf("trace_id must be 32 hexadecimal characters")
	}
	return traceID, nil
}

func buildEvolutionEvidencePack(
	sourceKind EvolutionSourceKind,
	traceID string,
	trace *TraceDetail,
	createdAt time.Time,
) (EvolutionEvidencePack, error) {
	if !sourceKind.Valid() || strings.TrimSpace(traceID) == "" {
		return EvolutionEvidencePack{}, fmt.Errorf("Evolution evidence source is invalid")
	}
	if trace == nil && sourceKind == EvolutionSourceTrace {
		return EvolutionEvidencePack{}, fmt.Errorf("stored Trace evidence is required")
	}
	pack := EvolutionEvidencePack{
		Schema:        evolutionEvidenceSchema,
		SourceKind:    sourceKind,
		SourceTraceID: traceID,
		Spans:         []EvolutionEvidenceSpan{},
		Redacted:      true,
		Boundary: EvolutionEvidenceBoundary{
			TargetAgentExecutedByCatena: false,
			CreatesRelease:              false,
			ReleaseAuthority:            evolutionReleaseAuthority,
			CandidateStatus:             evolutionCandidateStatus,
			ReviewScope:                 evolutionReviewScope,
		},
		CreatedAt: createdAt.UTC(),
	}
	if trace != nil {
		if trace.Summary.TraceID != traceID {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace identity does not match the Evolution source")
		}
		if len(trace.Spans) == 0 {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace contains no spans")
		}
		pack.SourceRuntimeKind = detectOTLPRuntime(trace.Spans)
		pack.TraceSummary = trace.Summary
		pack.TraceSummary.RootName = bounded(redactMemoryText(pack.TraceSummary.RootName), 1000)
		pack.TraceSummary.ServiceName = bounded(redactMemoryText(pack.TraceSummary.ServiceName), 1000)
		pack.TraceSummary.Model = bounded(redactMemoryText(pack.TraceSummary.Model), 1000)
		pack.TotalSpanCount = trace.Summary.SpanCount
		if pack.TotalSpanCount == 0 {
			pack.TotalSpanCount = uint64(len(trace.Spans))
		}
		spans := append([]TraceSpan(nil), trace.Spans...)
		sort.SliceStable(spans, func(i, j int) bool {
			if spans[i].StartTime.Equal(spans[j].StartTime) {
				return spans[i].SpanID < spans[j].SpanID
			}
			return spans[i].StartTime.Before(spans[j].StartTime)
		})
		if len(spans) > maxEvolutionEvidenceSpans {
			spans = spans[:maxEvolutionEvidenceSpans]
			pack.Truncated = true
		}
		for _, span := range spans {
			if span.TraceID != traceID {
				return EvolutionEvidencePack{}, fmt.Errorf("stored Trace contains a mismatched span")
			}
			eventNames := make([]string, 0, len(span.Events))
			for _, event := range span.Events {
				if name := bounded(redactMemoryText(event.Name), 256); name != "" {
					eventNames = append(eventNames, name)
				}
				if len(eventNames) == 16 {
					pack.Truncated = pack.Truncated || len(span.Events) > len(eventNames)
					break
				}
			}
			input := sanitizeEvolutionText(span.Input, maxEvolutionEvidenceFieldBytes)
			output := sanitizeEvolutionText(span.Output, maxEvolutionEvidenceFieldBytes)
			if len(span.Input) > len(input) || len(span.Output) > len(output) {
				pack.Truncated = true
			}
			pack.Spans = append(pack.Spans, EvolutionEvidenceSpan{
				SpanID:        span.SpanID,
				ParentSpanID:  span.ParentSpanID,
				Name:          bounded(redactMemoryText(span.Name), 1000),
				ServiceName:   bounded(redactMemoryText(span.ServiceName), 1000),
				StartTime:     span.StartTime,
				EndTime:       span.EndTime,
				StatusCode:    span.StatusCode,
				StatusMessage: bounded(redactMemoryText(span.StatusMessage), 1000),
				Model:         bounded(redactMemoryText(span.Model), 1000),
				ToolName: bounded(redactMemoryText(firstAttributeString(
					span.Attributes,
					"tool.name",
					"gen_ai.tool.name",
					"tool.call.name",
					"xiaoba.tool.name",
				)), 256),
				Input:      input,
				Output:     output,
				EventNames: eventNames,
			})
		}
	}
	pack.IncludedSpanCount = len(pack.Spans)
	for {
		encoded, err := evolutionEvidenceDigestInput(pack)
		if err != nil {
			return EvolutionEvidencePack{}, err
		}
		if len(encoded) <= maxEvolutionEvidencePackBytes {
			digest := sha256.Sum256(encoded)
			pack.SHA256 = hex.EncodeToString(digest[:])
			return pack, nil
		}
		pack.Truncated = true
		if len(pack.Spans) > 1 {
			pack.Spans = pack.Spans[:len(pack.Spans)-1]
			pack.IncludedSpanCount = len(pack.Spans)
			continue
		}
		return EvolutionEvidencePack{}, fmt.Errorf("Evolution Evidence Pack exceeds the safe prompt limit")
	}
}

func buildAgentTraceSetEvidencePack(
	agentID string,
	windowStart time.Time,
	windowEnd time.Time,
	traces []TraceDetail,
	totalTraceCount int,
	createdAt time.Time,
) (EvolutionEvidencePack, error) {
	if strings.TrimSpace(agentID) == "" || len(traces) < 2 || !windowEnd.After(windowStart) {
		return EvolutionEvidencePack{}, fmt.Errorf("Agent Trace Set evidence source is invalid")
	}
	windowStart = windowStart.UTC()
	windowEnd = windowEnd.UTC()
	pack := EvolutionEvidencePack{
		Schema:            evolutionAgentEvidenceSchema,
		SourceKind:        EvolutionSourceAgentTraceSet,
		SourceAgentID:     agentID,
		SourceRuntimeKind: detectEvolutionRuntime(traces),
		WindowStart:       &windowStart,
		WindowEnd:         &windowEnd,
		SourceTraceIDs:    []string{},
		Spans:             []EvolutionEvidenceSpan{},
		Traces:            []EvolutionEvidenceTrace{},
		TotalTraceCount:   totalTraceCount,
		Redacted:          true,
		Boundary: EvolutionEvidenceBoundary{
			TargetAgentExecutedByCatena: false,
			CreatesRelease:              false,
			ReleaseAuthority:            evolutionReleaseAuthority,
			CandidateStatus:             evolutionCandidateStatus,
			ReviewScope:                 evolutionReviewScope,
		},
		CreatedAt: createdAt.UTC(),
	}
	if pack.TotalTraceCount < len(traces) {
		pack.TotalTraceCount = len(traces)
	}
	pack.Truncated = pack.TotalTraceCount > len(traces)
	perTraceBudget := maxAgentEvolutionEvidenceSpans / len(traces)
	if perTraceBudget < 1 {
		perTraceBudget = 1
	}
	if perTraceBudget > 12 {
		perTraceBudget = 12
	}
	for _, trace := range traces {
		if trace.Summary.TraceID == "" || !traceSummaryBelongsToAgent(trace.Summary, agentID) || len(trace.Spans) == 0 {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace does not belong to the selected Agent")
		}
		if trace.Summary.EndTime.Before(windowStart) || trace.Summary.StartTime.After(windowEnd) {
			return EvolutionEvidencePack{}, fmt.Errorf("stored Trace is outside the selected Agent window")
		}
		legacy, err := buildEvolutionEvidencePack(
			EvolutionSourceTrace, trace.Summary.TraceID, &trace, createdAt,
		)
		if err != nil {
			return EvolutionEvidencePack{}, err
		}
		spans := selectAgentEvidenceSpans(legacy.Spans, perTraceBudget)
		traceEvidence := EvolutionEvidenceTrace{
			Summary:           legacy.TraceSummary,
			Spans:             spans,
			IncludedSpanCount: len(spans),
			TotalSpanCount:    legacy.TotalSpanCount,
			Truncated:         legacy.Truncated || len(spans) < len(legacy.Spans),
		}
		pack.SourceTraceIDs = append(pack.SourceTraceIDs, trace.Summary.TraceID)
		pack.Traces = append(pack.Traces, traceEvidence)
		pack.IncludedSpanCount += traceEvidence.IncludedSpanCount
		pack.TotalSpanCount += traceEvidence.TotalSpanCount
		pack.Truncated = pack.Truncated || traceEvidence.Truncated
	}
	pack.IncludedTraceCount = len(pack.Traces)
	for {
		encoded, err := evolutionEvidenceDigestInput(pack)
		if err != nil {
			return EvolutionEvidencePack{}, err
		}
		if len(encoded) <= maxEvolutionEvidencePackBytes {
			digest := sha256.Sum256(encoded)
			pack.SHA256 = hex.EncodeToString(digest[:])
			return pack, nil
		}
		pack.Truncated = true
		trimmed := false
		for index := len(pack.Traces) - 1; index >= 0; index-- {
			if len(pack.Traces[index].Spans) > 1 {
				pack.Traces[index].Spans = pack.Traces[index].Spans[:len(pack.Traces[index].Spans)-1]
				pack.Traces[index].IncludedSpanCount--
				pack.Traces[index].Truncated = true
				pack.IncludedSpanCount--
				trimmed = true
				break
			}
		}
		if trimmed {
			continue
		}
		if len(pack.Traces) > 1 {
			last := len(pack.Traces) - 1
			pack.IncludedSpanCount -= pack.Traces[last].IncludedSpanCount
			pack.Traces = pack.Traces[:last]
			pack.SourceTraceIDs = pack.SourceTraceIDs[:last]
			pack.IncludedTraceCount = len(pack.Traces)
			continue
		}
		return EvolutionEvidencePack{}, fmt.Errorf("Agent Trace Set Evidence Pack exceeds the safe prompt limit")
	}
}

func detectEvolutionRuntime(traces []TraceDetail) string {
	detected := ""
	for _, trace := range traces {
		runtimeKind := detectOTLPRuntime(trace.Spans)
		if runtimeKind == "otel" || runtimeKind == "" {
			return "otel"
		}
		if detected != "" && detected != runtimeKind {
			return "otel"
		}
		detected = runtimeKind
	}
	if detected == "" {
		return "otel"
	}
	return detected
}

func selectAgentEvidenceSpans(values []EvolutionEvidenceSpan, limit int) []EvolutionEvidenceSpan {
	selected := append([]EvolutionEvidenceSpan(nil), values...)
	sort.SliceStable(selected, func(i, j int) bool {
		leftPriority := agentEvidenceSpanPriority(selected[i])
		rightPriority := agentEvidenceSpanPriority(selected[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if !selected[i].StartTime.Equal(selected[j].StartTime) {
			return selected[i].StartTime.Before(selected[j].StartTime)
		}
		return selected[i].SpanID < selected[j].SpanID
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if !selected[i].StartTime.Equal(selected[j].StartTime) {
			return selected[i].StartTime.Before(selected[j].StartTime)
		}
		return selected[i].SpanID < selected[j].SpanID
	})
	return selected
}

func agentEvidenceSpanPriority(span EvolutionEvidenceSpan) int {
	if span.StatusCode == 2 {
		return 0
	}
	if span.ToolName != "" {
		return 1
	}
	if span.ParentSpanID == "" {
		return 2
	}
	if span.Input != "" || span.Output != "" {
		return 3
	}
	return 4
}

func evolutionEvidenceDigestInput(pack EvolutionEvidencePack) ([]byte, error) {
	pack.SHA256 = ""
	encoded, err := json.Marshal(pack)
	if err != nil {
		return nil, err
	}
	// PostgreSQL JSONB deliberately discards whitespace and object-key order,
	// including inside json.RawMessage fields. Hash a canonical decoded value so
	// an Evidence Pack keeps the same identity after a durable round trip.
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func validEvolutionEvidencePack(pack EvolutionEvidencePack, job EvolutionJob) bool {
	if pack.SourceKind != job.SourceKind ||
		pack.SourceRuntimeKind != job.SourceRuntimeKind ||
		pack.Boundary.TargetAgentExecutedByCatena || pack.Boundary.CreatesRelease ||
		pack.Boundary.ReleaseAuthority != evolutionReleaseAuthority ||
		pack.Boundary.CandidateStatus != evolutionCandidateStatus ||
		pack.Boundary.ReviewScope != evolutionReviewScope {
		return false
	}
	if job.SourceKind == EvolutionSourceAgentTraceSet {
		if pack.Schema != evolutionAgentEvidenceSchema ||
			pack.SourceAgentID == "" || pack.SourceAgentID != job.SourceAgentID ||
			pack.WindowStart == nil || pack.WindowEnd == nil ||
			job.WindowStart == nil || job.WindowEnd == nil ||
			!pack.WindowStart.Equal(*job.WindowStart) || !pack.WindowEnd.Equal(*job.WindowEnd) ||
			len(pack.SourceTraceIDs) < 2 ||
			!equalStringSlices(pack.SourceTraceIDs, job.SourceTraceIDs) ||
			pack.IncludedTraceCount != len(pack.Traces) || len(pack.Traces) < 2 {
			return false
		}
	} else if job.SourceKind == EvolutionSourceConversation {
		if pack.Schema != "catena.conversation_evidence.v2" || pack.SourceConversationID != job.SourceConversationID || pack.SourceAgentID != job.SourceAgentID || pack.Document == "" {
			return false
		}
	} else if pack.Schema != evolutionEvidenceSchema ||
		pack.SourceTraceID != job.SourceTraceID {
		return false
	}
	encoded, err := evolutionEvidenceDigestInput(pack)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(encoded)
	return pack.SHA256 == hex.EncodeToString(digest[:])
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sanitizeEvolutionJSON(value json.RawMessage, max int) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return boundedEvolutionJSON(value, max, "invalid evidence JSON omitted")
	}
	encoded, err := json.Marshal(sanitizeEvolutionValue("", decoded))
	if err != nil {
		return json.RawMessage(`{"omitted":true,"reason":"evidence sanitization failed"}`)
	}
	return boundedEvolutionJSON(encoded, max, "evidence exceeded prompt limit")
}

func sanitizeEvolutionText(value string, max int) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) == nil {
		if encoded, err := json.Marshal(sanitizeEvolutionValue("", decoded)); err == nil {
			return bounded(string(encoded), max)
		}
	}
	return bounded(redactMemoryText(value), max)
}

func sanitizeEvolutionValue(key string, value any) any {
	normalized := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(key))
	for _, marker := range []string{
		"authorization", "api_key", "access_token", "password", "secret",
		"chain_of_thought", "hidden_reasoning", "reasoning_content", "reasoning",
	} {
		if strings.Contains(normalized, marker) {
			return "[REDACTED]"
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			result[childKey] = sanitizeEvolutionValue(childKey, childValue)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, sanitizeEvolutionValue(key, child))
		}
		return result
	case string:
		return redactMemoryText(typed)
	default:
		return value
	}
}

func evolutionJobTraceIDs(job EvolutionJob) []string {
	if job.SourceKind == EvolutionSourceConversation {
		return []string{"conversation:" + job.SourceAgentID + ":" + job.SourceConversationID}
	}
	if len(job.SourceTraceIDs) > 0 {
		return append([]string(nil), job.SourceTraceIDs...)
	}
	if job.SourceTraceID != "" {
		return []string{job.SourceTraceID}
	}
	return []string{}
}

func validEvolutionJobSource(job EvolutionJob) bool {
	switch job.SourceKind {
	case EvolutionSourceConversation:
		return job.SourceConversationID != "" && job.SourceAgentID != "" && job.SourceTraceID == "" && job.Pipeline == "memory"
	case EvolutionSourceTrace:
		return job.SourceTraceID != "" &&
			job.SourceAgentID == "" && len(job.SourceTraceIDs) == 0
	case EvolutionSourceAgentTraceSet:
		return job.SourceTraceID == "" &&
			job.SourceAgentID != "" && len(job.SourceTraceIDs) >= 2 &&
			job.WindowStart != nil && job.WindowEnd != nil && job.WindowEnd.After(*job.WindowStart)
	default:
		return false
	}
}

func evolutionJobSourceKey(job EvolutionJob) string {
	if job.SourceKind == EvolutionSourceConversation {
		return "conversation:" + job.SourceAgentID + ":" + job.SourceConversationID
	}
	if job.SourceKind == EvolutionSourceAgentTraceSet {
		return "agent:" + job.SourceAgentID
	}
	return "trace:" + job.SourceTraceID
}

func validRelativeAssetPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validAssetName(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func frontmatterValue(markdown string, key string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			return strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		}
	}
	return ""
}

func cloneEvolutionStages(input []EvolutionStage) []EvolutionStage {
	return append([]EvolutionStage(nil), input...)
}

func boundedEvolutionJSON(value json.RawMessage, max int, reason string) json.RawMessage {
	if len(value) <= max {
		return cloneJSON(value)
	}
	digest := sha256.Sum256(value)
	encoded, _ := json.Marshal(map[string]any{
		"omitted": true,
		"reason":  reason,
		"sha256":  fmt.Sprintf("%x", digest[:]),
	})
	return encoded
}

func conversationEvidencePack(agent, id string, messages []ConversationMessage, now time.Time) (EvolutionEvidencePack, error) {
	if len(messages) == 0 {
		return EvolutionEvidencePack{}, ErrNotFound
	}
	doc := ConversationDocument{Summary: summarizeConversation(messages), Messages: messages}
	pack := EvolutionEvidencePack{Schema: "catena.conversation_evidence.v2", SourceKind: EvolutionSourceConversation, SourceConversationID: id, SourceAgentID: agent, Document: conversationMemoryDocument(doc), Redacted: true, CreatedAt: now, Boundary: EvolutionEvidenceBoundary{ReleaseAuthority: evolutionReleaseAuthority, CandidateStatus: evolutionCandidateStatus, ReviewScope: evolutionReviewScope}}
	encoded, err := evolutionEvidenceDigestInput(pack)
	if err != nil {
		return pack, err
	}
	hash := sha256.Sum256(encoded)
	pack.SHA256 = hex.EncodeToString(hash[:])
	return pack, nil
}
