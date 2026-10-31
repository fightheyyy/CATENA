package control

import (
	"net/http"
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
		writeProblem(w, statusFor(err), "Agent profile lookup failed")
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

	existing, err := s.store.GetAgentProfileByOwner(r.Context(), user.ID)
	if err != nil {
		writeProblem(w, statusFor(err), "Agent profile lookup failed")
		return
	}
	existing.DisplayName = request.DisplayName
	existing.Bio = request.Bio
	existing.IsPublic = request.IsPublic
	existing.UpdatedAt = time.Now().UTC()
	updated, err := s.store.UpdateAgentProfile(r.Context(), existing)
	if err != nil {
		writeProblem(w, statusFor(err), "Agent profile update failed")
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

func (s *HTTPServer) capabilitiesForOwner(r *http.Request, owner string) ([]CapabilitySummary, error) {
	return []CapabilitySummary{}, nil
}
