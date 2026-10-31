package control

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestArtifactWritesUseExistingFencedJobAndOwner(t *testing.T) {
	s := NewMemoryStore()
	j := seedQueueJob(t, s)
	j, l, err := s.ClaimEvolutionJob(context.Background(), "artifact-worker", time.Minute, 3)
	if err != nil {
		t.Fatal(err)
	}
	a := Artifact{ID: "artifact-1", OwnerID: j.OwnerUserID, JobID: j.ID, Type: "rsi_candidate", SchemaVersion: "catena.artifact.v1", EvidenceRefs: []string{j.SourceTraceID}, Payload: json.RawMessage(`{"files":[]}`), Provenance: json.RawMessage(`{"adapter":"harness2"}`), ValidationStatus: "unverified", CreatedAt: time.Now().UTC()}
	j.Artifacts = []Artifact{a}
	if err = s.SaveLeasedEvolutionJob(context.Background(), l, j); err != nil {
		t.Fatal(err)
	}
	j.Artifacts[0].OwnerID = "foreign"
	if err = s.SaveLeasedEvolutionJob(context.Background(), l, j); err == nil {
		t.Fatal("foreign artifact accepted")
	}
	rows, err := s.ListArtifactsByOwner(context.Background(), "foreign", "", 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("cross-owner artifacts visible", err)
	}
	rows, err = s.ListArtifactsByOwner(context.Background(), "", "rsi_candidate", 100)
	if err != nil || len(rows) != 1 {
		t.Fatal("artifact did not survive job round trip", err)
	}
}
