#!/usr/bin/env python3
"""Stop only the dedicated local Collector during real managed work, then verify replay."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import time
import urllib.request

import psycopg
from psycopg.rows import dict_row


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    env = json.loads(args.environment.read_text())
    api = env["CATENA_MANAGED_AUTH_URL"].removesuffix("/internal/managed/authorize")
    db = psycopg.connect(
        env["BARENA_DATABASE_URL"], autocommit=True, row_factory=dict_row
    )
    service = "catena-managed-telemetry-otel-gateway-1"
    info = json.loads(
        subprocess.check_output(["docker", "inspect", service], text=True)
    )[0]
    assert (
        info["Config"]["Labels"].get("com.docker.compose.project")
        == "catena-managed-telemetry"
    ), "refusing to stop unrelated Collector"
    assert (
        db.execute(
            "SELECT count(*) AS n FROM agent_runs WHERE state='RUNNING'"
        ).fetchone()["n"]
        > 0
    ), "run concurrently with real managed acceptance"
    record = {
        "started_at": datetime.now(timezone.utc).isoformat(),
        "service": service,
        "real_run_evidence": [],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)

    def save():
        args.output.write_text(json.dumps(record, indent=2, default=str))

    subprocess.run(["docker", "stop", service], check=True, stdout=subprocess.DEVNULL)
    try:
        deadline = time.monotonic() + 35
        while time.monotonic() < deadline:
            time.sleep(1)
        pending = db.execute(
            "SELECT run_id,trace_id,trace_event_cursor,trace_base_exported,state FROM agent_runs WHERE state IN ('COMPLETED','FAILED','NEEDS_REVIEW','CANCELLED') AND NOT trace_exported AND finished_at>=%s",
            (record["started_at"],),
        ).fetchall()
        record["pending_before_resume"] = pending
    finally:
        subprocess.run(
            ["docker", "start", service], check=True, stdout=subprocess.DEVNULL
        )
        record["resumed_at"] = datetime.now(timezone.utc).isoformat()
        save()
    assert pending, "no real managed run finalized during the controlled outage"
    for run in pending:
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            row = db.execute(
                "SELECT trace_exported FROM agent_runs WHERE run_id=%s",
                (run["run_id"],),
            ).fetchone()
            if row["trace_exported"]:
                try:
                    with urllib.request.urlopen(
                        api + "/v1/traces/" + run["trace_id"], timeout=5
                    ) as response:
                        trace = json.load(response)
                    expected = {
                        row["event_id"]
                        for row in db.execute(
                            "SELECT event_id FROM agent_run_events WHERE run_id=%s",
                            (run["run_id"],),
                        ).fetchall()
                    }
                    actual = [
                        event["attributes"]["catena.event.id"]
                        for span in trace["spans"]
                        for event in (span.get("events") or [])
                        if "catena.event.id" in event.get("attributes", {})
                    ]
                    if set(actual) == expected and len(actual) == len(expected):
                        record["real_run_evidence"].append(
                            {
                                "run_id": run["run_id"],
                                "trace_id": run["trace_id"],
                                "expected_events": len(expected),
                                "observed_events": len(actual),
                                "span_count": len(trace["spans"]),
                                "exact_event_id_match": True,
                            }
                        )
                        save()
                        break
                except OSError:
                    pass
            time.sleep(1)
        else:
            raise RuntimeError("trace replay did not converge")
    record["recovery_verified"] = True
    save()
    db.close()
    print("real Collector retry verified", len(pending), flush=True)


if __name__ == "__main__":
    main()
