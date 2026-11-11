package control

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

type schedulerWorkers struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (w *schedulerWorkers) start() {
	w.ctx, w.cancel = context.WithCancel(context.Background())
}

func (w *schedulerWorkers) launch(run func(context.Context)) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		run(w.ctx)
	}()
}

func (w *schedulerWorkers) close() {
	if w.cancel != nil {
		w.cancel()
		w.wg.Wait()
	}
}

func waitScheduler(ctx context.Context, delay time.Duration, wake <-chan struct{}) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-wake:
		return true
	case <-t.C:
		return true
	}
}

func schedulerBackoff(attempt int, base, limit time.Duration) time.Duration {
	d := base
	for i := 1; i < attempt && d < limit; i++ {
		d = min(d*2, limit)
	}
	return d/2 + time.Duration(rand.Int64N(max(int64(d-d/2), 1)))
}

func pollScheduler(ctx context.Context, poll, base, limit time.Duration, wake <-chan struct{},
	claim func(context.Context) (func(context.Context), error), onFailure func(error, int)) {
	failures := 0
	for ctx.Err() == nil {
		call, stop := context.WithTimeout(ctx, 5*time.Second)
		run, err := claim(call)
		stop()
		if ctx.Err() != nil {
			return
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			failures++
			onFailure(err, failures)
			// Submission signals cannot bypass outage backoff.
			if !waitScheduler(ctx, schedulerBackoff(failures, base, limit), nil) {
				return
			}
			continue
		}
		failures = 0
		if run != nil {
			run(ctx)
		} else if !waitScheduler(ctx, poll, wake) {
			return
		}
	}
}

func runWithLease(parent context.Context, timeout, interval time.Duration,
	renew func(context.Context) bool, run func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				call, stop := context.WithTimeout(ctx, min(5*time.Second, interval))
				keep := renew(call)
				stop()
				if !keep {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	return run(ctx)
}
