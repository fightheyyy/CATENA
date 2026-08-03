package main

import "testing"

func TestWorkerEndpoint(t *testing.T) {
	t.Parallel()
	for marker, expected := range map[string]string{
		"engine-worker.js":            "/v1/engine/run",
		"evolution-runtime-worker.js": "/v1/evolution/run",
	} {
		marker, expected := marker, expected
		t.Run(marker, func(t *testing.T) {
			t.Parallel()
			actual, err := workerEndpoint(marker)
			if err != nil || actual != expected {
				t.Fatalf("workerEndpoint(%q) = %q, %v", marker, actual, err)
			}
		})
	}
	if _, err := workerEndpoint("other.js"); err == nil {
		t.Fatal("unsupported marker was accepted")
	}
}

func TestRunnerURL(t *testing.T) {
	t.Parallel()
	if _, err := runnerURL("http://spiral-runner:8790"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "file:///tmp/runner", "http://user@example.com", "http://example.com?x=1"} {
		if _, err := runnerURL(value); err == nil {
			t.Fatalf("runnerURL(%q) unexpectedly succeeded", value)
		}
	}
}
