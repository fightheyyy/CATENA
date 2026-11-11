package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ManagedConfig struct {
	URL             string
	Token           string
	ControlURL      string
	Workers         int
	Capacity        int
	Lease           time.Duration
	Idle            time.Duration
	ProviderTimeout time.Duration
	OperationLease  time.Duration
	Namespace       string
}

func ManagedConfigFromEnv() (ManagedConfig, error) {
	c := ManagedConfig{URL: os.Getenv("CATENA_MANAGED_RUNNER_URL"), Token: os.Getenv("CATENA_MANAGED_RUNNER_TOKEN"), ControlURL: os.Getenv("CATENA_MANAGED_CONTROL_URL"), Workers: 2, Capacity: 4, Lease: 30 * time.Second, Idle: 5 * time.Minute, ProviderTimeout: 10 * time.Second, OperationLease: 30 * time.Second}
	if c.URL == "" {
		return c, nil
	}
	for k, dst := range map[string]*int{"CATENA_MANAGED_WORKERS": &c.Workers, "CATENA_MANAGED_CAPACITY": &c.Capacity} {
		if v := os.Getenv(k); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil {
				return c, e
			}
			*dst = n
		}
	}
	for k, dst := range map[string]*time.Duration{"CATENA_MANAGED_LEASE": &c.Lease, "CATENA_MANAGED_IDLE": &c.Idle, "CATENA_MANAGED_PROVIDER_TIMEOUT": &c.ProviderTimeout, "CATENA_MANAGED_OPERATION_LEASE": &c.OperationLease} {
		if v := os.Getenv(k); v != "" {
			n, e := time.ParseDuration(v)
			if e != nil {
				return c, e
			}
			*dst = n
		}
	}
	return c, c.validate()
}
func (c ManagedConfig) validate() error {
	if (c.ProviderTimeout != 0 && (c.ProviderTimeout < time.Millisecond || c.ProviderTimeout > time.Minute)) || (c.OperationLease != 0 && c.OperationLease <= c.ProviderTimeout) {
		return errors.New("operation lease must exceed provider timeout")
	}
	if c.Workers < 1 || c.Workers > 8 || c.Capacity < 1 || c.Capacity > 8 || len(c.Token) < 32 || c.Lease < time.Second || c.Lease > time.Minute || c.Idle < time.Second {
		return errors.New("invalid managed worker limits or token (32+ characters required)")
	}
	for _, v := range []string{c.URL, c.ControlURL} {
		u, e := url.Parse(v)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return errors.New("managed endpoints must be absolute HTTP(S) URLs")
		}
	}
	u, _ := url.Parse(c.ControlURL)
	if net.ParseIP(u.Hostname()) == nil || u.Port() == "" {
		return errors.New("managed model gateway must use the measured Docker host IP and explicit port; DNS egress is disabled")
	}
	return nil
}

type managedPlatform struct {
	store  *PostgresStore
	server *HTTPServer
	config ManagedConfig
	client *http.Client
	pool   schedulerWorkers
}

func startManaged(s *HTTPServer, c ManagedConfig) (*managedPlatform, error) {
	if c.URL == "" {
		return nil, nil
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	store, ok := s.store.(*PostgresStore)
	if !ok {
		return nil, errors.New("managed execution requires PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	namespace, err := randomURLToken(18)
	if err != nil {
		cancel()
		return nil, err
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO managed_settings(key,value) VALUES('namespace',$1) ON CONFLICT DO NOTHING`, namespace)
	if err == nil {
		err = store.db.QueryRowContext(ctx, `SELECT value FROM managed_settings WHERE key='namespace'`).Scan(&c.Namespace)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	p := &managedPlatform{store: store, server: s, config: c, client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return p, nil
}
func (p *managedPlatform) start() {
	if p == nil {
		return
	}
	p.pool.start()
	for i := 0; i < p.config.Workers; i++ {
		p.pool.launch(func(ctx context.Context) { p.loop(ctx, newID("managed-worker"), p.config.Capacity, false) })
	}
	if p.server.analysisRuntime != nil {
		p.pool.launch(func(ctx context.Context) { p.loop(ctx, newID("workflow-worker"), 1, true) })
	}
	p.pool.launch(func(ctx context.Context) {
		for {
			opctx, stop := context.WithTimeout(ctx, 45*time.Second)
			if e := p.reconcile(opctx); e != nil && ctx.Err() == nil {
				slog.Warn("managed reconcile", "error", e)
			}
			stop()
			traceCtx, traceStop := context.WithTimeout(ctx, 30*time.Second)
			if e := p.exportManagedTraces(traceCtx); e != nil && ctx.Err() == nil {
				slog.Warn("managed trace export", "error", e)
			}
			traceStop()
			if !waitScheduler(ctx, time.Second, nil) {
				return
			}
		}
	})
}
func (p *managedPlatform) close() {
	if p != nil {
		p.pool.close()
	}
}
func (p *managedPlatform) request(ctx context.Context, action sandboxAction, payload any) (*http.Response, error) {
	raw, e := json.Marshal(payload)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(p.config.URL, "/")+"/v1/"+string(action), bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+p.config.Token)
	req.Header.Set("Content-Type", "application/json")
	res, e := p.client.Do(req)
	if e != nil {
		return nil, e
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("managed runner %s returned %d", action, res.StatusCode)
	}
	return res, nil
}
func (p *managedPlatform) call(ctx context.Context, action sandboxAction, payload, out any) error {
	res, e := p.request(ctx, action, payload)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	return json.NewDecoder(io.LimitReader(res.Body, 24*1024*1024)).Decode(out)
}
func (p *managedPlatform) loop(ctx context.Context, id string, capacity int, ephemeral bool) {
	pollScheduler(ctx, 500*time.Millisecond, time.Second, 30*time.Second, nil,
		func(call context.Context) (func(context.Context), error) {
			l, err := p.store.claimHarness(call, id, p.config.Lease, capacity, ephemeral)
			if err != nil || l == nil {
				return nil, err
			}
			return func(ctx context.Context) { p.execute(ctx, l) }, nil
		}, func(err error, failures int) {
			if failures == 1 || failures%10 == 0 {
				slog.Warn("managed claim", "error", err, "consecutive_failures", failures)
			}
		})
}

func (p *managedPlatform) execute(parent context.Context, l *managedLease) {
	_ = runWithLease(parent, time.Duration(l.Run.Timeout)*time.Second, p.config.Lease/4,
		func(ctx context.Context) bool { return p.store.heartbeatManaged(ctx, l, p.config.Lease) == nil },
		func(ctx context.Context) error {
			p.executeLeased(ctx, l)
			return nil
		})
}

func (p *managedPlatform) executeLeased(ctx context.Context, l *managedLease) {
	allocatedAt := time.Now()
	fail := func(e error) {
		failureCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		slog.Warn("managed execution interrupted; reconciler will isolate before recovery", "run", l.Run.ID, "error", e)
		_, _ = p.store.db.ExecContext(failureCtx, `UPDATE agent_runs SET lease_until=now(),credential_hash=NULL,error=$4 WHERE run_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='RUNNING'`, l.Run.ID, l.Run.Worker, l.Run.Fence, bounded(redactEvidenceText(e.Error()), 1000))
	}
	if e := p.store.managedEvent(ctx, l, "claimed", map[string]any{"attempt": l.Run.Attempt, "sandbox_id": l.Box.ID}); e != nil {
		fail(e)
		return
	}
	rebuild := l.Box.Provider == ""
	if rebuild {
		e := managedCAS(p.store.db.ExecContext(ctx, `UPDATE managed_sandboxes SET state='CREATING' WHERE sandbox_id=$1 AND state='PENDING'`, l.Box.ID))
		if e != nil {
			fail(e)
			return
		}
		_, e = p.store.db.ExecContext(ctx, `INSERT INTO sandbox_operations(operation_id,sandbox_id,kind,state,attempts) VALUES($1,$1,'create','PENDING',1)`, l.Box.ID)
		if e != nil {
			fail(e)
			return
		}
		payload := sandboxCreateRequest{sandboxBinding: p.binding(l), CPU: l.Box.CPU, Memory: l.Box.Memory}
		var box sandboxRef
		if e = p.call(ctx, sandboxCreate, payload, &box); e != nil || box.ProviderID == "" {
			if e == nil {
				e = errors.New("sandbox create returned an empty provider ID")
			}
			writeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = p.store.db.ExecContext(writeCtx, `UPDATE sandbox_operations SET state='UNKNOWN',error='create outcome unknown',updated_at=now() WHERE operation_id=$1`, l.Box.ID)
			stop()
			fail(e)
			return
		}
		l.Box.Provider = box.ProviderID
		e = managedCAS(p.store.db.ExecContext(ctx, `UPDATE managed_sandboxes SET provider_sandbox_id=$2,state='BUSY' WHERE sandbox_id=$1 AND state='CREATING' AND EXISTS(SELECT 1 FROM agent_runs WHERE run_id=$3 AND fencing_token=$4 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=agent_runs.session_id))`, l.Box.ID, box.ProviderID, l.Run.ID, l.Run.Fence))
		if e != nil {
			fail(e)
			return
		}
		_, e = p.store.db.ExecContext(ctx, `UPDATE sandbox_operations SET state='SUCCEEDED',updated_at=now() WHERE operation_id=$1`, l.Box.ID)
		if e != nil {
			fail(e)
			return
		}
	}
	_, e := p.store.db.ExecContext(ctx, `UPDATE agent_sessions SET sandbox_binding=$2,updated_at=now() WHERE session_id=$1`, l.Run.Session, l.Box.ID)
	if e != nil {
		fail(e)
		return
	}
	if e := p.store.managedEvent(ctx, l, "sandbox.ready", map[string]any{"sandbox_id": l.Box.ID, "reused": !rebuild, "allocation_ms": time.Since(allocatedAt).Milliseconds()}); e != nil {
		fail(e)
		return
	}
	credentials, e := p.server.analysisModelCredentials(ctx, l.Run.Tenant)
	if e != nil {
		fail(e)
		return
	}
	base := strings.TrimRight(p.config.ControlURL, "/")
	payload := sandboxTurn{
		sandboxBinding: p.binding(l), Input: l.Run.Input, Profile: l.Agent.Profile, ProfileVersion: l.Agent.Version,
		Instructions: l.Agent.Instructions, ToolPolicy: l.Agent.ToolPolicy, Skills: l.Agent.Skills, Knowledge: l.Agent.Knowledge,
		Model: credentials.Model, Timeout: l.Run.Timeout, Gateway: base + "/internal/managed/model",
		ToolGateway: base + "/internal/harness/tool",
	}
	if rebuild {
		payload.Checkpoint = base64.StdEncoding.EncodeToString(l.Checkpoint)
	}
	e = managedCAS(p.store.db.ExecContext(ctx, `UPDATE agent_runs SET execution_started_at=now() WHERE run_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=agent_runs.session_id) AND execution_started_at IS NULL`, l.Run.ID, l.Run.Worker, l.Run.Fence))
	if e != nil {
		fail(e)
		return
	}
	res, e := p.request(ctx, sandboxExecute, payload)
	if e != nil {
		fail(e)
		return
	}
	defer res.Body.Close()
	if e = p.consumeManagedExecution(ctx, l, res.Body); e != nil {
		fail(e)
	}
}

func (p *managedPlatform) consumeManagedExecution(ctx context.Context, l *managedLease, body io.Reader) error {
	limited := &io.LimitedReader{R: body, N: 32*1024*1024 + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 24*1024*1024)
	type frame struct {
		Kind     string          `json:"kind"`
		Data     json.RawMessage `json:"data"`
		Result   string          `json:"result"`
		Snapshot string          `json:"snapshot"`
		Success  bool            `json:"success"`
		Error    string          `json:"error"`
		Exited   bool            `json:"command_exited"`
		ExitCode *int            `json:"exit_code"`
	}
	var final *frame
	for scanner.Scan() {
		var event frame
		if e := json.Unmarshal(scanner.Bytes(), &event); e != nil {
			return e
		}
		if final != nil {
			return errors.New("unexpected frame after command exit")
		}
		if event.Kind == "result" {
			if !event.Exited || event.ExitCode == nil || *event.ExitCode != 0 {
				return errors.New("unconfirmed sandbox command exit")
			}
			final = &event
			continue
		}
		clean := redactEvidenceText(strings.ReplaceAll(string(event.Data), l.Token, "[REDACTED]"))
		var data any
		if len(clean) > 48*1024 || json.Unmarshal([]byte(clean), &data) != nil {
			data = map[string]string{"text": bounded(clean, 48*1024)}
		}
		if e := p.store.managedEvent(ctx, l, "runtime", data); e != nil {
			return e
		}
		if event.Kind == "adapter.error" {
			return fmt.Errorf("adapter execution failed: %s", bounded(clean, 1000))
		}
	}
	if e := scanner.Err(); e != nil {
		return e
	}
	if limited.N == 0 {
		return errors.New("execution stream exceeded size limit before EOF")
	}
	if final == nil {
		return errors.New("runtime disconnected without confirmed command exit")
	}
	var snapshot []byte
	if l.Ephemeral {
		if final.Snapshot != "" || len(final.Result) > 256*1024 || !json.Valid([]byte(final.Result)) {
			return errors.New("invalid ephemeral turn result")
		}
	} else {
		var err error
		snapshot, err = base64.StdEncoding.DecodeString(final.Snapshot)
		if err != nil || len(snapshot) == 0 || len(snapshot) > 16*1024*1024 {
			return errors.New("checkpoint missing or invalid")
		}
	}
	// EOF and the adapter's SDK exit acknowledgement precede every terminal write.
	e := managedCAS(p.store.db.ExecContext(ctx, `UPDATE agent_runs SET command_exited_at=now() WHERE run_id=$1 AND worker_id=$2 AND fencing_token=$3 AND state='RUNNING' AND lease_until>now() AND NOT cancel_requested AND EXISTS(SELECT 1 FROM active_harness_sessions live WHERE live.session_id=agent_runs.session_id)`, l.Run.ID, l.Run.Worker, l.Run.Fence))
	if e != nil {
		return e
	}
	state := "COMPLETED"
	if !final.Success {
		state = "FAILED"
	}
	return p.store.finishManaged(ctx, l, state, strings.ReplaceAll(final.Result, l.Token, "[REDACTED]"), final.Error, snapshot)
}
