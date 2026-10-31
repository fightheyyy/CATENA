package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (s *HTTPServer) listEvolutionJobs(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := evolutionListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var jobs []EvolutionJob
	if user == nil {
		jobs, err = s.store.ListEvolutionJobs(r.Context(), limit)
	} else {
		jobs, err = s.store.ListEvolutionJobsByOwner(r.Context(), user.ID, limit)
	}
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if jobs == nil {
		jobs = []EvolutionJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"evolution_jobs": jobs})
}

func (s *HTTPServer) getEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetEvolutionJob(r.Context(), r.PathValue("job_id"))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	if !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Evolution Job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) deleteEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetEvolutionJob(r.Context(), r.PathValue("job_id"))
	if err != nil || !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Evolution Job not found")
		return
	}
	if !job.State.Terminal() {
		writeProblem(w, http.StatusConflict, "Evolution Job can only be deleted after it finishes")
		return
	}
	if err := s.store.DeleteEvolutionJob(r.Context(), job.OwnerUserID, job.ID); err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) saveEvolutionProgress(ctx context.Context, lease EvolutionLease, job EvolutionJob) error {
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.store.SaveLeasedEvolutionJob(call, lease, job)
	if err == nil || errors.Is(err, ErrLeaseLost) || ctx.Err() != nil {
		return err
	}
	return fmt.Errorf("%w: stage persistence unavailable", errEvolutionTemporary)
}

func (s *HTTPServer) cancelEvolutionJob(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetEvolutionJob(r.Context(), r.PathValue("job_id"))
	if err != nil || !resourceOwnedBy(job.OwnerUserID, user) {
		writeProblem(w, http.StatusNotFound, "Evolution Job not found")
		return
	}
	job, err = s.store.CancelEvolutionJob(r.Context(), job.OwnerUserID, job.ID)
	if err != nil {
		writeProblem(w, statusFor(err), "Evolution Job cancellation failed")
		return
	}
	s.jobs.notify()
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) evolutionQueueStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var owner *string
	if user != nil {
		owner = &user.ID
	}
	stats, err := s.store.EvolutionQueueStats(r.Context(), owner)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Analysis queue status unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": stats, "owner_capacity": evolutionOwnerQueueCapacity})
}
