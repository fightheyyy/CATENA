package control

import (
	"bytes"
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

func TestGatewaySecretValidation(t *testing.T) {
	if err := (AuthConfig{GatewaySecret: "too-short"}).Validate(); err == nil {
		t.Fatal("short gateway secret should be rejected")
	}
}

func TestAPITokenEncryptionConfigurationValidation(t *testing.T) {
	if err := (AuthConfig{APITokenEncryptionKey: "too-short"}).Validate(); err == nil {
		t.Fatal("short API token encryption key should be rejected")
	}
	if err := (AuthConfig{
		GitHubClientID:     "client",
		GitHubClientSecret: "secret",
		RedirectURL:        "https://catena.example/callback",
	}).Validate(); err == nil {
		t.Fatal("GitHub authentication without token encryption should fail closed")
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
