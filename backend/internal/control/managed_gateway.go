package control

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *HTTPServer) managedAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.managed == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.managed.config.Token)) != 1 {
		writeProblem(w, 401, "unauthorized")
		return
	}
	var in struct {
		Run        string `json:"run_id"`
		Sandbox    string `json:"sandbox_id"`
		Generation int64  `json:"generation"`
		Token      string `json:"token"`
		Fence      int64  `json:"fencing_token"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	var valid bool
	err := s.managed.store.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM agent_runs r JOIN managed_sandboxes b ON b.session_id=r.session_id WHERE r.run_id=$1 AND b.sandbox_id=$2 AND b.generation=$3 AND r.credential_hash=$4 AND r.fencing_token=$5 AND r.state='RUNNING' AND r.lease_until>now() AND NOT r.cancel_requested AND b.state IN ('PENDING','CREATING','READY','BUSY') AND (r.execution_started_at IS NULL OR r.execution_started_at+r.execution_timeout*interval '1 second'>now()))`, in.Run, in.Sandbox, in.Generation, sessionTokenHash(in.Token), in.Fence).Scan(&valid)
	if err != nil || !valid {
		writeProblem(w, 403, "execution lease revoked")
		return
	}
	writeJSON(w, 200, map[string]bool{"authorized": true})
}
func (s *HTTPServer) managedModel(w http.ResponseWriter, r *http.Request) {
	if s.managed == nil {
		writeProblem(w, 503, "managed runtime disabled")
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(token) != 43 || strings.ContainsAny(token, " \t\r\n") {
		writeProblem(w, 401, "run capability required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024*1024)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeProblem(w, 400, "invalid model request")
		return
	}
	// No arbitrary upstream URL, model routing, credential headers or background execution.
	if body["background"] == true {
		writeProblem(w, 400, "background responses are disabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var valid bool
				e := s.managed.store.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE credential_hash=$1 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested)`, sessionTokenHash(token)).Scan(&valid)
				if e != nil || !valid {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-watcherDone }()
	for attempt := 0; attempt < 3; attempt++ {
		tenant, run, trace, maxTokens, requestID, err := s.reserveManagedModel(ctx, sessionTokenHash(token))
		if err != nil {
			if errors.Is(err, ErrQueueFull) {
				writeProblem(w, 429, "model rate or run budget exhausted")
			} else {
				writeProblem(w, 403, "execution capability expired or revoked")
			}
			return
		}
		credentials, err := s.analysisModelCredentials(ctx, tenant)
		if err != nil {
			writeProblem(w, 503, "model configuration unavailable")
			return
		}
		if body["model"] != credentials.Model {
			writeProblem(w, 400, "model must match tenant configuration")
			return
		}
		if n, ok := body["max_output_tokens"].(float64); !ok || n > float64(maxTokens) {
			body["max_output_tokens"] = maxTokens
		}
		body["store"] = false
		payload, err := json.Marshal(body)
		if err != nil {
			writeProblem(w, 400, "invalid model payload")
			return
		}
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(credentials.BaseURL, "/")+"/responses", bytes.NewReader(payload))
		if err != nil {
			writeProblem(w, 503, "invalid model endpoint")
			return
		}
		req.Header.Set("Authorization", "Bearer "+credentials.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("traceparent", "00-"+trace+"-"+trace[:16]+"-01")
		req.Header.Set("X-Catena-Run-ID", run)
		started := time.Now()
		resp, err := s.managed.client.Do(req)
		if err != nil {
			writeProblem(w, 502, "model transport failed; outcome unknown")
			return
		}
		_, _ = s.managed.store.db.ExecContext(ctx, `UPDATE managed_model_requests SET status=$2,latency_ms=$3 WHERE request_id=$1`, requestID, resp.StatusCode, time.Since(started).Milliseconds())
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < 2 {
			resp.Body.Close()
			if !waitAnalysisBackoff(ctx, time.Duration(1<<attempt)*time.Second) {
				return
			}
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			writeProblem(w, resp.StatusCode, "upstream model request rejected")
			return
		}
		content := resp.Header.Get("Content-Type")
		w.Header().Set("Content-Type", content)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		if strings.Contains(content, "text/event-stream") {
			scanner := bufio.NewScanner(io.LimitReader(resp.Body, 32*1024*1024))
			scanner.Buffer(make([]byte, 4096), 4*1024*1024)
			for scanner.Scan() {
				line := scanner.Bytes()
				s.accountManagedUsage(ctx, requestID, run, bytes.TrimPrefix(line, []byte("data: ")))
				if _, err = w.Write(append(append([]byte{}, line...), '\n')); err != nil {
					break
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		} else {
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
			if err == nil {
				s.accountManagedUsage(ctx, requestID, run, raw)
				_, _ = w.Write(raw)
			}
		}
		resp.Body.Close()
		_, _ = s.managed.store.db.ExecContext(ctx, `UPDATE managed_model_requests SET latency_ms=$2 WHERE request_id=$1`, requestID, time.Since(started).Milliseconds())
		return
	}
}
func (s *HTTPServer) reserveManagedModel(ctx context.Context, hash string) (tenant, run, trace string, maxTokens int, requestID int64, err error) {
	tx, e := managedTx(ctx, s.managed.store.db)
	if e != nil {
		err = e
		return
	}
	defer tx.Rollback()
	var used, limit, rate, recent int
	e = tx.QueryRowContext(ctx, `SELECT r.tenant_id,r.run_id,r.trace_id,q.max_output_tokens,r.model_requests,q.max_model_requests,q.requests_per_minute,(SELECT count(*) FROM managed_model_requests m WHERE m.tenant_id=r.tenant_id AND m.created_at>now()-interval '1 minute') FROM agent_runs r JOIN tenant_quotas q USING(tenant_id) WHERE r.credential_hash=$1 AND r.state='RUNNING' AND r.lease_until>now() AND NOT r.cancel_requested AND r.execution_started_at+r.execution_timeout*interval '1 second'>now() FOR UPDATE OF r`, hash).Scan(&tenant, &run, &trace, &maxTokens, &used, &limit, &rate, &recent)
	if e != nil {
		err = e
		return
	}
	if used >= limit || recent >= rate {
		err = ErrQueueFull
		return
	}
	_, e = tx.ExecContext(ctx, `UPDATE agent_runs SET model_requests=model_requests+1 WHERE run_id=$1`, run)
	if e != nil {
		err = e
		return
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO managed_model_requests(tenant_id,run_id) VALUES($1,$2) RETURNING request_id`, tenant, run).Scan(&requestID)
	if e != nil {
		err = e
		return
	}
	err = tx.Commit()
	return
}
func (s *HTTPServer) accountManagedUsage(ctx context.Context, id int64, run string, raw []byte) {
	var event struct {
		Usage *struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
		} `json:"usage"`
		Response *struct {
			Usage *struct {
				Input  int64 `json:"input_tokens"`
				Output int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	usage := event.Usage
	if usage == nil && event.Response != nil {
		usage = event.Response.Usage
	}
	if usage == nil || usage.Input < 0 || usage.Output < 0 {
		return
	}
	// Each request is accounted once; unknown provider usage remains NULL.
	_, _ = s.managed.store.db.ExecContext(ctx, `WITH u AS (UPDATE managed_model_requests SET input_tokens=$2,output_tokens=$3 WHERE request_id=$1 AND input_tokens IS NULL RETURNING run_id) UPDATE agent_runs SET input_tokens=input_tokens+$2,output_tokens=output_tokens+$3 WHERE run_id=$4 AND EXISTS(SELECT 1 FROM u)`, id, usage.Input, usage.Output, run)
}
