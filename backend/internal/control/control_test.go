package control

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func decodeTestJSON(
	t *testing.T,
	client *http.Client,
	method string,
	target string,
	body io.Reader,
	destination any,
) {
	t.Helper()
	request, _ := http.NewRequest(method, target, body)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s returned %d: %s", method, target, response.StatusCode, responseBody)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func bearerRequest(
	t *testing.T,
	method string,
	target string,
	token string,
	body io.Reader,
) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
