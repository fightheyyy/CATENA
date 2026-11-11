"""Trusted workflow client for the Go-owned Sandbox scheduler and Tool Gateway."""

import asyncio
import json
import math
import os
import threading
import uuid
from contextvars import ContextVar
from dataclasses import dataclass, field
from urllib.parse import urlsplit

from .contracts import OptimizationError
from .harness_transport import private_post
from .runtime.cancellation import cancellable

execution = ContextVar("workflow_execution", default=None)
callbacks = {}
registry_lock = threading.Lock()


@dataclass
class ToolCallback:
    mode: str
    operation: object
    cancelled: threading.Event
    closed: threading.Event = field(default_factory=threading.Event)
    lock: threading.Lock = field(default_factory=threading.Lock)

    def is_set(self):
        return self.closed.is_set() or (self.cancelled is not None and self.cancelled.is_set())

    def invoke(self, tool, arguments):
        with self.lock:
            if self.is_set() or not isinstance(arguments, dict):
                raise OptimizationError("workflow callback expired or invalid")
            if self.mode == "analyze" and tool in ("list_traces", "get_trace"):
                operation = self.operation(tool, arguments)
            elif self.mode == "improve" and tool == "evaluate_candidate":
                operation = self.operation(arguments)
            else:
                raise OptimizationError("tool outside the workflow profile")
            return asyncio.run(cancellable(operation, self))


def dispatch_tool(owner, job, callback, payload):
    with registry_lock:
        registered = callbacks.get((owner, job, callback))
    if registered is None or set(payload) != {"tool", "arguments"}:
        raise OptimizationError("workflow callback unavailable")
    return registered.invoke(payload["tool"], payload["arguments"])


class WorkflowHarness:
    def __init__(self, model):
        self.model = model
        self.identity = execution.get()
        self.base = os.environ.get("CATENA_CONTROL_URL", "").rstrip("/")

    def preflight(self):
        base = urlsplit(self.base)
        if (
            self.identity is None
            or len(self.identity[2]) != 43
            or base.scheme not in ("http", "https")
            or not base.hostname
            or base.username
            or base.query
            or base.fragment
        ):
            raise OptimizationError("OpenSandbox workflow capability is required")

    def run(self, prompt, timeout, tool, output, cancelled, mode="improve"):
        self.preflight()
        owner, job, token = self.identity
        callback = uuid.uuid4().hex
        key = owner, job, callback
        registration = ToolCallback(mode, tool, cancelled)
        with registry_lock:
            callbacks[key] = registration
        seconds = min(1800, max(1, math.ceil(timeout)))
        try:
            result = private_post(
                self.base + "/internal/harness/turn",
                token,
                {"prompt": prompt, "timeout_seconds": seconds, "callback": callback},
                seconds + 120,
                256 * 1024,
            )
            if registration.is_set() or not isinstance(result, dict) or set(result) != {"value", "input_tokens", "output_tokens"}:
                raise OptimizationError("invalid or cancelled Harness result")
            if any(type(result[k]) is not int or result[k] < 0 for k in ("input_tokens", "output_tokens")):
                raise OptimizationError("invalid Harness usage")
            output.mkdir(mode=0o700)
            with os.fdopen(os.open(output / "result.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w") as report:
                json.dump(result, report, ensure_ascii=False)
            return result
        finally:
            registration.closed.set()
            with registry_lock:
                callbacks.pop(key, None)
            # An already admitted Harbor callback owns cleanup of its environment.
            with registration.lock:
                pass
