package control

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var memorySecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:sk|ghp|github_pat|barena_pat)[_-][A-Za-z0-9_-]{8,}`),
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
