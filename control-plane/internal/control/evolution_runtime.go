package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	evolutionRequestSchema  = "barena.xiaoba_evolution_request.v1"
	evolutionResponseSchema = "barena.xiaoba_evolution_response.v1"
	evolutionManifestSchema = "barena.xiaoba_evolution_runtime.v1"
)

var safeRuntimeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type EvolutionRuntimeConfig struct {
	NodeCommand   string
	WorkerPath    string
	XiaoBaCommand string
	ProjectRoot   string
	RolesRoot     string
	SkillsRoot    string
	WorkspaceRoot string
	EnvAllowlist  []string
	ProbeTimeout  time.Duration
	CacheTTL      time.Duration
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
	Telemetry json.RawMessage
}

type EvolutionRuntimeManager struct {
	config EvolutionRuntimeConfig
	mu     sync.Mutex
	cached EvolutionRuntimeManifest
	until  time.Time
}

type evolutionWorkerRuntimeConfig struct {
	Command        string   `json:"command,omitempty"`
	ProjectRoot    string   `json:"project_root,omitempty"`
	RolesRoot      string   `json:"roles_root,omitempty"`
	SkillsRoot     string   `json:"skills_root,omitempty"`
	EnvAllowlist   []string `json:"env_allowlist,omitempty"`
	ProbeTimeoutMS int64    `json:"probe_timeout_ms,omitempty"`
}

type evolutionWorkerResponse struct {
	Schema    string                    `json:"schema"`
	RequestID string                    `json:"request_id"`
	Operation string                    `json:"operation"`
	Status    string                    `json:"status"`
	Runtime   *EvolutionRuntimeManifest `json:"runtime,omitempty"`
	Result    json.RawMessage           `json:"result,omitempty"`
	Error     *struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"error,omitempty"`
}

type evolutionWorkerTurnResult struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Assistant  *struct {
		Content string `json:"content"`
	} `json:"assistant,omitempty"`
}

func NewEvolutionRuntimeManager(config EvolutionRuntimeConfig) (*EvolutionRuntimeManager, error) {
	if config.NodeCommand == "" {
		config.NodeCommand = "node"
	}
	if config.XiaoBaCommand == "" {
		config.XiaoBaCommand = "xiaoba"
	}
	if config.ProbeTimeout <= 0 {
		config.ProbeTimeout = 8 * time.Second
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 5 * time.Second
	}
	worker, err := filepath.Abs(config.WorkerPath)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(worker); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Evolution Runtime Worker does not exist: %s", worker)
	}
	workspace, err := filepath.Abs(config.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return nil, err
	}
	config.WorkerPath = worker
	config.WorkspaceRoot = workspace
	config.ProjectRoot = absoluteOptional(config.ProjectRoot)
	config.RolesRoot = absoluteOptional(config.RolesRoot)
	config.SkillsRoot = absoluteOptional(config.SkillsRoot)
	config.EnvAllowlist = uniqueStrings(config.EnvAllowlist)
	return &EvolutionRuntimeManager{config: config}, nil
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
	requestID := newID("runtime-probe")
	response, err := m.execute(probeCtx, map[string]any{
		"schema":     evolutionRequestSchema,
		"request_id": requestID,
		"operation":  "probe",
		"runtime":    m.workerRuntimeConfig(),
	})
	manifest := blockedEvolutionManifest("runtime_error")
	if err == nil && response.RequestID == requestID && response.Operation == "probe" &&
		response.Status == "ok" && response.Runtime != nil &&
		validEvolutionManifest(*response.Runtime) {
		manifest = sanitizeEvolutionManifest(*response.Runtime)
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
		return nil, errors.New("Evolution Runtime is not configured")
	}
	if !safeRuntimeID.MatchString(input.RequestID) || !safeRuntimeID.MatchString(input.RunID) {
		return nil, errors.New("request and Run identifiers must be safe")
	}
	if !isEvolutionRole(input.Role) {
		return nil, errors.New("role is not allowed in the embedded Evolution Runtime")
	}
	if strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 1_000_000 {
		return nil, errors.New("prompt must contain from 1 to 1000000 bytes")
	}
	if input.Timeout <= 0 || input.Timeout > 15*time.Minute {
		return nil, errors.New("turn timeout must be from 1ms to 15m")
	}
	workspace := filepath.Join(m.config.WorkspaceRoot, input.RunID, input.RequestID)
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return nil, err
	}
	request := map[string]any{
		"schema":     evolutionRequestSchema,
		"request_id": input.RequestID,
		"operation":  "turn",
		"run_id":     input.RunID,
		"role":       input.Role,
		"prompt":     input.Prompt,
		"workspace":  workspace,
		"timeout_ms": input.Timeout.Milliseconds(),
		"runtime":    m.workerRuntimeConfig(),
	}
	if len(input.Telemetry) > 0 {
		var telemetry any
		if err := json.Unmarshal(input.Telemetry, &telemetry); err != nil {
			return nil, errors.New("telemetry must be valid JSON")
		}
		request["telemetry"] = telemetry
	}
	response, err := m.execute(ctx, request)
	if err != nil {
		return nil, err
	}
	if response.RequestID != input.RequestID || response.Operation != "turn" {
		return nil, errors.New("Evolution Runtime Worker returned a mismatched response")
	}
	if response.Status != "ok" || len(response.Result) == 0 {
		return nil, errors.New("Evolution Runtime role turn failed")
	}
	var turn evolutionWorkerTurnResult
	if err := json.Unmarshal(response.Result, &turn); err != nil {
		return nil, errors.New("Evolution Runtime role turn returned an invalid result")
	}
	if turn.Status != "completed" {
		detail := bounded(strings.TrimSpace(turn.Detail), 500)
		if detail == "" {
			detail = "the Runtime did not complete the role turn"
		}
		reason := bounded(strings.TrimSpace(turn.ReasonCode), 120)
		if reason != "" {
			return nil, fmt.Errorf("Evolution Runtime role turn %s: %s", reason, detail)
		}
		return nil, fmt.Errorf("Evolution Runtime role turn failed: %s", detail)
	}
	if turn.Assistant == nil || strings.TrimSpace(turn.Assistant.Content) == "" {
		return nil, errors.New("Evolution Runtime role turn completed without assistant output")
	}
	return append(json.RawMessage(nil), response.Result...), nil
}

func (m *EvolutionRuntimeManager) workerRuntimeConfig() evolutionWorkerRuntimeConfig {
	probeTimeout := m.config.ProbeTimeout - 500*time.Millisecond
	if probeTimeout < time.Millisecond {
		probeTimeout = m.config.ProbeTimeout
	}
	return evolutionWorkerRuntimeConfig{
		Command:        m.config.XiaoBaCommand,
		ProjectRoot:    m.config.ProjectRoot,
		RolesRoot:      m.config.RolesRoot,
		SkillsRoot:     m.config.SkillsRoot,
		EnvAllowlist:   append([]string(nil), m.config.EnvAllowlist...),
		ProbeTimeoutMS: probeTimeout.Milliseconds(),
	}
}

func (m *EvolutionRuntimeManager) execute(
	ctx context.Context,
	request any,
) (evolutionWorkerResponse, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return evolutionWorkerResponse{}, err
	}
	cmd := exec.Command(m.config.NodeCommand, m.config.WorkerPath)
	prepareCommand(cmd)
	cmd.Stdin = bytes.NewReader(requestBytes)
	stdout := &boundedCommandBuffer{limit: 512 * 1024}
	stderr := &boundedCommandBuffer{limit: 64 * 1024}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return evolutionWorkerResponse{}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = interruptCommand(cmd)
		select {
		case <-done:
		case <-time.After(750 * time.Millisecond):
			_ = killCommand(cmd)
			<-done
		}
		return evolutionWorkerResponse{}, ctx.Err()
	}
	if err != nil {
		return evolutionWorkerResponse{}, fmt.Errorf("Evolution Runtime Worker failed: %w", err)
	}
	var response evolutionWorkerResponse
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
		return evolutionWorkerResponse{}, errors.New("Evolution Runtime Worker returned invalid JSON")
	}
	if response.Schema != evolutionResponseSchema {
		return evolutionWorkerResponse{}, errors.New("Evolution Runtime Worker returned an unsupported schema")
	}
	return response, nil
}

func validEvolutionManifest(manifest EvolutionRuntimeManifest) bool {
	if manifest.Schema != evolutionManifestSchema || manifest.RuntimeID != "xiaobaos-evolution" ||
		manifest.Kind != "embedded_evolution" || (manifest.Status != "ready" && manifest.Status != "blocked") ||
		manifest.Capabilities.TargetRuntimeHosted || len(manifest.Roles) != 4 {
		return false
	}
	seen := make(map[string]bool, 4)
	for _, role := range manifest.Roles {
		if !isEvolutionRole(role.ID) || seen[role.ID] {
			return false
		}
		seen[role.ID] = true
	}
	return len(seen) == 4
}

func sanitizeEvolutionManifest(input EvolutionRuntimeManifest) EvolutionRuntimeManifest {
	manifest := baseEvolutionManifest()
	manifest.Status = input.Status
	manifest.Version = safeVersion(input.Version)
	manifest.ReasonCode = input.ReasonCode
	if input.Status == "ready" {
		manifest.Detail = "Embedded XiaoBaOS is ready with all four evaluator/evolution roles."
	} else {
		manifest.Detail = "The embedded XiaoBaOS evaluator/evolution Runtime is currently blocked."
	}
	return manifest
}

func blockedEvolutionManifest(reason string) EvolutionRuntimeManifest {
	manifest := baseEvolutionManifest()
	manifest.Status = "blocked"
	manifest.ReasonCode = reason
	if reason == "not_configured" {
		manifest.Detail = "The XiaoBaOS evaluator/evolution Runtime is not configured."
	} else {
		manifest.Detail = "The embedded XiaoBaOS evaluator/evolution Runtime is currently blocked."
	}
	return manifest
}

func baseEvolutionManifest() EvolutionRuntimeManifest {
	return EvolutionRuntimeManifest{
		Schema:      evolutionManifestSchema,
		RuntimeID:   "xiaobaos-evolution",
		DisplayName: "XiaoBa Evolution Runtime",
		Kind:        "embedded_evolution",
		Source:      "configured",
		Roles: []EvolutionRuntimeRole{
			{ID: "user-cat", DisplayName: "UserCat", Responsibility: "Simulate one natural, incomplete user turn without judging the Agent.", Output: "user turn"},
			{ID: "inspector-cat", DisplayName: "InspectorCat", Responsibility: "Locate a failure mode in retained evidence and pair it with a replayable Case.", Output: "finding + case"},
			{ID: "reviewer-cat", DisplayName: "ReviewerCat", Responsibility: "Review verifier-backed evidence and emit a semantic pass, fail, or blocked verdict.", Output: "semantic review"},
			{ID: "evolution-cat", DisplayName: "EvolutionCat", Responsibility: "Create a minimal Role, Skill, or Memory candidate from an accepted finding.", Output: "role / skill / memory candidate"},
		},
		Capabilities: EvolutionRuntimeCapabilities{
			Probe:               true,
			RoleTurn:            true,
			Cancellation:        true,
			Telemetry:           "native",
			TargetRuntimeHosted: false,
		},
	}
}

func cloneEvolutionManifest(input EvolutionRuntimeManifest) EvolutionRuntimeManifest {
	input.Roles = append([]EvolutionRuntimeRole(nil), input.Roles...)
	return input
}

func isEvolutionRole(role string) bool {
	switch role {
	case "user-cat", "inspector-cat", "reviewer-cat", "evolution-cat":
		return true
	default:
		return false
	}
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

func absoluteOptional(value string) string {
	if value == "" {
		return ""
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return value
	}
	return absolute
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

type boundedCommandBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedCommandBuffer) Write(value []byte) (int, error) {
	if b.Buffer.Len()+len(value) > b.limit {
		return 0, errors.New("worker output exceeded limit")
	}
	return b.Buffer.Write(value)
}
