package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Protocol evidence only: no model, environment or verifier executes here.
func comparisonFixture() json.RawMessage {
	return json.RawMessage(`{"baseline_id":"baseline","compare":[{"case_id":"fixture","baseline":{"id":"base-trial","case_id":"fixture","candidate_id":"baseline","phase":"validation","valid":true,"rewards":{"reward":0}},"candidate":{"id":"candidate-trial","case_id":"fixture","candidate_id":"candidate-version","phase":"validation","valid":true,"rewards":{"reward":1}}}]}`)
}

func reviewContract(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	job := seedQueueJob(t, store)
	job.State = AnalysisJobCompleted
	job.Artifacts = []Artifact{{ID: "artifact", JobID: job.ID, OwnerID: job.OwnerUserID, Type: "rsi_candidate", SchemaVersion: "catena.artifact.v1", EvidenceRefs: analysisJobTraceIDs(job), Payload: json.RawMessage(`{"id":"candidate-version"}`), Provenance: comparisonFixture(), ValidationStatus: "review_required", CreatedAt: time.Now()}}
	if err := store.UpdateAnalysisJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	review := HumanReview{"artifact", "adopt", "Reviewed the verifier and trajectory; limited to this Case.", "local", time.Now().UTC()}
	if _, err := store.ReviewAnalysisJob(ctx, "foreign", job.ID, review); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign decision accepted", err)
	}
	for i := 0; i < 2; i++ {
		saved, err := store.ReviewAnalysisJob(ctx, job.OwnerUserID, job.ID, review)
		if err != nil || len(saved.Reviews) != 1 {
			t.Fatal("review/retry failed", err)
		}
	}
	changed := review
	changed.Decision = "reject"
	if _, err := store.ReviewAnalysisJob(ctx, job.OwnerUserID, job.ID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("decision overwritten", err)
	}
	if err := store.DeleteAnalysisJob(ctx, job.OwnerUserID, job.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("review audit could be deleted", err)
	}
	saved, err := store.GetAnalysisJob(ctx, job.ID)
	if err != nil || saved.Reviews[0].Decision != "adopt" {
		t.Fatal("decision not retained", err)
	}
}

func TestMemoryStoreHumanReview(t *testing.T) { reviewContract(t, NewMemoryStore()) }

func TestPostgresHumanReview(t *testing.T) {
	store, _ := isolatedQueuePostgres(t)
	reviewContract(t, store)
}

func TestReviewRequiresIndependentComparison(t *testing.T) {
	job := AnalysisJob{State: AnalysisJobCompleted, Artifacts: []Artifact{{ID: "artifact", Type: "rsi_candidate", ValidationStatus: "review_required", Payload: json.RawMessage(`{"id":"candidate-version"}`), Provenance: json.RawMessage(`{"decision":"review_required","trials":[{"phase":"probe","valid":true}]}`)}}}
	if err := appendReview(&job, HumanReview{"artifact", "adopt", "LLM said so", "local", time.Now()}); !errors.Is(err, ErrConflict) {
		t.Fatal("probe accepted as independent comparison")
	}
	for _, mutation := range []string{`"valid":false`, `"phase":"probe"`, `"case_id":"foreign"`} {
		raw := strings.Replace(string(comparisonFixture()), `"valid":true`, mutation, 1)
		if hasComparison(json.RawMessage(raw), "candidate-version") {
			t.Fatal("invalid comparison accepted", mutation)
		}
	}
	h := mustHTTPHandler(NewMemoryStore())
	defer h.Close()
	out := httptest.NewRecorder()
	h.ServeHTTP(out, httptest.NewRequest("POST", "/v1/analysis-jobs/unknown/reviews", strings.NewReader(`{"artifact_id":"artifact","decision":"adopt","reason":"check","reviewer":"forged"}`)))
	if out.Code != 400 {
		t.Fatal("caller could supply reviewer identity", out.Code)
	}
}
