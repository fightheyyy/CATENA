package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostgresPrivatePythonPipelinesPersistArtifacts(t *testing.T) {
	python := os.Getenv("CATENA_ENGINE_TEST_PYTHON")
	if python == "" {
		t.Skip("CATENA_ENGINE_TEST_PYTHON is not configured; cross-process fixture only")
	}
	store, dsn := isolatedQueuePostgres(t)
	if _, err := store.UpsertUser(context.Background(), User{ID: "local", GitHubID: 424242, Login: "fixture", DisplayName: "Fixture", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	seedTestAnalysisModelConfig(t, store, "local")
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, filepath.Join(root, "engine/tests/private_pipeline_fixture.py"))
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(root, "engine"), "CATENA_ENGINE_EXECUTIONS_ROOT="+t.TempDir())
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "engine-stderr-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addresses := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			addresses <- scanner.Text()
		} else {
			addresses <- ""
		}
	}()
	var address string
	select {
	case address = <-addresses:
	case <-time.After(10 * time.Second):
		t.Fatal("fixture startup timed out")
	}
	if !strings.HasPrefix(address, "http://127.0.0.1:") {
		t.Fatalf("fixture startup failed: %s", stderr.Name())
	}
	engine, _ := NewEngineClient(EngineConfig{URL: address, Token: "fixture-private"})
	handler, err := NewHTTPHandler(store, HTTPConfig{Auth: testAnalysisAuthConfig(), Engine: engine, Traces: &analysisTraceFixture{}, Workers: fastWorkerConfig()})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, pipeline := range []string{"rsi"} {
		raw := `{"pipeline":"` + pipeline + `","trace_id":"00112233445566778899aabbccddeeff","options":{"case_id":"fixture"}}`
		req := httptest.NewRequest("POST", "/v1/analysis-jobs", strings.NewReader(raw))
		req.Header.Set("Idempotency-Key", "pipeline-"+pipeline)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != 202 {
			t.Fatal(response.Body.String())
		}
		var job AnalysisJob
		json.Unmarshal(response.Body.Bytes(), &job)
		ids = append(ids, job.ID)
		eventually(t, func() bool {
			saved, _ := store.GetAnalysisJob(context.Background(), job.ID)
			return saved.State.Terminal()
		})
		saved, _ := store.GetAnalysisJob(context.Background(), job.ID)
		if saved.State != AnalysisJobCompleted || len(saved.Artifacts) != 1 {
			t.Fatalf("%s: state=%s error=%s", pipeline, saved.State, saved.Error)
		}
	}
	handler.Close()
	reopened, err := OpenPostgres(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err := reopened.ListArtifactsByOwner(context.Background(), "", "", 20)
	if err != nil || len(rows) != 1 {
		t.Fatalf("restart artifacts: %d %v", len(rows), err)
	}
	foreign, err := reopened.ListArtifactsByOwner(context.Background(), "foreign", "", 20)
	if err != nil || len(foreign) != 0 {
		t.Fatal("foreign artifacts visible")
	}
	for _, id := range ids {
		job, err := reopened.GetAnalysisJob(context.Background(), id)
		if err != nil || job.State != AnalysisJobCompleted {
			t.Fatal("terminal persistence failed", err)
		}
	}
}

func TestV2MigrationPreservesLegacyDocumentAndRetiresOnlyActiveJobs(t *testing.T) {
	store, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	job := seedQueueJob(t, store)
	_, err := store.db.ExecContext(ctx, `UPDATE spiral_evolution_jobs SET document=jsonb_set(document,'{schema}','"spiral.evolution_job.v1"') || '{"legacy_role_output":{"preserved":true}}'::jsonb WHERE job_id=$1;`, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, `DELETE FROM catena_schema_migrations WHERE version=2`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = store.migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var doc []byte
	var state string
	if err = store.db.QueryRowContext(ctx, `SELECT state,document FROM spiral_evolution_jobs WHERE job_id=$1`, job.ID).Scan(&state, &doc); err != nil {
		t.Fatal(err)
	}
	if state != "failed" || !bytes.Contains(doc, []byte("legacy_role_output")) {
		t.Fatal("legacy data lost or work replayed")
	}
	var retiredTables int
	if err = store.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('barena_runs','barena_engine_events','catena_experiments')`).Scan(&retiredTables); err != nil || retiredTables != 0 {
		t.Fatal("fresh V2 creates retired tables", err)
	}
}

func TestRebirthMigrationRetiresOnlyActiveMemoryAndPreservesHistory(t *testing.T) {
	store, _ := isolatedQueuePostgres(t)
	ctx := context.Background()
	var retired int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('catena_memory_tasks','barena_agent_profiles')`).Scan(&retired); err != nil || retired != 0 {
		t.Fatal("fresh install creates retired tables", err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE catena_memory_tasks (task_id TEXT PRIMARY KEY, document JSONB); INSERT INTO catena_memory_tasks VALUES ('historical','{"preserved":true}')`); err != nil {
		t.Fatal(err)
	}
	active, terminal, rsi := seedQueueJob(t, store), seedQueueJob(t, store), seedQueueJob(t, store)
	for _, job := range []AnalysisJob{active, terminal} {
		job.Pipeline = "memory"
		job.Checkpoint = json.RawMessage(`{"provider_task_id":"preserve-this"}`)
		if job.ID == terminal.ID {
			job.State = AnalysisJobCompleted
		}
		if err := store.UpdateAnalysisJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM catena_schema_migrations WHERE version=3`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := store.migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var history []byte
	if err := store.db.QueryRowContext(ctx, `SELECT document FROM catena_memory_tasks WHERE task_id='historical'`).Scan(&history); err != nil || !bytes.Contains(history, []byte("preserved")) {
		t.Fatal("historical table lost", err)
	}
	for _, job := range []AnalysisJob{active, terminal, rsi} {
		saved, err := store.GetAnalysisJob(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		expected := job.State
		if job.ID == active.ID {
			expected = AnalysisJobFailed
		}
		if job.ID == terminal.ID {
			expected = AnalysisJobCompleted
		}
		if saved.State != expected {
			t.Fatalf("%s: %s", job.ID, saved.State)
		}
		if job.ID != rsi.ID && !bytes.Contains(saved.Checkpoint, []byte("preserve-this")) {
			t.Fatal("provider checkpoint lost")
		}
	}
}
