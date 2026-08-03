package control

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RunnerConfig struct {
	NodeCommand string
	WorkerPath  string
	RunsRoot    string
	KillGrace   time.Duration
}

type RunnerManager struct {
	store     Store
	config    RunnerConfig
	processMu sync.Mutex
	processes map[string]*exec.Cmd
}

func NewRunnerManager(store Store, config RunnerConfig) (*RunnerManager, error) {
	if config.NodeCommand == "" {
		config.NodeCommand = "node"
	}
	worker, err := filepath.Abs(config.WorkerPath)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(worker); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Engine Worker does not exist: %s", worker)
	}
	runsRoot, err := filepath.Abs(config.RunsRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runsRoot, 0o755); err != nil {
		return nil, err
	}
	if config.KillGrace <= 0 {
		config.KillGrace = 5 * time.Second
	}
	config.WorkerPath = worker
	config.RunsRoot = runsRoot
	return &RunnerManager{
		store:     store,
		config:    config,
		processes: make(map[string]*exec.Cmd),
	}, nil
}

func (m *RunnerManager) Start(ctx context.Context, request CreateRunRequest) (Run, error) {
	return m.StartOwned(ctx, request, "")
}

func (m *RunnerManager) StartOwned(
	ctx context.Context,
	request CreateRunRequest,
	ownerUserID string,
) (Run, error) {
	if err := request.Validate(); err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	run := Run{
		ID:          newID("run"),
		RequestID:   newID("req"),
		OwnerUserID: ownerUserID,
		Origin:      OriginLocal,
		Operation:   request.Operation,
		State:       StateQueued,
		Input:       request.Input,
		Runtime:     request.Runtime,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.store.CreateRun(ctx, run); err != nil {
		return Run{}, err
	}
	go m.execute(context.WithoutCancel(ctx), run)
	return run, nil
}

func (m *RunnerManager) StartReplayOwned(
	ctx context.Context,
	sourceCase Case,
	ownerUserID string,
	idempotencyKey string,
) (Run, HarnessVersion, bool, error) {
	if sourceCase.ID == "" ||
		sourceCase.OwnerUserID != ownerUserID ||
		idempotencyKey == "" {
		return Run{}, HarnessVersion{}, false, ErrConflict
	}
	runtime := cloneJSON(sourceCase.Runtime)
	if len(runtime) == 0 {
		runtime = json.RawMessage(`{}`)
	}
	input, err := json.Marshal(struct {
		PlatformCase Case   `json:"platform_case"`
		CaseBaseDir  string `json:"case_base_dir"`
	}{
		PlatformCase: sourceCase,
		CaseBaseDir:  m.config.RunsRoot,
	})
	if err != nil {
		return Run{}, HarnessVersion{}, false, err
	}
	request := CreateRunRequest{
		Operation: OperationReplay,
		Input:     input,
		Runtime:   runtime,
	}
	if err := request.Validate(); err != nil {
		return Run{}, HarnessVersion{}, false, err
	}
	now := time.Now().UTC()
	run := Run{
		ID:          newID("run"),
		RequestID:   newID("req"),
		OwnerUserID: ownerUserID,
		Origin:      OriginLocal,
		Operation:   OperationReplay,
		State:       StateQueued,
		Input:       request.Input,
		Runtime:     request.Runtime,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	harness := HarnessVersion{
		ID:             newID("harness"),
		OwnerUserID:    ownerUserID,
		CaseID:         sourceCase.ID,
		RunID:          run.ID,
		SourceRunID:    sourceCase.SourceRunID,
		SourceTraceID:  sourceCase.SourceTraceID,
		IdempotencyKey: idempotencyKey,
		Runtime:        cloneJSON(runtime),
		CreatedAt:      now,
	}
	storedRun, storedHarness, created, err := m.store.CreateReplay(ctx, run, harness)
	if err != nil {
		return Run{}, HarnessVersion{}, false, err
	}
	if created {
		go m.execute(context.WithoutCancel(ctx), storedRun)
	}
	return storedRun, storedHarness, created, nil
}

func (m *RunnerManager) Cancel(ctx context.Context, runID string) (Run, error) {
	run, err := m.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if run.State.Terminal() {
		if run.State == StateCancelled {
			return run, nil
		}
		return Run{}, ErrConflict
	}
	run.CancelRequested = true
	run.State = StateCancelled
	run.UpdatedAt = time.Now().UTC()
	if err := m.store.UpdateRun(ctx, run); err != nil {
		return Run{}, err
	}

	m.processMu.Lock()
	cmd := m.processes[runID]
	m.processMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = interruptCommand(cmd)
		time.AfterFunc(m.config.KillGrace, func() {
			m.processMu.Lock()
			active := m.processes[runID] == cmd
			m.processMu.Unlock()
			if active {
				_ = killCommand(cmd)
			}
		})
	}
	return run, nil
}

func (m *RunnerManager) execute(ctx context.Context, initial Run) {
	run, err := m.store.GetRun(ctx, initial.ID)
	if err != nil || run.CancelRequested {
		return
	}
	run.State = StateRunning
	run.CurrentPhase = "starting"
	run.CurrentActor = "engine"
	run.UpdatedAt = time.Now().UTC()
	if err := m.store.UpdateRun(ctx, run); err != nil {
		return
	}

	request := EngineRequest{
		Schema:    "barena.engine_request.v1",
		RequestID: run.RequestID,
		RunID:     run.ID,
		Operation: run.Operation,
		RunsRoot:  m.config.RunsRoot,
		Input:     run.Input,
		Runtime:   run.Runtime,
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		m.failRun(ctx, run.ID, err)
		return
	}
	cmd := exec.Command(m.config.NodeCommand, m.config.WorkerPath)
	prepareCommand(cmd)
	cmd.Stdin = bytes.NewReader(requestBytes)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.failRun(ctx, run.ID, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.failRun(ctx, run.ID, err)
		return
	}
	if err := cmd.Start(); err != nil {
		m.failRun(ctx, run.ID, err)
		return
	}
	m.processMu.Lock()
	m.processes[run.ID] = cmd
	m.processMu.Unlock()
	if latest, latestErr := m.store.GetRun(ctx, run.ID); latestErr == nil &&
		(latest.CancelRequested || latest.State == StateCancelled) {
		_ = interruptCommand(cmd)
		time.AfterFunc(m.config.KillGrace, func() {
			m.processMu.Lock()
			active := m.processes[run.ID] == cmd
			m.processMu.Unlock()
			if active {
				_ = killCommand(cmd)
			}
		})
	}

	stderrResult := make(chan string, 1)
	go func() {
		var buffer limitedBuffer
		_, _ = io.Copy(&buffer, stderr)
		stderrResult <- buffer.String()
	}()

	terminalEvent, scanErr := m.consumeEvents(ctx, run, stdout)
	waitErr := cmd.Wait()
	stderrText := <-stderrResult
	m.processMu.Lock()
	delete(m.processes, run.ID)
	m.processMu.Unlock()

	current, err := m.store.GetRun(ctx, run.ID)
	if err != nil {
		return
	}
	if current.CancelRequested || current.State == StateCancelled {
		current.State = StateCancelled
		current.UpdatedAt = time.Now().UTC()
		_ = m.store.UpdateRun(ctx, current)
		return
	}
	if scanErr != nil {
		m.failRun(ctx, run.ID, scanErr)
		return
	}
	runPackage, packageErr := readVerifiedRunPackage(m.config.RunsRoot, run.ID)
	if waitErr == nil && packageErr == nil && runPackage.Status == "complete" {
		if run.Operation == OperationReplay {
			_, harnessErr := m.store.GetHarnessVersionByRun(ctx, run.ID)
			if harnessErr == nil {
				fact, factErr := replayFactFromTerminal(terminalEvent, runPackage)
				if factErr != nil {
					m.failRun(ctx, run.ID, factErr)
					return
				}
				if _, _, _, finalizeErr := m.store.FinalizeReplay(
					ctx,
					run.ID,
					fact,
					time.Now().UTC(),
				); finalizeErr != nil {
					m.failRun(ctx, run.ID, finalizeErr)
				}
				return
			}
			if !errors.Is(harnessErr, ErrNotFound) {
				m.failRun(ctx, run.ID, harnessErr)
				return
			}
		}
		current.State = StateCompleted
		current.CurrentPhase = "complete"
		current.CurrentActor = "engine"
		current.UpdatedAt = time.Now().UTC()
		_ = m.store.UpdateRun(ctx, current)
		return
	}
	detail := stderrText
	if detail == "" && packageErr != nil {
		detail = packageErr.Error()
	}
	if detail == "" && waitErr != nil {
		detail = waitErr.Error()
	}
	if detail == "" {
		detail = "Engine Worker ended without a complete Run package"
	}
	m.failRun(ctx, run.ID, errors.New(detail))
}

func (m *RunnerManager) consumeEvents(
	ctx context.Context,
	run Run,
	stdout io.Reader,
) (EngineEvent, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var last EngineEvent
	for scanner.Scan() {
		var event EngineEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return EngineEvent{}, fmt.Errorf("invalid Engine Event JSON: %w", err)
		}
		if err := event.Validate(run); err != nil {
			return EngineEvent{}, err
		}
		if err := m.store.AppendEvent(ctx, event); err != nil {
			return EngineEvent{}, err
		}
		last = event
		current, err := m.store.GetRun(ctx, run.ID)
		if err != nil {
			return EngineEvent{}, err
		}
		if current.CancelRequested || current.State == StateCancelled {
			continue
		}
		current.CurrentPhase = event.Phase
		current.CurrentActor = event.Actor
		current.UpdatedAt = time.Now().UTC()
		if err := m.store.UpdateRun(ctx, current); err != nil {
			return EngineEvent{}, err
		}
	}
	return last, scanner.Err()
}

func (m *RunnerManager) failRun(ctx context.Context, runID string, cause error) {
	run, err := m.store.GetRun(ctx, runID)
	if err != nil || run.State == StateCancelled {
		return
	}
	run.State = StateFailed
	run.Error = bounded(cause.Error(), 2000)
	run.UpdatedAt = time.Now().UTC()
	_ = m.store.UpdateRun(ctx, run)
}

type verifiedRunPackage struct {
	Status    string
	ResultRef string
}

func readPackageStatus(runsRoot, runID string) (string, error) {
	value, err := readVerifiedRunPackage(runsRoot, runID)
	return value.Status, err
}

func readVerifiedRunPackage(runsRoot, runID string) (verifiedRunPackage, error) {
	runRoot := filepath.Join(runsRoot, runID)
	bytes, err := os.ReadFile(filepath.Join(runRoot, "run-package.json"))
	if err != nil {
		return verifiedRunPackage{}, err
	}
	var value struct {
		Schema    string `json:"schema"`
		RunID     string `json:"run_id"`
		Status    string `json:"status"`
		ResultRef string `json:"result_ref"`
		Files     []struct {
			Ref    string `json:"ref"`
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(bytes, &value); err != nil {
		return verifiedRunPackage{}, err
	}
	if value.Schema != "barena.run_package.v1" || value.RunID != runID {
		return verifiedRunPackage{}, errors.New("Run package identity is invalid")
	}
	if len(value.Files) == 0 {
		return verifiedRunPackage{}, errors.New("Run package has no files")
	}
	resultListed := false
	seen := make(map[string]bool)
	realRoot, err := filepath.EvalSymlinks(runRoot)
	if err != nil {
		return verifiedRunPackage{}, err
	}
	for _, file := range value.Files {
		if seen[file.Ref] {
			return verifiedRunPackage{}, fmt.Errorf("Run package repeats %s", file.Ref)
		}
		seen[file.Ref] = true
		if file.Ref == value.ResultRef {
			resultListed = true
		}
		if !safePackageRef(file.Ref) {
			return verifiedRunPackage{}, fmt.Errorf("Run package ref is unsafe: %s", file.Ref)
		}
		current := runRoot
		for _, segment := range strings.Split(file.Ref, "/") {
			current = filepath.Join(current, segment)
			info, statErr := os.Lstat(current)
			if statErr != nil {
				return verifiedRunPackage{}, statErr
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return verifiedRunPackage{}, fmt.Errorf("Run package ref contains a symlink: %s", file.Ref)
			}
		}
		realFile, evalErr := filepath.EvalSymlinks(current)
		if evalErr != nil {
			return verifiedRunPackage{}, evalErr
		}
		relative, relativeErr := filepath.Rel(realRoot, realFile)
		if relativeErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return verifiedRunPackage{}, fmt.Errorf("Run package ref escapes the Run: %s", file.Ref)
		}
		content, readErr := os.ReadFile(realFile)
		if readErr != nil {
			return verifiedRunPackage{}, readErr
		}
		if int64(len(content)) != file.Size {
			return verifiedRunPackage{}, fmt.Errorf("Run package size mismatch for %s", file.Ref)
		}
		hash := sha256.Sum256(content)
		if hex.EncodeToString(hash[:]) != file.SHA256 {
			return verifiedRunPackage{}, fmt.Errorf("Run package hash mismatch for %s", file.Ref)
		}
	}
	if !resultListed {
		return verifiedRunPackage{}, errors.New("Run package result_ref is not listed")
	}
	return verifiedRunPackage{Status: value.Status, ResultRef: value.ResultRef}, nil
}

func replayFactFromTerminal(
	event EngineEvent,
	runPackage verifiedRunPackage,
) (ReplayFact, error) {
	if event.Kind != "terminal" ||
		event.Phase != "complete" ||
		event.Actor != "engine" {
		return ReplayFact{}, errors.New("Replay requires a final terminal Engine Event")
	}
	if !validOTelTraceID(event.TraceID) {
		return ReplayFact{}, errors.New("Replay terminal Engine Event requires a retained trace_id")
	}
	var payload struct {
		Status       string          `json:"status"`
		ResultStatus string          `json:"result_status"`
		Decision     ReleaseDecision `json:"decision"`
		Summary      string          `json:"summary"`
		ResultRef    string          `json:"result_ref"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return ReplayFact{}, fmt.Errorf("invalid terminal Engine Event payload: %w", err)
	}
	if payload.Status != "complete" || payload.Status != runPackage.Status {
		return ReplayFact{}, errors.New("terminal Engine Event and Run Package status do not match")
	}
	if !payload.Decision.Valid() {
		return ReplayFact{}, errors.New("terminal Engine decision must be cleared, held, or rejected")
	}
	if payload.ResultRef == "" || payload.ResultRef != runPackage.ResultRef {
		return ReplayFact{}, errors.New("terminal Engine Event and Run Package result_ref do not match")
	}
	if strings.TrimSpace(payload.ResultStatus) == "" ||
		len(payload.ResultStatus) > 128 ||
		strings.TrimSpace(payload.Summary) == "" ||
		len(payload.Summary) > 12000 {
		return ReplayFact{}, errors.New("terminal Engine result status and summary are required within audit limits")
	}
	return ReplayFact{
		TerminalEventID: event.EventID,
		ReplayTraceID:   event.TraceID,
		PackageStatus:   runPackage.Status,
		ResultStatus:    payload.ResultStatus,
		Decision:        payload.Decision,
		Summary:         payload.Summary,
		ResultRef:       runPackage.ResultRef,
	}, nil
}

func safePackageRef(ref string) bool {
	if ref == "" || strings.Contains(ref, "\\") || strings.HasPrefix(ref, "/") {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref)))
	return cleaned == ref && ref != "." && ref != ".." && !strings.HasPrefix(ref, "../")
}

func validOTelTraceID(value string) bool {
	if len(value) != 32 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	for _, current := range decoded {
		if current != 0 {
			return true
		}
	}
	return false
}

type limitedBuffer struct {
	bytes []byte
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	const max = 64 * 1024
	remaining := max - len(b.bytes)
	if remaining > 0 {
		if len(value) < remaining {
			remaining = len(value)
		}
		b.bytes = append(b.bytes, value[:remaining]...)
	}
	return len(value), nil
}

func (b *limitedBuffer) String() string {
	return string(b.bytes)
}

func newID(prefix string) string {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().UnixMilli(), hex.EncodeToString(entropy[:]))
}

func bounded(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
