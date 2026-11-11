#!/usr/bin/env python3
"""Real local Harness checks; the optional Improvement callback is a protocol fixture."""

import argparse
import json
import os
import time
import uuid
from pathlib import Path
from urllib.parse import urlsplit

from managed_smoke import http


def poll(api, job, timeout):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = http(api, f"/v1/analysis-jobs/{job}")
        if result["state"] in ("completed", "failed", "cancelled"):
            return result
        time.sleep(1)
    raise TimeoutError(job)


def contract_engine(env, evidence):
    from catena_engine.contracts import Candidate, OptimizationError
    from catena_engine.improvement import candidate
    from catena_engine.protocol import _model_config
    from catena_engine.server import create_server
    from catena_engine.workflow_harness import WorkflowHarness

    os.environ.update(env)
    address = env["CATENA_ENGINE_ADDR"]
    if address.rsplit(":", 1)[0] != "127.0.0.1":
        raise ValueError("contract Engine must bind an isolated loopback port")

    def probe(payload, cancelled):
        identity = payload["request_id"]
        record = {"job_id": identity, "mode": "real Codex, fixture response, no Harbor evaluation"}

        async def evaluate(value):
            parsed = candidate(value, Candidate({"AGENTS.md": "Preserve source evidence."}))
            if not parsed.files["AGENTS.md"].strip():
                raise OptimizationError("expected an AGENTS.md proposal")
            record.update(tool="evaluate_candidate", candidate_scope_verified=True)
            return {"contract_probe": "read back this tool response; no evaluation was executed"}

        try:
            result = WorkflowHarness(_model_config(payload["model"])).run(
                'Call evaluate_candidate once with text "contract check" and one AGENTS.md replacement containing '
                '"Preserve trace evidence." After reading the tool reply, return the proposal as structured JSON.',
                90, evaluate, evidence.parent / identity, cancelled,
            )
            if not record.get("candidate_scope_verified"):
                raise OptimizationError("real tool call not observed")
            candidate(result["value"], Candidate({"AGENTS.md": "Preserve source evidence."}))
            record.update(structured_reply_verified=True, input_tokens=result["input_tokens"],
                          output_tokens=result["output_tokens"])
        finally:
            evidence.write_text(json.dumps(record, indent=2) + "\n")
        raise OptimizationError("contract fixture intentionally does not publish business artifacts")

    with create_server(address, env["CATENA_ENGINE_TOKEN"], improvement_handler=probe) as server:
        server.serve_forever()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--trace")
    group.add_argument("--analysis-job")
    group.add_argument("--contract-engine", action="store_true")
    args = parser.parse_args()
    env = json.loads(args.environment.read_text())
    args.output.parent.mkdir(parents=True, exist_ok=True)
    if args.contract_engine:
        contract_engine(env, args.output)
        return
    api = env["CATENA_MANAGED_AUTH_URL"].removesuffix("/internal/managed/authorize")
    if urlsplit(api).hostname != "127.0.0.1":
        raise ValueError("the live check uses an isolated local Core")
    payload = (
        {"pipeline": "analyze", "trace_id": args.trace,
         "objective": "Use list_traces and get_trace to inspect the actual trace. Cite a concrete observation and separate uncertainty."}
        if args.trace else
        {"pipeline": "improve", "analysis_job_id": args.analysis_job, "options": {"case_id": "contract-probe"}}
    )
    started = http(api, "/v1/analysis-jobs", payload, headers={"Idempotency-Key": uuid.uuid4().hex})
    job = poll(api, started["job_id"], 360)
    record = {"job_id": job["job_id"], "state": job["state"], "artifacts": len(job["artifacts"])}
    if args.trace:
        assert job["state"] == "completed" and job["artifacts"], job.get("error")
        calls = [call["tool"] for call in job["checkpoint"]["tool_evidence"]]
        assert calls[0] == "list_traces" and "get_trace" in calls
        record.update(trace=args.trace, tools=calls, evidence_refs=job["checkpoint"]["evidence_refs"])
    else:
        assert job["state"] == "failed" and not job["artifacts"], "contract fixture must not publish artifacts"
    args.output.write_text(json.dumps(record, indent=2) + "\n")
    print(json.dumps(record), flush=True)


if __name__ == "__main__":
    main()
