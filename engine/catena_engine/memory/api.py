"""Private transport boundary. Owner never comes from request JSON."""
import os

from ..rsi.contracts import OptimizationError
from .adapters.openviking import OpenViking


def memory_request(owner, operation, payload, executions):
    with __import__("httpx").Client(timeout=180) as client:
        adapter = OpenViking(os.environ.get("CATENA_OPENVIKING_URL", ""), os.environ.get("CATENA_OPENVIKING_API_KEY", ""), client)
        return dispatch(adapter, owner, operation, payload, executions)


def dispatch(adapter, owner, operation, payload, executions):
    if operation == "extract":
        status, receipt = executions.execute(owner, payload, lambda _: adapter.extract(owner, payload))
        if status != 200 or receipt.get("state") != "completed":
            raise OptimizationError("uncertain extraction must be reviewed, not automatically restarted")
        return receipt["output"]
    if operation == "task":
        return adapter.task(owner, payload["task_id"])
    if operation == "list":
        limit = payload.get("limit", 24)
        if not isinstance(limit, int) or not 1 <= limit <= 100:
            raise OptimizationError("invalid limit")
        return adapter.list(owner, limit)
    if operation == "search":
        return adapter.search(owner, payload)
    if operation == "graph":
        return adapter.graph(owner, payload["fact_id"])
    if operation == "note":
        return adapter.note(owner, payload)
    if operation == "status":
        return adapter.ping()
    raise OptimizationError("unknown memory operation")
