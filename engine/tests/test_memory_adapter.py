import json

import httpx
import pytest

from catena_engine.executions import Executions
from catena_engine.memory.adapters.openviking import OpenViking, file_id, owned, project, root
from catena_engine.memory.api import dispatch
from catena_engine.rsi.contracts import OptimizationError


def test_openviking_extract_receipt_restart_and_owner_isolation(tmp_path):
    calls = []

    def transport(request):
        calls.append((request.url.path, request.headers["X-OpenViking-User"]))
        assert request.headers["X-API-Key"] == "private-key"
        assert request.headers["X-OpenViking-Role"] == "user"
        path = request.url.path
        if path.endswith("/commit"):
            value = {"task_id": "native-task"}
        elif path.endswith("/native-task"):
            value = {"status": "completed", "result": {"memory_count": 1}}
        elif path == "/api/v1/fs/ls":
            value = [
                {"uri": root("alice") + "/events/a.md", "modTime": "now"},
                {"uri": root("bob") + "/events/b.md"},
                {"uri": root("alice") + "/../secret"},
            ]
        elif path == "/api/v1/content/read":
            value = "# Summary\nUse bounded retries."
        elif path == "/api/v1/search/find":
            assert json.loads(request.content)["target_uri"] == root("alice")
            value = {
                "memories": [
                    {"uri": root("bob") + "/private.md", "content": "private"},
                    {"uri": root("alice") + "/events/a.md", "content": "safe", "score": 0.8},
                ]
            }
        else:
            value = {}
        return httpx.Response(200, json={"status": "ok", "result": value})

    adapter = OpenViking("http://private", "private-key", httpx.Client(transport=httpx.MockTransport(transport)))
    payload = {"request_id": "analysis-one", "evidence": "Synthetic failed command", "owner": "bob"}
    receipt = dispatch(adapter, "alice", "extract", payload, Executions(tmp_path))
    assert receipt["task_id"].startswith(project("alice") + "_ov_")
    assert dispatch(adapter, "alice", "extract", payload, Executions(tmp_path)) == receipt
    assert sum(path.endswith("/commit") for path, _ in calls) == 1
    result = dispatch(adapter, "alice", "task", receipt, Executions(tmp_path))
    assert result["status"] == "completed" and result["result"]["memory_count"] == 1
    with pytest.raises(OptimizationError):
        adapter.task("bob", receipt["task_id"])
    items = adapter.list("alice", 24)["memories"]
    assert len(items) == 1 and items[0]["id"] == str(file_id(root("alice") + "/events/a.md"))
    assert len(adapter.search("alice", {"query": "safe"})["facts"]) == 1
    assert all(user == project("alice") for _, user in calls)
    assert "private-key" not in "".join(p.read_text() for p in tmp_path.rglob("*.json"))
    changed = dict(payload, evidence="different")
    with pytest.raises(OptimizationError):
        dispatch(adapter, "alice", "extract", changed, Executions(tmp_path))


def test_original_go_identity_and_uri_guards():
    import hashlib

    assert project("alice") == "catena_" + hashlib.sha256(b"catena-memory:alice").hexdigest()[:24]
    for suffix in ("/../a", "/a%2fb", "/a?x", "/a#x", "/a\\b", "/.//a"):
        assert not owned("alice", root("alice") + suffix)
    assert owned("alice", root("alice") + "/events/a.md")
