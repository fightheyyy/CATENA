# Local Trace loading repair — 2026-09-16

The real local Overview displayed `经历暂时无法读取 / signal timed out`.
Core and both databases were healthy, but an initial `/v1/traces?limit=100`
request took 23.1 seconds. The client correctly stopped the read after 15s.
The list query parsed large input and attribute columns across the entire
history before applying its limit.

## Change

Global and Agent-window lists now select a bounded set of recent Trace IDs
using narrow metadata first. They read summary evidence only for those IDs.
Both stages retain `FINAL`; explicit `PREWHERE` uses only the immutable
`owner_id` and `trace_id` sorting keys. Agent/time filters precede the limit,
the selected spans retain the original overlap window, and equal end times
use Trace ID as a deterministic secondary sort key. Input previews are
bounded to 512 Unicode characters before aggregation. Full detail is intact.

No schema migration or source-data rewrite was needed. The browser deadline
was not increased. The local Core image was rebuilt and replaced; PostgreSQL
and ClickHouse stayed running. The previous image remains available as
`catena/core:before-trace-load-20260916`.

## Verification

| Read | Before | After |
| --- | --- | --- |
| Global list, initial live HTTP | 23.12s | 1.01s (subsequent live check) |
| Global list, warm SQL | 10.48s / 7.65GB read | 1.02s / 1.14GB read |
| Agent list, 500 rows, warm SQL | 7.63s / 7.65GB read | 3.39s / 3.13GB read |
| Agent list, 500 rows, live HTTP | — | 3.75s |

These are individual local measurements, not a cold-cache or load-test SLA.
All 100 global and 500 Agent summaries matched the old queries field by
field. The initial historical import remains 301 sessions, 2,582 traces and
83,837 spans.

- `go test ./...` and `go vet ./...` passed with Node available for existing
  worker tests. The initial stripped build lacked Node; its harness was fixed.
- The existing embedded-Web smoke check was updated to validate JavaScript
  MIME type instead of searching for the removed `Trace Farm` navigation copy.
- Live ClickHouse tests ran in the separate database
  `catena_load_regression_20260916`. They cover limits, tied ordering, owner
  isolation with reused Trace IDs, Agent filtering, Session-key precedence,
  Unicode previews, span replacement and old-version time-window exclusion.
- The actual browser displayed seven real recent records, opened the imported
  Catena task's exact Trace and showed the 500-record Agent index and narrative.
  There were no browser script errors or horizontal overflow.
- Private machine-readable measurements are under the ignored local directory
  `.local/trace-load-20260916/`; no conversation payload is included here.

The real workspace is `http://127.0.0.1:5570`. Memory and generated outputs
remain separate from Trace import; no model execution was started by this fix.
