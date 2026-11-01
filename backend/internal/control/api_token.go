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
	for index := range tokens {
		tokens[index] = s.presentAPIToken(tokens[index])
	}
	w.Header().Set("Cache-Control", "no-store")
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
	tokenID := newID("pat")
	encrypted, err := encryptAPIToken(plaintext, tokenID, s.auth.APITokenEncryptionKey)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "API token recovery is not configured")
		return
	}
	token := APIToken{
		ID:             tokenID,
		TokenHash:      sessionTokenHash(plaintext),
		EncryptedToken: encrypted,
		UserID:         user.ID,
		Name:           request.Name,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.CreateAPIToken(r.Context(), token); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_token": s.presentAPIToken(token),
		"token":     plaintext,
	})
}

func (s *HTTPServer) revealAPIToken(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	token, err := s.store.GetAPITokenByUser(
		r.Context(),
		user.ID,
		r.PathValue("token_id"),
	)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if token.EncryptedToken == "" {
		writeProblem(w, http.StatusConflict, "this legacy API token cannot be recovered; create a replacement")
		return
	}
	plaintext, err := decryptAPIToken(
		token.EncryptedToken,
		token.ID,
		s.auth.APITokenEncryptionKey,
	)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "API token recovery failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"token": plaintext})
}

func (s *HTTPServer) presentAPIToken(token APIToken) APIToken {
	token.MaskedToken = maskAPIToken("")
	token.Recoverable = false
	if token.EncryptedToken == "" {
		return token
	}
	plaintext, err := decryptAPIToken(
		token.EncryptedToken,
		token.ID,
		s.auth.APITokenEncryptionKey,
	)
	if err != nil {
		return token
	}
	token.MaskedToken = maskAPIToken(plaintext)
	token.Recoverable = true
	return token
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
