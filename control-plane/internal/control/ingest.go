package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (s *HTTPServer) createEdgeRun(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireIngestUser(w, r)
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
	projectID := strings.TrimSpace(r.Header.Get(gatewayProjectHeader))
	if projectID != "" {
		boundInput, err := attachEdgeProjectContext(request.Input, projectID)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "input could not retain Platform project context")
			return
		}
		request.Input = boundInput
	}
	now := time.Now().UTC()
	run := Run{
		ID:           newID("run"),
		RequestID:    newID("req"),
		OwnerUserID:  user.ID,
		Origin:       OriginEdge,
		Operation:    request.Operation,
		State:        StateRunning,
		CurrentPhase: "starting",
		CurrentActor: "runner",
		Input:        request.Input,
		Runtime:      request.Runtime,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateRun(r.Context(), run); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// attachEdgeProjectContext binds evidence to the authenticated Platform project
// instead of trusting a project identifier supplied by an endpoint Runner.
// The metadata follows the Run into Issue -> Case -> Replay so the execution
// plane can export the replay Trace back to the same project.
func attachEdgeProjectContext(input json.RawMessage, projectID string) (json.RawMessage, error) {
	var object map[string]any
	if err := json.Unmarshal(input, &object); err != nil || object == nil {
		return nil, errors.New("input must be a JSON object")
	}
	source, _ := object["source"].(map[string]any)
	if source == nil {
		source = make(map[string]any)
	}
	source["kind"] = "barena_edge_runner"
	source["project_id"] = projectID
	object["source"] = source
	return json.Marshal(object)
}

func (s *HTTPServer) appendEdgeRunEvent(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireIngestUser(w, r)
	if !ok {
		return
	}
	run, ok := s.ownedEdgeRun(w, r, user.ID)
	if !ok {
		return
	}
	if run.State.Terminal() {
		writeProblem(w, http.StatusConflict, "a terminal Run cannot accept Events")
		return
	}
	var event EngineEvent
	if err := decodeJSON(w, r, &event); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := event.Validate(run); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.AppendEvent(r.Context(), event); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	run.CurrentPhase = event.Phase
	run.CurrentActor = event.Actor
	run.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateRun(r.Context(), run); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) finishEdgeRun(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireIngestUser(w, r)
	if !ok {
		return
	}
	run, ok := s.ownedEdgeRun(w, r, user.ID)
	if !ok {
		return
	}
	var request FinishRunRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := request.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if run.State.Terminal() {
		if run.State == request.State && run.Error == request.Error {
			writeJSON(w, http.StatusOK, run)
			return
		}
		writeProblem(w, http.StatusConflict, "Run already finished with a different state")
		return
	}
	if request.State == StateCompleted {
		events, err := s.store.ListEventsAfter(r.Context(), run.ID, 0, 10000)
		if err != nil {
			writeProblem(w, statusFor(err), err.Error())
			return
		}
		if len(events) == 0 || events[len(events)-1].Kind != "terminal" {
			writeProblem(
				w,
				http.StatusConflict,
				"a completed edge Run requires a final terminal Event",
			)
			return
		}
	}
	run.State = request.State
	run.Error = bounded(request.Error, 2000)
	run.CancelRequested = request.State == StateCancelled
	if request.State == StateCompleted {
		run.CurrentPhase = "complete"
		run.CurrentActor = "runner"
	}
	run.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateRun(r.Context(), run); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *HTTPServer) ownedEdgeRun(
	w http.ResponseWriter,
	r *http.Request,
	ownerUserID string,
) (Run, bool) {
	run, err := s.store.GetRun(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return Run{}, false
	}
	if run.OwnerUserID != ownerUserID {
		writeProblem(w, http.StatusNotFound, "Run not found")
		return Run{}, false
	}
	if run.Origin != OriginEdge {
		writeProblem(w, http.StatusConflict, "Run is not owned by an edge Runner")
		return Run{}, false
	}
	if !run.Origin.Valid() {
		writeProblem(w, http.StatusInternalServerError, errors.New("Run origin is invalid").Error())
		return Run{}, false
	}
	return run, true
}
