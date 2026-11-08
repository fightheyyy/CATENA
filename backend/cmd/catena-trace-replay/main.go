// Offline owner-scoped replay. Does not ALTER/drop the old table or write
// ClickHouse directly. Delivery uses the same official Collector pipeline.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fightheyyy/barena/platform/internal/control"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func run() error {
	owner := flag.String("owner", "", "required legacy owner ID; administrator-only offline tool")
	after := flag.String("after-trace", "", "last successfully replayed Trace ID (resume cursor)")
	page := flag.Int("page-size", 100, "Trace IDs per read page (1..1000)")
	flag.Parse()
	if *owner == "" {
		return errors.New("--owner is required; freeze old ingestion or use a stable snapshot before replay")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	reader, e := control.OpenLegacyTraceReader(ctx, os.Getenv("CATENA_CLICKHOUSE_DSN"))
	if e != nil {
		return e
	}
	defer reader.Close()
	sink, e := control.NewCollectorTraceSink(os.Getenv("CATENA_COLLECTOR_URL"))
	if e != nil {
		return e
	}
	cursor := *after
	traces, spans := 0, 0
	for {
		ids, e := reader.LegacyTraceIDs(ctx, *owner, cursor, *page)
		if e != nil {
			return e
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			detail, e := reader.GetTrace(ctx, *owner, id)
			if e != nil {
				return e
			}
			for begin := 0; begin < len(detail.Spans); {
				size := min(4096, len(detail.Spans)-begin)
				for {
					e = sink.InsertSpans(ctx, *owner, detail.Spans[begin:begin+size])
					if !errors.Is(e, control.ErrCollectorBatchTooLarge) || size == 1 {
						break
					}
					size /= 2
				}
				if e != nil {
					return e
				}
				begin += size
			}
			cursor = id
			traces++
			spans += len(detail.Spans)
			if e = json.NewEncoder(os.Stdout).Encode(map[string]any{"last_trace_id": cursor, "traces_submitted": traces, "spans_submitted": spans}); e != nil {
				return e
			}
		}
	}
}
