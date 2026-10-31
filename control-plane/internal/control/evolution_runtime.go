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
	"time"
)

const engineTurnRequestSchema = "catena.analysis_request.v2"
const engineMaxResponseBytes = 2 * 1024 * 1024

var safeRuntimeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type EvolutionRuntimeConfig struct {
	URL, Token                          string
	ProbeTimeout, TurnTimeout, CacheTTL time.Duration
	Client                              *http.Client
}
type EvolutionRuntimeManifest struct {
	Schema    string   `json:"schema"`
	Status    string   `json:"status"`
	Pipelines []string `json:"pipelines"`
	Detail    string   `json:"detail"`
}
type EvolutionRuntimeManager struct {
	config EvolutionRuntimeConfig
	base   string
}

func NewEvolutionRuntimeManager(c EvolutionRuntimeConfig) (*EvolutionRuntimeManager, error) {
	if c.URL == "" {
		return nil, nil
	}
	u, e := url.Parse(c.URL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid private Engine URL")
	}
	if c.Client == nil {
		c.Client = &http.Client{}
	}
	if c.ProbeTimeout <= 0 {
		c.ProbeTimeout = 3 * time.Second
	}
	if c.TurnTimeout <= 0 {
		c.TurnTimeout = 20 * time.Minute
	}
	return &EvolutionRuntimeManager{c, strings.TrimRight(c.URL, "/")}, nil
}
func (m *EvolutionRuntimeManager) Probe(ctx context.Context) EvolutionRuntimeManifest {
	out := EvolutionRuntimeManifest{Schema: "catena.engine_manifest.v2", Status: "not_configured", Pipelines: []string{}}
	if m == nil {
		return out
	}
	ctx, c := context.WithTimeout(ctx, m.config.ProbeTimeout)
	defer c()
	status, e := m.call(ctx, "", http.MethodGet, "/v2/manifest", nil, &out)
	if e != nil || status != 200 {
		out.Status = "blocked"
	}
	return out
}
func (m *EvolutionRuntimeManager) call(ctx context.Context, owner, method, path string, input, output any) (int, error) {
	if m == nil {
		return 0, errors.New("Engine not configured")
	}
	var body io.Reader
	if input != nil {
		raw, e := json.Marshal(input)
		if e != nil {
			return 0, e
		}
		body = bytes.NewReader(raw)
	}
	req, e := http.NewRequestWithContext(ctx, method, m.base+path, body)
	if e != nil {
		return 0, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.config.Token)
	req.Header.Set("X-Catena-Owner", owner)
	r, e := m.config.Client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("%w: Engine unreachable", errEvolutionTemporary)
	}
	defer r.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(r.Body, engineMaxResponseBytes+1))
	if e != nil || len(raw) > engineMaxResponseBytes {
		return r.StatusCode, errors.New("invalid Engine response size")
	}
	if r.StatusCode == 429 || r.StatusCode >= 500 {
		return r.StatusCode, fmt.Errorf("%w: Engine HTTP %d", errEvolutionTemporary, r.StatusCode)
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return r.StatusCode, fmt.Errorf("Engine rejected HTTP %d; interrupted or invalid execution requires review", r.StatusCode)
	}
	if output != nil {
		if e = json.Unmarshal(raw, output); e != nil {
			return r.StatusCode, errors.New("invalid Engine JSON")
		}
	}
	return r.StatusCode, nil
}

// Retained only until the old Experiment callers retire in phase 5.
func (m *EvolutionRuntimeManager) do(ctx context.Context, method, path string, input, output any) (int, error) {
	return m.call(ctx, "", method, path, input, output)
}
func (m *EvolutionRuntimeManager) cancelExecution(owner, id string) {
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_, _ = m.call(ctx, owner, http.MethodDelete, "/v2/executions/"+id, nil, nil)
}
