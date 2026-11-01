package control

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const analysisModelEnvelopePrefix = "catena-evolution-model:"

type analysisModelConfigRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

type AnalysisModelSettings struct {
	Provider         string    `json:"provider"`
	BaseURL          string    `json:"base_url"`
	Model            string    `json:"model"`
	APIKeyConfigured bool      `json:"api_key_configured"`
	Configured       bool      `json:"configured"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}

type AnalysisModelCredentials struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
}

func (s *HTTPServer) getAnalysisModelConfig(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	config, err := s.store.GetAnalysisModelConfigByOwner(r.Context(), traceOwnerID(user))
	if errors.Is(err, ErrNotFound) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, AnalysisModelSettings{})
		return
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, presentAnalysisModelConfig(config))
}

func (s *HTTPServer) putAnalysisModelConfig(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if user == nil {
		now := time.Now().UTC()
		persisted, err := s.store.UpsertUser(r.Context(), User{
			ID:          "local",
			GitHubID:    (int64(1) << 62) - 1,
			Login:       "local",
			DisplayName: "Local workspace",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "local workspace persistence failed")
			return
		}
		user = &persisted
	}
	var request analysisModelConfigRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	request.Model = strings.TrimSpace(request.Model)
	if err := validateAnalysisModelConfigRequest(request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerUserID := traceOwnerID(user)
	existing, existingErr := s.store.GetAnalysisModelConfigByOwner(r.Context(), ownerUserID)
	if existingErr != nil && !errors.Is(existingErr, ErrNotFound) {
		writeProblem(w, statusFor(existingErr), existingErr.Error())
		return
	}
	encrypted := ""
	if request.APIKey == "" && existingErr == nil {
		encrypted = existing.EncryptedAPIKey
	} else if request.APIKey != "" {
		var err error
		encrypted, err = encryptAPIToken(
			request.APIKey,
			analysisModelEnvelopePrefix+ownerUserID,
			s.auth.APITokenEncryptionKey,
		)
		if err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "LLM credential encryption is not configured")
			return
		}
	}
	if encrypted == "" {
		writeProblem(w, http.StatusBadRequest, "api_key is required when creating an LLM configuration")
		return
	}
	stored, err := s.store.UpsertAnalysisModelConfig(r.Context(), AnalysisModelConfig{
		OwnerUserID:     ownerUserID,
		Provider:        request.Provider,
		BaseURL:         request.BaseURL,
		Model:           request.Model,
		EncryptedAPIKey: encrypted,
		UpdatedAt:       time.Now().UTC(),
	})
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, presentAnalysisModelConfig(stored))
}

func (s *HTTPServer) deleteAnalysisModelConfig(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteAnalysisModelConfigByOwner(r.Context(), traceOwnerID(user))
	if errors.Is(err, ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) analysisModelCredentials(
	ctx context.Context,
	ownerUserID string,
) (AnalysisModelCredentials, error) {
	if strings.TrimSpace(ownerUserID) == "" {
		ownerUserID = "local"
	}
	config, err := s.store.GetAnalysisModelConfigByOwner(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AnalysisModelCredentials{}, errors.New("请先在 API 管理中配置 LLM")
		}
		return AnalysisModelCredentials{}, err
	}
	apiKey, err := decryptAPIToken(
		config.EncryptedAPIKey,
		analysisModelEnvelopePrefix+ownerUserID,
		s.auth.APITokenEncryptionKey,
	)
	if err != nil {
		return AnalysisModelCredentials{}, errors.New("LLM credential recovery failed")
	}
	credentials := AnalysisModelCredentials{
		Provider: config.Provider,
		BaseURL:  config.BaseURL,
		Model:    config.Model,
		APIKey:   apiKey,
	}
	if !credentials.Valid() {
		return AnalysisModelCredentials{}, errors.New("LLM configuration is incomplete")
	}
	return credentials, nil
}

func (c AnalysisModelCredentials) Valid() bool {
	return c.Provider != "" && c.BaseURL != "" && c.Model != "" && c.APIKey != ""
}

func presentAnalysisModelConfig(config AnalysisModelConfig) AnalysisModelSettings {
	configured := config.Provider != "" && config.BaseURL != "" && config.Model != "" && config.EncryptedAPIKey != ""
	return AnalysisModelSettings{
		Provider:         config.Provider,
		BaseURL:          config.BaseURL,
		Model:            config.Model,
		APIKeyConfigured: config.EncryptedAPIKey != "",
		Configured:       configured,
		UpdatedAt:        config.UpdatedAt,
	}
}

func validateAnalysisModelConfigRequest(request analysisModelConfigRequest) error {
	if !safeRuntimeID.MatchString(request.Provider) {
		return errors.New("provider must contain from 1 to 128 letters, numbers, dots, dashes, or underscores")
	}
	if request.Model == "" || len(request.Model) > 240 {
		return errors.New("model must contain from 1 to 240 characters")
	}
	if len(request.APIKey) > 16*1024 {
		return errors.New("api_key must not exceed 16384 bytes")
	}
	parsed, err := url.Parse(request.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(request.BaseURL) > 1000 {
		return errors.New("base_url must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return nil
}
