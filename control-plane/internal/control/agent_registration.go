package control

import (
	"net/http"
	"strings"
	"time"
)

type createRegisteredAgentRequest struct {
	DisplayName string `json:"display_name"`
}

func (s *HTTPServer) getRegisteredAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	agent, err := s.store.GetRegisteredAgentByOwner(r.Context(), user.ID, r.PathValue("agent_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	tokens, err := s.store.ListAPITokensByUser(r.Context(), user.ID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	var credential *APIToken
	for _, token := range tokens {
		if token.AgentID != agent.ID {
			continue
		}
		presented := s.presentAPIToken(token)
		credential = &presented
		break
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"agent": agent, "connected": !agent.LastSeenAt.IsZero(), "credential": credential,
	})
}

func (s *HTTPServer) createAgentConnectionKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	agent, err := s.store.GetRegisteredAgentByOwner(r.Context(), user.ID, r.PathValue("agent_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	plaintext, token, err := s.newAgentConnectionKey(user.ID, agent.ID, agent.DisplayName, time.Now().UTC())
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := s.store.CreateAPIToken(r.Context(), token); err != nil {
		writeProblem(w, statusFor(err), "this Agent already has an active connection key")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"api_token": s.presentAPIToken(token), "token": plaintext})
}

func (s *HTTPServer) createRegisteredAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	var request createRegisteredAgentRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	displayName := strings.TrimSpace(request.DisplayName)
	if displayName == "" || len([]rune(displayName)) > 80 || strings.ContainsAny(displayName, "\x00\r\n") {
		writeProblem(w, http.StatusBadRequest, "Agent name must contain from 1 to 80 characters")
		return
	}
	now := time.Now().UTC()
	agent := RegisteredAgent{
		ID: newID("agent"), OwnerUserID: user.ID, DisplayName: displayName,
		CreatedAt: now, UpdatedAt: now,
	}
	plaintext, token, err := s.newAgentConnectionKey(user.ID, agent.ID, displayName, now)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := s.store.CreateAgentWithAPIToken(r.Context(), agent, token); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"agent": agent, "api_token": s.presentAPIToken(token), "token": plaintext,
	})
}

func (s *HTTPServer) newAgentConnectionKey(
	userID string,
	agentID string,
	displayName string,
	now time.Time,
) (string, APIToken, error) {
	secret, err := randomURLToken(32)
	if err != nil {
		return "", APIToken{}, err
	}
	plaintext := "catena_agent_" + secret
	tokenID := newID("agent_key")
	encrypted, err := encryptAPIToken(plaintext, tokenID, s.auth.APITokenEncryptionKey)
	if err != nil {
		return "", APIToken{}, err
	}
	return plaintext, APIToken{
		ID: tokenID, TokenHash: sessionTokenHash(plaintext), EncryptedToken: encrypted,
		UserID: userID, AgentID: agentID, Name: displayName, CreatedAt: now,
	}, nil
}

func shouldReplaceRuntimeKind(current string, observed string) bool {
	if observed == "" {
		return false
	}
	return current == "" || current == "otel" || observed != "otel"
}

func detectOTLPRuntime(spans []TraceSpan) string {
	for _, span := range spans {
		values := []string{span.ServiceName, span.ScopeName}
		for _, key := range []string{"service.name", "telemetry.sdk.name", "gen_ai.system", "agent.runtime"} {
			if value, ok := span.ResourceAttributes[key]; ok {
				values = append(values, strings.TrimSpace(strings.ToLower(toTraceString(value))))
			}
		}
		joined := strings.ToLower(strings.Join(values, " "))
		switch {
		case strings.Contains(joined, "xiaoba"):
			return "xiaobaos"
		case strings.Contains(joined, "codex"):
			return "codex"
		case strings.Contains(joined, "claude") || strings.Contains(joined, "anthropic"):
			return "claude_code"
		case strings.Contains(joined, "hermes"):
			return "hermes"
		case strings.Contains(joined, "openclaw"):
			return "openclaw"
		}
	}
	return "otel"
}

func toTraceString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
