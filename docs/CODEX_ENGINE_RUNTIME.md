# Codex analysis runtime

The Engine uses Codex App Server `0.160.0` over stdio, with a private HOME/CODEX_HOME,
explicit native Skill input (private root registration and load confirmation) and two controlled task modes: analysis has read-only `list_traces` / `get_trace` tools; improvement has one `evaluate_candidate` dynamic tool. Native Codex
owns reasoning/tool continuation. The former JSON `codex exec` transport and
Harness² Proposer were deleted.

The process runs read-only with shell, host Skills, plugins, hooks, MCP and subagents
disabled. Unknown requests are refused. A bounded, key-redacted protocol audit
persists; temporary homes are cleaned and process groups terminated on completion,
timeout or cancellation. Harbor target cleanup is awaited separately.

Go supplies the owner's Responses-compatible model privately. API keys and ephemeral Trace-query capabilities are excluded
from execution fingerprints and reports. The query token is never sent to the model. Receipt/run volumes remain persistent and
single-Engine-owned. One turn is not an inference-request, token or dollar cap.

See [Workflow](IMPROVEMENT_WORKFLOW.md) for order, restrictions and compatibility.
Harbor's installed target Agent uses its own native execution; it is independent
of the internal analysis process. The analysis Skill stays outside the target
workspace and is rejected from generated target harnesses.
