"""Fail-open background OTLP exporter used by the Catena Tap wrapper."""

from __future__ import annotations

import json
import os
import queue
import sys
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any

from catena_tap.normalizer import TraceNormalizer


@dataclass
class ExportStats:
    captured_records: int = 0
    uploaded_traces: int = 0
    failed_uploads: int = 0
    dropped_records: int = 0


class OTLPHTTPClient:
    def __init__(self, endpoint: str, api_key: str, *, timeout: float = 2.0, debug: bool = False):
        self.endpoint = endpoint
        self.api_key = api_key
        self.timeout = timeout
        self.debug = debug
        self._warned = False

    def send(self, payload: dict[str, Any]) -> bool:
        body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        request = urllib.request.Request(
            self.endpoint,
            data=body,
            method="POST",
            headers={
                "Authorization": f"Bearer {self.api_key}",
                "Content-Type": "application/json",
                "User-Agent": "catena-tap/0.1.0",
            },
        )
        last_error: Exception | None = None
        for attempt in range(2):
            try:
                with urllib.request.urlopen(request, timeout=self.timeout) as response:
                    if 200 <= response.status < 300:
                        return True
                    last_error = RuntimeError(f"HTTP {response.status}")
            except (OSError, urllib.error.HTTPError, urllib.error.URLError) as exc:
                last_error = exc
            if attempt == 0:
                time.sleep(0.2)
        if self.debug or not self._warned:
            print(f"catena tap: Trace upload failed; Agent execution was not affected ({last_error})", file=sys.stderr)
            self._warned = True
        return False


class BackgroundTraceExporter:
    """Serialize capture callbacks off the Agent's event-loop thread."""

    def __init__(self, runtime: str, client: OTLPHTTPClient, *, max_queue: int = 1024):
        self.normalizer = TraceNormalizer(runtime)
        self.client = client
        self.stats = ExportStats()
        self._queue: queue.Queue[tuple[str, dict[str, Any]] | None] = queue.Queue(maxsize=max_queue)
        self._thread = threading.Thread(target=self._run, name="catena-tap-export", daemon=True)
        self._thread.start()

    def submit(self, session_id: str, record: dict[str, Any]) -> None:
        self.stats.captured_records += 1
        try:
            self._queue.put_nowait((session_id, record))
        except queue.Full:
            self.stats.dropped_records += 1

    def close(self, timeout: float = 5.0) -> ExportStats:
        self._queue.put(None)
        self._thread.join(timeout=timeout)
        return self.stats

    def _run(self) -> None:
        while True:
            item = self._queue.get()
            try:
                if item is None:
                    for payload in self.normalizer.flush():
                        self._send(payload)
                    return
                session_id, record = item
                for payload in self.normalizer.ingest(session_id, record):
                    self._send(payload)
            except Exception as exc:  # capture must never break the wrapped Runtime
                self.stats.failed_uploads += 1
                if self.client.debug:
                    print(f"catena tap: normalizer failure: {exc}", file=sys.stderr)
            finally:
                self._queue.task_done()

    def _send(self, payload: dict[str, Any]) -> None:
        if self.client.send(payload):
            self.stats.uploaded_traces += 1
        else:
            self.stats.failed_uploads += 1


def endpoint_from_environment(explicit_endpoint: str = "", explicit_url: str = "") -> str:
    endpoint = explicit_endpoint.strip() or os.environ.get("CATENA_OTLP_ENDPOINT", "").strip()
    if endpoint:
        return endpoint
    base_url = explicit_url.strip() or os.environ.get("CATENA_URL", "").strip() or "http://127.0.0.1:5670"
    return base_url.rstrip("/") + "/v1/otlp/v1/traces"
