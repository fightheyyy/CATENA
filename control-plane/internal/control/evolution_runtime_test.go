package control

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvolutionRuntimeManagerProbesSanitizedFourRoleManifest(t *testing.T) {
	manager := newFakeEvolutionRuntimeManager(t)
	manifest := manager.Probe(context.Background())
	if manifest.Status != "ready" || manifest.RuntimeID != "xiaobaos-evolution" {
		t.Fatalf("unexpected Runtime manifest: %+v", manifest)
	}
	if len(manifest.Roles) != 4 || manifest.Capabilities.TargetRuntimeHosted {
		t.Fatalf("unexpected Runtime boundary: %+v", manifest)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/host/secret") {
		t.Fatalf("host path leaked through Runtime manifest: %s", encoded)
	}
}

func TestEvolutionRuntimeManagerRunsOnlyAllowlistedRoles(t *testing.T) {
	manager := newFakeEvolutionRuntimeManager(t)
	result, err := manager.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
		RequestID: "turn-001",
		RunID:     "run-001",
		Role:      "inspector-cat",
		Prompt:    "Return one Finding and replayable Case.",
		Timeout:   2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"status":"completed"`) ||
		!strings.Contains(string(result), "inspector-cat") {
		t.Fatalf("unexpected role turn result: %s", result)
	}
	if _, err := manager.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
		RequestID: "turn-blocked",
		RunID:     "run-001",
		Role:      "engineer-cat",
		Prompt:    "Write source code.",
		Timeout:   2 * time.Second,
	}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("functional Role should be rejected before Worker execution, got %v", err)
	}
}

func TestEvolutionRuntimeManagerRejectsBlockedRoleTurn(t *testing.T) {
	root := t.TempDir()
	worker := filepath.Join(root, "blocked-evolution-worker.mjs")
	source := `
import fs from "node:fs";
const request = JSON.parse(fs.readFileSync(0, "utf8"));
console.log(JSON.stringify({
  schema: "barena.xiaoba_evolution_response.v1",
  request_id: request.request_id,
  operation: "turn",
  status: "ok",
  result: {
    status: "blocked",
    reason_code: "turn_timeout",
    detail: "XiaoBaOS exceeded the hard deadline."
  }
}));
`
	if err := os.WriteFile(worker, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{
		NodeCommand:   "node",
		WorkerPath:    worker,
		XiaoBaCommand: "fake-xiaoba",
		WorkspaceRoot: filepath.Join(root, "workspaces"),
		ProbeTimeout:  2 * time.Second,
		CacheTTL:      time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.RunRoleTurn(context.Background(), EvolutionRoleTurnInput{
		RequestID: "turn-timeout",
		RunID:     "run-timeout",
		Role:      "inspector-cat",
		Prompt:    "Inspect retained evidence.",
		Timeout:   2 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "turn_timeout") ||
		!strings.Contains(err.Error(), "hard deadline") {
		t.Fatalf("blocked role turn must fail with an auditable reason, got %v", err)
	}
}

func TestRuntimeAPIExposesEmbeddedRuntimeAndExternalTargetBoundary(t *testing.T) {
	manager := newFakeEvolutionRuntimeManager(t)
	handler, err := NewHTTPHandlerWithRuntime(
		NewMemoryStore(),
		nil,
		AuthConfig{},
		manager,
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/runtimes")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK ||
		!strings.Contains(string(body), `"runtime_id":"xiaobaos-evolution"`) ||
		!strings.Contains(string(body), `"target_runtime_hosted":false`) ||
		strings.Contains(string(body), "/host/secret") {
		t.Fatalf("unexpected Runtime API response: %d %s", response.StatusCode, body)
	}

	response, err = http.Get(server.URL + "/v1/system/status")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `"evolution_runtime":"ready"`) ||
		!strings.Contains(string(body), `"run_bundle":"barena.run_bundle.v1"`) ||
		!strings.Contains(string(body), `"evolution_protocol":"barena.xiaoba_evolution_request.v1"`) ||
		strings.Contains(string(body), `"engine_protocol"`) ||
		strings.Contains(string(body), `"run_package"`) {
		t.Fatalf("system status does not include Runtime readiness: %s", body)
	}
}

func TestRuntimeAPIReportsBlockedWhenEvolutionRuntimeIsNotConfigured(t *testing.T) {
	server := httptest.NewServer(NewHTTPHandler(NewMemoryStore(), nil))
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/runtimes")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK ||
		!strings.Contains(string(body), `"reason_code":"not_configured"`) {
		t.Fatalf("unexpected unconfigured Runtime response: %d %s", response.StatusCode, body)
	}
}

func newFakeEvolutionRuntimeManager(t *testing.T) *EvolutionRuntimeManager {
	t.Helper()
	root := t.TempDir()
	worker, err := filepath.Abs("testdata/fake-evolution-worker.mjs")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewEvolutionRuntimeManager(EvolutionRuntimeConfig{
		NodeCommand:   "node",
		WorkerPath:    worker,
		XiaoBaCommand: "fake-xiaoba",
		ProjectRoot:   "/host/secret/project",
		RolesRoot:     "/host/secret/project/roles",
		SkillsRoot:    "/host/secret/project/skills",
		WorkspaceRoot: filepath.Join(root, "workspaces"),
		ProbeTimeout:  2 * time.Second,
		CacheTTL:      time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
