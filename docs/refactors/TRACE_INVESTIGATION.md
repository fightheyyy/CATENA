# Tool-driven Trace investigation

Analysis now starts with an objective and a fixed scope, not a preloaded Evidence
prompt. The existing Codex Runtime exposes only `list_traces` and `get_trace` in
analysis mode. Codex chooses queries, inspects redacted pages, tests hypotheses and
returns problems with inspected Trace/Span references. Human-started improvement
and final Human Review are unchanged.

The concrete Go bridge reuses TraceStore and existing evidence sanitization.
Capabilities are execution-scoped, checked against the active lease, revoked on
return, and never passed to the model. Owner/Agent/window cannot be widened.
Limits: 20 calls, 300 seconds, scan at most 100 traces, list pages at most 20,
span pages at most 64, 64 KiB per response and 512 KiB cumulative evidence.
Argument/scope errors are returned to the model for correction; transport/storage
failures do not masquerade as empty evidence. No new framework, queue, middleware,
retrieval engine or evaluation runner was introduced.

## Real smoke

[Retained metadata](evidence/trace-investigation/real-investigation.json) records a
real CLIProxyAPI model run on the retained Trace. Seven native tool calls included
one rejected oversized request, correction and five successful detail pages.
Three problems were persisted. The query responses, span references and hashes
remain in the owner-scoped report; full raw data stays private.

Only part of one Trace was read. No cross-Trace comparison, causal validation,
Candidate, Harbor trial or automatic improvement occurred. The earlier failed
attempt remains recorded separately and is excluded from success claims.

Validation reused the Go admission/worker test with capability, scope, pagination
and revocation assertions; added small Python query/continuation checks; retained
receipt deduplication. Go tests/vet, the targeted race test, PostgreSQL/private-Python
protocol fixture, Python suite/ruff and Compose parsing passed. No frontend source
or evaluation behavior changed; no new paid optimization was run.
