package control

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *HTTPServer) myAgentProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if user == nil {
		writeProblem(w, http.StatusConflict, "GitHub authentication is required to create a public profile")
		return
	}
	profile, err := s.store.GetAgentProfileByOwner(r.Context(), user.ID)
	if err != nil {
		writeProblem(w, statusFor(err), "XiaoBa profile lookup failed")
		return
	}
	capabilities, err := s.capabilitiesForOwner(r, user.ID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "capability aggregation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":      profile,
		"capabilities": capabilities,
	})
}

func (s *HTTPServer) updateMyAgentProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if user == nil {
		writeProblem(w, http.StatusConflict, "GitHub authentication is required to publish a profile")
		return
	}
	var request struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
		IsPublic    bool   `json:"is_public"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Bio = strings.TrimSpace(request.Bio)
	if request.DisplayName == "" || len([]rune(request.DisplayName)) > 80 {
		writeProblem(w, http.StatusBadRequest, "display_name must be from 1 to 80 characters")
		return
	}
	if len([]rune(request.Bio)) > 280 {
		writeProblem(w, http.StatusBadRequest, "bio must be at most 280 characters")
		return
	}
	capabilities, err := s.capabilitiesForOwner(r, user.ID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "capability aggregation failed")
		return
	}
	if request.IsPublic && len(capabilities) == 0 {
		writeProblem(
			w,
			http.StatusConflict,
			"complete at least one OTLP-backed Barena Run before publishing",
		)
		return
	}
	existing, err := s.store.GetAgentProfileByOwner(r.Context(), user.ID)
	if err != nil {
		writeProblem(w, statusFor(err), "XiaoBa profile lookup failed")
		return
	}
	existing.DisplayName = request.DisplayName
	existing.Bio = request.Bio
	existing.IsPublic = request.IsPublic
	existing.UpdatedAt = time.Now().UTC()
	updated, err := s.store.UpdateAgentProfile(r.Context(), existing)
	if err != nil {
		writeProblem(w, statusFor(err), "XiaoBa profile update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":      updated,
		"capabilities": capabilities,
	})
}

func (s *HTTPServer) communityProfiles(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.ListPublicAgentProfiles(r.Context(), 50)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "community lookup failed")
		return
	}
	profiles := make([]CommunityProfile, 0, len(records))
	for _, record := range records {
		capabilities, capabilityErr := s.capabilitiesForOwner(r, record.User.ID)
		if capabilityErr != nil {
			writeProblem(w, http.StatusInternalServerError, "capability aggregation failed")
			return
		}
		profiles = append(profiles, publicCommunityProfile(record, capabilities))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

func (s *HTTPServer) communityProfile(w http.ResponseWriter, r *http.Request) {
	record, err := s.store.GetPublicAgentProfile(
		r.Context(),
		r.PathValue("slug"),
	)
	if err != nil {
		writeProblem(w, statusFor(err), "community profile not found")
		return
	}
	capabilities, err := s.capabilitiesForOwner(r, record.User.ID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "capability aggregation failed")
		return
	}
	writeJSON(w, http.StatusOK, publicCommunityProfile(record, capabilities))
}

func (s *HTTPServer) capabilitiesForOwner(
	r *http.Request,
	ownerUserID string,
) ([]CapabilitySummary, error) {
	runs, err := s.store.ListRunsByOwner(r.Context(), ownerUserID, 200)
	if err != nil {
		return nil, err
	}
	type aggregate struct {
		summary CapabilitySummary
	}
	aggregates := make(map[string]*aggregate)
	for _, run := range runs {
		events, eventErr := s.store.ListEventsAfter(r.Context(), run.ID, 0, 10_000)
		if eventErr != nil {
			return nil, eventErr
		}
		spans, successful := capabilityEvidence(run, events)
		if spans < 1 {
			continue
		}
		role, skill := capabilityTargets(run.Input)
		for _, candidate := range []struct {
			kind  string
			value string
		}{
			{kind: "role", value: role},
			{kind: "skill", value: skill},
		} {
			if candidate.value == "" {
				continue
			}
			key := candidate.kind + ":" + candidate.value
			current := aggregates[key]
			if current == nil {
				current = &aggregate{summary: CapabilitySummary{
					Key:   key,
					Kind:  candidate.kind,
					Label: candidate.value,
				}}
				aggregates[key] = current
			}
			current.summary.SampleCount++
			current.summary.OTLPSpans += spans
			if successful {
				current.summary.SuccessCount++
			}
		}
	}
	result := make([]CapabilitySummary, 0, len(aggregates))
	for _, item := range aggregates {
		summary := item.summary
		summary.SuccessRate = float64(summary.SuccessCount) / float64(summary.SampleCount)
		switch {
		case summary.SampleCount >= 3 &&
			summary.SuccessCount >= 3 &&
			summary.SuccessRate >= 0.8:
			summary.Level = "stable"
		case summary.SuccessCount >= 1:
			summary.Level = "verified"
		default:
			summary.Level = "observed"
		}
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool {
		leftRank := capabilityLevelRank(result[i].Level)
		rightRank := capabilityLevelRank(result[j].Level)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if result[i].SuccessCount != result[j].SuccessCount {
			return result[i].SuccessCount > result[j].SuccessCount
		}
		return result[i].Key < result[j].Key
	})
	return result, nil
}

func capabilityTargets(input json.RawMessage) (string, string) {
	var envelope struct {
		Scenario struct {
			Target struct {
				Role  string `json:"role"`
				Skill string `json:"skill"`
			} `json:"target"`
		} `json:"scenario"`
	}
	if json.Unmarshal(input, &envelope) != nil {
		return "", ""
	}
	return strings.TrimSpace(envelope.Scenario.Target.Role),
		strings.TrimSpace(envelope.Scenario.Target.Skill)
}

func capabilityEvidence(run Run, events []EngineEvent) (int, bool) {
	spans := 0
	successful := false
	for _, event := range events {
		var payload struct {
			Verdict      string `json:"verdict"`
			Decision     string `json:"decision"`
			ResultStatus string `json:"result_status"`
			Evidence     struct {
				OTLPSpans int `json:"otlp_spans"`
			} `json:"evidence"`
		}
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		if payload.Evidence.OTLPSpans > spans {
			spans = payload.Evidence.OTLPSpans
		}
		if run.State == StateCompleted &&
			(payload.Verdict == "pass" ||
				payload.Decision == "cleared" ||
				payload.ResultStatus == "pass") {
			successful = true
		}
	}
	return spans, successful
}

func publicCommunityProfile(
	record ProfileRecord,
	capabilities []CapabilitySummary,
) CommunityProfile {
	return CommunityProfile{
		Slug:         record.Profile.Slug,
		DisplayName:  record.Profile.DisplayName,
		Bio:          record.Profile.Bio,
		GitHubLogin:  record.User.Login,
		AvatarURL:    record.User.AvatarURL,
		Capabilities: capabilities,
		UpdatedAt:    record.Profile.UpdatedAt,
	}
}

func capabilityLevelRank(level string) int {
	switch level {
	case "stable":
		return 3
	case "verified":
		return 2
	default:
		return 1
	}
}
