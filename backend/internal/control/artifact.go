package control

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// Artifact is a shared envelope, not a common payload schema for RSI and Memory.
// It is stored inside the leased Job document, atomically with job progress.
type Artifact struct {
	ID               string          `json:"id"`
	OwnerID          string          `json:"owner_id"`
	JobID            string          `json:"job_id"`
	Type             string          `json:"type"`
	SchemaVersion    string          `json:"schema_version"`
	EvidenceRefs     []string        `json:"evidence_refs"`
	Payload          json.RawMessage `json:"payload"`
	Provenance       json.RawMessage `json:"provenance"`
	ValidationStatus string          `json:"validation_status"`
	CreatedAt        time.Time       `json:"created_at"`
}

func validJobArtifacts(job AnalysisJob) error {
	seen := map[string]bool{}
	for _, a := range job.Artifacts {
		if a.ID == "" || seen[a.ID] || a.OwnerID != job.OwnerUserID || a.JobID != job.ID ||
			(a.Type != "rsi_candidate" && a.Type != "engineering_memory") || a.SchemaVersion != "catena.artifact.v1" ||
			len(a.EvidenceRefs) == 0 || !equalStringSlices(a.EvidenceRefs, analysisJobTraceIDs(job)) || !json.Valid(a.Payload) || !json.Valid(a.Provenance) || a.CreatedAt.IsZero() {
			return errors.New("invalid artifact identity, source or payload")
		}
		switch a.ValidationStatus {
		case "unverified", "review_required", "extracted", "confirmed":
		default:
			return errors.New("invalid artifact validation status")
		}
		seen[a.ID] = true
	}
	return nil
}

// No second artifact database: the existing fenced Job document owns writes.
func (s *MemoryStore) ListArtifactsByOwner(ctx context.Context, owner, kind string, limit int) ([]Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Artifact{}
	for _, job := range s.analysisJobs {
		if job.OwnerUserID != owner {
			continue
		}
		for _, a := range job.Artifacts {
			if kind != "" && a.Type != kind {
				continue
			}
			a.OwnerID = owner
			a.Payload = cloneJSON(a.Payload)
			a.Provenance = cloneJSON(a.Provenance)
			a.EvidenceRefs = append([]string(nil), a.EvidenceRefs...)
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *PostgresStore) ListArtifactsByOwner(ctx context.Context, owner, kind string, limit int) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a FROM spiral_evolution_jobs j CROSS JOIN LATERAL jsonb_array_elements(COALESCE(j.document->'artifacts','[]'::jsonb)) a WHERE COALESCE(j.owner_user_id,'')=$1 AND ($2='' OR a->>'type'=$2) ORDER BY a->>'created_at' DESC LIMIT $3`, owner, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var a Artifact
		if err = json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		a.OwnerID = owner
		out = append(out, a)
	}
	return out, rows.Err()
}
