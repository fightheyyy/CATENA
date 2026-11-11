package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type AnalysisWorkerConfig struct {
	Workers           int
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
	JobTimeout        time.Duration
	MaxAttempts       int
	RetryBase         time.Duration
	RetryMax          time.Duration
}

func DefaultAnalysisWorkerConfig() AnalysisWorkerConfig {
	return AnalysisWorkerConfig{Workers: 1, LeaseTTL: 30 * time.Second, HeartbeatInterval: 5 * time.Second, PollInterval: 500 * time.Millisecond, JobTimeout: 20 * time.Minute, MaxAttempts: 3, RetryBase: time.Second, RetryMax: 30 * time.Second}
}

func AnalysisWorkerConfigFromEnv(get func(string) string) (AnalysisWorkerConfig, error) {
	c := DefaultAnalysisWorkerConfig()
	for name, dst := range map[string]*int{"CATENA_JOB_WORKERS": &c.Workers, "CATENA_JOB_MAX_ATTEMPTS": &c.MaxAttempts} {
		if v := get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return c, fmt.Errorf("invalid %s", name)
			}
			*dst = n
		}
	}
	for name, dst := range map[string]*time.Duration{"CATENA_JOB_LEASE_TTL": &c.LeaseTTL, "CATENA_JOB_HEARTBEAT": &c.HeartbeatInterval, "CATENA_JOB_POLL_INTERVAL": &c.PollInterval, "CATENA_JOB_TIMEOUT": &c.JobTimeout, "CATENA_JOB_RETRY_BASE": &c.RetryBase, "CATENA_JOB_RETRY_MAX": &c.RetryMax} {
		if v := get(name); v != "" {
			n, err := time.ParseDuration(v)
			if err != nil {
				return c, fmt.Errorf("invalid %s", name)
			}
			*dst = n
		}
	}
	return c, c.validate()
}

func (c AnalysisWorkerConfig) validate() error {
	if c.Workers < 1 || c.Workers > 32 || c.MaxAttempts < 1 || c.MaxAttempts > 20 || c.LeaseTTL < 100*time.Millisecond || c.LeaseTTL > 10*time.Minute || c.HeartbeatInterval <= 0 || c.HeartbeatInterval >= c.LeaseTTL/2 || c.PollInterval < time.Millisecond || c.JobTimeout <= 0 || c.JobTimeout > time.Hour || c.RetryBase <= 0 || c.RetryMax < c.RetryBase || c.RetryMax > time.Hour {
		return errors.New("invalid Analysis worker limits")
	}
	return nil
}

type AnalysisWorkerCounters struct {
	Claims         atomic.Uint64
	Completed      atomic.Uint64
	FailedAttempts atomic.Uint64
	LeaseLosses    atomic.Uint64
	StoreErrors    atomic.Uint64
	Active         atomic.Int64
}

type analysisWorker struct {
	store    Store
	config   AnalysisWorkerConfig
	run      func(context.Context, AnalysisLease, AnalysisJob) error
	pool     schedulerWorkers
	wake     chan struct{}
	counters AnalysisWorkerCounters
}

func newAnalysisWorker(store Store, config AnalysisWorkerConfig, run func(context.Context, AnalysisLease, AnalysisJob) error) (*analysisWorker, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	w := &analysisWorker{store: store, config: config, run: run, wake: make(chan struct{}, config.Workers)}
	w.pool.start()
	for i := 0; i < config.Workers; i++ {
		w.pool.launch(func(ctx context.Context) { w.loop(ctx, newID("job-worker")) })
	}
	return w, nil
}

func (w *analysisWorker) notify() {
	if w == nil {
		return
	}
	for i := 0; i < w.config.Workers; i++ {
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}
func (w *analysisWorker) close() {
	if w != nil {
		w.pool.close()
	}
}

func (w *analysisWorker) loop(ctx context.Context, id string) {
	pollScheduler(ctx, w.config.PollInterval, w.config.RetryBase, w.config.RetryMax, w.wake,
		func(call context.Context) (func(context.Context), error) {
			j, l, err := w.store.ClaimAnalysisJob(call, id, w.config.LeaseTTL, w.config.MaxAttempts)
			if err != nil {
				return nil, err
			}
			return func(ctx context.Context) {
				w.counters.Claims.Add(1)
				slog.Info("Analysis job claimed", "job_id", j.ID, "attempt", j.Attempt, "fence", l.Fence, "age_seconds", time.Since(j.CreatedAt).Seconds())
				w.attempt(ctx, l, j)
			}, nil
		}, func(_ error, failures int) {
			w.counters.StoreErrors.Add(1)
			if failures == 1 || failures%10 == 0 {
				slog.Warn("Analysis queue claim unavailable", "worker", id, "consecutive_failures", failures)
			}
		})
}

func (w *analysisWorker) attempt(parent context.Context, l AnalysisLease, j AnalysisJob) {
	started := time.Now()
	w.counters.Active.Add(1)
	defer w.counters.Active.Add(-1)
	var heartbeatFailed atomic.Bool
	err := runWithLease(parent, w.config.JobTimeout, w.config.HeartbeatInterval,
		func(ctx context.Context) bool {
			requested, err := w.store.HeartbeatAnalysisJob(ctx, l, w.config.LeaseTTL)
			if err != nil {
				w.counters.LeaseLosses.Add(1)
				heartbeatFailed.Store(true)
			}
			return err == nil && !requested
		}, func(ctx context.Context) (err error) {
			defer func() {
				if recover() != nil {
					err = errors.New("Analysis execution panicked")
				}
			}()
			return w.run(ctx, l, j)
		})
	if err == nil {
		w.counters.Completed.Add(1)
		slog.Info("Analysis job completed", "job_id", j.ID, "attempt", j.Attempt, "execution_seconds", time.Since(started).Seconds())
		return
	}
	w.counters.FailedAttempts.Add(1)
	// Shutdown leaves the lease to expire. Another process can safely acquire it;
	// never publish a terminal failure merely because this process is stopping.
	if parent.Err() != nil {
		return
	}
	retry := heartbeatFailed.Load() || errors.Is(err, errAnalysisTemporary) || errors.Is(err, context.DeadlineExceeded)
	call, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if releaseErr := w.store.ReleaseAnalysisJob(call, l, err.Error(), retry, schedulerBackoff(j.Attempt, w.config.RetryBase, w.config.RetryMax), w.config.MaxAttempts); releaseErr != nil {
		if !errors.Is(releaseErr, ErrLeaseLost) {
			w.counters.StoreErrors.Add(1)
		}
		slog.Warn("Analysis job release deferred to lease recovery", "job_id", j.ID, "attempt", j.Attempt)
	}
}

// ManagedHTTPHandler owns background workers; callers must close it during
// shutdown. HTTP requests do not own task lifetime.
type ManagedHTTPHandler struct {
	http.Handler
	workers *analysisWorker
	managed *managedPlatform
}

func (h *ManagedHTTPHandler) Close() error { h.workers.close(); h.managed.close(); return nil }
