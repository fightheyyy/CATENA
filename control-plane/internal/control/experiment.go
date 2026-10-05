package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type ExperimentTrial struct {
	NativeSessionVerified bool               `json:"native_session_verified,omitempty"`
	EnvironmentVerified   bool               `json:"environment_verified,omitempty"`
	Attempt               int                `json:"attempt,omitempty"`
	DurationSeconds       *float64           `json:"duration_seconds,omitempty"`
	Variant               string             `json:"variant"`
	Task                  string             `json:"task"`
	Valid                 bool               `json:"valid"`
	Rewards               map[string]float64 `json:"rewards"`
	InputTokens           int64              `json:"input_tokens"`
	OutputTokens          int64              `json:"output_tokens"`
	CommandCount          int                `json:"command_count"`
	MissingRGErrors       int                `json:"missing_rg_errors"`
	PolicyRejections      int                `json:"policy_rejections"`
}
type Experiment struct {
	TargetAgent    string             `json:"target_agent,omitempty"`
	Study          string             `json:"study,omitempty"`
	Attempts       int                `json:"attempts,omitempty"`
	Arms           []string           `json:"arms,omitempty"`
	MemoryContext  []MemoryRecallItem `json:"memory_context,omitempty"`
	ID             string             `json:"experiment_id"`
	Owner          string             `json:"-"`
	RequestID      string             `json:"request_id"`
	CaseID         string             `json:"case_id"`
	State          string             `json:"state"`
	JobID          string             `json:"harbor_job_id,omitempty"`
	Model          string             `json:"model"`
	Environment    string             `json:"environment"`
	Trials         []ExperimentTrial  `json:"trials"`
	TasksUnchanged bool               `json:"tasks_unchanged"`
	Message        string             `json:"message,omitempty"`
	Summary        string             `json:"summary,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

func (e Experiment) active() bool { return e.State == "queued" || e.State == "running" }
func cloneExperiment(e Experiment) Experiment {
	b, _ := json.Marshal(e)
	var c Experiment
	_ = json.Unmarshal(b, &c)
	c.Owner = e.Owner
	return c
}

func (s *MemoryStore) CreateExperiment(_ context.Context, e Experiment) (Experiment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, old := range s.experiments {
		if old.Owner == e.Owner && old.RequestID == e.RequestID {
			return cloneExperiment(old), false, nil
		}
	}
	for _, old := range s.experiments {
		if old.active() {
			return Experiment{}, false, ErrConflict
		}
	}
	s.experiments[e.ID] = cloneExperiment(e)
	return e, true, nil
}
func (s *MemoryStore) UpdateExperiment(_ context.Context, e Experiment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.experiments[e.ID]
	if !ok || old.Owner != e.Owner {
		return ErrNotFound
	}
	s.experiments[e.ID] = cloneExperiment(e)
	return nil
}
func (s *MemoryStore) GetExperiment(_ context.Context, owner, id string) (Experiment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.experiments[id]
	if !ok || e.Owner != owner {
		return Experiment{}, ErrNotFound
	}
	return cloneExperiment(e), nil
}
func (s *MemoryStore) ListExperiments(_ context.Context, owner string) ([]Experiment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []Experiment{}
	for _, e := range s.experiments {
		if e.Owner == owner {
			result = append(result, cloneExperiment(e))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > 50 {
		result = result[:50]
	}
	return result, nil
}
func (s *PostgresStore) CreateExperiment(ctx context.Context, e Experiment) (Experiment, bool, error) {
	// ON CONFLICT covers both owner idempotency and the single-worker capacity limit.
	b, err := json.Marshal(e)
	if err != nil {
		return Experiment{}, false, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO catena_experiments(experiment_id,owner_user_id,request_id,state,document,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, e.ID, e.Owner, e.RequestID, e.State, b, e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return Experiment{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		return e, true, nil
	}
	var document []byte
	err = s.db.QueryRowContext(ctx, `SELECT document FROM catena_experiments WHERE owner_user_id=$1 AND request_id=$2`, e.Owner, e.RequestID).Scan(&document)
	if errors.Is(err, sql.ErrNoRows) {
		return Experiment{}, false, ErrConflict
	}
	if err != nil {
		return Experiment{}, false, err
	}
	var old Experiment
	err = json.Unmarshal(document, &old)
	old.Owner = e.Owner
	return old, false, err
}
func (s *PostgresStore) UpdateExperiment(ctx context.Context, e Experiment) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE catena_experiments SET state=$1,document=$2,updated_at=$3 WHERE experiment_id=$4 AND owner_user_id=$5`, e.State, b, e.UpdatedAt, e.ID, e.Owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *PostgresStore) GetExperiment(ctx context.Context, owner, id string) (Experiment, error) {
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT document FROM catena_experiments WHERE owner_user_id=$1 AND experiment_id=$2`, owner, id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return Experiment{}, ErrNotFound
	}
	if err != nil {
		return Experiment{}, err
	}
	var e Experiment
	err = json.Unmarshal(b, &e)
	e.Owner = owner
	return e, err
}
func (s *PostgresStore) ListExperiments(ctx context.Context, owner string) ([]Experiment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT document FROM catena_experiments WHERE owner_user_id=$1 ORDER BY created_at DESC LIMIT 50`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Experiment{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var e Experiment
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		e.Owner = owner
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *HTTPServer) experimentCases(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	item := map[string]any{"case_id": "rg-missing-search", "title": "搜索工具缺失时的恢复", "description": "三个冻结搜索任务，对比原始 Agent、历史记忆和搜索 Skill。", "task_count": 3, "trial_count": 6, "target": "Codex CLI / SDK 搜索 Agent", "environment": "Linux / PowerShell 7 · Docker", "limitation": "匿名重建的 Linux / PowerShell 环境，不是原 Windows 会话重放；结论仅适用于本次选定的 Agent 与这三个任务。", "evidence_summary": "人工核对五个独立 Codex 会话，发现五次 rg 缺失错误；随后可见原生 PowerShell 搜索恢复。历史证据说明重复工具错误，不说明任务最终失败。"}
	if s.traces != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		const traceID = "32224e4d48f38a77ef965b9ae585aa80"
		if _, err := s.traces.GetTrace(ctx, traceOwnerID(user), traceID); err == nil {
			item["source_trace_id"] = traceID
		}
	}
	writeJSON(w, 200, map[string]any{"cases": []map[string]any{item}})
}
func (s *HTTPServer) createExperiment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	owner := traceOwnerID(user)
	var req struct {
		CaseID        string `json:"case_id"`
		RequestID     string `json:"request_id"`
		IncludeMemory bool   `json:"include_memory"`
		TargetAgent   string `json:"target_agent"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if req.CaseID != "rg-missing-search" || !safeRuntimeID.MatchString(req.RequestID) {
		writeProblem(w, 400, "Unsupported Case or invalid request_id")
		return
	}
	if req.TargetAgent == "" {
		req.TargetAgent = "sdk"
	}
	if req.TargetAgent != "sdk" && req.TargetAgent != "codex" || req.TargetAgent == "codex" && req.IncludeMemory {
		writeProblem(w, 400, "Codex currently supports baseline/Skill comparison without the memory arm")
		return
	}
	if s.evolutionRuntime == nil {
		writeProblem(w, 503, "Experiment engine is not configured")
		return
	}
	model, err := s.evolutionModelCredentials(r.Context(), owner)
	if err != nil {
		writeProblem(w, 409, err.Error())
		return
	}
	// Reconcile the owner's previous active record before reserving worker capacity.
	prior, err := s.store.ListExperiments(r.Context(), owner)
	if err != nil {
		writeProblem(w, 500, "Cannot read experiment records")
		return
	}
	for _, e := range prior {
		if e.active() {
			s.refreshExperiment(r.Context(), e)
		}
	}
	now := time.Now().UTC()
	plan := Experiment{ID: newID("experiment"), Owner: owner, RequestID: req.RequestID, CaseID: req.CaseID, State: "queued", Model: model.Model, Environment: "Linux / PowerShell 7 · Docker", Trials: []ExperimentTrial{}, CreatedAt: now, UpdatedAt: now}
	plan.TargetAgent = req.TargetAgent
	if req.IncludeMemory {
		// Return the previous record before recall, so retries freeze the same snapshot.
		for _, old := range prior {
			if old.RequestID == req.RequestID {
				writeJSON(w, 200, old)
				return
			}
		}
		if s.memory == nil {
			writeProblem(w, 409, "Memory backend is required for this comparison")
			return
		}
		memoryCtx, memoryCancel := context.WithTimeout(r.Context(), 15*time.Second)
		recalled, recallErr := s.memory.Search(memoryCtx, owner, MemorySearchRequest{Query: "PowerShell 搜索缺少 rg 历史错误恢复", TopK: 1})
		memoryCancel()
		if recallErr != nil || len(recalled.Facts) == 0 || recalled.Facts[0].Score < 0.35 {
			writeProblem(w, 409, "No relevant memory was retrieved; add verified historical evidence first")
			return
		}
		item := recalled.Facts[0]
		item.Content = bounded(redactMemoryText(item.Content), 2000)
		item.Title = bounded(redactMemoryText(item.Title), 200)
		item.ID = bounded(item.ID, 160)
		item.Metadata = map[string]any{"provider": "memory_backend"}
		plan.MemoryContext = []MemoryRecallItem{item}
		plan.Study, plan.Attempts, plan.Arms = "memory_skill", 2, []string{"baseline", "memory", "candidate"}
	}
	e, created, err := s.store.CreateExperiment(r.Context(), plan)
	if err != nil {
		writeProblem(w, statusFor(err), "Another experiment is running; wait for its result")
		return
	}
	if !created {
		writeJSON(w, 200, e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	var state harborState
	payload := map[string]any{"case_id": e.CaseID, "request_id": e.ID, "environment": "docker", "model": modelPayload(model)}
	payload["target_agent"] = e.TargetAgent
	if e.Study == "memory_skill" {
		payload["study"], payload["memory_context"] = e.Study, e.MemoryContext
	}
	status, callErr := s.evolutionRuntime.do(ctx, http.MethodPost, "/v1/harbor/submit", payload, &state)
	if callErr != nil || status != 200 {
		e.Message = "提交状态待确认，正在检查执行记录；不会自动重复创建实验。"
		if status == 409 || status == 400 {
			e.State = "failed"
			e.Message = "执行端拒绝提交，请等待其他实验完成后重试。"
		}
	} else {
		e = applyHarborState(e, state)
	}
	e.UpdatedAt = time.Now().UTC()
	if err = s.store.UpdateExperiment(r.Context(), e); err != nil {
		writeProblem(w, 500, "Cannot persist experiment status")
		return
	}
	if e.active() && e.JobID != "" {
		go s.summarizeExperiment(e, model)
	}
	if e.active() {
		go s.monitorExperiment(e)
	}
	writeJSON(w, http.StatusAccepted, e)
}
func modelPayload(m EvolutionModelCredentials) map[string]string {
	return map[string]string{"provider": m.Provider, "base_url": m.BaseURL, "model": m.Model, "api_key": m.APIKey}
}

type harborState struct {
	JobID          string            `json:"job_id"`
	Status         string            `json:"status"`
	Trials         []ExperimentTrial `json:"trials"`
	TasksUnchanged bool              `json:"tasks_unchanged"`
}

func applyHarborState(e Experiment, state harborState) Experiment {
	if !e.active() && state.Status == "running" {
		return e
	}
	e.JobID = state.JobID
	e.Trials = state.Trials
	if e.Trials == nil {
		e.Trials = []ExperimentTrial{}
	}
	e.TasksUnchanged = state.TasksUnchanged
	switch state.Status {
	case "running", "completed", "invalid", "failed", "blocked":
		e.State = state.Status
	default:
		e.State = "failed"
	}
	if e.State == "completed" {
		counts := map[string]int{}
		arms, attempts := []string{"baseline", "candidate"}, 1
		if e.Study == "memory_skill" {
			arms, attempts = []string{"baseline", "memory", "candidate"}, 2
		}
		verified := e.TasksUnchanged && len(e.Trials) == 3*attempts*len(arms)
		identities := map[string]bool{}
		for _, trial := range e.Trials {
			reward, present := trial.Rewards["reward"]
			verified = verified && trial.Valid && present && (reward == 0 || reward == 1)
			if e.TargetAgent == "codex" {
				verified = verified && trial.NativeSessionVerified && trial.EnvironmentVerified
			}
			counts[trial.Variant]++
			if e.Study == "memory_skill" {
				key := fmt.Sprintf("%s/%s/%d", trial.Variant, trial.Task, trial.Attempt)
				verified = verified && !identities[key] && trial.Attempt >= 1 && trial.Attempt <= attempts && (trial.Task == "catena/command-handler" || trial.Task == "catena/role-config" || trial.Task == "catena/absent-symbol")
				identities[key] = true
			}
		}
		for _, arm := range arms {
			verified = verified && counts[arm] == 3*attempts
		}
		if !verified {
			e.State = "invalid"
		}
	}
	e.Message = ""
	if e.State == "failed" || e.State == "blocked" {
		e.Message = "实验未完成，请检查执行服务。"
	}
	if e.State == "invalid" {
		e.Message = "实验的有效性检查未通过，不能作为 Skill 效果结论。"
	}
	e.UpdatedAt = time.Now().UTC()
	return e
}

func (s *MemoryStore) ListActiveExperiments(_ context.Context) ([]Experiment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []Experiment{}
	for _, e := range s.experiments {
		if e.active() {
			result = append(result, cloneExperiment(e))
		}
	}
	return result, nil
}
func (s *PostgresStore) ListActiveExperiments(ctx context.Context) ([]Experiment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT owner_user_id,document FROM catena_experiments WHERE state IN ('queued','running')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Experiment{}
	for rows.Next() {
		var owner string
		var b []byte
		if err = rows.Scan(&owner, &b); err != nil {
			return nil, err
		}
		var e Experiment
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		e.Owner = owner
		result = append(result, e)
	}
	return result, rows.Err()
}
func (s *HTTPServer) monitorExperiment(e Experiment) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := s.store.GetExperiment(ctx, e.Owner, e.ID)
			if err != nil || !current.active() {
				return
			}
			s.refreshExperiment(ctx, current)
		}
	}
}
func (s *HTTPServer) refreshExperiment(ctx context.Context, e Experiment) Experiment {
	if !e.active() || s.evolutionRuntime == nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var state harborState
	status, err := s.evolutionRuntime.do(ctx, http.MethodPost, "/v1/harbor/lookup", map[string]string{"request_id": e.ID}, &state)
	if err == nil && status == 200 {
		e = applyHarborState(e, state)
	} else if status == 404 && time.Since(e.CreatedAt) > 2*time.Minute {
		e.State = "failed"
		e.Message = "执行记录不存在，提交未成功或执行服务已丢失记录。"
	} else {
		e.Message = "暂时无法同步执行状态，已有结果已保留。"
	}
	e.UpdatedAt = time.Now().UTC()
	if s.store.UpdateExperiment(ctx, e) != nil {
		e.Message = "执行状态已读取，但保存失败，请重试。"
	}
	return e
}
func (s *HTTPServer) listExperiments(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	items, err := s.store.ListExperiments(r.Context(), traceOwnerID(user))
	if err != nil {
		writeProblem(w, 500, "Cannot read experiments")
		return
	}
	for i, e := range items {
		if e.active() {
			items[i] = s.refreshExperiment(r.Context(), e)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"experiments": items})
}
func (s *HTTPServer) getExperiment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	e, err := s.store.GetExperiment(r.Context(), traceOwnerID(user), r.PathValue("experiment_id"))
	if err != nil {
		writeProblem(w, statusFor(err), "Experiment not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, s.refreshExperiment(r.Context(), e))
}

type ExperimentEvidence struct {
	Task    string `json:"task"`
	Variant string `json:"variant"`
	Answer  string `json:"answer"`
	Bounded bool   `json:"bounded"`
	Calls   []struct {
		Command  string `json:"command"`
		ExitCode *int   `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	} `json:"calls"`
}

func (s *HTTPServer) getExperimentEvidence(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	e, err := s.store.GetExperiment(r.Context(), traceOwnerID(user), r.PathValue("experiment_id"))
	if err != nil {
		writeProblem(w, 404, "Experiment not found")
		return
	}
	index, err := strconv.Atoi(r.PathValue("trial_index"))
	if err != nil || index < 0 || index >= len(e.Trials) {
		writeProblem(w, 404, "Trial not found")
		return
	}
	if s.evolutionRuntime == nil || e.JobID == "" {
		writeProblem(w, 503, "Evidence service unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var evidence ExperimentEvidence
	status, err := s.evolutionRuntime.do(ctx, http.MethodPost, "/v1/harbor/evidence", map[string]any{"job_id": e.JobID, "trial_index": index}, &evidence)
	if err != nil || status != 200 {
		writeProblem(w, 503, "Cannot read trial artifacts")
		return
	}
	if evidence.Task != e.Trials[index].Task || evidence.Variant != e.Trials[index].Variant {
		writeProblem(w, 502, "Evidence identity mismatch")
		return
	}
	evidence.Answer = bounded(redactMemoryText(evidence.Answer), 4000)
	if len(evidence.Calls) > 20 {
		evidence.Calls = evidence.Calls[:20]
	}
	for i := range evidence.Calls {
		evidence.Calls[i].Command = bounded(redactMemoryText(evidence.Calls[i].Command), 1000)
		evidence.Calls[i].Stdout = bounded(redactMemoryText(evidence.Calls[i].Stdout), 3500)
		evidence.Calls[i].Stderr = bounded(redactMemoryText(evidence.Calls[i].Stderr), 1500)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, evidence)
}

func (s *HTTPServer) summarizeExperiment(e Experiment, m EvolutionModelCredentials) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	payload := map[string]any{"schema": engineTurnRequestSchema, "request_id": e.ID, "run_id": e.ID, "role": "inspector", "prompt": "使用 Docker 读取并等待已提交的 rg-missing-search 实验，准确总结所有组别的真实结果、耗时和令牌。根据结果中的 target_agent 标识说明实际被测对象。区分历史记忆和 Skill，不声称小样本具有统计显著性。", "timeout_ms": 900000, "environment": "docker", "target_agent": e.TargetAgent, "model": modelPayload(m)}
	var response struct {
		Result struct {
			Assistant struct {
				Content string `json:"content"`
			} `json:"assistant"`
			Experiment harborState `json:"experiment"`
		} `json:"result"`
	}
	status, err := s.evolutionRuntime.do(ctx, http.MethodPost, "/v1/experiment", payload, &response)
	if err != nil || status != 200 {
		return
	}
	current, err := s.store.GetExperiment(ctx, e.Owner, e.ID)
	if err != nil {
		return
	}
	if response.Result.Experiment.JobID == current.JobID {
		current = applyHarborState(current, response.Result.Experiment)
	}
	// Keep structured verifier results authoritative. Model prose is supplementary.
	var prose struct {
		Summary string `json:"summary"`
	}
	if json.Unmarshal([]byte(response.Result.Assistant.Content), &prose) == nil {
		current.Summary = bounded(prose.Summary, 4000)
	}
	_ = s.store.UpdateExperiment(ctx, current)
}
