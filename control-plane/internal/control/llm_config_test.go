package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEvolutionModelConfigLifecycleNeverReturnsSecret(t *testing.T) {
	store := NewMemoryStore()
	handler, err := NewHTTPHandlerWithConfig(store, nil, testEvolutionAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := putLLMConfig(t, server.URL, map[string]string{
		"provider": "openai",
		"base_url": "https://dashscope.example.test/compatible-mode/v1",
		"model":    "qwen-test",
		"api_key":  "sk-private-owner-value",
	})
	assertSafeLLMSettings(t, response, true)

	response, err = http.Get(server.URL + "/v1/me/llm-config")
	if err != nil {
		t.Fatal(err)
	}
	assertSafeLLMSettings(t, response, true)

	response = putLLMConfig(t, server.URL, map[string]string{
		"provider": "openai",
		"base_url": "https://dashscope.example.test/compatible-mode/v1",
		"model":    "qwen-test-2",
		"api_key":  "",
	})
	assertSafeLLMSettings(t, response, true)
	credentials, err := (&HTTPServer{store: store, auth: testEvolutionAuthConfig()}).evolutionModelCredentials(context.Background(), "local")
	if err != nil || credentials.APIKey != "sk-private-owner-value" || credentials.Model != "qwen-test-2" {
		t.Fatalf("blank update did not preserve encrypted credential: %#v err=%v", credentials, err)
	}

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/me/llm-config", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete returned %d", response.StatusCode)
	}
	response, err = http.Get(server.URL + "/v1/me/llm-config")
	if err != nil {
		t.Fatal(err)
	}
	assertSafeLLMSettings(t, response, false)
}

func TestEvolutionModelConfigRejectsCredentialBearingBaseURL(t *testing.T) {
	handler, err := NewHTTPHandlerWithConfig(NewMemoryStore(), nil, testEvolutionAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := putLLMConfig(t, server.URL, map[string]string{
		"provider": "openai",
		"base_url": "https://user:secret@example.test/v1?key=secret",
		"model":    "test",
		"api_key":  "secret",
	})
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || strings.Contains(string(body), "user:secret") {
		t.Fatalf("unsafe base URL returned %d: %s", response.StatusCode, body)
	}
}

func TestEvolutionModelConfigIsIsolatedByOwner(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	users := []User{
		{ID: "owner-a", GitHubID: 7101, Login: "owner-a", CreatedAt: now, UpdatedAt: now},
		{ID: "owner-b", GitHubID: 7102, Login: "owner-b", CreatedAt: now, UpdatedAt: now},
	}
	sessions := []string{"session-owner-a", "session-owner-b"}
	for index, user := range users {
		if _, err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateSession(context.Background(), Session{
			TokenHash: sessionTokenHash(sessions[index]), UserID: user.ID,
			ExpiresAt: now.Add(time.Hour), CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	server := &HTTPServer{store: store, auth: testAgentAuthConfig()}
	values := []map[string]string{
		{"provider": "openai", "base_url": "https://owner-a.example/v1", "model": "model-a", "api_key": "secret-a"},
		{"provider": "anthropic", "base_url": "https://owner-b.example/v1", "model": "model-b", "api_key": "secret-b"},
	}
	for index, value := range values {
		body, _ := json.Marshal(value)
		request := httptest.NewRequest(http.MethodPut, "/v1/me/llm-config", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessions[index]})
		recorder := httptest.NewRecorder()
		server.putEvolutionModelConfig(recorder, request)
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), value["api_key"]) {
			t.Fatalf("owner %d save returned %d: %s", index, recorder.Code, recorder.Body.String())
		}
	}
	for index, user := range users {
		request := httptest.NewRequest(http.MethodGet, "/v1/me/llm-config", nil)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessions[index]})
		recorder := httptest.NewRecorder()
		server.getEvolutionModelConfig(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), values[index]["model"]) ||
			strings.Contains(recorder.Body.String(), values[1-index]["model"]) ||
			strings.Contains(recorder.Body.String(), values[index]["api_key"]) {
			t.Fatalf("owner %d read crossed tenant boundary: %d %s", index, recorder.Code, recorder.Body.String())
		}
		credentials, err := server.evolutionModelCredentials(context.Background(), user.ID)
		if err != nil || credentials.Model != values[index]["model"] || credentials.APIKey != values[index]["api_key"] {
			t.Fatalf("owner %d execution credentials crossed tenant boundary: %#v err=%v", index, credentials, err)
		}
	}
}

func putLLMConfig(t *testing.T, baseURL string, value map[string]string) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPut, baseURL+"/v1/me/llm-config", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertSafeLLMSettings(t *testing.T, response *http.Response, configured bool) {
	t.Helper()
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK ||
		strings.Contains(string(body), "sk-private-owner-value") ||
		strings.Contains(string(body), "api_key\"") ||
		strings.Contains(string(body), "encrypted") ||
		strings.Contains(string(body), `"configured":`+map[bool]string{true: "false", false: "true"}[configured]) {
		t.Fatalf("unsafe or unexpected LLM settings response: %d %s", response.StatusCode, body)
	}
}
