package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvolutionJobRunsThreeEvidenceStagesAndIsIdempotent(t *testing.T) {
	store := NewMemoryStore()
	seedTestEvolutionModelConfig(t, store, "local")
	traceID := "11111111111111111111111111111111"
	run := seedEvolutionRun(t, store, "", StateCompleted, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, testEvolutionAuthConfig(), manager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	first := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolve-once",
		map[string]any{"trace_id": traceID, "objective": "Find a replayable boundary."},
	)
	if first.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(first.Body)
		first.Body.Close()
		t.Fatalf("Evolution Job start returned %d: %s", first.StatusCode, body)
	}
	var started EvolutionJob
	if err := json.NewDecoder(first.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()

	retry := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolve-once",
		map[string]any{"trace_id": traceID, "objective": "Find a replayable boundary."},
	)
	if retry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retry.Body)
		retry.Body.Close()
		t.Fatalf("idempotent retry returned %d: %s", retry.StatusCode, body)
	}
	var duplicate EvolutionJob
	if err := json.NewDecoder(retry.Body).Decode(&duplicate); err != nil {
		t.Fatal(err)
	}
	retry.Body.Close()
	if duplicate.ID != started.ID {
		t.Fatalf("idempotent retry created another Job: %s != %s", duplicate.ID, started.ID)
	}

	mutated := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolve-once",
		map[string]any{"trace_id": traceID, "objective": "A different request."},
	)
	if mutated.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(mutated.Body)
		mutated.Body.Close()
		t.Fatalf("mutated idempotent request returned %d: %s", mutated.StatusCode, body)
	}
	mutated.Body.Close()

	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobCompleted)
	if job.Schema != evolutionJobSchema || job.SourceRunID != run.ID ||
		job.SourceTraceID != traceID || job.CurrentStage != "complete" {
		t.Fatalf("unexpected completed Job identity: %#v", job)
	}
	if len(job.Stages) != 3 ||
		job.Stages[0].Role != "inspector-cat" ||
		job.Stages[1].Role != "evolution-cat" ||
		job.Stages[2].Role != "reviewer-cat" {
		t.Fatalf("unexpected stage order: %#v", job.Stages)
	}
	for _, stage := range job.Stages {
		if stage.State != EvolutionStageCompleted || len(stage.RawOutput) == 0 ||
			stage.StartedAt == nil || stage.FinishedAt == nil {
			t.Fatalf("stage evidence is incomplete: %#v", stage)
		}
	}
	if job.Finding == nil || job.CaseProposal != nil || job.Candidate == nil || job.Review == nil {
		t.Fatalf("structured outputs are incomplete: %#v", job)
	}
	if job.Candidate.Kind != EvolutionCandidateSkill ||
		job.Candidate.Status != evolutionCandidateStatus ||
		job.Review.Verdict != "pass" ||
		job.Review.Scope != evolutionReviewScope ||
		job.Review.CandidateStatus != evolutionCandidateStatus {
		t.Fatalf("Job overclaimed Candidate verification: %#v", job)
	}

	response, err := http.Get(server.URL + "/v1/evolution-jobs/" + job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Job GET returned %d: %s", response.StatusCode, body)
	}
	response.Body.Close()
	response, err = http.Get(server.URL + "/v1/evolution-jobs")
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Jobs []EvolutionJob `json:"evolution_jobs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(listed.Jobs) != 1 || listed.Jobs[0].ID != job.ID {
		t.Fatalf("unexpected Job list: %#v", listed.Jobs)
	}
}

func TestTraceEvolutionJobUsesStoredToolEvidenceWithoutSyntheticRun(t *testing.T) {
	store := NewMemoryStore()
	seedTestEvolutionModelConfig(t, store, "local")
	traceID := "00112233445566778899aabbccddeeff"
	startedAt := time.Now().UTC().Add(-time.Second)
	traces := &memoryTraceStore{trace: TraceDetail{
		Summary: TraceSummary{
			TraceID: traceID, RootName: "agent.turn", ServiceName: "arbitrary-agent",
			Model: "model-a", StartTime: startedAt, EndTime: startedAt.Add(time.Second),
			DurationMS: 1000, SpanCount: 1,
		},
		Spans: []TraceSpan{{
			TraceID: traceID, SpanID: "0011223344556677", Name: "tool.read_file",
			ServiceName: "arbitrary-agent", StartTime: startedAt,
			EndTime: startedAt.Add(time.Second), StatusCode: 1, Model: "model-a",
			Attributes: map[string]any{"xiaoba.tool.name": "read_file"},
			Input:      `{"path":"README.md","api_key":"sk_private_value"}`,
			Output:     "file contents",
			Events:     []TraceEvent{{Name: "tool.result", Time: startedAt.Add(time.Second)}},
		}},
	}}
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithServices(store, nil, testEvolutionAuthConfig(), manager, traces)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postEvolutionJob(
		t,
		server.URL+"/v1/traces/"+traceID+"/evolution-jobs",
		"trace-only-evolution",
		map[string]any{"objective": "Find one tool-use boundary."},
	)
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Trace Evolution start returned %d: %s", response.StatusCode, body)
	}
	var started EvolutionJob
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	retry := postEvolutionJob(
		t,
		server.URL+"/v1/traces/"+traceID+"/evolution-jobs",
		"trace-only-evolution",
		map[string]any{"objective": "Find one tool-use boundary."},
	)
	if retry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retry.Body)
		retry.Body.Close()
		t.Fatalf("Trace Evolution retry returned %d: %s", retry.StatusCode, body)
	}
	var duplicate EvolutionJob
	if err := json.NewDecoder(retry.Body).Decode(&duplicate); err != nil {
		t.Fatal(err)
	}
	retry.Body.Close()
	if duplicate.ID != started.ID {
		t.Fatalf("Trace Evolution retry created another Job: %s != %s", duplicate.ID, started.ID)
	}
	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobCompleted)
	if job.SourceKind != EvolutionSourceTrace || job.SourceRunID != "" ||
		job.SourceTraceID != traceID || job.EvidencePack == nil {
		t.Fatalf("Trace-only source was not retained honestly: %#v", job)
	}
	pack := job.EvidencePack
	if pack.Schema != evolutionEvidenceSchema || pack.TraceSummary.RootName != "agent.turn" ||
		len(pack.Spans) != 1 || pack.Spans[0].ToolName != "read_file" ||
		pack.Spans[0].Input == "" || pack.Spans[0].Output != "file contents" ||
		pack.Boundary.TargetAgentExecutedByCatena || pack.Boundary.CreatesRelease ||
		pack.Boundary.ReleaseAuthority != evolutionReleaseAuthority {
		t.Fatalf("stored Trace Evidence Pack is incomplete or misleading: %#v", pack)
	}
	encoded, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sk_private_value") || !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("Trace Evidence Pack did not redact credentials: %s", encoded)
	}
	digestInput, err := evolutionEvidenceDigestInput(*pack)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(digestInput)
	if pack.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("Evidence Pack digest mismatch: %s", pack.SHA256)
	}
	if job.CaseProposal != nil || job.Candidate == nil || job.Candidate.SourceTraceID != traceID ||
		job.Candidate.EvidencePackSHA256 != pack.SHA256 {
		t.Fatalf("candidate provenance is incomplete: %#v", job)
	}
	runs, err := store.ListRuns(context.Background(), 100)
	if err != nil || len(runs) != 0 {
		t.Fatalf("Trace-only Evolution synthesized a Run: runs=%#v err=%v", runs, err)
	}
	releases, err := store.ListReleases(context.Background(), 100)
	if err != nil || len(releases) != 0 {
		t.Fatalf("Evolution proposal created a Release decision: releases=%#v err=%v", releases, err)
	}
}

func TestAgentEvolutionJobFreezesMultipleTracesAndKeepsPluralProvenance(t *testing.T) {
	store := NewMemoryStore()
	seedTestEvolutionModelConfig(t, store, "local")
	now := time.Now().UTC()
	agentID := "agent-codex-runtime"
	serviceName := "catena-runtime-codex"
	traceStore := &agentEvolutionTraceStore{ownerID: "local"}
	for index, traceID := range []string{
		"10112233445566778899aabbccddeeff",
		"20112233445566778899aabbccddeeff",
		"30112233445566778899aabbccddeeff",
	} {
		startedAt := now.Add(time.Duration(-index-1) * time.Hour)
		statusCode := int32(1)
		if index == 1 {
			statusCode = 2
		}
		traceStore.traces = append(traceStore.traces, TraceDetail{
			Summary: TraceSummary{
				AgentID: agentID, TraceID: traceID, RootName: "agent.turn", ServiceName: serviceName,
				StartTime: startedAt, EndTime: startedAt.Add(time.Second),
				DurationMS: 1000, SpanCount: 2,
				ErrorCount: uint64(map[bool]int{true: 1, false: 0}[statusCode == 2]),
			},
			Spans: []TraceSpan{
				{
					AgentID: agentID, TraceID: traceID, SpanID: fmt.Sprintf("%016d", index+1), Name: "agent.turn",
					ServiceName: serviceName, StartTime: startedAt, EndTime: startedAt.Add(time.Second),
					StatusCode: statusCode, Input: fmt.Sprintf(`{"task":"task-%d"}`, index+1),
					Output: "result",
				},
			},
		})
	}
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithServices(store, nil, testEvolutionAuthConfig(), manager, traceStore)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	body := map[string]any{
		"window_start": now.Add(-24 * time.Hour),
		"window_end":   now,
		"objective":    "Find one repeated failure boundary.",
	}
	response := postEvolutionJob(
		t,
		server.URL+"/v1/agents/"+agentID+"/evolution-jobs",
		"agent-window-once",
		body,
	)
	if response.StatusCode != http.StatusAccepted {
		encoded, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Agent Evolution start returned %d: %s", response.StatusCode, encoded)
	}
	var started EvolutionJob
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	retry := postEvolutionJob(
		t,
		server.URL+"/v1/agents/"+agentID+"/evolution-jobs",
		"agent-window-once",
		body,
	)
	if retry.StatusCode != http.StatusOK {
		encoded, _ := io.ReadAll(retry.Body)
		retry.Body.Close()
		t.Fatalf("Agent Evolution retry returned %d: %s", retry.StatusCode, encoded)
	}
	var duplicate EvolutionJob
	if err := json.NewDecoder(retry.Body).Decode(&duplicate); err != nil {
		t.Fatal(err)
	}
	retry.Body.Close()
	if duplicate.ID != started.ID {
		t.Fatalf("idempotent Agent Evolution created another Job: %s != %s", duplicate.ID, started.ID)
	}

	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobCompleted)
	if job.SourceKind != EvolutionSourceAgentTraceSet || job.SourceAgentID != agentID ||
		job.SourceTraceID != "" || len(job.SourceTraceIDs) != 3 || job.EvidencePack == nil {
		t.Fatalf("Agent Trace Set source was not frozen: %#v", job)
	}
	pack := job.EvidencePack
	if pack.Schema != evolutionAgentEvidenceSchema || len(pack.Traces) != 3 ||
		pack.IncludedTraceCount != 3 || pack.TotalTraceCount != 3 ||
		!validEvolutionEvidencePack(*pack, job) {
		t.Fatalf("Agent Trace Set Evidence Pack is invalid: %#v", pack)
	}
	if job.CaseProposal != nil || job.Candidate == nil ||
		job.Candidate.SourceAgentID != agentID || len(job.Candidate.SourceTraceIDs) != 3 {
		t.Fatalf("plural Candidate provenance is incomplete: %#v", job)
	}
}

func TestAgentEvolutionJobUsesOnlyBarenaTargetEvidence(t *testing.T) {
	store := NewMemoryStore()
	seedTestEvolutionModelConfig(t, store, "local")
	now := time.Now().UTC()
	serviceNames := []string{
		"barena-explore-engine",
		"barena-xiaoba-user_simulator",
		"barena-xiaoba-target",
		"barena-xiaoba-target",
		"barena-xiaoba-inspector",
		"barena-xiaoba-reviewer",
	}
	traceStore := &agentEvolutionTraceStore{ownerID: "local"}
	for index, serviceName := range serviceNames {
		traceID := fmt.Sprintf("%032x", index+400)
		startedAt := now.Add(time.Duration(-index-1) * time.Minute)
		traceStore.traces = append(traceStore.traces, TraceDetail{
			Summary: TraceSummary{
				TraceID: traceID, RootName: "xiaoba.session", ServiceName: serviceName,
				StartTime: startedAt, EndTime: startedAt.Add(time.Second), DurationMS: 1000, SpanCount: 1,
			},
			Spans: []TraceSpan{{
				TraceID: traceID, SpanID: fmt.Sprintf("%016x", index+400), Name: "xiaoba.session",
				ServiceName: serviceName, StartTime: startedAt, EndTime: startedAt.Add(time.Second),
				Input: fmt.Sprintf(`{"task":"barena-%d"}`, index), Output: "result",
			}},
		})
	}
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithServices(store, nil, testEvolutionAuthConfig(), manager, traceStore)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postEvolutionJob(
		t,
		server.URL+"/v1/agents/barena-xiaoba-target/evolution-jobs",
		"barena-target-only",
		map[string]any{"window_start": now.Add(-time.Hour), "window_end": now},
	)
	if response.StatusCode != http.StatusAccepted {
		encoded, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("XiaoBaOS Evolution start returned %d: %s", response.StatusCode, encoded)
	}
	var started EvolutionJob
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobCompleted)
	if job.SourceAgentID != "xiaobaos" || len(job.SourceTraceIDs) != 2 || job.EvidencePack == nil ||
		len(job.EvidencePack.Traces) != 2 || job.EvidencePack.IncludedTraceCount != 2 ||
		job.EvidencePack.TotalTraceCount != 2 {
		t.Fatalf("Barena internal evidence entered XiaoBaOS Trace Set: %#v", job)
	}
	for _, trace := range job.EvidencePack.Traces {
		if trace.Summary.ServiceName != "barena-xiaoba-target" {
			t.Fatalf("internal source %q entered XiaoBaOS Evidence Pack", trace.Summary.ServiceName)
		}
	}
	if job.Candidate == nil || job.Candidate.SourceAgentID != "xiaobaos" || job.CaseProposal != nil {
		t.Fatalf("canonical XiaoBaOS provenance was not propagated: %#v", job)
	}
}

func TestAgentEvolutionJobRejectsWindowsWithFewerThanTwoTraces(t *testing.T) {
	now := time.Now().UTC()
	agentID := "codex-desktop"
	oneTrace := TraceDetail{
		Summary: TraceSummary{
			TraceID:     "10112233445566778899aabbccddeeff",
			RootName:    "agent.turn",
			ServiceName: agentID,
			StartTime:   now.Add(-time.Hour),
			EndTime:     now.Add(-time.Hour + time.Second),
			DurationMS:  1000,
			SpanCount:   1,
		},
		Spans: []TraceSpan{{
			TraceID:     "10112233445566778899aabbccddeeff",
			SpanID:      "1011223344556677",
			Name:        "agent.turn",
			ServiceName: agentID,
			StartTime:   now.Add(-time.Hour),
			EndTime:     now.Add(-time.Hour + time.Second),
		}},
	}

	for _, test := range []struct {
		name   string
		traces []TraceDetail
	}{
		{name: "no traces", traces: nil},
		{name: "one trace", traces: []TraceDetail{oneTrace}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewMemoryStore()
			traceStore := &agentEvolutionTraceStore{ownerID: "local", traces: test.traces}
			handler, err := NewHTTPHandlerWithServices(store, nil, AuthConfig{}, nil, traceStore)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()

			response := postEvolutionJob(
				t,
				server.URL+"/v1/agents/"+agentID+"/evolution-jobs",
				"insufficient-agent-traces",
				map[string]any{
					"window_start": now.Add(-24 * time.Hour),
					"window_end":   now,
				},
			)
			defer response.Body.Close()
			if response.StatusCode != http.StatusUnprocessableEntity {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("Agent Evolution with %d Traces returned %d, want 422: %s", len(test.traces), response.StatusCode, body)
			}
			var problem struct {
				Status int    `json:"status"`
				Detail string `json:"detail"`
			}
			if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
				t.Fatal(err)
			}
			if problem.Status != http.StatusUnprocessableEntity ||
				problem.Detail != "At least two Traces are required to evolve an Agent" {
				t.Fatalf("unexpected insufficient-Trace problem: %#v", problem)
			}
			jobs, err := store.ListEvolutionJobs(context.Background(), 100)
			if err != nil {
				t.Fatal(err)
			}
			if len(jobs) != 0 {
				t.Fatalf("insufficient Agent evidence persisted Evolution Jobs: %#v", jobs)
			}
		})
	}
}

func TestEvolutionEvidenceDigestSurvivesJSONBObjectReordering(t *testing.T) {
	base := EvolutionEvidencePack{
		Schema:        evolutionEvidenceSchema,
		SourceKind:    EvolutionSourceRunTrace,
		SourceRunID:   "run-1",
		SourceTraceID: "11111111111111111111111111111111",
		Run: &EvolutionEvidenceRun{
			RunID: "run-1",
			Input: json.RawMessage(`{"z":1,"a":{"y":2,"x":1}}`),
		},
		Spans: []EvolutionEvidenceSpan{},
		RunEvents: []EngineEvent{{
			Payload: json.RawMessage(`{"status":"fail","detail":{"z":2,"a":1}}`),
		}},
		Boundary: EvolutionEvidenceBoundary{
			ReleaseAuthority: evolutionReleaseAuthority,
			CandidateStatus:  evolutionCandidateStatus,
			ReviewScope:      evolutionReviewScope,
		},
	}
	reordered := base
	reordered.Run = &EvolutionEvidenceRun{
		RunID: "run-1",
		Input: json.RawMessage("{\n  \"a\": {\"x\": 1, \"y\": 2},\n  \"z\": 1\n}"),
	}
	reordered.RunEvents = []EngineEvent{{
		Payload: json.RawMessage(`{"detail":{"a":1,"z":2},"status":"fail"}`),
	}}
	first, err := evolutionEvidenceDigestInput(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := evolutionEvidenceDigestInput(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Evidence Pack digest must survive PostgreSQL JSONB formatting")
	}
}

func TestEvolutionJobRejectsNonterminalOrUnretainedEvidence(t *testing.T) {
	store := NewMemoryStore()
	traceID := "22222222222222222222222222222222"
	run := seedEvolutionRun(t, store, "", StateRunning, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, AuthConfig{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	missing := postEvolutionJob(
		t,
		server.URL+"/v1/runs/missing-run/evolution-jobs",
		"missing-run",
		map[string]any{"trace_id": traceID},
	)
	if missing.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(missing.Body)
		missing.Body.Close()
		t.Fatalf("missing Run returned %d: %s", missing.StatusCode, body)
	}
	missing.Body.Close()

	nonterminal := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"nonterminal",
		map[string]any{"trace_id": traceID},
	)
	if nonterminal.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(nonterminal.Body)
		nonterminal.Body.Close()
		t.Fatalf("nonterminal Run returned %d: %s", nonterminal.StatusCode, body)
	}
	nonterminal.Body.Close()

	run.State = StateFailed
	run.UpdatedAt = time.Now().UTC()
	if err := store.UpdateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	unretained := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"unretained",
		map[string]any{"trace_id": "33333333333333333333333333333333"},
	)
	if unretained.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(unretained.Body)
		unretained.Body.Close()
		t.Fatalf("unretained Trace returned %d: %s", unretained.StatusCode, body)
	}
	unretained.Body.Close()
}

func TestEvolutionJobPlatformTenantIsolation(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	projectA := platformProjectUser("evolution-project-a", now)
	projectB := platformProjectUser("evolution-project-b", now)
	for _, user := range []User{projectA, projectB} {
		if _, err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	seedTestEvolutionModelConfig(t, store, projectA.ID)
	traceID := "44444444444444444444444444444444"
	run := seedEvolutionRun(t, store, projectA.ID, StateCompleted, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, AuthConfig{
		GatewaySecret:         testGatewaySecret,
		APITokenEncryptionKey: testEvolutionEncryptionKey,
	}, manager)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"trace_id": traceID})
	start := signedPlatformRequest(
		t,
		http.MethodPost,
		"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolution-project-a",
		"actor-a",
		body,
	)
	start.Header.Set("Idempotency-Key", "tenant-a-job")
	started := httptest.NewRecorder()
	handler.ServeHTTP(started, start)
	if started.Code != http.StatusAccepted {
		t.Fatalf("project A start returned %d: %s", started.Code, started.Body.String())
	}
	var job EvolutionJob
	if err := json.Unmarshal(started.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	waitEvolutionJobState(t, store, job.ID, EvolutionJobCompleted)

	crossGet := httptest.NewRecorder()
	handler.ServeHTTP(crossGet, signedPlatformRequest(
		t,
		http.MethodGet,
		"/v1/evolution-jobs/"+job.ID,
		"evolution-project-b",
		"actor-b",
		nil,
	))
	if crossGet.Code != http.StatusNotFound {
		t.Fatalf("cross-project GET returned %d: %s", crossGet.Code, crossGet.Body.String())
	}

	crossList := httptest.NewRecorder()
	handler.ServeHTTP(crossList, signedPlatformRequest(
		t,
		http.MethodGet,
		"/v1/evolution-jobs",
		"evolution-project-b",
		"actor-b",
		nil,
	))
	if crossList.Code != http.StatusOK ||
		!strings.Contains(crossList.Body.String(), `"evolution_jobs":[]`) {
		t.Fatalf("cross-project list leaked Jobs: %d %s", crossList.Code, crossList.Body.String())
	}

	crossStart := signedPlatformRequest(
		t,
		http.MethodPost,
		"/v1/runs/"+run.ID+"/evolution-jobs",
		"evolution-project-b",
		"actor-b",
		body,
	)
	crossStart.Header.Set("Idempotency-Key", "tenant-b-cross-job")
	crossStartResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossStartResponse, crossStart)
	if crossStartResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-project start returned %d: %s", crossStartResponse.Code, crossStartResponse.Body.String())
	}
}

func TestEvolutionJobDeleteRequiresTerminalOwnedJob(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	projectA := platformProjectUser("evolution-delete-a", now)
	projectB := platformProjectUser("evolution-delete-b", now)
	for _, user := range []User{projectA, projectB} {
		if _, err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	create := func(id string, state EvolutionJobState) EvolutionJob {
		job := EvolutionJob{
			Schema: evolutionJobSchema, ID: id, OwnerUserID: projectA.ID,
			SourceKind: EvolutionSourceTrace, SourceTraceID: newID("delete-trace"),
			IdempotencyKey: newID("delete-key"), RequestFingerprint: newID("delete-fingerprint"),
			State: state, Stages: cloneEvolutionStages(evolutionJobStages), CreatedAt: now, UpdatedAt: now,
		}
		created, wasCreated, err := store.CreateEvolutionJob(context.Background(), job)
		if err != nil || !wasCreated {
			t.Fatalf("create deletion fixture: created=%v err=%v", wasCreated, err)
		}
		return created
	}
	completed := create(newID("delete-completed"), EvolutionJobCompleted)
	running := create(newID("delete-running"), EvolutionJobRunning)
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret, APITokenEncryptionKey: testEvolutionEncryptionKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	foreign := httptest.NewRecorder()
	handler.ServeHTTP(foreign, signedPlatformRequest(
		t, http.MethodDelete, "/v1/evolution-jobs/"+completed.ID,
		"evolution-delete-b", "actor-b", nil,
	))
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-owner delete returned %d: %s", foreign.Code, foreign.Body.String())
	}

	nonterminal := httptest.NewRecorder()
	handler.ServeHTTP(nonterminal, signedPlatformRequest(
		t, http.MethodDelete, "/v1/evolution-jobs/"+running.ID,
		"evolution-delete-a", "actor-a", nil,
	))
	if nonterminal.Code != http.StatusConflict {
		t.Fatalf("running delete returned %d: %s", nonterminal.Code, nonterminal.Body.String())
	}
	if _, err := store.GetEvolutionJob(context.Background(), running.ID); err != nil {
		t.Fatalf("running Job was removed after conflict: %v", err)
	}

	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, signedPlatformRequest(
		t, http.MethodDelete, "/v1/evolution-jobs/"+completed.ID,
		"evolution-delete-a", "actor-a", nil,
	))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("terminal delete returned %d: %s", deleted.Code, deleted.Body.String())
	}
	if _, err := store.GetEvolutionJob(context.Background(), completed.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted Job is still readable: %v", err)
	}
}

func TestEvolutionJobFailsTerminallyWhenRuntimeStageFails(t *testing.T) {
	store := NewMemoryStore()
	seedTestEvolutionModelConfig(t, store, "local")
	traceID := "55555555555555555555555555555555"
	run := seedEvolutionRun(t, store, "", StateCompleted, traceID)
	manager := newStructuredEvolutionRuntimeManager(t, "evolution-cat")
	handler, err := NewHTTPHandlerWithRuntime(store, nil, testEvolutionAuthConfig(), manager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postEvolutionJob(
		t,
		server.URL+"/v1/runs/"+run.ID+"/evolution-jobs",
		"runtime-failure",
		map[string]any{"trace_id": traceID},
	)
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("Evolution Job start returned %d: %s", response.StatusCode, body)
	}
	var started EvolutionJob
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	job := waitEvolutionJobState(t, store, started.ID, EvolutionJobFailed)
	if job.CurrentStage != "evolution" || job.Error == "" ||
		job.Stages[0].State != EvolutionStageCompleted ||
		job.Stages[1].State != EvolutionStageFailed ||
		job.Stages[2].State != EvolutionStageQueued ||
		job.Candidate != nil || job.Review != nil {
		t.Fatalf("unexpected failed Job: %#v", job)
	}
}

func TestEvolutionJobUsesConservativeEnvelopeForUnstructuredRoleOutput(t *testing.T) {
	raw := json.RawMessage(`{"status":"completed","assistant":{"role":"assistant","content":"plain text only"}}`)
	finding := inspectorOutput(raw)
	candidate := candidateOutput(raw, "codex")
	review := reviewOutput(raw)
	if finding.Severity != "unknown" ||
		candidate.Kind != EvolutionCandidateAgentMD ||
		candidate.Status != evolutionCandidateStatus ||
		review.Verdict != "blocked" ||
		review.Scope != evolutionReviewScope ||
		review.CandidateStatus != evolutionCandidateStatus {
		t.Fatalf(
			"unstructured output must remain explicitly unverified: finding=%#v candidate=%#v review=%#v",
			finding,
			candidate,
			review,
		)
	}
}

func TestTraceFarmAcceptsOnlyPortableAgentMDAndXiaoBaPackages(t *testing.T) {
	agentMD := candidateOutput(json.RawMessage(`{"candidate":{"kind":"agent_md","title":"Operating rules","summary":"Portable instructions.","content":{"root":"agent.md","files":[{"path":"agent.md","content":"# Rules\n\nCheck tool results."}]}}}`), "codex")
	if agentMD.Kind != EvolutionCandidateAgentMD {
		t.Fatalf("valid agent.md asset was rejected: %#v", agentMD)
	}
	skill := candidateOutput(json.RawMessage(`{"candidate":{"kind":"skill","title":"Clarify first","summary":"Portable clarification behavior.","content":{"root":"skills/clarify-first","files":[{"path":"skills/clarify-first/SKILL.md","content":"---\nname: clarify-first\ndescription: Ask for missing constraints before acting.\n---\n\n# Clarify first"},{"path":"skills/clarify-first/scripts/check.sh","content":"#!/bin/sh\necho check"}]}}}`), "codex")
	if skill.Kind != EvolutionCandidateSkill {
		t.Fatalf("valid SKILL.md asset was rejected: %#v", skill)
	}
	role := candidateOutput(json.RawMessage(`{"candidate":{"kind":"role","title":"Evidence reviewer","summary":"Portable review role.","content":{"root":"roles/evidence-reviewer","files":[{"path":"roles/evidence-reviewer/role.json","content":"{\"name\":\"evidence-reviewer\",\"displayName\":\"Evidence Reviewer\",\"description\":\"Review retained evidence.\",\"promptFile\":\"evidence-reviewer.md\",\"inheritBaseTools\":false}"},{"path":"roles/evidence-reviewer/prompts/evidence-reviewer.md","content":"# Evidence reviewer\n\nReview retained evidence."},{"path":"roles/evidence-reviewer/skills/grounding/SKILL.md","content":"---\nname: grounding\ndescription: Check retained evidence.\n---\n\n# Grounding"}]}}}`), "codex")
	if role.Kind != EvolutionCandidateRole {
		t.Fatalf("valid Role package was rejected: %#v", role)
	}

	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"candidate":{"kind":"memory","title":"Remember user","summary":"Wrong evidence path.","content":{"memory":"value"}}}`),
		json.RawMessage(`{"candidate":{"kind":"case","title":"Replay task","summary":"Wrong product output.","content":{"prompt":"task"}}}`),
		json.RawMessage(`{"candidate":{"kind":"harness","title":"Loop guard","summary":"Runtime change.","content":{"change":"limit loop"}}}`),
		json.RawMessage(`{"candidate":{"kind":"skill","title":"Advice only","summary":"Not a file.","content":{"instruction":"Ask first."}}}`),
		json.RawMessage(`{"candidate":{"kind":"role","title":"Unsafe path","summary":"Path traversal.","content":{"root":"roles/unsafe","files":[{"path":"../role.json","content":"{}"}]}}}`),
	} {
		if candidate := candidateOutput(raw, "codex"); candidate.Kind != EvolutionCandidateAgentMD || candidate.Title != "Unclassified EvolutionCat draft" {
			t.Fatalf("non-portable Codex asset was accepted: %#v", candidate)
		}
	}

	xiaobaHarness := candidateOutput(json.RawMessage(`{"candidate":{"kind":"harness","title":"Loop guard","summary":"XiaoBaOS Runtime change.","content":{"target":"xiaobaos","change":"limit loop"}}}`), "xiaobaos")
	if xiaobaHarness.Kind != EvolutionCandidateAgentMD || xiaobaHarness.Title != "Unclassified EvolutionCat draft" {
		t.Fatalf("Trace Farm accepted a fourth asset type: %#v", xiaobaHarness)
	}
}

func TestEvolutionCandidateKindsCoverAllDraftArtifactTypes(t *testing.T) {
	for _, kind := range []EvolutionCandidateKind{
		EvolutionCandidateAgentMD,
		EvolutionCandidateMemory,
		EvolutionCandidateRole,
		EvolutionCandidateSkill,
		EvolutionCandidateHarness,
		EvolutionCandidateCase,
	} {
		if !kind.Valid() {
			t.Fatalf("Evolution candidate kind %q is not queryable", kind)
		}
	}
}

type agentEvolutionTraceStore struct {
	ownerID string
	traces  []TraceDetail
}

func (s *agentEvolutionTraceStore) Ping(context.Context) error { return nil }
func (s *agentEvolutionTraceStore) Close() error               { return nil }
func (s *agentEvolutionTraceStore) InsertSpans(context.Context, string, []TraceSpan) error {
	return nil
}
func (s *agentEvolutionTraceStore) ListTraces(context.Context, string, int) ([]TraceSummary, error) {
	result := make([]TraceSummary, 0, len(s.traces))
	for _, trace := range s.traces {
		result = append(result, trace.Summary)
	}
	return result, nil
}
func (s *agentEvolutionTraceStore) ListAgentTraces(
	_ context.Context,
	ownerID string,
	agentID string,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]TraceSummary, error) {
	if s.ownerID != "" && ownerID != s.ownerID {
		return []TraceSummary{}, nil
	}
	result := make([]TraceSummary, 0, len(s.traces))
	for _, trace := range s.traces {
		if !traceSummaryBelongsToAgent(trace.Summary, agentID) || trace.Summary.EndTime.Before(windowStart) ||
			trace.Summary.StartTime.After(windowEnd) {
			continue
		}
		result = append(result, trace.Summary)
		if limit > 0 && len(result) == limit {
			break
		}
	}
	return result, nil
}
func (s *agentEvolutionTraceStore) GetTrace(_ context.Context, ownerID string, traceID string) (TraceDetail, error) {
	if s.ownerID != "" && ownerID != s.ownerID {
		return TraceDetail{}, ErrNotFound
	}
	for _, trace := range s.traces {
		if trace.Summary.TraceID == traceID {
			return trace, nil
		}
	}
	return TraceDetail{}, ErrNotFound
}
func (s *agentEvolutionTraceStore) ListAgents(context.Context, string, int) ([]AgentSummary, error) {
	return nil, nil
}

func seedEvolutionRun(
	t *testing.T,
	store *MemoryStore,
	ownerUserID string,
	state RunState,
	traceID string,
) Run {
	t.Helper()
	now := time.Now().UTC()
	run := Run{
		ID:          newID("evolution-source"),
		RequestID:   newID("evolution-request"),
		OwnerUserID: ownerUserID,
		Origin:      OriginPlatform,
		Operation:   OperationExplore,
		State:       state,
		Input: json.RawMessage(
			`{"scenario":{"objective":"Create result.txt from incomplete input."}}`,
		),
		Runtime:   json.RawMessage(`{"runtime":"xiaobaos","role":"base"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	kind := "terminal"
	phase := "complete"
	if !state.Terminal() {
		kind = "progress"
		phase = "agent"
	}
	if err := store.AppendEvent(context.Background(), EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   run.ID + ".1",
		RunID:     run.ID,
		Sequence:  1,
		Timestamp: now,
		Operation: run.Operation,
		Kind:      kind,
		Phase:     phase,
		Actor:     "engine",
		TraceID:   traceID,
		Payload:   json.RawMessage(`{"status":"retained","artifact":"result.txt"}`),
	}); err != nil {
		t.Fatal(err)
	}
	return run
}

func postEvolutionJob(
	t *testing.T,
	url string,
	idempotencyKey string,
	payload map[string]any,
) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func waitEvolutionJobState(
	t *testing.T,
	store Store,
	jobID string,
	want EvolutionJobState,
) EvolutionJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := store.GetEvolutionJob(context.Background(), jobID)
		if err == nil && job.State == want {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := store.GetEvolutionJob(context.Background(), jobID)
	t.Fatalf("Job did not reach %s: job=%#v err=%v", want, job, err)
	return EvolutionJob{}
}

func newStructuredEvolutionRuntimeManager(
	t *testing.T,
	failRole string,
) *EvolutionRuntimeManager {
	t.Helper()
	root := t.TempDir()
	worker := filepath.Join(root, "structured-evolution-worker.mjs")
	source := fmt.Sprintf(`
import fs from "node:fs";
const request = JSON.parse(fs.readFileSync(0, "utf8"));
if (request.telemetry !== undefined) process.exit(8);
if (request.operation === "turn" && request.role === %q) process.exit(7);
const content = request.role === "inspector-cat"
  ? JSON.stringify({
      finding: {title: "Ambiguous request was not clarified", summary: "The retained trace reached a terminal response without a clarification turn.", severity: "high", evidence: ["The retained trace contains the terminal response."]},
      case_proposal: {title: "Clarify incomplete input", replay_prompt: "Ask for missing constraints before writing result.txt.", success_criteria: "The Agent asks one clarification question.", verifier: {kind: "artifact_assertions", artifacts: [{path: "result.txt", exists: true}]}}
    })
	  : request.role === "evolution-cat"
	    ? JSON.stringify({candidate: {kind: "skill", title: "Clarify first", summary: "Draft a reusable clarification behavior.", content: {root: "skills/clarify-first", files: [{path: "skills/clarify-first/SKILL.md", content: "---\nname: clarify-first\ndescription: Ask for missing constraints before acting.\n---\n\n# Clarify first\n\nAsk for missing constraints before acting."}]}}})
    : JSON.stringify({review: {verdict: "pass", summary: "The draft is grounded in the retained finding, but remains unverified."}});
console.log(JSON.stringify({
  schema: "barena.xiaoba_evolution_response.v1",
  request_id: request.request_id,
  operation: "turn",
  status: "ok",
  result: {status: "completed", assistant: {role: "assistant", content}, process: {exit_code: 0, signal: null, duration_ms: 1, stdout: "", stderr: ""}, telemetry: {mode: "native"}, native_trace_refs: []}
}));
`, failRole)
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
	return manager
}

const testEvolutionEncryptionKey = "catena-test-evolution-encryption-key-0001"

func TestEvolutionOutputLanguageIsExplicitAcrossEveryRole(t *testing.T) {
	job := EvolutionJob{
		OutputLanguage: "zh-CN",
		SourceAgentID:  "agent-local",
		SourceTraceIDs: []string{"00112233445566778899aabbccddeeff"},
	}
	evidence := json.RawMessage(`{"trace":"retained"}`)
	for name, prompt := range map[string]string{
		"inspector": buildInspectorPrompt(job, evidence),
		"evolution": buildCandidatePrompt(job),
		"reviewer":  buildReviewerPrompt(job, evidence),
	} {
		if !strings.Contains(prompt, "简体中文") {
			t.Fatalf("%s prompt does not preserve the requested Chinese output language: %s", name, prompt)
		}
	}

	job.OutputLanguage = "en"
	if prompt := buildCandidatePrompt(job); !strings.Contains(prompt, "in English") {
		t.Fatalf("English asset prompt does not preserve its output language: %s", prompt)
	}
}

func TestNormalizedEvolutionOutputLanguageDefaultsToChinese(t *testing.T) {
	for input, want := range map[string]string{"": "zh-CN", "zh": "zh-CN", "zh-Hans": "zh-CN", "en-US": "en"} {
		got, err := normalizedEvolutionOutputLanguage(input)
		if err != nil || got != want {
			t.Fatalf("normalizedEvolutionOutputLanguage(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := normalizedEvolutionOutputLanguage("fr"); err == nil {
		t.Fatal("unsupported output language was accepted")
	}
}

func testEvolutionAuthConfig() AuthConfig {
	return AuthConfig{APITokenEncryptionKey: testEvolutionEncryptionKey}
}

func seedTestEvolutionModelConfig(t *testing.T, store *MemoryStore, ownerUserID string) {
	t.Helper()
	if strings.TrimSpace(ownerUserID) == "" {
		ownerUserID = "local"
	}
	encrypted, err := encryptAPIToken(
		"test-model-api-key",
		evolutionModelEnvelopePrefix+ownerUserID,
		testEvolutionEncryptionKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertEvolutionModelConfig(context.Background(), EvolutionModelConfig{
		OwnerUserID:     ownerUserID,
		Provider:        "openai",
		BaseURL:         "https://llm.example.test/v1",
		Model:           "test-model",
		EncryptedAPIKey: encrypted,
		UpdatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
}
