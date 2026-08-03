package control

import (
	"net/http"
	"strings"
	"time"
)

type createAPITokenRequest struct {
	Name string `json:"name"`
}

func (s *HTTPServer) listAPITokens(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	tokens, err := s.store.ListAPITokensByUser(r.Context(), user.ID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_tokens": tokens})
}

func (s *HTTPServer) createAPIToken(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	var request createAPITokenRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 80 {
		writeProblem(w, http.StatusBadRequest, "name must contain from 1 to 80 characters")
		return
	}
	secret, err := randomURLToken(32)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "API token generation failed")
		return
	}
	plaintext := "barena_pat_" + secret
	token := APIToken{
		ID:        newID("pat"),
		TokenHash: sessionTokenHash(plaintext),
		UserID:    user.ID,
		Name:      request.Name,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateAPIToken(r.Context(), token); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_token": token,
		"token":     plaintext,
	})
}

func (s *HTTPServer) deleteAPIToken(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteAPIToken(
		r.Context(),
		user.ID,
		r.PathValue("token_id"),
	); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
