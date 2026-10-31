"""External-operation receipts, not a task queue. Go owns leases and scheduling."""

import hashlib
import json
import os
import threading
from pathlib import Path

from .rsi.contracts import safe_id


class Executions:
    def __init__(self, root):
        self.root = Path(root)
        self.lock = threading.Lock()
        self.active = {}

    def _path(self, owner, identity):
        safe_id(identity)
        return self.root / hashlib.sha256(owner.encode()).hexdigest() / identity / "receipt.json"

    @staticmethod
    def _write(path, value):
        path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        tmp = path.with_suffix(".tmp")
        with os.fdopen(os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), "w") as out:
            json.dump(value, out, ensure_ascii=False)
        os.replace(tmp, path)

    def lookup(self, owner, identity):
        with self.lock:
            path = self._path(owner, identity)
            if not path.exists():
                return 404, {"error": "unknown execution"}
            receipt = json.loads(path.read_text())
            if receipt["state"] == "running":
                if (owner, identity) in self.active:
                    return 202, receipt
                return 409, {"error": "interrupted execution requires review; it was not restarted"}
            return 200, receipt

    def cancel(self, owner, identity):
        with self.lock:
            event = self.active.get((owner, identity))
            if event:
                event.set()
                return 202, {"request_id": identity, "state": "cancelling"}
        return self.lookup(owner, identity)

    def replay(self, owner, payload):
        identity = safe_id(payload.get("request_id"))
        sanitized = json.loads(json.dumps(payload))
        if isinstance(sanitized.get("model"), dict):
            sanitized["model"].pop("api_key", None)
        fingerprint = hashlib.sha256(json.dumps(sanitized, sort_keys=True).encode()).hexdigest()
        path = self._path(owner, identity)
        with self.lock:
            if not path.exists():
                return None
            receipt = json.loads(path.read_text())
            if receipt.get("fingerprint") != fingerprint:
                return 409, {"error": "execution request changed"}
            if receipt["state"] == "running":
                return (
                    (202, receipt)
                    if (owner, identity) in self.active
                    else (409, {"error": "interrupted execution requires review"})
                )
            return 200, receipt

    def execute(self, owner, payload, operation):
        identity = safe_id(payload.get("request_id"))
        # Credentials are neither stored nor used to distinguish task identity.
        sanitized = json.loads(json.dumps(payload))
        if isinstance(sanitized.get("model"), dict):
            sanitized["model"].pop("api_key", None)
        fingerprint = hashlib.sha256(json.dumps(sanitized, sort_keys=True).encode()).hexdigest()
        path = self._path(owner, identity)
        with self.lock:
            if path.exists():
                receipt = json.loads(path.read_text())
                if receipt.get("fingerprint") != fingerprint:
                    return 409, {"error": "execution identity conflicts with the original request"}
                if receipt["state"] == "running":
                    return (
                        (202, receipt)
                        if (owner, identity) in self.active
                        else (409, {"error": "interrupted execution requires review"})
                    )
                return 200, receipt
            cancelled = threading.Event()
            self.active[owner, identity] = cancelled
            receipt = {"request_id": identity, "state": "running", "fingerprint": fingerprint}
            self._write(path, receipt)
        try:
            receipt["output"] = operation(cancelled)
            receipt["state"] = "cancelled" if cancelled.is_set() else "completed"
        except Exception:
            receipt.update(state="failed", output={"status": "failed", "error": "pipeline execution failed"})
        finally:
            with self.lock:
                self._write(path, receipt)
                self.active.pop((owner, identity), None)
        return 200, receipt
