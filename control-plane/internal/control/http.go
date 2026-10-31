package control

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

type HTTPServer struct {
	store           Store
	traces          TraceStore
	analysisRuntime *EngineClient
	memory          MemoryBackend
	auth            AuthConfig
	jobs            *analysisWorker
	ingestion       *traceAdmission
}

//go:embed web/*
var webAssets embed.FS

type HTTPConfig struct {
	Auth      AuthConfig
	Engine    *EngineClient
	Traces    TraceStore
	Memory    MemoryBackend
	Workers   AnalysisWorkerConfig
	Ingestion TraceIngestConfig
}

func NewHTTPHandler(store Store, config HTTPConfig) (*ManagedHTTPHandler, error) {
	auth, analysisRuntime, traces, memory, workers, ingestion := config.Auth, config.Engine, config.Traces, config.Memory, config.Workers, config.Ingestion
	if workers.Workers == 0 {
		workers = DefaultAnalysisWorkerConfig()
	}
	if ingestion.Concurrency == 0 {
		ingestion = DefaultTraceIngestConfig()
	}

	if err := auth.Validate(); err != nil {
		return nil, err
	}
	if err := ingestion.validate(); err != nil {
		return nil, err
	}
	server := &HTTPServer{
		store:           store,
		traces:          traces,
		analysisRuntime: analysisRuntime,
		memory:          memory,
		auth:            auth.normalized(),
		ingestion:       &traceAdmission{config: ingestion, owners: make(map[string]int)},
	}
	if analysisRuntime != nil {
		pool, err := newAnalysisWorker(store, workers, server.executeAnalysisJob)
		if err != nil {
			return nil, err
		}
		server.jobs = pool
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/analysis-jobs", server.createAnalysisJob)
	mux.HandleFunc("GET /v1/analysis-jobs", server.listAnalysisJobs)
	mux.HandleFunc("GET /v1/analysis-jobs/queue", server.analysisQueueStatus)
	mux.HandleFunc("GET /v1/analysis-jobs/{job_id}", server.getAnalysisJob)
	mux.HandleFunc("POST /v1/analysis-jobs/{job_id}/cancel", server.cancelAnalysisJob)
	mux.HandleFunc("DELETE /v1/analysis-jobs/{job_id}", server.deleteAnalysisJob)
	mux.HandleFunc("GET /v1/artifacts", server.listArtifacts)
	mux.HandleFunc("GET /v1/evaluation-cases", server.evaluationCases)
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /v1/system/status", server.systemStatus)
	mux.HandleFunc("GET /v1/me/llm-config", server.getAnalysisModelConfig)
	mux.HandleFunc("PUT /v1/me/llm-config", server.putAnalysisModelConfig)
	mux.HandleFunc("DELETE /v1/me/llm-config", server.deleteAnalysisModelConfig)
	mux.HandleFunc("GET /v1/auth/session", server.authSession)
	mux.HandleFunc("GET /v1/auth/github", server.githubLogin)
	mux.HandleFunc("GET /v1/auth/github/callback", server.githubCallback)
	// Keep the callback already registered by existing Catena GitHub OAuth Apps
	// while the public Web and API share this server.
	mux.HandleFunc("GET /api/auth/callback/github", server.githubCallback)
	mux.HandleFunc("POST /v1/auth/logout", server.logout)
	mux.HandleFunc("GET /v1/me/api-tokens", server.listAPITokens)
	mux.HandleFunc("POST /v1/me/api-tokens", server.createAPIToken)
	mux.HandleFunc("POST /v1/me/api-tokens/{token_id}/reveal", server.revealAPIToken)
	mux.HandleFunc("DELETE /v1/me/api-tokens/{token_id}", server.deleteAPIToken)
	mux.HandleFunc("GET /v1/me/profile", server.myAgentProfile)
	mux.HandleFunc("PUT /v1/me/profile", server.updateMyAgentProfile)
	mux.HandleFunc("GET /v1/community/profiles", server.communityProfiles)
	mux.HandleFunc("GET /v1/community/profiles/{slug}", server.communityProfile)
	mux.HandleFunc("GET /v1/runtimes", server.runtimes)
	mux.HandleFunc("GET /v1/agents", server.listAgents)
	mux.HandleFunc("POST /v1/agents", server.createRegisteredAgent)
	mux.HandleFunc("GET /v1/agents/{agent_id}", server.getRegisteredAgent)
	mux.HandleFunc("POST /v1/agents/{agent_id}/api-key", server.createAgentConnectionKey)
	mux.HandleFunc("GET /v1/agents/{agent_id}/traces", server.listAgentTraces)
	mux.HandleFunc("GET /v1/traces", server.listTraces)
	mux.HandleFunc("GET /v1/traces/{trace_id}", server.getTrace)
	mux.HandleFunc("GET /v1/memories/status", server.memoryStatus)
	mux.HandleFunc("GET /v1/memories/tasks", server.listMemoryTasks)
	mux.HandleFunc("GET /v1/memories/tasks/{task_id}", server.memoryTaskStatus)
	mux.HandleFunc("GET /v1/memories", server.listMemories)
	mux.HandleFunc("POST /v1/memories", server.createMemoryNote)
	mux.HandleFunc("POST /v1/memories/search", server.searchMemories)
	mux.HandleFunc("GET /v1/memories/facts/{fact_id}/graph", server.memoryFactGraph)
	mux.HandleFunc("POST /v1/otlp/v1/traces", server.ingestOTLPTraces)
	mux.HandleFunc("GET /v1/conversations", server.listConversations)
	mux.HandleFunc("GET /v1/conversations/{conversation_id}", server.getConversation)
	apiNotFound := func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, http.StatusNotFound, "API endpoint not found")
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /v1/", apiNotFound)
	}
	webRoot, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", newSPAHandler(webRoot))
	return &ManagedHTTPHandler{Handler: requestMiddleware(mux), workers: server.jobs}, nil
}

type spaHandler struct {
	root   fs.FS
	assets http.Handler
}

func newSPAHandler(root fs.FS) http.Handler {
	return &spaHandler{
		root:   root,
		assets: http.FileServer(http.FS(root)),
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requested := strings.TrimPrefix(r.URL.Path, "/")
	if requested == "" {
		requested = "index.html"
	}
	if fs.ValidPath(requested) {
		if info, err := fs.Stat(h.root, requested); err == nil && !info.IsDir() {
			h.assets.ServeHTTP(w, r)
			return
		}
	}
	index, err := fs.ReadFile(h.root, "index.html")
	if err != nil {
		http.Error(w, "Catena Web is unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}

func (s *HTTPServer) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *HTTPServer) systemStatus(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	analysisRuntime := s.analysisRuntime.Probe(r.Context())
	traceStore := "unavailable"
	if s.traces != nil {
		if err := s.traces.Ping(r.Context()); err == nil {
			traceStore = "available"
		}
	}
	memoryStore := "unavailable"
	if s.memory != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.memory.Ping(ctx); err == nil {
			memoryStore = "available"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ready",
		"auth_mode":         map[bool]string{true: "github", false: "local"}[s.auth.Enabled()],
		"edge_ingest":       "available",
		"analysis_protocol": engineTurnRequestSchema,
		"analysis_engine":   analysisRuntime.Status,
		"trace_store":       traceStore,
		"memory_store":      memoryStore,
	})
}

func (s *HTTPServer) runtimes(w http.ResponseWriter, r *http.Request) {
	runtime := s.analysisRuntime.Probe(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"runtimes":              []EngineManifest{runtime},
		"target_runtime_hosted": false,
	})
}

func requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; connect-src 'self'; img-src 'self' https://avatars.githubusercontent.com; style-src 'self' 'unsafe-inline'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'",
		)
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, status, map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": bounded(detail, 1000),
	})
}

func statusFor(err error) int {
	if errors.Is(err, ErrQueueFull) {
		return http.StatusTooManyRequests
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
