package control

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type HTTPServer struct {
	store            Store
	runner           *RunnerManager
	evolutionRuntime *EvolutionRuntimeManager
	auth             AuthConfig
}

//go:embed web/*
var webAssets embed.FS

func NewHTTPHandler(store Store, runner *RunnerManager) http.Handler {
	handler, err := NewHTTPHandlerWithConfig(store, runner, AuthConfig{})
	if err != nil {
		panic(err)
	}
	return handler
}

func NewHTTPHandlerWithConfig(
	store Store,
	runner *RunnerManager,
	auth AuthConfig,
) (http.Handler, error) {
	return NewHTTPHandlerWithRuntime(store, runner, auth, nil)
}

func NewHTTPHandlerWithRuntime(
	store Store,
	runner *RunnerManager,
	auth AuthConfig,
	evolutionRuntime *EvolutionRuntimeManager,
) (http.Handler, error) {
	if err := auth.Validate(); err != nil {
		return nil, err
	}
	server := &HTTPServer{
		store:            store,
		runner:           runner,
		evolutionRuntime: evolutionRuntime,
		auth:             auth.normalized(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /v1/system/status", server.systemStatus)
	mux.HandleFunc("GET /v1/auth/session", server.authSession)
	mux.HandleFunc("GET /v1/auth/github", server.githubLogin)
	mux.HandleFunc("GET /v1/auth/github/callback", server.githubCallback)
	mux.HandleFunc("POST /v1/auth/logout", server.logout)
	mux.HandleFunc("GET /v1/me/api-tokens", server.listAPITokens)
	mux.HandleFunc("POST /v1/me/api-tokens", server.createAPIToken)
	mux.HandleFunc("DELETE /v1/me/api-tokens/{token_id}", server.deleteAPIToken)
	mux.HandleFunc("GET /v1/me/profile", server.myAgentProfile)
	mux.HandleFunc("PUT /v1/me/profile", server.updateMyAgentProfile)
	mux.HandleFunc("GET /v1/community/profiles", server.communityProfiles)
	mux.HandleFunc("GET /v1/community/profiles/{slug}", server.communityProfile)
	mux.HandleFunc("GET /v1/runtimes", server.runtimes)
	mux.HandleFunc("POST /v1/runs", server.createRun)
	mux.HandleFunc("GET /v1/runs", server.listRuns)
	mux.HandleFunc("GET /v1/runs/{run_id}", server.getRun)
	mux.HandleFunc("POST /v1/runs/{run_id}/cancel", server.cancelRun)
	mux.HandleFunc("GET /v1/runs/{run_id}/events", server.runEvents)
	mux.HandleFunc("POST /v1/runs/{run_id}/evolution-jobs", server.createEvolutionJob)
	mux.HandleFunc("GET /v1/evolution-jobs", server.listEvolutionJobs)
	mux.HandleFunc("GET /v1/evolution-jobs/{job_id}", server.getEvolutionJob)
	mux.HandleFunc("POST /v1/runs/{run_id}/issues", server.createIssue)
	mux.HandleFunc("GET /v1/issues", server.listIssues)
	mux.HandleFunc("GET /v1/issues/{issue_id}", server.getIssue)
	mux.HandleFunc("POST /v1/issues/{issue_id}/promote", server.promoteIssue)
	mux.HandleFunc("GET /v1/cases", server.listCases)
	mux.HandleFunc("GET /v1/cases/{case_id}", server.getCase)
	mux.HandleFunc("POST /v1/cases/{case_id}/replay", server.replayCase)
	mux.HandleFunc("GET /v1/evaluations", server.listEvaluations)
	mux.HandleFunc("GET /v1/evaluations/{evaluation_id}", server.getEvaluation)
	mux.HandleFunc("GET /v1/releases", server.listReleases)
	mux.HandleFunc("GET /v1/releases/{release_id}", server.getRelease)
	mux.HandleFunc("POST /v1/platform/scenario-runs/adopt", server.adoptScenarioRun)
	mux.HandleFunc("POST /v1/ingest/runs", server.createEdgeRun)
	mux.HandleFunc("POST /v1/ingest/runs/{run_id}/events", server.appendEdgeRunEvent)
	mux.HandleFunc("POST /v1/ingest/runs/{run_id}/finish", server.finishEdgeRun)
	webRoot, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(webRoot)))
	return requestMiddleware(mux), nil
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
	evolutionRuntime := s.evolutionRuntime.Probe(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ready",
		"auth_mode":         map[bool]string{true: "github", false: "local"}[s.auth.Enabled()],
		"engine_protocol":   "barena.engine_request.v1",
		"event_protocol":    "barena.engine_event.v1",
		"run_package":       "barena.run_package.v1",
		"edge_ingest":       "available",
		"evolution_runtime": evolutionRuntime.Status,
	})
}

func (s *HTTPServer) runtimes(w http.ResponseWriter, r *http.Request) {
	runtime := s.evolutionRuntime.Probe(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"runtimes":              []EvolutionRuntimeManifest{runtime},
		"target_runtime_hosted": false,
	})
}

func (s *HTTPServer) createRun(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var request CreateRunRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerUserID := ""
	if user != nil {
		ownerUserID = user.ID
	}
	run, err := s.runner.StartOwned(r.Context(), request, ownerUserID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *HTTPServer) listRuns(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1000 {
			writeProblem(w, http.StatusBadRequest, "limit must be from 1 to 1000")
			return
		}
		limit = value
	}
	var runs []Run
	var err error
	if user == nil {
		runs, err = s.store.ListRuns(r.Context(), limit)
	} else {
		runs, err = s.store.ListRunsByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if runs == nil {
		runs = []Run{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *HTTPServer) getRun(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	run, err := s.store.GetRun(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if user != nil && run.OwnerUserID != user.ID {
		writeProblem(w, http.StatusNotFound, "Run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *HTTPServer) cancelRun(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	existing, err := s.store.GetRun(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if user != nil && existing.OwnerUserID != user.ID {
		writeProblem(w, http.StatusNotFound, "Run not found")
		return
	}
	run, err := s.runner.Cancel(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *HTTPServer) runEvents(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	runID := r.PathValue("run_id")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if user != nil && run.OwnerUserID != user.ID {
		writeProblem(w, http.StatusNotFound, "Run not found")
		return
	}
	after, err := lastSequence(r.Header.Get("Last-Event-ID"), runID)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, listErr := s.store.ListEventsAfter(r.Context(), runID, after, 1000)
		if listErr != nil {
			return
		}
		for _, event := range events {
			bytes, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "id: %s\nevent: engine\ndata: %s\n\n", event.EventID, bytes)
			after = event.Sequence
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		run, err = s.store.GetRun(r.Context(), runID)
		if err != nil {
			return
		}
		if run.State.Terminal() && len(events) == 0 {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func lastSequence(lastEventID, runID string) (int64, error) {
	if lastEventID == "" {
		return 0, nil
	}
	prefix := runID + "."
	if !strings.HasPrefix(lastEventID, prefix) {
		return 0, errors.New("Last-Event-ID does not belong to this Run")
	}
	value, err := strconv.ParseInt(strings.TrimPrefix(lastEventID, prefix), 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("Last-Event-ID is invalid")
	}
	return value, nil
}

func requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; connect-src 'self'; img-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'",
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
	writeJSON(w, status, map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": bounded(detail, 1000),
	})
}

func statusFor(err error) int {
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
