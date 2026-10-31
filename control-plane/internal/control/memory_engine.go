package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// EngineMemoryClient sends only Go-authenticated owner context. Provider credentials live in Engine.
type EngineMemoryClient struct{ runtime *EngineClient }

func NewEngineMemoryClient(runtime *EngineClient) *EngineMemoryClient {
	return &EngineMemoryClient{runtime}
}
func (c *EngineMemoryClient) call(ctx context.Context, owner, operation string, payload, result any) error {
	_, err := c.runtime.call(ctx, owner, http.MethodPost, "/v2/memory/"+operation, payload, result)
	return err
}
func (c *EngineMemoryClient) Ping(ctx context.Context) error {
	return c.call(ctx, "local", "status", map[string]any{}, nil)
}
func (c *EngineMemoryClient) MemoryProvider() (string, []string) {
	return "openviking", []string{"semantic", "filesystem", "session_extraction", "file_relations"}
}

func (c *EngineMemoryClient) Task(ctx context.Context, owner, id string) (out MemoryTaskStatus, err error) {
	err = c.call(ctx, owner, "task", map[string]any{"task_id": id}, &out)
	return
}
func (c *EngineMemoryClient) List(ctx context.Context, owner string, limit int) (out MemoryList, err error) {
	err = c.call(ctx, owner, "list", map[string]any{"limit": limit}, &out)
	return
}
func (c *EngineMemoryClient) Search(ctx context.Context, owner string, req MemorySearchRequest) (out MemoryRecallBundle, err error) {
	err = c.call(ctx, owner, "search", req, &out)
	return
}
func (c *EngineMemoryClient) Graph(ctx context.Context, owner string, id int64) (out MemoryFactGraph, err error) {
	err = c.call(ctx, owner, "graph", map[string]any{"fact_id": id}, &out)
	return
}
func (c *EngineMemoryClient) CreateNote(ctx context.Context, owner string, req MemoryNoteRequest) (out MemoryRecord, err error) {
	req.Title = redactMemoryText(req.Title)
	req.Content = redactMemoryText(req.Content)
	err = c.call(ctx, owner, "note", req, &out)
	return
}

func (s *HTTPServer) executeMemoryJob(ctx context.Context, lease AnalysisLease, job AnalysisJob) error {
	if job.Adapter != "openviking" {
		return errors.New("unknown Memory adapter")
	}
	owner := job.OwnerUserID
	if owner == "" {
		owner = "local"
	}
	var receipt MemoryIngestReceipt
	if len(job.Checkpoint) > 0 {
		if err := json.Unmarshal(job.Checkpoint, &receipt); err != nil {
			return err
		}
	}
	if receipt.TaskID == "" {
		evidence, _ := json.Marshal(map[string]any{"pack": job.EvidencePack, "objective": job.Objective})
		raw, err := s.analysisRuntime.execute(ctx, owner, job.ID, map[string]any{"request_id": job.ID, "evidence": string(evidence)}, "/v2/memory/extract")
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &receipt); err != nil {
			return err
		}

		if receipt.TaskID == "" {
			return errors.New("provider returned no extraction task")
		}
		job.Checkpoint, _ = json.Marshal(receipt)
		// Save native task identity with the original fencing token before any status polling.
		if err := s.saveAnalysisProgress(ctx, lease, job); err != nil {
			return err
		}
	}
	for {
		var state MemoryTaskStatus
		if _, err := s.analysisRuntime.call(ctx, owner, http.MethodPost, "/v2/memory/task", map[string]any{"task_id": receipt.TaskID}, &state); err != nil {
			return err
		}
		switch state.Status {
		case "completed":
			// The artifact is the provider's extraction receipt/result, not the owner's entire memory collection.
			// Retrieval remains provider-scoped; no unrelated existing files are attributed to this job.
			state.Result = sanitizeAnalysisJSON(state.Result, 512*1024)
			payload, _ := json.Marshal(map[string]any{"extraction": receipt, "result": state, "provider": "openviking"})
			provenance, _ := json.Marshal(map[string]any{"pipeline": "memory", "adapter": job.Adapter, "evidence_sha256": job.EvidencePack.SHA256, "provider_task_id": receipt.TaskID, "execution_id": job.ID, "scope": "provider extraction task; unconfirmed engineering memory"})
			hash := sha256.Sum256([]byte(job.ID + ":memory"))
			job.Artifacts = []Artifact{{ID: "artifact-" + hex.EncodeToString(hash[:16]), OwnerID: job.OwnerUserID, JobID: job.ID, Type: "engineering_memory", SchemaVersion: "catena.artifact.v1", EvidenceRefs: analysisJobTraceIDs(job), Payload: payload, Provenance: provenance, ValidationStatus: "extracted", CreatedAt: time.Now().UTC()}}
			job.State = AnalysisJobCompleted
			job.CurrentStage = "complete"
			job.Stages[0].State = AnalysisStageCompleted
			return s.saveAnalysisProgress(ctx, lease, job)
		case "failed":
			return errors.New("provider extraction failed")
		case "pending", "processing":
		default:
			return errors.New("provider returned invalid extraction state")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
