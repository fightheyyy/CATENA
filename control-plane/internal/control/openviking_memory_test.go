package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenVikingSearchScopesAndFiltersOwner(t *testing.T) {
	owner := "alice"
	uri := openVikingRoot(owner) + "/preferences/editor.md"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-OpenViking-User") != memoryProjectID(owner) || r.Header.Get("X-OpenViking-Account") != "catena" || r.Header.Get("X-OpenViking-Role") != "user" || r.Header.Get("X-API-Key") != "service-secret" {
			t.Error("missing trusted owner identity")
		}
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input["target_uri"] != openVikingRoot(owner) || input["limit"] != float64(8) {
			t.Errorf("unscoped search: %v", input)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "result": map[string]any{"memories": []map[string]any{
			{"uri": uri, "content": "prefers a quiet editor", "score": 0.9},
			{"uri": openVikingRoot("bob") + "/profile.md", "content": "private", "score": 1},
		}}})
	}))
	defer server.Close()
	client, err := NewOpenVikingMemoryClient(server.URL, "service-secret")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Search(context.Background(), owner, MemorySearchRequest{Query: "editor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 || result.Facts[0].Metadata["uri"] != uri || strings.Contains(fmt.Sprint(result), "private") {
		t.Fatalf("cross-owner result: %+v", result)
	}
	if _, err := client.Task(context.Background(), "bob", memoryProjectID(owner)+"_ov_task-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign task was not rejected: %v", err)
	}
}

func TestOpenVikingTraceCommitTracksExtractionAndRedacts(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/messages/batch") {
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if strings.Contains(fmt.Sprint(input), "sk_abcdefghijk") {
				t.Error("trace secret forwarded")
			}
			if !strings.Contains(fmt.Sprint(input), "historical agent execution evidence") {
				t.Error("trace was not identified as evidence")
			}
		}
		result := map[string]any{}
		if strings.HasSuffix(r.URL.Path, "/commit") {
			result["task_id"] = "commit-1"
		}
		if strings.Contains(r.URL.Path, "/tasks/") {
			result["status"] = "completed"
			result["created_at"] = 1791042715.45
			result["created_at_iso"] = "2026-10-03T15:51:55Z"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "result": result})
	}))
	defer server.Close()
	client, _ := NewOpenVikingMemoryClient(server.URL, "key")
	receipt, err := client.IngestTrace(context.Background(), "alice", TraceDetail{Summary: TraceSummary{TraceID: "aabb"}, Spans: []TraceSpan{{Input: "api_key=sk_abcdefghijk"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 || receipt.Indexed || receipt.Status != "pending" || receipt.TraceID != "aabb" {
		t.Fatalf("invalid receipt: %+v %v", receipt, paths)
	}
	status, err := client.Task(context.Background(), "alice", receipt.TaskID)
	if err != nil || status.Status != "completed" || status.Progress != 1 || status.CreatedAt != "2026-10-03T15:51:55Z" {
		t.Fatalf("task state: %+v %v", status, err)
	}
}

func TestOpenVikingStableFileGraphRejectsEscapingLinks(t *testing.T) {
	owner := "alice"
	uri := openVikingRoot(owner) + "/notes/a.md"
	target := openVikingRoot(owner) + "/notes/b.md"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result any
		switch r.URL.Path {
		case "/api/v1/fs/ls":
			result = []map[string]any{{"uri": uri, "isDir": false}, {"uri": openVikingRoot("bob") + "/notes/private.md", "isDir": false}, {"uri": openVikingRoot(owner) + "/.abstract.md", "isDir": false}}
		case "/api/v1/content/read":
			result = "[Other](b.md) [External](https://example.com) [Private](viking://user/" + memoryProjectID("bob") + "/memories/profile.md)"
			if r.URL.Query().Get("raw") == "true" {
				fields, _ := json.Marshal(map[string]any{"links": []map[string]any{{"from_uri": uri, "to_uri": target, "link_type": "related_to"}, {"from_uri": uri, "to_uri": openVikingRoot("bob") + "/profile.md", "link_type": "related_to"}}})
				result = fmt.Sprint(result) + "\n\n<!-- MEMORY_FIELDS\n" + string(fields) + "\n-->"
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "result": result})
	}))
	defer server.Close()
	client, _ := NewOpenVikingMemoryClient(server.URL, "key")
	list, err := client.List(context.Background(), owner, 30)
	if err != nil || len(list.Memories) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	graph, err := client.Graph(context.Background(), owner, openVikingID(uri))
	if err != nil {
		t.Fatal(err)
	}
	if graph.TotalRelations != 2 || graph.Relations[1].Target != target || graph.Relations[1].Type != "related_to" || graph.Relations[1].Origin != "openviking_link" {
		t.Fatalf("graph %+v", graph)
	}
	if openVikingOwnedURI(owner, openVikingRoot(owner)+"/../bob.md") || openVikingOwnedURI(owner, openVikingRoot(owner)+"/%2e%2e/bob.md") {
		t.Error("path escape accepted")
	}
}
