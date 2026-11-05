package control

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"
)

type HumanReview struct {
	ArtifactID string    `json:"artifact_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason"`
	Reviewer   string    `json:"reviewer"`
	CreatedAt  time.Time `json:"created_at"`
}

type evaluationTrial struct {
	ID          string             `json:"id"`
	CaseID      string             `json:"case_id"`
	CandidateID string             `json:"candidate_id"`
	Phase       string             `json:"phase"`
	Valid       bool               `json:"valid"`
	Rewards     map[string]float64 `json:"rewards"`
}

// A model's selection or a probe alone cannot authorize adoption.
func hasComparison(raw json.RawMessage, candidateID string) bool {
	var result struct {
		BaselineID string `json:"baseline_id"`
		Compare    []struct {
			CaseID    string          `json:"case_id"`
			Baseline  evaluationTrial `json:"baseline"`
			Candidate evaluationTrial `json:"candidate"`
		} `json:"compare"`
	}
	if json.Unmarshal(raw, &result) != nil || result.BaselineID == "" || result.BaselineID == candidateID || len(result.Compare) == 0 {
		return false
	}
	for _, pair := range result.Compare {
		if pair.CaseID == "" || pair.Baseline.CandidateID != result.BaselineID || pair.Candidate.CandidateID != candidateID || pair.Baseline.ID == pair.Candidate.ID {
			return false
		}
		for _, trial := range []evaluationTrial{pair.Baseline, pair.Candidate} {
			if trial.ID == "" || trial.CaseID != pair.CaseID || !trial.Valid || trial.Phase != "validation" || len(trial.Rewards) == 0 {
				return false
			}
			for _, reward := range trial.Rewards {
				if math.IsNaN(reward) || math.IsInf(reward, 0) {
					return false
				}
			}
		}
	}
	return true
}

func appendReview(job *AnalysisJob, review HumanReview) error {
	if job.State != AnalysisJobCompleted || review.Reviewer == "" || review.CreatedAt.IsZero() ||
		(review.Decision != "adopt" && review.Decision != "reject") || strings.TrimSpace(review.Reason) == "" || len(review.Reason) > 4000 {
		return ErrConflict
	}
	for _, previous := range job.Reviews {
		if previous.ArtifactID == review.ArtifactID {
			if previous.Decision == review.Decision && previous.Reason == review.Reason && previous.Reviewer == review.Reviewer {
				return nil
			}
			return ErrConflict
		}
	}
	for _, artifact := range job.Artifacts {
		if artifact.ID != review.ArtifactID || artifact.Type != "rsi_candidate" || artifact.ValidationStatus != "review_required" {
			continue
		}
		var candidate struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(artifact.Payload, &candidate) != nil || !hasComparison(artifact.Provenance, candidate.ID) {
			return ErrConflict
		}
		job.Reviews = append(job.Reviews, review)
		return nil
	}
	return ErrConflict
}

func (s *HTTPServer) reviewAnalysisJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	owner, reviewer := "", "local"
	if user != nil {
		owner, reviewer = user.ID, user.ID
	}
	var input struct {
		ArtifactID string `json:"artifact_id"`
		Decision   string `json:"decision"`
		Reason     string `json:"reason"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if input.ArtifactID == "" || (input.Decision != "adopt" && input.Decision != "reject") || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 4000 {
		writeProblem(w, 400, "artifact, adopt/reject and a reason of 1–4000 bytes are required")
		return
	}
	job, err := s.store.ReviewAnalysisJob(r.Context(), owner, r.PathValue("job_id"), HumanReview{input.ArtifactID, input.Decision, sanitizeAnalysisText(input.Reason, 4000), reviewer, time.Now().UTC()})
	if err != nil {
		writeProblem(w, statusFor(err), "Review requires an owned, completed candidate with independent Harbor comparison; decisions are immutable")
		return
	}
	writeJSON(w, 200, job)
}

func (s *MemoryStore) ReviewAnalysisJob(_ context.Context, owner, id string, review HumanReview) (AnalysisJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.analysisJobs[id]
	if !ok || job.OwnerUserID != owner {
		return AnalysisJob{}, ErrNotFound
	}
	if err := appendReview(&job, review); err != nil {
		return AnalysisJob{}, err
	}
	s.analysisJobs[id] = cloneAnalysisJob(job)
	return cloneAnalysisJob(job), nil
}

func (s *PostgresStore) ReviewAnalysisJob(ctx context.Context, owner, id string, review HumanReview) (AnalysisJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AnalysisJob{}, err
	}
	defer tx.Rollback()
	job, err := scanAnalysisJob(tx.QueryRowContext(ctx, `SELECT COALESCE(owner_user_id,''),idempotency_key,request_fingerprint,document FROM spiral_evolution_jobs WHERE job_id=$1 AND COALESCE(owner_user_id,'')=$2 FOR UPDATE`, id, owner))
	if err != nil {
		return AnalysisJob{}, err
	}
	if err = appendReview(&job, review); err != nil {
		return AnalysisJob{}, err
	}
	reviews, err := json.Marshal(job.Reviews)
	if err != nil {
		return AnalysisJob{}, err
	}
	// Only append the human decision; preserve the worker's terminal document and fencing token.
	if _, err = tx.ExecContext(ctx, `UPDATE spiral_evolution_jobs SET document=jsonb_set(document,'{reviews}',$2::jsonb) WHERE job_id=$1`, id, reviews); err != nil {
		return AnalysisJob{}, err
	}
	return job, tx.Commit()
}
