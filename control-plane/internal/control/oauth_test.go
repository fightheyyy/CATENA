package control

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGitHubLoginCanonicalizesHostBeforeIssuingFlowCookies(t *testing.T) {
	server := &HTTPServer{auth: AuthConfig{
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		RedirectURL:        "http://localhost:5670/api/auth/callback/github",
	}.normalized()}

	alternateRequest := httptest.NewRequest(
		http.MethodGet,
		"http://127.0.0.1:5670/v1/auth/github",
		nil,
	)
	alternateResponse := httptest.NewRecorder()
	server.githubLogin(alternateResponse, alternateRequest)

	if alternateResponse.Code != http.StatusFound ||
		alternateResponse.Header().Get("Location") != "http://localhost:5670/v1/auth/github" {
		t.Fatalf("unexpected canonical redirect: status=%d location=%q", alternateResponse.Code, alternateResponse.Header().Get("Location"))
	}
	if len(alternateResponse.Result().Cookies()) != 0 {
		t.Fatal("OAuth flow cookies were issued on the non-canonical host")
	}

	canonicalRequest := httptest.NewRequest(
		http.MethodGet,
		"http://localhost:5670/v1/auth/github",
		nil,
	)
	canonicalResponse := httptest.NewRecorder()
	server.githubLogin(canonicalResponse, canonicalRequest)
	authorizeURL, err := url.Parse(canonicalResponse.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if authorizeURL.Host != "github.com" || authorizeURL.Query().Get("state") == "" ||
		authorizeURL.Query().Get("redirect_uri") != server.auth.RedirectURL {
		t.Fatalf("unexpected GitHub authorization redirect: %s", authorizeURL)
	}
	if len(canonicalResponse.Result().Cookies()) != 2 {
		t.Fatalf("canonical login issued %d flow cookies", len(canonicalResponse.Result().Cookies()))
	}
}

func TestGitHubCallbackRejectsStaleStateWithSafeRecovery(t *testing.T) {
	server := &HTTPServer{auth: AuthConfig{
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		RedirectURL:        "http://localhost:5670/api/auth/callback/github",
	}.normalized()}
	request := httptest.NewRequest(
		http.MethodGet,
		"http://localhost:5670/api/auth/callback/github?code=old-code&state=stale-state",
		nil,
	)
	response := httptest.NewRecorder()

	server.githubCallback(response, request)

	if response.Code != http.StatusSeeOther ||
		response.Header().Get("Location") != "http://localhost:5670/?auth_error=state" {
		t.Fatalf("unexpected recovery response: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if strings.Contains(response.Body.String(), "OAuth state") {
		t.Fatalf("raw OAuth problem leaked to the browser: %s", response.Body.String())
	}
}
