package control

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestClickHouseTraceRoundTrip(t *testing.T) {
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
	ownerID := fmt.Sprintf("trace-integration-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = store.conn.Exec(cleanupCtx,
			"ALTER TABLE catena_spans DELETE WHERE owner_id = ? SETTINGS mutations_sync = 1",
			ownerID,
		)
	})
	start := time.Now().UTC().Truncate(time.Microsecond)
	traceID := "00112233445566778899aabbccddeeff"
	spans := []TraceSpan{
		{
			TraceID:            traceID,
			SpanID:             "0011223344556677",
			Name:               "agent.turn",
			ServiceName:        "xiaoba-test",
			StartTime:          start,
			EndTime:            start.Add(2 * time.Second),
			Attributes:         map[string]any{"gen_ai.operation.name": "agent"},
			ResourceAttributes: map[string]any{"service.name": "xiaoba-test"},
			Events:             []TraceEvent{},
			Links:              []TraceLink{},
		},
		{
			TraceID:            traceID,
			SpanID:             "8899aabbccddeeff",
			ParentSpanID:       "0011223344556677",
			Name:               "tool.read_file",
			ServiceName:        "xiaoba-test",
			StartTime:          start.Add(200 * time.Millisecond),
			EndTime:            start.Add(600 * time.Millisecond),
			StatusCode:         2,
			Model:              "gpt-5.6-sol",
			Input:              `{"path":"README.md"}`,
			Output:             "contents",
			Attributes:         map[string]any{"tool.name": "read_file"},
			ResourceAttributes: map[string]any{"service.name": "xiaoba-test"},
			Events:             []TraceEvent{},
			Links:              []TraceLink{},
		},
	}
	if err := store.InsertSpans(ctx, ownerID, spans); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListTraces(ctx, ownerID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].RootName != "agent.turn" ||
		summaries[0].SpanCount != 2 || summaries[0].ErrorCount != 1 ||
		summaries[0].Model != "gpt-5.6-sol" {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}

	detail, err := store.GetTrace(ctx, ownerID, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Spans) != 2 || detail.Summary.DurationMS != 2000 ||
		detail.Spans[1].Input != `{"path":"README.md"}` {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	codexServices := []string{"codex", "codex-app-server", "Codex Desktop"}
	codexTraceIDs := []string{
		"11112233445566778899aabbccddeeff",
		"22222233445566778899aabbccddeeff",
		"33332233445566778899aabbccddeeff",
	}
	for index, serviceName := range codexServices {
		span := TraceSpan{
			TraceID:            codexTraceIDs[index],
			SpanID:             fmt.Sprintf("%016d", index+1),
			Name:               "codex.turn",
			ServiceName:        serviceName,
			StartTime:          start.Add(time.Duration(index+1) * time.Minute),
			EndTime:            start.Add(time.Duration(index+1)*time.Minute + time.Second),
			Attributes:         map[string]any{},
			ResourceAttributes: map[string]any{"service.name": serviceName},
			Events:             []TraceEvent{},
			Links:              []TraceLink{},
		}
		if err := store.InsertSpans(ctx, ownerID, []TraceSpan{span}); err != nil {
			t.Fatal(err)
		}
	}

	agents, err := store.ListAgents(ctx, ownerID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("unexpected agents: %+v", agents)
	}
	var codex AgentSummary
	for _, agent := range agents {
		if agent.AgentID == "codex" {
			codex = agent
		}
	}
	if codex.AgentID != "codex" || codex.DisplayName != "Codex" ||
		codex.IdentitySource != agentIdentitySourceAlias || codex.TraceCount != 3 ||
		codex.SpanCount != 3 || len(codex.Sources) != 3 {
		t.Fatalf("unexpected canonical Codex Agent: %+v", codex)
	}
	codexTraces, err := store.ListAgentTraces(
		ctx,
		ownerID,
		"  CoDeX DeSkToP  ",
		start,
		start.Add(10*time.Minute),
		10,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(codexTraces) != 3 {
		t.Fatalf("canonical Codex filter returned %+v", codexTraces)
	}
	seenSources := make(map[string]bool)
	for _, summary := range codexTraces {
		seenSources[summary.ServiceName] = true
	}
	for _, serviceName := range codexServices {
		if !seenSources[serviceName] {
			t.Fatalf("canonical Codex filter lost source %q: %+v", serviceName, codexTraces)
		}
	}

	barenaServices := []string{
		"barena-explore-engine",
		"barena-xiaoba-target",
		"barena-xiaoba-target",
		"barena-xiaoba-user_simulator",
		"barena-xiaoba-inspector",
		"barena-xiaoba-reviewer",
	}
	barenaTraceIDs := []string{
		"44442233445566778899aabbccddeeff",
		"55552233445566778899aabbccddeeff",
		"66662233445566778899aabbccddeeff",
		"77772233445566778899aabbccddeeff",
		"88882233445566778899aabbccddeeff",
		"99992233445566778899aabbccddeeff",
	}
	for index, serviceName := range barenaServices {
		span := TraceSpan{
			TraceID:            barenaTraceIDs[index],
			SpanID:             fmt.Sprintf("%016x", index+100),
			Name:               "xiaoba.session",
			ServiceName:        serviceName,
			StartTime:          start.Add(time.Duration(index+11) * time.Minute),
			EndTime:            start.Add(time.Duration(index+11)*time.Minute + time.Second),
			Attributes:         map[string]any{},
			ResourceAttributes: map[string]any{"service.name": serviceName},
			Events:             []TraceEvent{},
			Links:              []TraceLink{},
		}
		if err := store.InsertSpans(ctx, ownerID, []TraceSpan{span}); err != nil {
			t.Fatal(err)
		}
	}

	agents, err = store.ListAgents(ctx, ownerID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 3 {
		t.Fatalf("Barena workflow sources leaked into Agent Registry: %+v", agents)
	}
	var xiaoBaOS AgentSummary
	for _, agent := range agents {
		if agent.AgentID == "xiaobaos" {
			xiaoBaOS = agent
		}
	}
	if xiaoBaOS.AgentID != "xiaobaos" || xiaoBaOS.DisplayName != "XiaoBaOS" ||
		xiaoBaOS.IdentitySource != agentIdentitySourceAlias || xiaoBaOS.TraceCount != 2 ||
		xiaoBaOS.SpanCount != 2 || len(xiaoBaOS.Sources) != 1 ||
		xiaoBaOS.Sources[0].ServiceName != "barena-xiaoba-target" ||
		xiaoBaOS.Sources[0].Kind != agentSourceKindNativeLive {
		t.Fatalf("unexpected XiaoBaOS Agent: %+v", xiaoBaOS)
	}

	xiaoBaTraces, err := store.ListAgentTraces(
		ctx,
		ownerID,
		"xiaobaos",
		start,
		start.Add(30*time.Minute),
		10,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(xiaoBaTraces) != 2 {
		t.Fatalf("XiaoBaOS filter returned %+v", xiaoBaTraces)
	}
	for _, summary := range xiaoBaTraces {
		if summary.ServiceName != "barena-xiaoba-target" {
			t.Fatalf("XiaoBaOS filter included internal source: %+v", xiaoBaTraces)
		}
	}

	allTraces, err := store.ListTraces(ctx, ownerID, 20)
	if err != nil {
		t.Fatal(err)
	}
	seenBarenaSources := make(map[string]int)
	for _, summary := range allTraces {
		if _, ok := internalAgentSources[normalizedAgentSource(summary.ServiceName)]; ok ||
			summary.ServiceName == "barena-xiaoba-target" {
			seenBarenaSources[summary.ServiceName]++
		}
	}
	for _, serviceName := range barenaServices {
		if seenBarenaSources[serviceName] == 0 {
			t.Fatalf("global Trace read lost Barena source %q: %+v", serviceName, allTraces)
		}
	}

	boundAgentID := "agent_integration_stable"
	boundTraceID := "aaaa2233445566778899aabbccddeeff"
	if err := store.InsertSpans(ctx, ownerID, []TraceSpan{{
		AgentID: boundAgentID, TraceID: boundTraceID, SpanID: "aaaaaaaaaaaaaaaa",
		Name: "claude.turn", ServiceName: "claude-code",
		StartTime: start.Add(40 * time.Minute), EndTime: start.Add(40*time.Minute + time.Second),
		Attributes: map[string]any{}, ResourceAttributes: map[string]any{"service.name": "claude-code"},
		Events: []TraceEvent{}, Links: []TraceLink{},
	}}); err != nil {
		t.Fatal(err)
	}
	agents, err = store.ListAgents(ctx, ownerID, 20)
	if err != nil {
		t.Fatal(err)
	}
	foundBoundAgent := false
	for _, agent := range agents {
		if agent.AgentID == boundAgentID && agent.IdentitySource == agentIdentitySourceCredential &&
			agent.TraceCount == 1 {
			foundBoundAgent = true
		}
	}
	if !foundBoundAgent {
		t.Fatalf("bound Agent identity was not retained: %+v", agents)
	}
	boundTraces, err := store.ListAgentTraces(
		ctx, ownerID, boundAgentID, start, start.Add(time.Hour), 10,
	)
	if err != nil || len(boundTraces) != 1 || boundTraces[0].TraceID != boundTraceID {
		t.Fatalf("bound Agent Trace filter = %+v, %v", boundTraces, err)
	}
}
