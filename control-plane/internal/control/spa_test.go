package control

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandlerServesIndexForProductRoute(t *testing.T) {
	root, err := fs.Sub(webAssets, "web")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/evolution", nil)

	newSPAHandler(root).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", contentType)
	}
	if !strings.Contains(recorder.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("response did not contain the React root")
	}
}

func TestSPAHandlerServesKnownAsset(t *testing.T) {
	root, err := fs.Sub(webAssets, "web")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/catena-mark.svg", nil)

	newSPAHandler(root).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "<svg") {
		t.Fatalf("response did not contain the requested SVG asset")
	}
}

func TestRequestMiddlewareAllowsGitHubAvatarOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	handler := requestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(recorder, request)

	policy := recorder.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "img-src 'self' https://avatars.githubusercontent.com") {
		t.Fatalf("Content-Security-Policy does not allow GitHub avatars: %q", policy)
	}
}
