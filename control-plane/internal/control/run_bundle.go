package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	runBundleSchema          = "barena.run_bundle.v1"
	maxRunBundleTerminalFact = 12 * 1024
)

type RunBundleRun struct {
	RunID     string          `json:"run_id"`
	Operation Operation       `json:"operation"`
	State     RunState        `json:"state"`
	Input     json.RawMessage `json:"input"`
	Runtime   json.RawMessage `json:"runtime,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type CreateRunBundleRequest struct {
	Schema             string        `json:"schema"`
	Run                RunBundleRun  `json:"run"`
	Events             []EngineEvent `json:"events"`
	TerminalFactSHA256 string        `json:"terminal_fact_sha256"`
}

type RunBundle struct {
	Schema             string        `json:"schema"`
	ID                 string        `json:"run_bundle_id"`
	OwnerUserID        string        `json:"-"`
	IdempotencyKey     string        `json:"-"`
	RequestFingerprint string        `json:"-"`
	Run                Run           `json:"run"`
	Events             []EngineEvent `json:"events"`
	TerminalFactSHA256 string        `json:"terminal_fact_sha256"`
	TerminalFactSchema string        `json:"terminal_fact_schema,omitempty"`
	TraceIDs           []string      `json:"trace_ids"`
	CreatedAt          time.Time     `json:"created_at"`
}

func (s *HTTPServer) createRunBundle(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireIngestUser(w, r)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if err := validateReplayIdempotencyKey(idempotencyKey); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var request CreateRunBundleRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	projectID := strings.TrimSpace(r.Header.Get(gatewayProjectHeader))
	if projectID != "" {
		boundInput, err := attachRunBundleProjectContext(request.Run.Input, projectID)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Run Bundle input could not retain Platform project context")
			return
		}
		request.Run.Input = boundInput
	}
	bundle, err := newRunBundle(request, user.ID, idempotencyKey, time.Now().UTC())
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, created, err := s.store.CreateRunBundle(r.Context(), bundle)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, stored)
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (s *HTTPServer) getRunBundle(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	bundle, err := s.store.GetRunBundle(r.Context(), r.PathValue("bundle_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(bundle.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Run Bundle not found")
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func newRunBundle(
	request CreateRunBundleRequest,
	ownerUserID string,
	idempotencyKey string,
	createdAt time.Time,
) (RunBundle, error) {
	request.Schema = strings.TrimSpace(request.Schema)
	request.Run.RunID = strings.TrimSpace(request.Run.RunID)
	request.Run.Error = strings.TrimSpace(request.Run.Error)
	request.TerminalFactSHA256 = strings.ToLower(strings.TrimSpace(request.TerminalFactSHA256))
	if request.Schema != runBundleSchema {
		return RunBundle{}, errors.New("unsupported Run Bundle schema")
	}
	if !validRunBundleID(request.Run.RunID) {
		return RunBundle{}, errors.New("run.run_id must contain 1 to 256 safe characters")
	}
	if !request.Run.Operation.Valid() || !request.Run.State.Terminal() {
		return RunBundle{}, errors.New("Run Bundle requires a valid operation and terminal state")
	}
	if !jsonObject(request.Run.Input) ||
		(len(request.Run.Runtime) > 0 && !jsonObject(request.Run.Runtime)) {
		return RunBundle{}, errors.New("run.input and optional run.runtime must be JSON objects")
	}
	if request.Run.CreatedAt.IsZero() || request.Run.UpdatedAt.IsZero() ||
		request.Run.UpdatedAt.Before(request.Run.CreatedAt) {
		return RunBundle{}, errors.New("Run Bundle timestamps are invalid")
	}
	if len(request.Run.Error) > 2000 ||
		(request.Run.State == StateCompleted && request.Run.Error != "") {
		return RunBundle{}, errors.New("completed Run Bundle cannot contain an error")
	}
	if len(request.Events) == 0 || len(request.Events) > 1000 {
		return RunBundle{}, errors.New("Run Bundle must contain from 1 to 1000 ordered Events")
	}
	run := Run{
		ID:              request.Run.RunID,
		RequestID:       runBundleRequestID(ownerUserID, idempotencyKey),
		OwnerUserID:     ownerUserID,
		Origin:          OriginEdge,
		Operation:       request.Run.Operation,
		State:           request.Run.State,
		Input:           cloneJSON(request.Run.Input),
		Runtime:         cloneJSON(request.Run.Runtime),
		CancelRequested: request.Run.State == StateCancelled,
		Error:           request.Run.Error,
		CreatedAt:       request.Run.CreatedAt.UTC(),
		UpdatedAt:       request.Run.UpdatedAt.UTC(),
	}
	events := append([]EngineEvent(nil), request.Events...)
	for index := range events {
		events[index].Timestamp = events[index].Timestamp.UTC()
		if events[index].TraceID != "" {
			normalized, err := normalizedEvolutionTraceID(events[index].TraceID)
			if err != nil {
				return RunBundle{}, fmt.Errorf("Run Bundle Event %d has an invalid trace_id", index+1)
			}
			events[index].TraceID = normalized
		}
		if events[index].Sequence != int64(index+1) || events[index].Validate(run) != nil ||
			!jsonObject(events[index].Payload) {
			return RunBundle{}, fmt.Errorf("Run Bundle Event %d is invalid or out of order", index+1)
		}
		if index < len(events)-1 && events[index].Kind == "terminal" {
			return RunBundle{}, errors.New("only the final Run Bundle Event may be terminal")
		}
	}
	terminal := events[len(events)-1]
	if terminal.Kind != "terminal" {
		return RunBundle{}, errors.New("Run Bundle requires a final terminal Event")
	}
	if len(terminal.Payload) > maxRunBundleTerminalFact {
		return RunBundle{}, errors.New("Run Bundle terminal fact exceeds 12 KiB")
	}
	digest, err := runBundleTerminalDigest(terminal.Payload)
	if err != nil {
		return RunBundle{}, errors.New("Run Bundle terminal fact is not valid JSON")
	}
	if len(request.TerminalFactSHA256) != sha256.Size*2 ||
		request.TerminalFactSHA256 != hex.EncodeToString(digest[:]) {
		return RunBundle{}, errors.New("terminal_fact_sha256 does not match the final Event payload")
	}
	run.CurrentPhase = terminal.Phase
	run.CurrentActor = terminal.Actor
	fingerprint, err := runBundleFingerprint(request)
	if err != nil {
		return RunBundle{}, err
	}
	traceIDs, err := runBundleTraceIDs(request.Run.Input, events)
	if err != nil {
		return RunBundle{}, err
	}
	if len(traceIDs) == 0 {
		return RunBundle{}, errors.New("Run Bundle requires at least one retained Trace identity")
	}
	var terminalFact struct {
		Schema string `json:"schema"`
	}
	_ = json.Unmarshal(terminal.Payload, &terminalFact)
	return RunBundle{
		Schema:             runBundleSchema,
		ID:                 runBundleID(ownerUserID, idempotencyKey),
		OwnerUserID:        ownerUserID,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		Run:                run,
		Events:             events,
		TerminalFactSHA256: request.TerminalFactSHA256,
		TerminalFactSchema: bounded(strings.TrimSpace(terminalFact.Schema), 200),
		TraceIDs:           traceIDs,
		CreatedAt:          createdAt.UTC(),
	}, nil
}

func runBundleTraceIDs(input json.RawMessage, events []EngineEvent) ([]string, error) {
	candidates := advertisedRunTraceIDs(input)
	for _, event := range events {
		if event.TraceID != "" {
			candidates = append(candidates, event.TraceID)
		}
	}
	traceIDs := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		traceID, err := normalizedEvolutionTraceID(candidate)
		if err != nil {
			return nil, errors.New("Run Bundle declares an invalid Trace identity")
		}
		if !seen[traceID] {
			seen[traceID] = true
			traceIDs = append(traceIDs, traceID)
		}
	}
	return traceIDs, nil
}

func advertisedRunTraceIDs(input json.RawMessage) []string {
	var object map[string]any
	if json.Unmarshal(input, &object) != nil || object == nil {
		return nil
	}
	result := make([]string, 0)
	appendString := func(value any) {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}
	appendStrings := func(value any) {
		values, ok := value.([]any)
		if !ok {
			return
		}
		for _, item := range values {
			appendString(item)
		}
	}
	appendString(object["primary_trace_id"])
	appendStrings(object["trace_ids"])
	if evidence, ok := object["evidence"].(map[string]any); ok {
		appendString(evidence["primary_trace_id"])
		appendString(evidence["root_trace_id"])
		appendStrings(evidence["trace_ids"])
		appendStrings(evidence["native_trace_ids"])
	}
	return result
}

func runInputHasTrace(input json.RawMessage, traceID string) bool {
	normalized, err := normalizedEvolutionTraceID(traceID)
	if err != nil {
		return false
	}
	for _, candidate := range advertisedRunTraceIDs(input) {
		value, err := normalizedEvolutionTraceID(candidate)
		if err == nil && value == normalized {
			return true
		}
	}
	return false
}

func runBundleFingerprint(request CreateRunBundleRequest) (string, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func runBundleID(ownerUserID string, idempotencyKey string) string {
	digest := sha256.Sum256([]byte("catena:run-bundle:" + ownerUserID + "\x00" + idempotencyKey))
	return "run-bundle-" + hex.EncodeToString(digest[:16])
}

func runBundleRequestID(ownerUserID string, idempotencyKey string) string {
	digest := sha256.Sum256([]byte("catena:run-bundle-request:" + ownerUserID + "\x00" + idempotencyKey))
	return "run-bundle-request-" + hex.EncodeToString(digest[:16])
}

func runBundleAdvisoryLockKey(bundle RunBundle) string {
	return bundle.ID
}

func runBundleTerminalDigest(payload json.RawMessage) ([sha256.Size]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(compact.Bytes()), nil
}

func validRunBundleID(value string) bool {
	if len(value) < 1 || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character == 0x7f || character == '/' || character == '\\' {
			return false
		}
	}
	return true
}

func attachRunBundleProjectContext(input json.RawMessage, projectID string) (json.RawMessage, error) {
	var object map[string]any
	if err := json.Unmarshal(input, &object); err != nil || object == nil {
		return nil, errors.New("input must be a JSON object")
	}
	source, _ := object["source"].(map[string]any)
	if source == nil {
		source = make(map[string]any)
	}
	source["kind"] = "barena_run_bundle"
	source["project_id"] = projectID
	object["source"] = source
	return json.Marshal(object)
}

func cloneRunBundle(input RunBundle) RunBundle {
	ownerUserID := input.OwnerUserID
	idempotencyKey := input.IdempotencyKey
	requestFingerprint := input.RequestFingerprint
	encoded, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	var output RunBundle
	if err := json.Unmarshal(encoded, &output); err != nil {
		panic(err)
	}
	output.OwnerUserID = ownerUserID
	output.IdempotencyKey = idempotencyKey
	output.RequestFingerprint = requestFingerprint
	return output
}

func validateStoredRunBundle(bundle RunBundle) error {
	if bundle.Schema != runBundleSchema || bundle.ID == "" || bundle.OwnerUserID != bundle.Run.OwnerUserID ||
		bundle.IdempotencyKey == "" || bundle.RequestFingerprint == "" || len(bundle.Events) == 0 ||
		bundle.ID != runBundleID(bundle.OwnerUserID, bundle.IdempotencyKey) ||
		bundle.Run.RequestID != runBundleRequestID(bundle.OwnerUserID, bundle.IdempotencyKey) ||
		bundle.Run.Origin != OriginEdge || !bundle.Run.State.Terminal() {
		return ErrConflict
	}
	for index, event := range bundle.Events {
		if event.Sequence != int64(index+1) || event.Validate(bundle.Run) != nil ||
			(index < len(bundle.Events)-1 && event.Kind == "terminal") {
			return ErrConflict
		}
	}
	terminal := bundle.Events[len(bundle.Events)-1]
	digest, err := runBundleTerminalDigest(terminal.Payload)
	if err != nil {
		return ErrConflict
	}
	if terminal.Kind != "terminal" || len(terminal.Payload) > maxRunBundleTerminalFact ||
		bundle.TerminalFactSHA256 != hex.EncodeToString(digest[:]) {
		return ErrConflict
	}
	traceIDs, err := runBundleTraceIDs(bundle.Run.Input, bundle.Events)
	if err != nil || len(traceIDs) == 0 || len(traceIDs) != len(bundle.TraceIDs) {
		return ErrConflict
	}
	for index := range traceIDs {
		if traceIDs[index] != bundle.TraceIDs[index] {
			return ErrConflict
		}
	}
	return nil
}
