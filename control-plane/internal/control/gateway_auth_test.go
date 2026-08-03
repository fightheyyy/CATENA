package control

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testGatewaySecret = "test-only-barena-gateway-secret-32-bytes"

func TestPlatformGatewayProjectIsolation(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	projectA := platformProjectUser("project-a", now)
	projectB := platformProjectUser("project-b", now)
	for _, user := range []User{projectA, projectB} {
		if _, err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	for _, run := range []Run{
		{
			ID: "run-a", RequestID: "request-a", OwnerUserID: projectA.ID,
			Origin: OriginPlatform, Operation: OperationExplore, State: StateCompleted,
			Input: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "run-b", RequestID: "request-b", OwnerUserID: projectB.ID,
			Origin: OriginPlatform, Operation: OperationExplore, State: StateCompleted,
			Input: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.CreateRun(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedPlatformRequest(
		t,
		http.MethodGet,
		"/v1/runs?limit=100",
		"project-a",
		"actor-a",
		nil,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("signed project list returned %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Runs []Run `json:"runs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Runs) != 1 || payload.Runs[0].ID != "run-a" {
		t.Fatalf("project A received cross-project records: %#v", payload.Runs)
	}

	tampered := signedPlatformRequest(
		t,
		http.MethodGet,
		"/v1/runs?limit=100",
		"project-a",
		"actor-a",
		nil,
	)
	tampered.Header.Set(gatewayProjectHeader, "project-b")
	tamperedResponse := httptest.NewRecorder()
	handler.ServeHTTP(tamperedResponse, tampered)
	if tamperedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("tampered project context returned %d", tamperedResponse.Code)
	}
	var problem map[string]any
	if err := json.Unmarshal(tamperedResponse.Body.Bytes(), &problem); err != nil {
		t.Fatalf("tampered response must contain exactly one JSON problem: %v", err)
	}
	if problem["detail"] != "invalid Barena platform signature" {
		t.Fatalf("unexpected tampered response: %#v", problem)
	}
}

func TestPlatformGatewayEmptyCollectionsAreArrays(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path string
		key  string
	}{
		{path: "/v1/runs?limit=100", key: "runs"},
		{path: "/v1/evolution-jobs?limit=100", key: "evolution_jobs"},
		{path: "/v1/issues?limit=100", key: "issues"},
		{path: "/v1/cases?limit=100", key: "cases"},
		{path: "/v1/evaluations?limit=100", key: "evaluations"},
		{path: "/v1/releases?limit=100", key: "releases"},
	} {
		t.Run(test.key, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, signedPlatformRequest(
				t,
				http.MethodGet,
				test.path,
				"fresh-project",
				"fresh-actor",
				nil,
			))
			if response.Code != http.StatusOK {
				t.Fatalf("empty %s list returned %d: %s", test.key, response.Code, response.Body.String())
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if string(payload[test.key]) != "[]" {
				t.Fatalf("empty %s must encode as [], got %s", test.key, payload[test.key])
			}
		})
	}
}

func TestPlatformGatewayVerificationRestoresBody(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	request := signedPlatformRequest(
		t,
		http.MethodPost,
		"/v1/example",
		"project-a",
		"actor-a",
		body,
	)
	if err := verifyPlatformGatewayRequest(
		request,
		testGatewaySecret,
		"project-a",
		"actor-a",
	); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.NewDecoder(request.Body).Decode(&decoded); err != nil {
		t.Fatalf("gateway verification consumed the request body: %v", err)
	}
	if decoded["hello"] != "world" {
		t.Fatalf("unexpected restored body: %#v", decoded)
	}
}

func TestPlatformGatewayOwnsCompleteEdgeIngestLifecycle(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, AuthConfig{
		GatewaySecret: testGatewaySecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	createBody := []byte(`{"operation":"explore","input":{"scenario":{"scenario_id":"project-key"}}}`)
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, signedPlatformRequest(
		t,
		http.MethodPost,
		"/v1/ingest/runs",
		"project-edge",
		"project-api-key",
		createBody,
	))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("signed edge create returned %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var run Run
	if err := json.Unmarshal(createResponse.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	storedCreated, err := store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Origin != OriginEdge || storedCreated.OwnerUserID != platformProjectUser("project-edge", time.Now().UTC()).ID {
		t.Fatalf("unexpected signed edge Run: %#v", run)
	}
	var retainedInput struct {
		Source struct {
			Kind      string `json:"kind"`
			ProjectID string `json:"project_id"`
		} `json:"source"`
	}
	if err := json.Unmarshal(storedCreated.Input, &retainedInput); err != nil {
		t.Fatal(err)
	}
	if retainedInput.Source.Kind != "barena_edge_runner" || retainedInput.Source.ProjectID != "project-edge" {
		t.Fatalf("signed project context was not retained: %#v", retainedInput.Source)
	}

	event, err := json.Marshal(EngineEvent{
		Schema:    "barena.engine_event.v1",
		EventID:   run.ID + ".1",
		RunID:     run.ID,
		Sequence:  1,
		Timestamp: time.Now().UTC(),
		Operation: OperationExplore,
		Kind:      "terminal",
		Phase:     "complete",
		Actor:     "runner",
		TraceID:   "project-edge-trace",
		Payload:   json.RawMessage(`{"status":"complete"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	eventPath := "/v1/ingest/runs/" + run.ID + "/events"
	eventResponse := httptest.NewRecorder()
	handler.ServeHTTP(eventResponse, signedPlatformRequest(
		t,
		http.MethodPost,
		eventPath,
		"project-edge",
		"project-api-key",
		event,
	))
	if eventResponse.Code != http.StatusNoContent {
		t.Fatalf("signed edge Event returned %d: %s", eventResponse.Code, eventResponse.Body.String())
	}

	foreignResponse := httptest.NewRecorder()
	handler.ServeHTTP(foreignResponse, signedPlatformRequest(
		t,
		http.MethodPost,
		eventPath,
		"other-project",
		"project-api-key",
		event,
	))
	if foreignResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-project edge Event returned %d", foreignResponse.Code)
	}

	finishBody := []byte(`{"state":"completed"}`)
	finishPath := "/v1/ingest/runs/" + run.ID + "/finish"
	finishResponse := httptest.NewRecorder()
	handler.ServeHTTP(finishResponse, signedPlatformRequest(
		t,
		http.MethodPost,
		finishPath,
		"project-edge",
		"project-api-key",
		finishBody,
	))
	if finishResponse.Code != http.StatusOK {
		t.Fatalf("signed edge finish returned %d: %s", finishResponse.Code, finishResponse.Body.String())
	}
	stored, err := store.GetRun(context.Background(), run.ID)
	if err != nil || stored.State != StateCompleted {
		t.Fatalf("signed edge Run was not completed: %#v err=%v", stored, err)
	}
}

func TestGatewaySecretValidation(t *testing.T) {
	if err := (AuthConfig{GatewaySecret: "too-short"}).Validate(); err == nil {
		t.Fatal("short gateway secret should be rejected")
	}
}

func signedPlatformRequest(
	t *testing.T,
	method string,
	requestURI string,
	projectID string,
	actorID string,
	body []byte,
) *http.Request {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(digest[:])
	canonical := strings.Join([]string{
		method,
		requestURI,
		projectID,
		actorID,
		timestamp,
		bodyHash,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(testGatewaySecret))
	_, _ = mac.Write([]byte(canonical))
	request := httptest.NewRequest(method, requestURI, bytes.NewReader(body))
	request.Header.Set(gatewayProjectHeader, projectID)
	request.Header.Set(gatewayActorHeader, actorID)
	request.Header.Set(gatewayTimestampHeader, timestamp)
	request.Header.Set(gatewayBodyHashHeader, bodyHash)
	request.Header.Set(gatewaySignatureHeader, hex.EncodeToString(mac.Sum(nil)))
	return request
}
