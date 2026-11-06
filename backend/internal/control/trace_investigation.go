package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// One live investigation capability, revoked when its leased execution returns.
type traceInvestigation struct {
	ctx    context.Context
	job    AnalysisJob
	lease  AnalysisLease
	token  string
	mu     sync.Mutex
	calls  int
	listed map[string]bool
}

func (s *HTTPServer) queryInvestigationTrace(w http.ResponseWriter, r *http.Request) {
	value, ok := s.traceInvestigations.Load(r.PathValue("job_id"))
	if !ok {
		writeProblem(w, 403, "investigation unavailable")
		return
	}
	q := value.(*traceInvestigation)
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(q.token)) != 1 || q.ctx.Err() != nil {
		writeProblem(w, 403, "investigation unavailable")
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	ctx, cancel := context.WithTimeout(q.ctx, 10*time.Second)
	defer cancel()
	cancelled, err := s.store.HeartbeatAnalysisJob(ctx, q.lease, s.jobs.config.LeaseTTL)
	if err != nil || cancelled {
		writeProblem(w, 403, "investigation lease ended")
		return
	}
	if q.calls >= 20 {
		writeProblem(w, 429, "investigation query budget exhausted")
		return
	}
	q.calls++
	var req struct {
		Tool      string `json:"tool"`
		Arguments struct {
			TraceID string `json:"trace_id,omitempty"`
			Status  string `json:"status,omitempty"`
			Limit   int    `json:"limit,omitempty"`
			Offset  int    `json:"offset,omitempty"`
		} `json:"arguments"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeProblem(w, 400, "invalid Trace query")
		return
	}
	owner := q.job.OwnerUserID
	if owner == "" {
		owner = "local"
	}
	switch req.Tool {
	case "list_traces":
		if req.Arguments.TraceID != "" || req.Arguments.Limit < 0 || req.Arguments.Limit > 20 || req.Arguments.Offset < 0 || req.Arguments.Offset > 100 || (req.Arguments.Status != "" && req.Arguments.Status != "all" && req.Arguments.Status != "error" && req.Arguments.Status != "ok") {
			writeProblem(w, 400, "invalid list arguments")
			return
		}
		var traces []TraceSummary
		if q.job.SourceAgentID != "" && q.job.WindowStart != nil && q.job.WindowEnd != nil {
			traces, err = s.traces.ListAgentTraces(ctx, owner, q.job.SourceAgentID, *q.job.WindowStart, *q.job.WindowEnd, maxAgentAnalysisTraceScan)
		} else {
			var trace TraceDetail
			trace, err = s.traces.GetTrace(ctx, owner, q.job.SourceTraceID)
			traces = []TraceSummary{trace.Summary}
		}
		if err != nil {
			writeProblem(w, 503, "Trace query failed")
			return
		}
		rows := []TraceSummary{}
		for _, t := range traces {
			if req.Arguments.Status == "error" && t.ErrorCount == 0 || req.Arguments.Status == "ok" && t.ErrorCount > 0 {
				continue
			}
			rows = append(rows, t)
		}
		limit := req.Arguments.Limit
		if limit == 0 {
			limit = 10
		}
		total := len(rows)
		offset := min(req.Arguments.Offset, total)
		rows = rows[offset:min(offset+limit, total)]
		for _, row := range rows {
			q.listed[row.TraceID] = true
		}
		raw, _ := json.Marshal(map[string]any{"traces": rows, "matched_in_scan": total, "scan_limit": maxAgentAnalysisTraceScan, "next_offset": offset + len(rows)})
		writeJSON(w, 200, json.RawMessage(sanitizeAnalysisJSON(raw, 64*1024)))
	case "get_trace":
		id, err := normalizedAnalysisTraceID(req.Arguments.TraceID)
		if err != nil || !q.listed[id] {
			writeProblem(w, 403, "Trace must first be listed within the investigation scope")
			return
		}
		if req.Arguments.Status != "" || req.Arguments.Limit < 0 || req.Arguments.Limit > 64 || req.Arguments.Offset < 0 || req.Arguments.Offset > 10000 {
			writeProblem(w, 400, "invalid detail arguments")
			return
		}
		trace, err := s.traces.GetTrace(ctx, owner, id)
		if err != nil {
			writeProblem(w, statusFor(err), "Trace unavailable")
			return
		}
		// Recheck scope on detail reads: an identity listed earlier is not a permission bypass.
		if q.job.SourceAgentID != "" && (!traceSummaryBelongsToAgent(trace.Summary, q.job.SourceAgentID) || q.job.WindowStart == nil || q.job.WindowEnd == nil || trace.Summary.EndTime.Before(*q.job.WindowStart) || trace.Summary.StartTime.After(*q.job.WindowEnd)) {
			writeProblem(w, 403, "Trace outside investigation scope")
			return
		}
		sort.SliceStable(trace.Spans, func(i, j int) bool {
			if trace.Spans[i].StartTime.Equal(trace.Spans[j].StartTime) {
				return trace.Spans[i].SpanID < trace.Spans[j].SpanID
			}
			return trace.Spans[i].StartTime.Before(trace.Spans[j].StartTime)
		})
		totalSpans := len(trace.Spans)
		offset := req.Arguments.Offset
		if offset >= totalSpans {
			writeProblem(w, 400, "span offset outside Trace")
			return
		}
		limit := req.Arguments.Limit
		if limit == 0 {
			limit = 32
		}
		trace.Spans = trace.Spans[offset:min(offset+limit, totalSpans)]
		pack, err := buildAnalysisEvidencePack(AnalysisSourceTrace, id, &trace, time.Now().UTC())
		if err != nil {
			writeProblem(w, 400, "Trace evidence unavailable")
			return
		}
		// Reuse existing sanitization; paginate long traces rather than hiding later spans.
		pack.Truncated = pack.Truncated || offset > 0 || offset+len(pack.Spans) < totalSpans
		encoded, _ := analysisEvidenceDigestInput(pack)
		digest := sha256.Sum256(encoded)
		pack.SHA256 = hex.EncodeToString(digest[:])
		for {
			raw, _ := json.Marshal(pack)
			var response map[string]any
			json.Unmarshal(raw, &response)
			response["span_offset"] = offset
			response["next_offset"] = nil
			if offset+len(pack.Spans) < totalSpans {
				response["next_offset"] = offset + len(pack.Spans)
			}
			raw, _ = json.Marshal(response)
			if len(raw) <= 60*1024 {
				writeJSON(w, 200, json.RawMessage(sanitizeAnalysisJSON(raw, 64*1024)))
				return
			}
			if len(pack.Spans) <= 1 {
				writeProblem(w, 413, "Trace evidence too large")
				return
			}
			pack.Spans = pack.Spans[:len(pack.Spans)-1]
			pack.IncludedSpanCount = len(pack.Spans)
			pack.Truncated = true
			encoded, _ := analysisEvidenceDigestInput(pack)
			digest := sha256.Sum256(encoded)
			pack.SHA256 = hex.EncodeToString(digest[:])
		}
	default:
		writeProblem(w, 400, "unsupported Trace tool")
	}
}

func (s *HTTPServer) openTraceInvestigation(ctx context.Context, lease AnalysisLease, job AnalysisJob) (map[string]string, func()) {
	q := &traceInvestigation{ctx: ctx, job: job, lease: lease, token: rand.Text(), listed: map[string]bool{}}
	s.traceInvestigations.Store(job.ID, q)
	return map[string]string{"job_id": job.ID, "token": q.token}, func() { s.traceInvestigations.CompareAndDelete(job.ID, q) }
}
