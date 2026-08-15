package control

import (
	"sort"
	"strings"
	"time"
)

const (
	agentIdentitySourceServiceName = "service.name"
	agentIdentitySourceAlias       = "catena.alias"
	agentIdentitySourceCredential  = "api_key"

	agentSourceKindNativeLive = "native_live"
	agentSourceKindOTel       = "otel"
)

type agentSourceClassification string

const (
	agentSourceClassificationTarget   agentSourceClassification = "target"
	agentSourceClassificationInternal agentSourceClassification = "internal"
)

type AgentSource struct {
	ServiceName string `json:"service_name"`
	Kind        string `json:"kind"`
}

type canonicalAgentIdentity struct {
	AgentID        string
	DisplayName    string
	IdentitySource string
	Aliases        []string
}

type classifiedAgentSource struct {
	Classification agentSourceClassification
	Identity       canonicalAgentIdentity
	Kind           string
}

type agentSourceAggregate struct {
	AgentID     string
	ServiceName string
	TraceCount  uint64
	SpanCount   uint64
	ErrorCount  uint64
	LastSeenAt  time.Time
}

var acceptedRuntimeSourceKinds = map[string]string{
	"catena-runtime-codex":       agentSourceKindNativeLive,
	"catena-runtime-claude-code": agentSourceKindNativeLive,
}

var xiaoBaOSSourceAliases = []string{"xiaobaos", "barena-xiaoba-target"}

var internalAgentSources = map[string]struct{}{
	"barena-explore-engine":        {},
	"barena-xiaoba-user_simulator": {},
	"barena-xiaoba-inspector":      {},
	"barena-xiaoba-reviewer":       {},
}

func classifyAgentSource(serviceName string) classifiedAgentSource {
	trimmed := strings.TrimSpace(serviceName)
	normalized := normalizedAgentSource(trimmed)
	if _, internal := internalAgentSources[normalized]; internal {
		return classifiedAgentSource{
			Classification: agentSourceClassificationInternal,
			Kind:           agentSourceKindOTel,
		}
	}
	if normalized == "xiaobaos" || normalized == "barena-xiaoba-target" {
		return classifiedAgentSource{
			Classification: agentSourceClassificationTarget,
			Identity: canonicalAgentIdentity{
				AgentID:        "xiaobaos",
				DisplayName:    "XiaoBaOS",
				IdentitySource: agentIdentitySourceAlias,
				Aliases:        append([]string(nil), xiaoBaOSSourceAliases...),
			},
			Kind: agentSourceKindNativeLive,
		}
	}
	kind := agentSourceKindOTel
	if acceptedKind, ok := acceptedRuntimeSourceKinds[normalized]; ok {
		kind = acceptedKind
	}
	return classifiedAgentSource{
		Classification: agentSourceClassificationTarget,
		Identity: canonicalAgentIdentity{
			AgentID:        trimmed,
			DisplayName:    trimmed,
			IdentitySource: agentIdentitySourceServiceName,
			Aliases:        []string{trimmed},
		},
		Kind: kind,
	}
}

func canonicalAgentForSource(serviceName string) canonicalAgentIdentity {
	return classifyAgentSource(serviceName).Identity
}

func canonicalAgentForID(agentID string) canonicalAgentIdentity {
	classified := classifyAgentSource(agentID)
	if classified.Classification != agentSourceClassificationTarget {
		return canonicalAgentIdentity{}
	}
	return classified.Identity
}

func normalizedAgentSource(serviceName string) string {
	return strings.ToLower(strings.TrimSpace(serviceName))
}

func agentSourceKind(serviceName string) string {
	return classifyAgentSource(serviceName).Kind
}

func serviceBelongsToAgent(serviceName string, agentID string) bool {
	classified := classifyAgentSource(serviceName)
	if classified.Classification != agentSourceClassificationTarget {
		return false
	}
	serviceIdentity := classified.Identity
	agentIdentity := canonicalAgentForID(agentID)
	return serviceIdentity.AgentID != "" && serviceIdentity.AgentID == agentIdentity.AgentID
}

func traceSummaryBelongsToAgent(summary TraceSummary, agentID string) bool {
	boundAgentID := strings.TrimSpace(summary.AgentID)
	if boundAgentID != "" {
		return boundAgentID == strings.TrimSpace(agentID)
	}
	return serviceBelongsToAgent(summary.ServiceName, agentID)
}

func mergeAgentSourceAggregates(values []agentSourceAggregate, limit int) []AgentSummary {
	type mergedAgent struct {
		summary AgentSummary
		sources map[string]AgentSource
	}
	merged := make(map[string]*mergedAgent)
	for _, value := range values {
		if value.AgentID != "" {
			entry := merged[value.AgentID]
			if entry == nil {
				entry = &mergedAgent{
					summary: AgentSummary{AgentID: value.AgentID, IdentitySource: agentIdentitySourceCredential},
					sources: make(map[string]AgentSource),
				}
				merged[value.AgentID] = entry
			}
			entry.summary.TraceCount += value.TraceCount
			entry.summary.SpanCount += value.SpanCount
			entry.summary.ErrorCount += value.ErrorCount
			if value.LastSeenAt.After(entry.summary.LastSeenAt) {
				entry.summary.LastSeenAt = value.LastSeenAt
			}
			serviceName := strings.TrimSpace(value.ServiceName)
			if serviceName != "" {
				entry.sources[normalizedAgentSource(serviceName)] = AgentSource{
					ServiceName: agentSourceDisplayName(serviceName), Kind: agentSourceKind(serviceName),
				}
			}
			continue
		}
		classified := classifyAgentSource(value.ServiceName)
		if classified.Classification != agentSourceClassificationTarget || classified.Identity.AgentID == "" {
			continue
		}
		identity := classified.Identity
		entry := merged[identity.AgentID]
		if entry == nil {
			entry = &mergedAgent{
				summary: AgentSummary{
					AgentID:        identity.AgentID,
					DisplayName:    identity.DisplayName,
					IdentitySource: identity.IdentitySource,
				},
				sources: make(map[string]AgentSource),
			}
			merged[identity.AgentID] = entry
		}
		entry.summary.TraceCount += value.TraceCount
		entry.summary.SpanCount += value.SpanCount
		entry.summary.ErrorCount += value.ErrorCount
		if value.LastSeenAt.After(entry.summary.LastSeenAt) {
			entry.summary.LastSeenAt = value.LastSeenAt
		}
		serviceName := strings.TrimSpace(value.ServiceName)
		if serviceName != "" {
			displayName := agentSourceDisplayName(serviceName)
			entry.sources[normalizedAgentSource(displayName)] = AgentSource{
				ServiceName: displayName,
				Kind:        classified.Kind,
			}
		}
	}

	result := make([]AgentSummary, 0, len(merged))
	for _, entry := range merged {
		entry.summary.Sources = make([]AgentSource, 0, len(entry.sources))
		for _, source := range entry.sources {
			entry.summary.Sources = append(entry.summary.Sources, source)
		}
		sort.Slice(entry.summary.Sources, func(i, j int) bool {
			leftRank := agentSourceKindRank(entry.summary.Sources[i].Kind)
			rightRank := agentSourceKindRank(entry.summary.Sources[j].Kind)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			left := strings.ToLower(entry.summary.Sources[i].ServiceName)
			right := strings.ToLower(entry.summary.Sources[j].ServiceName)
			if left != right {
				return left < right
			}
			return entry.summary.Sources[i].ServiceName < entry.summary.Sources[j].ServiceName
		})
		result = append(result, entry.summary)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].LastSeenAt.Equal(result[j].LastSeenAt) {
			return result[i].LastSeenAt.After(result[j].LastSeenAt)
		}
		return result[i].AgentID < result[j].AgentID
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func agentSourceDisplayName(serviceName string) string {
	return strings.TrimSpace(serviceName)
}

func agentSourceKindRank(kind string) int {
	switch kind {
	case agentSourceKindNativeLive:
		return 0
	default:
		return 1
	}
}
