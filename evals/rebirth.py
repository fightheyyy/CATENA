"""Submit one real experiment through CATENA's API; no Agent/Verifier execution here."""

import argparse
import json
import os
import time
import urllib.request
import uuid
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True)
    parser.add_argument("--analysis-job-id", required=True, help="Completed owned analysis selected by a human")
    parser.add_argument("--case-id", required=True)
    parser.add_argument("--baseline", required=True, type=Path, help="Authorized baseline AGENTS.md")
    parser.add_argument("--output", required=True, type=Path, help="New private output directory")
    args = parser.parse_args()
    headers = {"Content-Type": "application/json"}
    if token := os.environ.get("CATENA_API_TOKEN"):
        headers["Authorization"] = "Bearer " + token

    def request(path, payload=None, extra=None):
        data = json.dumps(payload).encode() if payload is not None else None
        req = urllib.request.Request(args.url.rstrip("/") + path, data=data, headers={**headers, **(extra or {})})
        with urllib.request.urlopen(req, timeout=20) as response:
            return json.load(response)

    def save(name, value):
        with os.fdopen(os.open(args.output / name, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), "w") as output:
            json.dump(value, output, ensure_ascii=False, indent=2)

    if not request("/v1/me/llm-config").get("configured"):
        raise RuntimeError("BLOCKED: configure the owner model in Connections first")
    if args.case_id not in {case["id"] for case in request("/v1/evaluation-cases")["cases"]}:
        raise RuntimeError("BLOCKED: register and human-confirm the Case first")
    payload = {
        "pipeline": "improve", "analysis_job_id": args.analysis_job_id,
        "options": {"case_id": args.case_id, "baseline": {"AGENTS.md": args.baseline.read_text(encoding="utf-8")},
                    "budget": {"agent_turns": 1, "trials": 4, "seconds": 900}},
    }
    args.output.mkdir(parents=True, mode=0o700)  # Existing output must be inspected, never silently rerun.
    key = uuid.uuid4().hex
    save("request.json", {"url": args.url, "idempotency_key": key, "payload": payload})
    job = request("/v1/analysis-jobs", payload, {"Idempotency-Key": key})
    save("job.json", job)
    print("Job:", job["job_id"], flush=True)
    deadline = time.monotonic() + 1200
    while job["state"] not in ("completed", "failed", "cancelled"):
        if time.monotonic() >= deadline:
            raise TimeoutError("Inspect the retained Job identity; it may still be running")
        time.sleep(2)
        job = request("/v1/analysis-jobs/" + job["job_id"])
        save("job.json", job)
    if job["state"] != "completed":
        raise RuntimeError("Real experiment failed; inspect retained job.json: " + job.get("error", ""))
    # Self-check provenance and independent Verifier evidence, not LLM quality claims.
    artifact, = job["artifacts"]
    result = artifact["provenance"]
    assert artifact["validation_status"] == "review_required"
    assert result["evidence_sha256"] == job["evidence_pack"]["sha256"]
    pair, = result["compare"]
    assert pair["case_id"] == args.case_id and not pair["held_out"]
    assert pair["baseline"]["candidate_id"] == result["baseline_id"]
    assert pair["candidate"]["candidate_id"] == artifact["payload"]["id"]
    assert all(trial["valid"] and trial["phase"] == "validation" and trial["rewards"] for trial in (pair["baseline"], pair["candidate"]))
    print("Engineering run completed; human review required. No general improvement established.")


if __name__ == "__main__":
    main()
