"""Exercise a real self-hosted sandbox; never fall back to E2B Cloud."""
import os
from urllib.parse import urlparse

from e2b import Sandbox


def main():
    for name in ("E2B_API_KEY", "E2B_API_URL", "E2B_SANDBOX_URL"):
        if not os.environ.get(name):
            raise SystemExit(f"Missing {name}; load your self-hosted SDK configuration")
    for name in ("E2B_API_URL", "E2B_SANDBOX_URL"):
        parsed = urlparse(os.environ[name])
        host = parsed.hostname or ""
        if parsed.scheme not in ("http", "https") or not host:
            raise SystemExit(f"Invalid {name}")
        if host == "e2b.app" or host.endswith(".e2b.app") or host == "e2b.dev" or host.endswith(".e2b.dev"):
            raise SystemExit("This smoke check requires a self-hosted endpoint")
    sandbox = Sandbox.create("base", timeout=120)
    sandbox_id = sandbox.sandbox_id
    try:
        sandbox.files.write("/tmp/catena-smoke.txt", "catena-selfhost-ok")
        result = sandbox.commands.run("cat /tmp/catena-smoke.txt")
        if result.exit_code != 0 or result.stdout.strip() != "catena-selfhost-ok":
            raise RuntimeError("Sandbox execution failed")
        sandbox.pause()
        sandbox = Sandbox.connect(sandbox_id, timeout=120)
        if sandbox.files.read("/tmp/catena-smoke.txt") != "catena-selfhost-ok":
            raise RuntimeError("Sandbox persistence failed")
        print("create / files / execution / pause / resume passed")
    finally:
        # Keep cleanup failures visible, including when resume failed.
        Sandbox.kill(sandbox_id)
        print("sandbox kill request completed")


if __name__ == "__main__":
    main()
