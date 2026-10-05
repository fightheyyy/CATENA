package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The analysis engine (engine/ in this repository) is a private HTTP service.
// The control plane owns prompts, evidence and output validation; the engine
// only runs one role turn on the owner's model.
const (
	engineTurnRequestSchema  = "catena.engine_turn_request.v1"
	engineTurnResponseSchema = "catena.engine_turn_response.v1"
	engineManifestSchema     = "catena.engine_manifest.v1"
	engineRuntimeID          = "catena-engine"
	engineMaxResponseBytes   = 512 * 1024
)

var safeRuntimeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// evolutionRoles are the engine roles in stage order.
var evolutionRoles = []string{"inspector", "evolution", "reviewer"}

type EvolutionRuntimeConfig struct {
	// URL is the engine origin, for example http://catena-engine:8790.
	URL          string
	Token        string
	ProbeTimeout time.Duration
	CacheTTL     time.Duration
	Client       *http.Client
}

type EvolutionRuntimeRole struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Responsibility string `json:"responsibility"`
	Output         string `json:"output"`
}

type EvolutionRuntimeCapabilities struct {
	Probe               bool   `json:"probe"`
	RoleTurn            bool   `json:"role_turn"`
	Cancellation        bool   `json:"cancellation"`
	Telemetry           string `json:"telemetry"`
	TargetRuntimeHosted bool   `json:"target_runtime_hosted"`
}

type EvolutionRuntimeManifest struct {
	Schema       string                       `json:"schema"`
	RuntimeID    string                       `json:"runtime_id"`
	DisplayName  string                       `json:"display_name"`
	Kind         string                       `json:"kind"`
	Source       string                       `json:"source"`
	Status       string                       `json:"status"`
	Version      string                       `json:"version,omitempty"`
	ReasonCode   string                       `json:"reason_code,omitempty"`
	Detail       string                       `json:"detail"`
	Roles        []EvolutionRuntimeRole       `json:"roles"`
	Capabilities EvolutionRuntimeCapabilities `json:"capabilities"`
}

type EvolutionRoleTurnInput struct {
	RequestID string
	RunID     string
	Role      string
	Prompt    string
	Timeout   time.Duration
	Model     EvolutionModelCredentials
}

type EvolutionRuntimeManager struct {
	config EvolutionRuntimeConfig
	base   string
	mu     sync.Mutex
	cached EvolutionRuntimeManifest
	until  time.Time
}

type engineTurnResponse struct {
	Schema    string          `json:"schema"`
	RequestID string          `json:"request_id"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"error,omitempty"`
}

type engineTurnResult struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Assistant  *struct {
		Content string `json:"content"`
	} `json:"assistant,omitempty"`
}

// NewEvolutionRuntimeManager returns nil without error when no engine URL is
// configured; a nil manager reports not_configured and refuses turns.
func NewEvolutionRuntimeManager(config EvolutionRuntimeConfig) (*EvolutionRuntimeManager, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(config.URL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("engine URL must be an HTTP(S) origin")
	}
	if config.ProbeTimeout <= 0 {
		config.ProbeTimeout = 8 * time.Second
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 5 * time.Second
	}
	if config.Client == nil {
		config.Client = &http.Client{}
	}
	return &EvolutionRuntimeManager{config: config, base: strings.TrimRight(parsed.String(), "/")}, nil
}

func (m *EvolutionRuntimeManager) Probe(ctx context.Context) EvolutionRuntimeManifest {
	if m == nil {
		return blockedEvolutionManifest("not_configured")
	}
	now := time.Now()
	m.mu.Lock()
	if !m.until.IsZero() && now.Before(m.until) {
		cached := cloneEvolutionManifest(m.cached)
		m.mu.Unlock()
		return cached
	}
	m.mu.Unlock()

	probeCtx, cancel := context.WithTimeout(ctx, m.config.ProbeTimeout)
	defer cancel()
	manifest := blockedEvolutionManifest("runtime_error")
	var remote EvolutionRuntimeManifest
	if status, err := m.do(probeCtx, http.MethodGet, "/v1/manifest", nil, &remote); err == nil &&
		status == http.StatusOK && validEvolutionManifest(remote) {
		manifest = sanitizeEvolutionManifest(remote)
	}
	m.mu.Lock()
	m.cached = cloneEvolutionManifest(manifest)
	m.until = time.Now().Add(m.config.CacheTTL)
	m.mu.Unlock()
	return manifest
}

func (m *EvolutionRuntimeManager) RunRoleTurn(
	ctx context.Context,
	input EvolutionRoleTurnInput,
) (json.RawMessage, error) {
	if m == nil {
		return nil, errors.New("Catena Engine is not configured")
	}
	if !safeRuntimeID.MatchString(input.RequestID) || !safeRuntimeID.MatchString(input.RunID) {
		return nil, errors.New("request and Run identifiers must be safe")
	}
	if !isEvolutionRole(input.Role) {
		return nil, errors.New("role is not allowed in Catena Engine")
	}
	if strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 1_000_000 {
		return nil, errors.New("prompt must contain from 1 to 1000000 bytes")
	}
	if input.Timeout <= 0 || input.Timeout > 15*time.Minute {
		return nil, errors.New("turn timeout must be from 1ms to 15m")
	}
	if !input.Model.Valid() {
		return nil, errors.New("owner LLM configuration is incomplete")
	}
	request := map[string]any{
		"schema":     engineTurnRequestSchema,
		"request_id": input.RequestID,
		"run_id":     input.RunID,
		"role":       input.Role,
		"prompt":     input.Prompt,
		"timeout_ms": input.Timeout.Milliseconds(),
		"model": map[string]string{
			"provider": input.Model.Provider,
			"base_url": input.Model.BaseURL,
			"model":    input.Model.Model,
			"api_key":  input.Model.APIKey,
		},
	}
	var response engineTurnResponse
	status, err := m.do(ctx, http.MethodPost, "/v1/turn", request, &response)
	if err != nil {
		return nil, err
	}
	if response.Schema != engineTurnResponseSchema {
		return nil, fmt.Errorf("Catena Engine returned HTTP %d with an unsupported schema", status)
	}
	if response.Status != "ok" || len(response.Result) == 0 {
		if response.Error != nil {
			return nil, fmt.Errorf("Catena Engine rejected the turn: %s", bounded(response.Error.Detail, 500))
		}
		return nil, errors.New("Catena Engine role turn failed")
	}
	if response.RequestID != input.RequestID {
		return nil, errors.New("Catena Engine returned a mismatched response")
	}
	var turn engineTurnResult
	if err := json.Unmarshal(response.Result, &turn); err != nil {
		return nil, errors.New("Catena Engine role turn returned an invalid result")
	}
	if turn.Status != "completed" {
		detail := bounded(strings.TrimSpace(turn.Detail), 500)
		if detail == "" {
			detail = "the engine did not complete the role turn"
		}
		if reason := bounded(strings.TrimSpace(turn.ReasonCode), 120); reason != "" {
			return nil, fmt.Errorf("Catena Engine role turn %s: %s", reason, detail)
		}
		return nil, fmt.Errorf("Catena Engine role turn failed: %s", detail)
	}
	if turn.Assistant == nil || strings.TrimSpace(turn.Assistant.Content) == "" {
		return nil, errors.New("Catena Engine role turn completed without assistant output")
	}
	return append(json.RawMessage(nil), response.Result...), nil
}

func (m *EvolutionRuntimeManager) do(
	ctx context.Context,
	method string,
	path string,
	body any,
	destination any,
) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, m.base+path, reader)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	if m.config.Token != "" {
		request.Header.Set("Authorization", "Bearer "+m.config.Token)
	}
	response, err := m.config.Client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		// Transport errors can echo the URL but never the request body.
		return 0, errors.New("Catena Engine is unreachable")
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, engineMaxResponseBytes+1))
	if err != nil {
		return response.StatusCode, errors.New("Catena Engine response could not be read")
	}
	if len(payload) > engineMaxResponseBytes {
		return response.StatusCode, errors.New("Catena Engine response exceeded the size limit")
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return response.StatusCode, fmt.Errorf("Catena Engine returned HTTP %d without valid JSON", response.StatusCode)
	}
	return response.StatusCode, nil
}

func validEvolutionManifest(manifest EvolutionRuntimeManifest) bool {
	if manifest.Schema != engineManifestSchema || manifest.RuntimeID != engineRuntimeID ||
		(manifest.Status != "ready" && manifest.Status != "blocked") ||
		manifest.Capabilities.TargetRuntimeHosted || len(manifest.Roles) != len(evolutionRoles) {
		return false
	}
	seen := make(map[string]bool, len(evolutionRoles))
	for _, role := range manifest.Roles {
		if !isEvolutionRole(role.ID) || seen[role.ID] {
			return false
		}
		seen[role.ID] = true
	}
	return true
}

// sanitizeEvolutionManifest keeps only status and version from the engine;
// every descriptive field comes from the control plane.
func sanitizeEvolutionManifest(input EvolutionRuntimeManifest) EvolutionRuntimeManifest {
	manifest := baseEvolutionManifest()
	manifest.Status = input.Status
	manifest.Version = safeVersion(input.Version)
	if input.Status == "ready" {
		manifest.Detail = "Catena Engine is ready with the Inspector, Evolution and Reviewer roles."
	} else {
		manifest.ReasonCode = safeVersion(input.ReasonCode)
		manifest.Detail = "Catena Engine is currently blocked."
	}
	return manifest
}

func blockedEvolutionManifest(reason string) EvolutionRuntimeManifest {
	manifest := baseEvolutionManifest()
	manifest.Status = "blocked"
	manifest.ReasonCode = reason
	if reason == "not_configured" {
		manifest.Detail = "Catena Engine is not configured."
	} else {
		manifest.Detail = "Catena Engine is currently unreachable or blocked."
	}
	return manifest
}

func baseEvolutionManifest() EvolutionRuntimeManifest {
	return EvolutionRuntimeManifest{
		Schema:      engineManifestSchema,
		RuntimeID:   engineRuntimeID,
		DisplayName: "Catena Engine",
		Kind:        "embedded_evolution",
		Source:      "configured",
		Roles: []EvolutionRuntimeRole{
			{ID: "inspector", DisplayName: "Inspector", Responsibility: "Locate one grounded failure pattern in retained Trace evidence.", Output: "finding"},
			{ID: "evolution", DisplayName: "Evolution", Responsibility: "Draft one small, reviewable agent.md or Skill that addresses an accepted finding.", Output: "Agent asset"},
			{ID: "reviewer", DisplayName: "Reviewer", Responsibility: "Check whether a proposal is coherent and grounded in the retained evidence.", Output: "grounding review"},
		},
		Capabilities: EvolutionRuntimeCapabilities{
			Probe:               true,
			RoleTurn:            true,
			Cancellation:        false,
			Telemetry:           "none",
			TargetRuntimeHosted: false,
		},
	}
}

func cloneEvolutionManifest(input EvolutionRuntimeManifest) EvolutionRuntimeManifest {
	input.Roles = append([]EvolutionRuntimeRole(nil), input.Roles...)
	return input
}

func isEvolutionRole(role string) bool {
	for _, known := range evolutionRoles {
		if role == known {
			return true
		}
	}
	return false
}

func safeVersion(value string) string {
	if len(value) > 120 {
		value = value[:120]
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune(" ._+-", r) {
			return r
		}
		return -1
	}, value)
}
