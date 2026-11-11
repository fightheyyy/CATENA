package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type workflowModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url,omitempty"`
	Key      string `json:"api_key,omitempty"`
}

type workflowRequest struct {
	Schema     string        `json:"schema"`
	RequestID  string        `json:"request_id"`
	Model      workflowModel `json:"model"`
	Capability string        `json:"harness_capability,omitempty"`
}

type investigationScope struct {
	Agent string     `json:"agent_id"`
	Trace string     `json:"trace_id"`
	Start *time.Time `json:"window_start"`
	End   *time.Time `json:"window_end"`
}

type investigationRequest struct {
	workflowRequest
	Objective string               `json:"objective"`
	Scope     investigationScope   `json:"scope"`
	Query     traceQueryCapability `json:"trace_query"`
}

type improvementOptions struct {
	Case            string          `json:"case_id"`
	Baseline        json.RawMessage `json:"baseline,omitempty"`
	Budget          json.RawMessage `json:"budget,omitempty"`
	ValidationCases []string        `json:"validation_case_ids,omitempty"`
}

type improvementRequest struct {
	workflowRequest
	improvementOptions
	AnalysisInput json.RawMessage `json:"analysis_input"`
	Evidence      string          `json:"evidence"`
	Refs          []string        `json:"evidence_refs"`
}

type analysisWorkflow struct {
	store interface {
		SaveLeasedAnalysisJob(context.Context, AnalysisLease, AnalysisJob) error
	}
	engine      *EngineClient
	credentials func(context.Context, string) (AnalysisModelCredentials, error)
	openQuery   func(context.Context, AnalysisLease, AnalysisJob) (traceQueryCapability, func())
	openTurn    func(context.Context, AnalysisLease, string, string) (string, func(), error)
}

func (s analysisWorkflow) execute(ctx context.Context, lease AnalysisLease, job AnalysisJob) error {
	if job.Schema != "catena.analysis_job.v2" || (job.Pipeline != "improve" && job.Pipeline != "analyze") {
		return errors.New("legacy execution retired; create a V2 Analysis Job")
	}
	if job.EvidencePack == nil || !validAnalysisEvidencePack(*job.EvidencePack, job) {
		return errors.New("invalid retained Evidence Pack")
	}
	credentials, err := s.credentials(ctx, job.OwnerUserID)
	if err != nil {
		return err
	}
	model := workflowModel{Provider: credentials.Provider, Model: credentials.Model}
	job.State = AnalysisJobRunning
	job.CurrentStage = "generate"
	job.Stages[0].State = AnalysisStageRunning
	if err = s.saveAnalysisProgress(ctx, lease, job); err != nil {
		return err
	}
	owner := job.OwnerUserID
	if owner == "" {
		owner = "local"
	}
	common := workflowRequest{RequestID: job.ID, Model: model}
	if s.openTurn != nil {
		capability, closeWorkflow, openErr := s.openTurn(ctx, lease, owner, job.Pipeline)
		if openErr != nil {
			return openErr
		}
		defer closeWorkflow()
		common.Capability = capability
	}
	var payload any
	var path string
	if job.Pipeline == "analyze" {
		query, closeQuery := s.openQuery(ctx, lease, job)
		defer closeQuery()
		common.Schema = engineTurnRequestSchema
		path = "/v3/analyze"
		payload = investigationRequest{
			workflowRequest: common, Objective: job.Objective, Query: query,
			Scope: investigationScope{job.SourceAgentID, job.SourceTraceID, job.WindowStart, job.WindowEnd},
		}
	} else {
		var options improvementOptions
		if len(job.Options) > 0 {
			d := json.NewDecoder(bytes.NewReader(job.Options))
			d.DisallowUnknownFields()
			if d.Decode(&options) != nil {
				return errors.New("invalid improvement options")
			}
		}
		if options.Case == "" || job.SourceAnalysisJobID == "" || !json.Valid(job.AnalysisInput) {
			return errors.New("registered Case and human-started Analysis reference are required")
		}
		if len(options.Baseline) == 0 || bytes.Equal(options.Baseline, []byte("null")) {
			options.Baseline = json.RawMessage(`{"AGENTS.md":"Complete the supplied task faithfully."}`)
		}
		evidence, err := json.Marshal(struct {
			Pack      *AnalysisEvidencePack `json:"pack"`
			Objective string                `json:"objective"`
		}{job.EvidencePack, job.Objective})
		if err != nil {
			return err
		}
		common.Schema = "catena.improvement_request.v1"
		common.Model.BaseURL, common.Model.Key = credentials.BaseURL, credentials.APIKey
		path = "/v3/improve"
		payload = improvementRequest{
			workflowRequest: common, improvementOptions: options,
			AnalysisInput: job.AnalysisInput, Evidence: string(evidence), Refs: analysisJobTraceIDs(job),
		}
	}
	result, err := s.engine.execute(ctx, owner, job.ID, payload, path)
	if err != nil {
		return err
	}
	result = sanitizeAnalysisJSON(result, engineMaxResponseBytes)
	var out struct {
		Status     string            `json:"status"`
		Candidates []json.RawMessage `json:"candidates"`
	}
	if err = json.Unmarshal(result, &out); err != nil {
		return errors.New("invalid improvement result")
	}
	job.Checkpoint = result
	if job.Pipeline == "analyze" && out.Status == "completed" {
		var report struct {
			Schema   string            `json:"schema"`
			Problems []json.RawMessage `json:"problems"`
		}
		if json.Unmarshal(result, &report) != nil || report.Schema != "catena.analysis_result.v1" || report.Problems == nil {
			return errors.New("invalid analysis report")
		}
		provenance, _ := json.Marshal(map[string]any{"evidence_sha256": job.EvidencePack.SHA256, "execution_id": job.ID, "report": json.RawMessage(result)})
		digest := sha256.Sum256([]byte(job.ID + ":analysis"))
		job.Artifacts = []Artifact{{ID: "artifact-" + hex.EncodeToString(digest[:16]), OwnerID: job.OwnerUserID, JobID: job.ID, Type: "analysis_report", SchemaVersion: "catena.artifact.v1", EvidenceRefs: analysisJobTraceIDs(job), Payload: result, Provenance: provenance, ValidationStatus: "unverified", CreatedAt: time.Now().UTC()}}
		job.State = AnalysisJobCompleted
		job.CurrentStage = "complete"
		job.Stages[0].State = AnalysisStageCompleted
		return s.saveAnalysisProgress(ctx, lease, job)
	}
	if out.Status != "completed" || len(out.Candidates) == 0 {
		if err = s.saveAnalysisProgress(ctx, lease, job); err != nil {
			return err
		}
		return errors.New("improvement execution failed; retained trial evidence requires review")
	}
	job.Artifacts = []Artifact{}
	var provenanceFields map[string]json.RawMessage
	json.Unmarshal(result, &provenanceFields)
	provenanceFields["evidence_sha256"], _ = json.Marshal(job.EvidencePack.SHA256)
	provenanceFields["execution_id"], _ = json.Marshal(job.ID)
	if job.SourceAnalysisJobID != "" {
		provenanceFields["analysis_job_id"], _ = json.Marshal(job.SourceAnalysisJobID)
		provenanceFields["analysis_input"] = job.AnalysisInput
		digest := sha256.Sum256(job.AnalysisInput)
		provenanceFields["analysis_sha256"], _ = json.Marshal(hex.EncodeToString(digest[:]))
	}
	provenance, _ := json.Marshal(provenanceFields)
	for i, payload := range out.Candidates {
		status := "unverified"
		var candidate struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(payload, &candidate) != nil || candidate.ID == "" {
			return errors.New("invalid candidate identity")
		}
		if hasComparison(result, candidate.ID) {
			status = "review_required"
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", job.ID, i)))
		job.Artifacts = append(job.Artifacts, Artifact{ID: "artifact-" + hex.EncodeToString(digest[:16]), OwnerID: job.OwnerUserID, JobID: job.ID, Type: "improvement_candidate", SchemaVersion: "catena.artifact.v1", EvidenceRefs: analysisJobTraceIDs(job), Payload: payload, Provenance: provenance, ValidationStatus: status, CreatedAt: time.Now().UTC()})
	}
	job.State = AnalysisJobCompleted
	job.CurrentStage = "complete"
	job.Stages[0].State = AnalysisStageCompleted
	return s.saveAnalysisProgress(ctx, lease, job)
}

func (s analysisWorkflow) saveAnalysisProgress(ctx context.Context, lease AnalysisLease, job AnalysisJob) error {
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.store.SaveLeasedAnalysisJob(call, lease, job)
	if err == nil || errors.Is(err, ErrLeaseLost) || ctx.Err() != nil {
		return err
	}
	return fmt.Errorf("%w: stage persistence unavailable", errAnalysisTemporary)
}
