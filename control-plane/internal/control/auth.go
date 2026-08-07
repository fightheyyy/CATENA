package control

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName       = "barena_session"
	oauthStateCookieName    = "barena_oauth_state"
	oauthVerifierCookieName = "barena_oauth_verifier"
	gatewayTimestampHeader  = "X-Barena-Gateway-Timestamp"
	gatewayBodyHashHeader   = "X-Barena-Gateway-Body-SHA256"
	gatewaySignatureHeader  = "X-Barena-Gateway-Signature"
	gatewayProjectHeader    = "X-Barena-Project-ID"
	gatewayActorHeader      = "X-Barena-Actor-ID"
	maxGatewayBodyBytes     = 2 * 1024 * 1024
	githubHTTPTimeout       = 25 * time.Second
	githubNetworkLimit      = 15 * time.Second
	githubNetworkAttempts   = 3
)

type AuthConfig struct {
	GitHubClientID        string
	GitHubClientSecret    string
	RedirectURL           string
	SecureCookies         bool
	SessionTTL            time.Duration
	AuthorizeURL          string
	TokenURL              string
	UserAPIURL            string
	HTTPClient            *http.Client
	GatewaySecret         string
	APITokenEncryptionKey string
}

func (c AuthConfig) Enabled() bool {
	return c.GitHubClientID != "" &&
		c.GitHubClientSecret != "" &&
		c.RedirectURL != ""
}

func (c AuthConfig) Validate() error {
	if c.GatewaySecret != "" && len(c.GatewaySecret) < 32 {
		return errors.New("BARENA_GATEWAY_SECRET must contain at least 32 characters")
	}
	if c.APITokenEncryptionKey != "" && len(c.APITokenEncryptionKey) < 32 {
		return errors.New("BARENA_API_TOKEN_ENCRYPTION_KEY must contain at least 32 characters")
	}
	configured := 0
	for _, value := range []string{
		c.GitHubClientID,
		c.GitHubClientSecret,
		c.RedirectURL,
	} {
		if value != "" {
			configured++
		}
	}
	if configured != 0 && configured != 3 {
		return errors.New(
			"BARENA_GITHUB_CLIENT_ID, BARENA_GITHUB_CLIENT_SECRET, and BARENA_GITHUB_REDIRECT_URL must be configured together",
		)
	}
	if configured == 3 && c.APITokenEncryptionKey == "" {
		return errors.New("BARENA_API_TOKEN_ENCRYPTION_KEY is required when GitHub authentication is enabled")
	}
	if c.RedirectURL != "" {
		parsed, err := url.Parse(c.RedirectURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("BARENA_GITHUB_REDIRECT_URL must be an absolute URL")
		}
	}
	return nil
}

func (c AuthConfig) normalized() AuthConfig {
	if c.SessionTTL <= 0 {
		c.SessionTTL = 30 * 24 * time.Hour
	}
	if c.AuthorizeURL == "" {
		c.AuthorizeURL = "https://github.com/login/oauth/authorize"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if c.UserAPIURL == "" {
		c.UserAPIURL = "https://api.github.com/user"
	}
	if c.HTTPClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		dialer := &net.Dialer{
			Timeout:   githubNetworkLimit,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = func(ctx context.Context, _ string, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", address)
		}
		transport.TLSHandshakeTimeout = githubNetworkLimit
		transport.ResponseHeaderTimeout = githubNetworkLimit
		// Some mainland-China egress paths blackhole Go's TLS 1.3 ClientHello
		// while GitHub's TLS 1.2 endpoint remains healthy. Keep OAuth reliable on
		// those paths without changing the public HTTPS policy of Catena itself.
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS12,
		}
		c.HTTPClient = &http.Client{
			Timeout:   githubHTTPTimeout,
			Transport: transport,
		}
	}
	return c
}

func (s *HTTPServer) authSession(w http.ResponseWriter, r *http.Request) {
	if !s.auth.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"mode":          "local",
			"authenticated": true,
			"user":          nil,
		})
		return
	}
	user, err := s.currentUser(r)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{
				"mode":          "github",
				"authenticated": false,
				"login_url":     "/v1/auth/github",
			})
			return
		}
		writeProblem(w, http.StatusInternalServerError, "session lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":          "github",
		"authenticated": true,
		"user":          user,
	})
}

func (s *HTTPServer) githubLogin(w http.ResponseWriter, r *http.Request) {
	if !s.auth.Enabled() {
		writeProblem(w, http.StatusNotFound, "GitHub authentication is not configured")
		return
	}
	if canonical, redirect := s.canonicalOAuthRequestURL(r); redirect {
		// OAuth flow cookies are host-only. Normalize localhost/127.0.0.1 (or
		// any other alternate entry host) before issuing state and PKCE cookies
		// so the configured callback can read the exact same flow state.
		http.Redirect(w, r, canonical, http.StatusFound)
		return
	}
	state, err := randomURLToken(32)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "OAuth state generation failed")
		return
	}
	verifier, err := randomURLToken(48)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "OAuth verifier generation failed")
		return
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	s.setFlowCookie(w, oauthStateCookieName, state)
	s.setFlowCookie(w, oauthVerifierCookieName, verifier)

	target, err := url.Parse(s.auth.AuthorizeURL)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "OAuth authorization URL is invalid")
		return
	}
	query := target.Query()
	query.Set("client_id", s.auth.GitHubClientID)
	query.Set("redirect_uri", s.auth.RedirectURL)
	query.Set("scope", "read:user")
	query.Set("state", state)
	query.Set("prompt", "select_account")
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (s *HTTPServer) githubCallback(w http.ResponseWriter, r *http.Request) {
	if !s.auth.Enabled() {
		writeProblem(w, http.StatusNotFound, "GitHub authentication is not configured")
		return
	}
	s.clearFlowCookies(w)
	if oauthError := strings.TrimSpace(r.URL.Query().Get("error")); oauthError != "" {
		http.Redirect(w, r, s.oauthRecoveryURL("cancelled"), http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	returnedState := strings.TrimSpace(r.URL.Query().Get("state"))
	stateCookie, stateErr := r.Cookie(oauthStateCookieName)
	verifierCookie, verifierErr := r.Cookie(oauthVerifierCookieName)
	if code == "" ||
		returnedState == "" ||
		stateErr != nil ||
		verifierErr != nil ||
		subtle.ConstantTimeCompare(
			[]byte(returnedState),
			[]byte(stateCookie.Value),
		) != 1 {
		http.Redirect(w, r, s.oauthRecoveryURL("state"), http.StatusSeeOther)
		return
	}

	accessToken, err := s.exchangeGitHubCode(r.Context(), code, verifierCookie.Value)
	if err != nil {
		slog.Warn("GitHub token exchange failed", "error", err)
		http.Redirect(w, r, s.oauthRecoveryURL("upstream"), http.StatusSeeOther)
		return
	}
	identity, err := s.fetchGitHubIdentity(r.Context(), accessToken)
	if err != nil {
		slog.Warn("GitHub identity lookup failed", "error", err)
		http.Redirect(w, r, s.oauthRecoveryURL("upstream"), http.StatusSeeOther)
		return
	}
	now := time.Now().UTC()
	displayName := identity.Name
	if displayName == "" {
		displayName = identity.Login
	}
	user, err := s.store.UpsertUser(r.Context(), User{
		ID:          newID("usr"),
		GitHubID:    identity.ID,
		Login:       identity.Login,
		DisplayName: displayName,
		AvatarURL:   identity.AvatarURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "GitHub user persistence failed")
		return
	}
	_, err = s.store.EnsureAgentProfile(r.Context(), AgentProfile{
		OwnerUserID: user.ID,
		Slug:        profileSlug(user.Login, user.GitHubID),
		DisplayName: fmt.Sprintf("%s 的小八", user.DisplayName),
		Bio:         "",
		IsPublic:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "XiaoBa profile persistence failed")
		return
	}
	sessionToken, err := randomURLToken(32)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "session generation failed")
		return
	}
	if err := s.store.CreateSession(r.Context(), Session{
		TokenHash: sessionTokenHash(sessionToken),
		UserID:    user.ID,
		ExpiresAt: now.Add(s.auth.SessionTTL),
		CreatedAt: now,
	}); err != nil {
		writeProblem(w, http.StatusInternalServerError, "session persistence failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   int(s.auth.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.auth.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *HTTPServer) canonicalOAuthRequestURL(r *http.Request) (string, bool) {
	redirect, err := url.Parse(s.auth.RedirectURL)
	if err != nil || redirect.Scheme == "" || redirect.Host == "" ||
		strings.EqualFold(strings.TrimSuffix(r.Host, "."), strings.TrimSuffix(redirect.Host, ".")) {
		return "", false
	}
	canonical := &url.URL{
		Scheme:   redirect.Scheme,
		Host:     redirect.Host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	return canonical.String(), true
}

func (s *HTTPServer) oauthRecoveryURL(reason string) string {
	redirect, err := url.Parse(s.auth.RedirectURL)
	if err != nil || redirect.Scheme == "" || redirect.Host == "" {
		return "/?auth_error=" + url.QueryEscape(reason)
	}
	redirect.Path = "/"
	redirect.RawPath = ""
	redirect.RawQuery = url.Values{"auth_error": []string{reason}}.Encode()
	redirect.Fragment = ""
	return redirect.String()
}

func (s *HTTPServer) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), sessionTokenHash(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.auth.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) currentUser(r *http.Request) (User, error) {
	if !s.auth.Enabled() {
		return User{}, ErrNotFound
	}
	if token, ok := bearerAPIToken(r); ok {
		return s.store.GetUserByAPITokenHash(
			r.Context(),
			sessionTokenHash(token),
		)
	}
	return s.currentSessionUser(r)
}

func (s *HTTPServer) currentSessionUser(r *http.Request) (User, error) {
	if !s.auth.Enabled() {
		return User{}, ErrNotFound
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return User{}, ErrNotFound
	}
	return s.store.GetUserBySessionHash(
		r.Context(),
		sessionTokenHash(cookie.Value),
		time.Now().UTC(),
	)
}

func (s *HTTPServer) requireUser(w http.ResponseWriter, r *http.Request) (*User, bool) {
	if platformUser, handled, ok := s.requirePlatformGatewayUser(w, r); handled {
		return platformUser, ok
	}
	if !s.auth.Enabled() {
		return nil, true
	}
	user, err := s.currentUser(r)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "GitHub sign-in is required")
		return nil, false
	}
	return &user, true
}

// requirePlatformGatewayUser authenticates project context supplied by the
// Catena platform. A caller cannot become another project merely by
// changing X-Barena-Project-ID: the method, request URI, project, actor,
// timestamp, and exact request body are all covered by the shared HMAC.
//
// The returned User is a compatibility principal for the existing owner-based
// schema. It is deliberately not given a community profile and must not be
// confused with a human GitHub identity.
func (s *HTTPServer) requirePlatformGatewayUser(
	w http.ResponseWriter,
	r *http.Request,
) (*User, bool, bool) {
	projectID := strings.TrimSpace(r.Header.Get(gatewayProjectHeader))
	hasGatewayHeaders := projectID != "" ||
		r.Header.Get(gatewayTimestampHeader) != "" ||
		r.Header.Get(gatewayBodyHashHeader) != "" ||
		r.Header.Get(gatewaySignatureHeader) != ""
	if !hasGatewayHeaders {
		return nil, false, false
	}
	if s.auth.GatewaySecret == "" {
		// Backward-compatible local mode for direct API users. Production
		// Platform deployments configure the secret and therefore fail closed.
		return nil, false, false
	}
	actorID := strings.TrimSpace(r.Header.Get(gatewayActorHeader))
	if err := verifyPlatformGatewayRequest(r, s.auth.GatewaySecret, projectID, actorID); err != nil {
		writeProblem(w, http.StatusUnauthorized, "invalid Barena platform signature")
		return nil, true, false
	}
	user := platformProjectUser(projectID, time.Now().UTC())
	persisted, err := s.store.UpsertUser(r.Context(), user)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "project identity persistence failed")
		return nil, true, false
	}
	return &persisted, true, true
}

func verifyPlatformGatewayRequest(
	r *http.Request,
	secret string,
	projectID string,
	actorID string,
) error {
	if projectID == "" || len(projectID) > 256 || actorID == "" || len(actorID) > 256 ||
		strings.ContainsAny(projectID+actorID, "\r\n\x00") {
		return errors.New("invalid project context")
	}
	timestampText := strings.TrimSpace(r.Header.Get(gatewayTimestampHeader))
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		return errors.New("invalid gateway timestamp")
	}
	now := time.Now().Unix()
	if timestamp < now-300 || timestamp > now+300 {
		return errors.New("expired gateway timestamp")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxGatewayBodyBytes+1))
	if err != nil || len(body) > maxGatewayBodyBytes {
		return errors.New("gateway request body is too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	bodyDigest := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodyDigest[:])
	providedBodyHash := strings.ToLower(strings.TrimSpace(r.Header.Get(gatewayBodyHashHeader)))
	if len(providedBodyHash) != sha256.Size*2 ||
		subtle.ConstantTimeCompare([]byte(providedBodyHash), []byte(bodyHash)) != 1 {
		return errors.New("gateway body digest mismatch")
	}
	canonical := strings.Join([]string{
		strings.ToUpper(r.Method),
		r.URL.RequestURI(),
		projectID,
		actorID,
		timestampText,
		bodyHash,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	expected := hex.EncodeToString(mac.Sum(nil))
	provided := strings.ToLower(strings.TrimSpace(r.Header.Get(gatewaySignatureHeader)))
	if len(provided) != len(expected) ||
		subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return errors.New("gateway signature mismatch")
	}
	return nil
}

func platformProjectUser(projectID string, now time.Time) User {
	digest := sha256.Sum256([]byte(projectID))
	hexDigest := hex.EncodeToString(digest[:])
	parsed, _ := strconv.ParseInt(hexDigest[:15], 16, 64)
	return User{
		ID:          "platform_project_" + hexDigest[:24],
		GitHubID:    (int64(1) << 61) | parsed,
		Login:       "platform-project-" + hexDigest[:12],
		DisplayName: "Platform project " + hexDigest[:8],
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (s *HTTPServer) requireSessionUser(
	w http.ResponseWriter,
	r *http.Request,
) (*User, bool) {
	if !s.auth.Enabled() {
		writeProblem(w, http.StatusNotFound, "endpoint credentials require GitHub authentication")
		return nil, false
	}
	user, err := s.currentSessionUser(r)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "GitHub sign-in is required")
		return nil, false
	}
	return &user, true
}

func (s *HTTPServer) requireAPITokenUser(
	w http.ResponseWriter,
	r *http.Request,
) (*User, bool) {
	if !s.auth.Enabled() {
		writeProblem(w, http.StatusNotFound, "edge ingestion requires configured authentication")
		return nil, false
	}
	token, ok := bearerAPIToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "a Barena API token is required")
		return nil, false
	}
	user, err := s.store.GetUserByAPITokenHash(
		r.Context(),
		sessionTokenHash(token),
	)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "the Barena API token is invalid or revoked")
		return nil, false
	}
	return &user, true
}

type agentIngestPrincipal struct {
	User  *User
	Agent *RegisteredAgent
}

func (s *HTTPServer) requireAgentAPITokenPrincipal(
	w http.ResponseWriter,
	r *http.Request,
) (agentIngestPrincipal, bool) {
	if !s.auth.Enabled() {
		writeProblem(w, http.StatusNotFound, "edge ingestion requires configured authentication")
		return agentIngestPrincipal{}, false
	}
	plaintext, ok := bearerAPIToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "an Agent connection key is required")
		return agentIngestPrincipal{}, false
	}
	hash := sessionTokenHash(plaintext)
	token, err := s.store.GetAPITokenByHash(r.Context(), hash)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "the Agent connection key is invalid or revoked")
		return agentIngestPrincipal{}, false
	}
	user, err := s.store.GetUserByAPITokenHash(r.Context(), hash)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "the Agent connection key is invalid or revoked")
		return agentIngestPrincipal{}, false
	}
	principal := agentIngestPrincipal{User: &user}
	if token.AgentID == "" {
		return principal, true
	}
	agent, err := s.store.GetRegisteredAgentByOwner(r.Context(), user.ID, token.AgentID)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "the Agent connection key is no longer bound")
		return agentIngestPrincipal{}, false
	}
	principal.Agent = &agent
	return principal, true
}

func (s *HTTPServer) requireConversationIngestPrincipal(
	w http.ResponseWriter,
	r *http.Request,
) (agentIngestPrincipal, bool) {
	if platformUser, handled, ok := s.requirePlatformGatewayUser(w, r); handled {
		return agentIngestPrincipal{User: platformUser}, ok
	}
	return s.requireAgentAPITokenPrincipal(w, r)
}

// requireIngestUser keeps the historical direct PAT path for local
// compatibility while making the signed Platform project principal canonical.
// Public project API keys terminate at spiral-app and are never forwarded to
// or stored by this service.
func (s *HTTPServer) requireIngestUser(
	w http.ResponseWriter,
	r *http.Request,
) (*User, bool) {
	if platformUser, handled, ok := s.requirePlatformGatewayUser(w, r); handled {
		return platformUser, ok
	}
	return s.requireAPITokenUser(w, r)
}

func bearerAPIToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if (!strings.HasPrefix(token, "barena_pat_") && !strings.HasPrefix(token, "catena_agent_")) ||
		len(token) > 256 ||
		strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}

func (s *HTTPServer) exchangeGitHubCode(
	ctx context.Context,
	code string,
	verifier string,
) (string, error) {
	form := url.Values{}
	form.Set("client_id", s.auth.GitHubClientID)
	form.Set("client_secret", s.auth.GitHubClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", s.auth.RedirectURL)
	form.Set("code_verifier", verifier)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.auth.TokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "barena")
	response, err := s.doGitHubRequest(request)
	if err != nil {
		return "", fmt.Errorf("GitHub token exchange failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("GitHub token response failed: %w", err)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", errors.New("GitHub token response was invalid")
	}
	if response.StatusCode != http.StatusOK ||
		payload.Error != "" ||
		payload.AccessToken == "" {
		return "", errors.New("GitHub token exchange was rejected")
	}
	return payload.AccessToken, nil
}

type githubIdentity struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func (s *HTTPServer) fetchGitHubIdentity(
	ctx context.Context,
	accessToken string,
) (githubIdentity, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		s.auth.UserAPIURL,
		nil,
	)
	if err != nil {
		return githubIdentity{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "barena")
	response, err := s.doGitHubRequest(request)
	if err != nil {
		return githubIdentity{}, fmt.Errorf("GitHub identity request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return githubIdentity{}, fmt.Errorf("GitHub identity response failed: %w", err)
	}
	var identity githubIdentity
	if err := json.Unmarshal(body, &identity); err != nil {
		return githubIdentity{}, errors.New("GitHub identity response was invalid")
	}
	if response.StatusCode != http.StatusOK ||
		identity.ID < 1 ||
		identity.Login == "" {
		return githubIdentity{}, errors.New("GitHub identity request was rejected")
	}
	return identity, nil
}

func (s *HTTPServer) doGitHubRequest(request *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < githubNetworkAttempts; attempt++ {
		current := request
		if attempt > 0 {
			current = request.Clone(request.Context())
			if request.GetBody != nil {
				body, err := request.GetBody()
				if err != nil {
					return nil, err
				}
				current.Body = body
			}
		}
		response, err := s.auth.HTTPClient.Do(current)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if request.Context().Err() != nil || !retryableGitHubNetworkError(err) {
			break
		}
		if attempt+1 < githubNetworkAttempts {
			time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
		}
	}
	return nil, lastErr
}

func retryableGitHubNetworkError(err error) bool {
	var networkErr net.Error
	return errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary())
}

func (s *HTTPServer) setFlowCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:  name,
		Value: value,
		// The public cutover keeps both the native v1 callback and the
		// callback path already registered by the former Web. A root path lets
		// the same short-lived PKCE flow work with either route.
		Path:     "/",
		MaxAge:   10 * 60,
		HttpOnly: true,
		Secure:   s.auth.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *HTTPServer) clearFlowCookies(w http.ResponseWriter) {
	for _, name := range []string{oauthStateCookieName, oauthVerifierCookieName} {
		for _, path := range []string{"/", "/v1/auth/github/callback", "/api/auth/callback/github"} {
			http.SetCookie(w, &http.Cookie{
				Name:     name,
				Value:    "",
				Path:     path,
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   s.auth.SecureCookies,
				SameSite: http.SameSiteLaxMode,
			})
		}
	}
}

func randomURLToken(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func profileSlug(login string, githubID int64) string {
	return strings.ToLower(login) + "-" + strconv.FormatInt(githubID, 10)
}
