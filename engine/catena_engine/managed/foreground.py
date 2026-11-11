"""Confirm quiescence even when a tool uses setsid or double-forks."""

import ctypes
import os
import subprocess
import sys

PR_SET_CHILD_SUBREAPER = 36


def run(command, env):
    # Linux reparents detached descendants to this wrapper before it releases the Run.
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) != 0:
        raise OSError(ctypes.get_errno(), "cannot establish Sandbox process ownership")
    code = subprocess.call(command, env=env)
    while True:
        try:
            pid, _ = os.waitpid(-1, os.WNOHANG)
        except ChildProcessError:
            return code
        if pid == 0:
            print("Sandbox retains a live descendant", file=sys.stderr)
            return 1


def main():
    environment = os.environ | {"PYTHONPATH": "/opt/catena"}
    command = ["runuser", "-u", "agent", "--", "python3", "-m", "catena_engine.managed.entry", sys.argv[1]]
    return run(command, environment)


if __name__ == "__main__":
    raise SystemExit(main())
