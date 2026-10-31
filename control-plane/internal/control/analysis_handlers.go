package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (s *HTTPServer) listAnalysisJobs(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := analysisListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var jobs []AnalysisJob
	if user == nil {
		jobs, err = s.store.ListAnalysisJobs(r.Context(), limit)
	} else {
		jobs, err = s.store.ListAnalysisJobsByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if jobs == nil {
		jobs = []AnalysisJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"analysis_jobs": jobs})
}

func (s *HTTPServer) getAnalysisJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetAnalysisJob(r.Context(), r.PathValue("job_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Analysis Job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) deleteAnalysisJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetAnalysisJob(r.Context(), r.PathValue("job_id"))
	if err != nil || !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Analysis Job not found")
		return
	}
	if !job.State.Terminal() {
		writeProblem(w, http.StatusConflict, "Analysis Job can only be deleted after it finishes")
		return
	}
	if err := s.store.DeleteAnalysisJob(r.Context(), job.OwnerUserID, job.ID); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) saveAnalysisProgress(ctx context.Context, lease AnalysisLease, job AnalysisJob) error {
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.store.SaveLeasedAnalysisJob(call, lease, job)
	if err == nil || errors.Is(err, ErrLeaseLost) || ctx.Err() != nil {
		return err
	}
	return fmt.Errorf("%w: stage persistence unavailable", errAnalysisTemporary)
}

func (s *HTTPServer) cancelAnalysisJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetAnalysisJob(r.Context(), r.PathValue("job_id"))
	if err != nil || !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Analysis Job not found")
		return
	}
	job, err = s.store.CancelAnalysisJob(r.Context(), job.OwnerUserID, job.ID)
	if err != nil {
		writeProblem(w, statusFor(err), "Analysis Job cancellation failed")
		return
	}
	s.jobs.notify()
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) analysisQueueStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var owner *string
	if user != nil {
		owner = &user.ID
	}
	stats, err := s.store.AnalysisQueueStats(r.Context(), owner)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Analysis queue status unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": stats, "owner_capacity": analysisOwnerQueueCapacity})
}
