"""Read-only, tool-driven Trace investigation. Improvement remains a human decision."""

import asyncio
import hashlib
import json
import os
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

from .contracts import OptimizationError, safe_id
from .protocol import ModelConfig
from .runtime.codex import SKILL
from .workflow_harness import WorkflowHarness


def validate_report(value, refs):
    if (
        not isinstance(value, dict)
        or set(value) != {"summary", "problems"}
        or not isinstance(value["summary"], str)
        or not isinstance(value["problems"], list)
        or len(value["problems"]) > 32
    ):
        raise OptimizationError("invalid analysis report")
    for problem in value["problems"]:
        if not isinstance(problem, dict) or set(problem) != {
            "title", "evidence", "evidence_refs", "hypothesis", "uncertainty",
        }:
            raise OptimizationError("invalid analysis problem")
        if any(not isinstance(problem[k], str) or not problem[k].strip() for k in ("title", "evidence", "hypothesis", "uncertainty")):
            raise OptimizationError("problem must separate evidence, hypothesis and uncertainty")
        citations = problem["evidence_refs"]
        if not isinstance(citations, list) or not citations or any(not isinstance(r, str) or r not in refs for r in citations):
            raise OptimizationError("problem cites evidence not inspected through get_trace")
    if len(json.dumps(value).encode()) > 128 * 1024:
        raise OptimizationError("analysis report is too large")
    return value


class TraceQueries:
    def __init__(self, query):
        if not isinstance(query, dict) or set(query) != {"job_id", "token"}:
            raise OptimizationError("invalid Trace query capability")
        identity = safe_id(query["job_id"])
        token = query["token"]
        base = os.environ.get("CATENA_CONTROL_URL", "").rstrip("/")
        url = urllib.parse.urlsplit(base)
        if url.scheme not in ("http", "https") or not url.netloc or url.username or url.password or url.query or url.fragment or not isinstance(token, str) or not 20 <= len(token) <= 128 or not token.isascii() or not token.isalnum():
            raise OptimizationError("private CATENA_CONTROL_URL and a query capability are required")
        self.url = base + "/internal/analysis-jobs/" + identity + "/trace-query"
        self.token = token
        self.calls = []
        self.refs = set()
        self.size = 0

    def _query(self, tool, arguments):
        request = urllib.request.Request(self.url, data=json.dumps({"tool": tool, "arguments": arguments}).encode(), headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/json"})
        # The configured control plane never redirects; do not forward a capability elsewhere.
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, req, fp, code, msg, headers, newurl):
                return None
        try:
            with urllib.request.build_opener(NoRedirect).open(request, timeout=12) as response:
                raw = response.read(64 * 1024 + 1)
        except urllib.error.HTTPError as error:
            if error.code not in (400, 403, 404, 413, 429):
                raise OptimizationError("authorized Trace query failed") from None
            raw = json.dumps({"error": "Trace query rejected", "status": error.code, "hint": "Check tool arguments, stay within scope, use limit <= 20 for list_traces and <= 64 for get_trace. If scope/lease is unavailable, finish with uncertainty."}).encode()
        except (urllib.error.URLError, OSError):
            raise OptimizationError("authorized Trace query failed") from None
        if len(raw) > 64 * 1024:
            raise OptimizationError("Trace response exceeded its limit")
        self.size += len(raw)
        if self.size > 512 * 1024:
            raise OptimizationError("investigation evidence budget exhausted")
        result = json.loads(raw)
        if not isinstance(result, dict) or result.get("omitted"):
            raise OptimizationError("Trace evidence unavailable or oversized")
        self.calls.append({"tool": tool, "arguments": arguments, "sha256": hashlib.sha256(raw).hexdigest(), "result": result})
        if tool == "get_trace" and "error" not in result:
            trace_id = result.get("source_trace_id")
            if trace_id != arguments.get("trace_id") or not isinstance(result.get("spans"), list):
                raise OptimizationError("Trace response identity mismatch")
            self.refs.add(trace_id)
            self.refs.update(trace_id + "/" + span["span_id"] for span in result["spans"])
        return result

    async def __call__(self, tool, arguments):
        if tool not in ("list_traces", "get_trace") or not isinstance(arguments, dict) or len(self.calls) >= 20:
            raise OptimizationError("unauthorized or over-budget Trace query")
        return await asyncio.to_thread(self._query, tool, arguments)


def run_payload(payload, cancelled=None):
    if payload.get("schema") != "catena.analysis_request.v2" or set(payload) != {"schema", "request_id", "objective", "scope", "trace_query", "model"}:
        raise OptimizationError("invalid investigation request")
    request_id = safe_id(payload.get("request_id"))
    if not isinstance(payload["objective"], str) or len(payload["objective"].encode()) > 16000 or not isinstance(payload["scope"], dict):
        raise OptimizationError("invalid investigation scope")
    queries = TraceQueries(payload["trace_query"])
    if payload["trace_query"]["job_id"] != request_id:
        raise OptimizationError("query capability is bound to another Job")
    root = os.environ.get("CATENA_RUNS_ROOT")
    if not root:
        raise OptimizationError("CATENA_RUNS_ROOT is required")
    output = Path(root) / (request_id + "-" + uuid.uuid4().hex)
    output.mkdir(parents=True, mode=0o700)
    model = payload["model"]
    if not isinstance(model, dict) or not all(isinstance(model.get(k), str) and model[k] for k in ("provider", "model")):
        raise OptimizationError("invalid Analysis model reference")
    runtime = WorkflowHarness(ModelConfig(model["provider"], "", model["model"], ""))
    runtime.preflight()
    reply = runtime.run(json.dumps({"objective": payload["objective"], "scope": payload["scope"], "query_budget": 20, "instruction": "Investigate with list_traces and get_trace. No trace content has been supplied. Return problems and evidence in Chinese; do not generate improvements."}, ensure_ascii=False), 300, queries, output / "agent", cancelled, mode="analyze")
    if not queries.calls or queries.calls[0]["tool"] != "list_traces":
        raise OptimizationError("investigation did not query its Trace scope")
    report = validate_report(reply["value"], queries.refs)
    skill = SKILL.parent.parent / "trace-analysis/SKILL.md"
    result = {
        "schema": "catena.analysis_result.v1", "status": "completed", "decision": "awaiting_human_decision", "request_id": request_id,
        **report,
        "evidence_refs": sorted(queries.refs), "tool_evidence": queries.calls,
        "experiment_id": output.name,
        "strategy": {"name": "trace-investigation", "skill_sha256": hashlib.sha256(skill.read_bytes()).hexdigest()},
        "model": {"provider": runtime.model.provider, "model": runtime.model.model, "runtime": "codex-app-server@0.160.0"},
        "usage": {"agent_turns": 1, "tool_calls": len(queries.calls), "input_tokens": reply["input_tokens"], "output_tokens": reply["output_tokens"], "attempted_trials": 0},
    }
    with os.fdopen(os.open(output / "result.json", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    return result
