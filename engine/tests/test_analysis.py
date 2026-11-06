import pytest

from catena_engine.analysis import validate_report
from catena_engine.contracts import OptimizationError


def test_report_requires_real_references_and_separate_uncertainty():
    report = {"summary": "Observed a recoverable failure", "problems": [{"title": "Missing tool", "evidence": "Tool returned not found", "evidence_refs": ["owned-trace"], "hypothesis": "Tool unavailable", "uncertainty": "One trace only"}]}
    assert validate_report(report, ["owned-trace"]) == report
    report["problems"][0]["evidence_refs"] = ["invented-trace"]
    with pytest.raises(OptimizationError):
        validate_report(report, ["owned-trace"])


def test_trace_tools_use_private_callback_and_only_fetched_span_refs(tmp_path, monkeypatch):
    import asyncio
    import http.server
    import json
    import threading

    from catena_engine.analysis import TraceQueries

    trace_id = "a" * 32
    seen = []

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            assert self.headers["Authorization"] == "Bearer " + "a" * 26
            assert self.path == "/internal/analysis-jobs/job-1/trace-query"
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            seen.append(body["tool"])
            if body["arguments"].get("limit") == 100:
                self.send_response(400)
                self.end_headers()
                return
            result = {"traces": [{"trace_id": trace_id}]} if body["tool"] == "list_traces" else {"source_trace_id": trace_id, "spans": [{"span_id": "span-1"}], "truncated": True}
            self.send_response(200)
            self.end_headers()
            self.wfile.write(json.dumps(result).encode())

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    monkeypatch.setenv("CATENA_CONTROL_URL", f"http://127.0.0.1:{server.server_port}")
    queries = TraceQueries({"job_id": "job-1", "token": "a" * 26})
    try:
        asyncio.run(queries("list_traces", {}))
        assert not queries.refs
        rejected = asyncio.run(queries("get_trace", {"trace_id": trace_id, "limit": 100}))
        assert rejected["status"] == 400 and not queries.refs
        asyncio.run(queries("get_trace", {"trace_id": trace_id}))
        assert queries.refs == {trace_id, trace_id + "/span-1"}
        assert seen == ["list_traces", "get_trace", "get_trace"] and len(queries.calls) == 3
        assert queries.calls[2]["result"]["truncated"] is True
        with pytest.raises(OptimizationError):
            asyncio.run(queries("shell", {}))
    finally:
        server.shutdown()
        server.server_close()
