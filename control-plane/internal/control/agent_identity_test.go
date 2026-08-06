package control

import (
	"reflect"
	"testing"
	"time"
)

func TestCanonicalAgentForSourceMergesCodexAliases(t *testing.T) {
	for _, serviceName := range []string{
		"codex",
		"CODEX",
		" codex-app-server ",
		"Codex Desktop",
		"  cOdEx DeSkToP  ",
	} {
		identity := canonicalAgentForSource(serviceName)
		if identity.AgentID != "codex" || identity.DisplayName != "Codex" ||
			identity.IdentitySource != agentIdentitySourceAlias {
			t.Fatalf("source %q resolved to %#v", serviceName, identity)
		}
		if !serviceBelongsToAgent(serviceName, "codex") {
			t.Fatalf("source %q does not belong to canonical Codex", serviceName)
		}
	}
}

func TestCanonicalAgentForSourceKeepsUnknownServiceIdentity(t *testing.T) {
	identity := canonicalAgentForSource("  xiaoba-role  ")
	if identity.AgentID != "xiaoba-role" || identity.DisplayName != "xiaoba-role" ||
		identity.IdentitySource != agentIdentitySourceServiceName ||
		!reflect.DeepEqual(identity.Aliases, []string{"xiaoba-role"}) {
		t.Fatalf("unknown source identity = %#v", identity)
	}
	if serviceBelongsToAgent("xiaoba-role", "another-role") {
		t.Fatal("unregistered service names were merged")
	}
}

func TestClassifyKnownBarenaSources(t *testing.T) {
	for _, serviceName := range []string{
		"barena-explore-engine",
		"barena-xiaoba-user_simulator",
		"barena-xiaoba-inspector",
		"barena-xiaoba-reviewer",
	} {
		classified := classifyAgentSource("  " + serviceName + "  ")
		if classified.Classification != agentSourceClassificationInternal {
			t.Fatalf("source %q classification = %q, want internal", serviceName, classified.Classification)
		}
		if classified.Identity.AgentID != "" {
			t.Fatalf("internal source %q resolved to selectable identity %#v", serviceName, classified.Identity)
		}
	}

	target := classifyAgentSource(" BARENA-XIAOBA-TARGET ")
	if target.Classification != agentSourceClassificationTarget ||
		target.Identity.AgentID != "xiaobaos" || target.Identity.DisplayName != "XiaoBaOS" ||
		target.Identity.IdentitySource != agentIdentitySourceAlias {
		t.Fatalf("XiaoBaOS target classification = %#v", target)
	}
	wantAliases := []string{"xiaobaos", "barena-xiaoba-target"}
	if !reflect.DeepEqual(target.Identity.Aliases, wantAliases) {
		t.Fatalf("XiaoBaOS aliases = %#v, want %#v", target.Identity.Aliases, wantAliases)
	}
}

func TestInternalSourcesAreNotSelectableAgents(t *testing.T) {
	for _, serviceName := range []string{
		"barena-explore-engine",
		"barena-xiaoba-user_simulator",
		"barena-xiaoba-inspector",
		"barena-xiaoba-reviewer",
	} {
		if serviceBelongsToAgent(serviceName, serviceName) || serviceBelongsToAgent(serviceName, "xiaobaos") {
			t.Fatalf("internal source %q belongs to a selectable Agent", serviceName)
		}
		if _, err := normalizedAgentID(serviceName); err == nil {
			t.Fatalf("normalizedAgentID(%q) accepted an internal source", serviceName)
		}
	}
}

func TestXiaoBaOSTargetMembershipUsesCanonicalID(t *testing.T) {
	for _, agentID := range []string{"xiaobaos", "barena-xiaoba-target", " XiaoBaOS "} {
		if !serviceBelongsToAgent("barena-xiaoba-target", agentID) {
			t.Fatalf("barena-xiaoba-target does not belong to %q", agentID)
		}
		normalized, err := normalizedAgentID(agentID)
		if err != nil {
			t.Fatal(err)
		}
		if normalized != "xiaobaos" {
			t.Fatalf("normalizedAgentID(%q) = %q, want xiaobaos", agentID, normalized)
		}
	}
	if serviceBelongsToAgent("barena-xiaoba-target", "codex") {
		t.Fatal("XiaoBaOS target belongs to Codex")
	}
}

func TestMergeAgentSourceAggregatesReturnsOneAuditableCodexAgent(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	values := []agentSourceAggregate{
		{ServiceName: "Codex Desktop", TraceCount: 29, SpanCount: 50, LastSeenAt: now.Add(-time.Hour)},
		{ServiceName: "codex-app-server", TraceCount: 3636, SpanCount: 356642, ErrorCount: 12, LastSeenAt: now},
		{ServiceName: " CODEX ", TraceCount: 3, SpanCount: 9, LastSeenAt: now.Add(-time.Minute)},
		{ServiceName: "xiaoba-role", TraceCount: 7, SpanCount: 21, LastSeenAt: now.Add(-2 * time.Hour)},
	}
	agents := mergeAgentSourceAggregates(values, 10)
	if len(agents) != 2 {
		t.Fatalf("agents = %#v, want one Codex and one unknown Agent", agents)
	}
	codex := agents[0]
	if codex.AgentID != "codex" || codex.DisplayName != "Codex" ||
		codex.IdentitySource != agentIdentitySourceAlias || codex.TraceCount != 3668 ||
		codex.SpanCount != 356701 || codex.ErrorCount != 12 {
		t.Fatalf("canonical Codex summary = %#v", codex)
	}
	wantSources := []AgentSource{
		{ServiceName: "codex", Kind: agentSourceKindNativeLive},
		{ServiceName: "codex-app-server", Kind: agentSourceKindNativeLive},
		{ServiceName: "Codex Desktop", Kind: agentSourceKindHistoryBackfill},
	}
	if !reflect.DeepEqual(codex.Sources, wantSources) {
		t.Fatalf("Codex sources = %#v, want %#v", codex.Sources, wantSources)
	}
	if agents[1].AgentID != "xiaoba-role" || len(agents[1].Sources) != 1 ||
		agents[1].Sources[0].Kind != agentSourceKindOTel {
		t.Fatalf("unknown Agent summary = %#v", agents[1])
	}
}

func TestMergeAgentSourceAggregatesExcludesInternalSources(t *testing.T) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	values := []agentSourceAggregate{
		{ServiceName: "barena-explore-engine", TraceCount: 1, SpanCount: 13, LastSeenAt: now},
		{ServiceName: "barena-xiaoba-user_simulator", TraceCount: 1, SpanCount: 2, LastSeenAt: now},
		{ServiceName: "barena-xiaoba-inspector", TraceCount: 1, SpanCount: 2, LastSeenAt: now},
		{ServiceName: "barena-xiaoba-reviewer", TraceCount: 1, SpanCount: 2, LastSeenAt: now},
		{ServiceName: "barena-xiaoba-target", TraceCount: 2, SpanCount: 12, LastSeenAt: now.Add(-time.Minute)},
		{ServiceName: "future-agent", TraceCount: 3, SpanCount: 10, LastSeenAt: now.Add(-2 * time.Minute)},
	}
	agents := mergeAgentSourceAggregates(values, 10)
	if len(agents) != 2 {
		t.Fatalf("agents = %#v, want XiaoBaOS target and unknown fallback", agents)
	}
	if agents[0].AgentID != "xiaobaos" || agents[0].DisplayName != "XiaoBaOS" ||
		agents[0].TraceCount != 2 || agents[0].SpanCount != 12 || agents[0].ErrorCount != 0 ||
		len(agents[0].Sources) != 1 || agents[0].Sources[0].Kind != agentSourceKindNativeLive {
		t.Fatalf("XiaoBaOS Agent summary = %#v", agents[0])
	}
	if agents[1].AgentID != "future-agent" || agents[1].IdentitySource != agentIdentitySourceServiceName {
		t.Fatalf("unknown fallback summary = %#v", agents[1])
	}
}

func TestNormalizedAgentIDAcceptsCodexAliasesButReturnsCanonicalID(t *testing.T) {
	for _, value := range []string{"codex", "Codex Desktop", " CODEX-APP-SERVER "} {
		agentID, err := normalizedAgentID(value)
		if err != nil {
			t.Fatal(err)
		}
		if agentID != "codex" {
			t.Fatalf("normalizedAgentID(%q) = %q", value, agentID)
		}
	}
}
