package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type EvolutionWorkerConfig struct {
	Workers           int
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
	JobTimeout        time.Duration
	MaxAttempts       int
	RetryBase         time.Duration
	RetryMax          time.Duration
}

func DefaultEvolutionWorkerConfig() EvolutionWorkerConfig {
	return EvolutionWorkerConfig{Workers: 2, LeaseTTL: 30 * time.Second, HeartbeatInterval: 5 * time.Second, PollInterval: 500 * time.Millisecond, JobTimeout: 10 * time.Minute, MaxAttempts: 3, RetryBase: time.Second, RetryMax: 30 * time.Second}
}

func EvolutionWorkerConfigFromEnv(get func(string) string) (EvolutionWorkerConfig, error) {
	c := DefaultEvolutionWorkerConfig()
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

func (c EvolutionWorkerConfig) validate() error {
	if c.Workers < 1 || c.Workers > 32 || c.MaxAttempts < 1 || c.MaxAttempts > 20 || c.LeaseTTL < 100*time.Millisecond || c.LeaseTTL > 10*time.Minute || c.HeartbeatInterval <= 0 || c.HeartbeatInterval >= c.LeaseTTL/2 || c.PollInterval < time.Millisecond || c.JobTimeout <= 0 || c.JobTimeout > time.Hour || c.RetryBase <= 0 || c.RetryMax < c.RetryBase || c.RetryMax > time.Hour {
		return errors.New("invalid Evolution worker limits")
	}
	return nil
}

type EvolutionWorkerCounters struct {
	Claims         atomic.Uint64
	Completed      atomic.Uint64
	FailedAttempts atomic.Uint64
	LeaseLosses    atomic.Uint64
	StoreErrors    atomic.Uint64
	Active         atomic.Int64
}

type evolutionWorker struct {
	store    Store
	config   EvolutionWorkerConfig
	run      func(context.Context, EvolutionLease, EvolutionJob) error
	cancel   context.CancelFunc
	wake     chan struct{}
	wg       sync.WaitGroup
	counters EvolutionWorkerCounters
}

func newEvolutionWorker(store Store, config EvolutionWorkerConfig, run func(context.Context, EvolutionLease, EvolutionJob) error) (*evolutionWorker, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &evolutionWorker{store: store, config: config, run: run, cancel: cancel, wake: make(chan struct{}, config.Workers)}
	for i := 0; i < config.Workers; i++ {
		w.wg.Add(1)
		go w.loop(ctx, newID("job-worker"))
	}
	return w, nil
}

func (w *evolutionWorker) notify() {
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
func (w *evolutionWorker) close() {
	if w != nil {
		w.cancel()
		w.wg.Wait()
	}
}

func (w *evolutionWorker) wait(ctx context.Context, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-w.wake:
		return true
	case <-t.C:
		return true
	}
}

func waitEvolutionBackoff(ctx context.Context, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (w *evolutionWorker) loop(ctx context.Context, id string) {
	defer w.wg.Done()
	failures := 0
	for ctx.Err() == nil {
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		j, l, err := w.store.ClaimEvolutionJob(call, id, w.config.LeaseTTL, w.config.MaxAttempts)
		cancel()
		if err != nil {
			delay := w.config.PollInterval
			storeUnavailable := !errors.Is(err, ErrNotFound) && ctx.Err() == nil
			if storeUnavailable {
				failures++
				w.counters.StoreErrors.Add(1)
				delay = w.backoff(failures)
				if failures == 1 || failures%10 == 0 {
					slog.Warn("Evolution queue claim unavailable", "worker", id, "consecutive_failures", failures)
				}
			} else {
				failures = 0
			}
			// New HTTP submissions must not bypass database-outage backoff.
			keepRunning := false
			if storeUnavailable {
				keepRunning = waitEvolutionBackoff(ctx, delay)
			} else {
				keepRunning = w.wait(ctx, delay)
			}
			if !keepRunning {
				return
			}
			continue
		}
		failures = 0
		w.counters.Claims.Add(1)
		slog.Info("Evolution job claimed", "job_id", j.ID, "attempt", j.Attempt, "fence", l.Fence, "age_seconds", time.Since(j.CreatedAt).Seconds())
		w.attempt(ctx, l, j)
	}
}

func (w *evolutionWorker) backoff(attempt int) time.Duration {
	d := w.config.RetryBase
	for i := 1; i < attempt && d < w.config.RetryMax; i++ {
		d *= 2
	}
	if d > w.config.RetryMax {
		d = w.config.RetryMax
	}
	// Half-to-full jitter keeps the configured maximum a hard upper bound.
	return d/2 + time.Duration(rand.Int64N(max(int64(d-d/2), 1)))
}

func (w *evolutionWorker) attempt(parent context.Context, l EvolutionLease, j EvolutionJob) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, w.config.JobTimeout)
	w.counters.Active.Add(1)
	defer w.counters.Active.Add(-1)
	done := make(chan struct{})
	var heartbeatFailed atomic.Bool
	go func() {
		defer close(done)
		ticker := time.NewTicker(w.config.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				call, stop := context.WithTimeout(ctx, min(5*time.Second, w.config.HeartbeatInterval))
				requested, err := w.store.HeartbeatEvolutionJob(call, l, w.config.LeaseTTL)
				stop()
				if err != nil || requested {
					if err != nil {
						w.counters.LeaseLosses.Add(1)
						heartbeatFailed.Store(true)
					}
					cancel()
					return
				}
			}
		}
	}()
	err := func() (err error) {
		defer func() {
			if recover() != nil {
				err = errors.New("Evolution execution panicked")
			}
		}()
		return w.run(ctx, l, j)
	}()
	cancel()
	<-done
	if err == nil {
		w.counters.Completed.Add(1)
		slog.Info("Evolution job completed", "job_id", j.ID, "attempt", j.Attempt, "execution_seconds", time.Since(started).Seconds())
		return
	}
	w.counters.FailedAttempts.Add(1)
	// Shutdown leaves the lease to expire. Another process can safely acquire it;
	// never publish a terminal failure merely because this process is stopping.
	if parent.Err() != nil {
		return
	}
	retry := heartbeatFailed.Load() || errors.Is(err, errEvolutionTemporary) || errors.Is(err, context.DeadlineExceeded)
	call, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if releaseErr := w.store.ReleaseEvolutionJob(call, l, err.Error(), retry, w.backoff(j.Attempt), w.config.MaxAttempts); releaseErr != nil {
		if !errors.Is(releaseErr, ErrLeaseLost) {
			w.counters.StoreErrors.Add(1)
		}
		slog.Warn("Evolution job release deferred to lease recovery", "job_id", j.ID, "attempt", j.Attempt)
	}
}

// ManagedHTTPHandler owns background workers; callers must close it during
// shutdown. HTTP requests do not own task lifetime.
type ManagedHTTPHandler struct {
	http.Handler
	workers *evolutionWorker
}

func (h *ManagedHTTPHandler) Close() error { h.workers.close(); return nil }
