package control

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Admission is process-local. The deployment must budget these limits across
// replicas; it is not a distributed rate limiter or a durable ingest queue.
type TraceIngestConfig struct {
	Concurrency int
	PerOwner    int
	MaxSpans    int
	Timeout     time.Duration
}

func DefaultTraceIngestConfig() TraceIngestConfig {
	return TraceIngestConfig{Concurrency: 4, PerOwner: 2, MaxSpans: 4096, Timeout: 15 * time.Second}
}

func TraceIngestConfigFromEnv(get func(string) string) (TraceIngestConfig, error) {
	c := DefaultTraceIngestConfig()
	for name, dst := range map[string]*int{"CATENA_INGEST_CONCURRENCY": &c.Concurrency, "CATENA_INGEST_PER_OWNER": &c.PerOwner, "CATENA_INGEST_MAX_SPANS": &c.MaxSpans} {
		if v := get(name); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil {
				return c, fmt.Errorf("invalid %s", name)
			}
			*dst = n
		}
	}
	if v := get("CATENA_INGEST_TIMEOUT"); v != "" {
		n, e := time.ParseDuration(v)
		if e != nil {
			return c, errors.New("invalid CATENA_INGEST_TIMEOUT")
		}
		c.Timeout = n
	}
	return c, c.validate()
}

func (c TraceIngestConfig) validate() error {
	if c.Concurrency < 1 || c.Concurrency > 64 || c.PerOwner < 1 || c.PerOwner > c.Concurrency || c.MaxSpans < 1 || c.MaxSpans > 65536 || c.Timeout < time.Second || c.Timeout > time.Minute {
		return errors.New("invalid Trace ingestion limits")
	}
	return nil
}

type traceAdmission struct {
	mu     sync.Mutex
	config TraceIngestConfig
	active int
	owners map[string]int
}

func (a *traceAdmission) acquire(owner string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active >= a.config.Concurrency || a.owners[owner] >= a.config.PerOwner {
		return false
	}
	a.active++
	a.owners[owner]++
	return true
}

func (a *traceAdmission) release(owner string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active--
	a.owners[owner]--
	if a.owners[owner] == 0 {
		delete(a.owners, owner)
	}
}
