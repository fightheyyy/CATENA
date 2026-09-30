package control

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClickHouseTraceListPaging(t *testing.T) {
	dsn := os.Getenv("CATENA_CLICKHOUSE_TEST_DSN")
	if dsn == "" {
		t.Skip("CATENA_CLICKHOUSE_TEST_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := OpenClickHouseTraceStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	owner := fmt.Sprintf("trace-paging-%d", time.Now().UnixNano())
	otherOwner := owner + "-other"
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = store.conn.Exec(cleanupCtx,
			"ALTER TABLE catena_spans DELETE WHERE owner_id IN (?, ?) SETTINGS mutations_sync = 1",
			owner, otherOwner)
	})
	start := time.Now().UTC().Truncate(time.Microsecond)
	makeSpan := func(traceID, agentID string, offset time.Duration) TraceSpan {
		return TraceSpan{
			AgentID: agentID, TraceID: traceID, SpanID: "0000000000000001",
			Name: "agent.turn", ServiceName: "catena-runtime-codex",
			StartTime: start.Add(offset), EndTime: start.Add(offset + 2*time.Second),
			Attributes: map[string]any{}, ResourceAttributes: map[string]any{},
			Events: []TraceEvent{}, Links: []TraceLink{},
		}
	}
	root := makeSpan("11111111111111111111111111111111", "agent-page", 10*time.Minute)
	root.Input = strings.Repeat("经历", 400)
	root.Attributes["gen_ai.conversation.id"] = "lower-priority-session"
	root.ResourceAttributes["agent.session.id"] = "resource-session"
	oldRoot := root
	oldRoot.EndTime = start.Add(2 * time.Hour)
	oldRoot.StatusCode = 2
	if err := store.InsertSpans(ctx, owner, []TraceSpan{oldRoot}); err != nil {
		t.Fatal(err)
	}
	child := makeSpan(root.TraceID, "agent-page", 10*time.Minute+time.Second)
	child.SpanID = "0000000000000002"
	child.ParentSpanID = root.SpanID
	child.EndTime = root.EndTime
	child.Name = "tool.read"
	child.Input = "tool input"
	child.StatusCode = 2
	child.Attributes["agent.session.id"] = "preferred-session"
	tied := makeSpan("22222222222222222222222222222222", "agent-page", 10*time.Minute)
	older := makeSpan("33333333333333333333333333333333", "agent-page", time.Minute)
	unrelated := makeSpan("44444444444444444444444444444444", "agent-other", 15*time.Minute)
	if err := store.InsertSpans(ctx, owner, []TraceSpan{root, child, tied, older, unrelated}); err != nil {
		t.Fatal(err)
	}
	foreign := root
	foreign.Name = "foreign owner"
	foreign.Input = "private to other owner"
	foreign.EndTime = start.Add(3 * time.Hour)
	if err := store.InsertSpans(ctx, otherOwner, []TraceSpan{foreign}); err != nil {
		t.Fatal(err)
	}

	global, err := store.ListTraces(ctx, owner, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(global) != 2 || global[0].TraceID != unrelated.TraceID || global[1].TraceID != root.TraceID {
		t.Fatalf("global page ordering/limit: %+v", global)
	}
	assertRoot := func(summary TraceSummary) {
		t.Helper()
		if summary.TraceID != root.TraceID || summary.AgentID != root.AgentID ||
			summary.SessionID != "preferred-session" || summary.RootName != root.Name ||
			summary.InputPreview != string([]rune(root.Input)[:512]) ||
			summary.SpanCount != 2 || summary.ErrorCount != 1 ||
			!summary.EndTime.Equal(root.EndTime) || summary.DurationMS != 2000 {
			t.Fatalf("selected summary lost identity, evidence or replacement semantics: %+v", summary)
		}
	}
	assertRoot(global[1])
	agent, err := store.ListAgentTraces(ctx, owner, root.AgentID, start, start.Add(time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(agent) != 1 {
		t.Fatalf("Agent filtering must precede LIMIT: %+v", agent)
	}
	assertRoot(agent[0])

	empty, err := store.ListAgentTraces(ctx, owner, root.AgentID, start.Add(25*time.Minute), start.Add(26*time.Minute), 2)
	if err != nil || len(empty) != 0 {
		t.Fatalf("superseded span version leaked into time window: %+v, %v", empty, err)
	}
	olderWindow, err := store.ListAgentTraces(ctx, owner, root.AgentID, start, start.Add(5*time.Minute), 1)
	if err != nil || len(olderWindow) != 1 || olderWindow[0].TraceID != older.TraceID {
		t.Fatalf("window filtering must precede LIMIT: %+v, %v", olderWindow, err)
	}
	detail, err := store.GetTrace(ctx, owner, root.TraceID)
	if err != nil || len(detail.Spans) != 2 || detail.Spans[0].Input != root.Input {
		t.Fatalf("full detail evidence was not retained: span count %d, %v", len(detail.Spans), err)
	}
}
