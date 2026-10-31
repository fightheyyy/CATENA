import concurrent.futures
import threading
from test_server import call
from catena_engine.server import create_server


def test_receipts_deduplicate_cancel_and_isolate_owners(tmp_path):
    started, ended = threading.Event(), threading.Event()
    calls = []

    def execute(payload, cancelled):
        calls.append(payload["request_id"])
        started.set()
        assert cancelled.wait(3)
        ended.set()
        return {"status": "failed"}

    server = create_server("127.0.0.1:0", "private", execute, tmp_path)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_port}"
    payload = {"request_id": "job-1", "model": {"api_key": "NEVER_PERSIST_THIS"}}
    try:
        assert call(base+"/v2/rsi",payload)[0] == 401
        with concurrent.futures.ThreadPoolExecutor() as pool:
            first = pool.submit(call,base+"/v2/rsi",payload,"private")
            assert started.wait(2)
            assert call(base+"/v2/rsi",payload,"private")[0] == 202
            assert call(base+"/v2/rsi",{"request_id":"other"},"private")[0] == 429
            assert call(base+"/v2/executions/job-1",token="private",owner="foreign")[0] == 404
            assert call(base+"/v2/executions/job-1",token="private",method="DELETE")[0] == 202
            assert first.result()[1]["state"] == "cancelled"
        assert ended.is_set() and calls == ["job-1"]
        assert call(base+"/v2/rsi",payload,"private")[1]["state"] == "cancelled"
        assert "NEVER_PERSIST_THIS" not in "".join(p.read_text() for p in tmp_path.rglob("*.json"))
    finally:
        server.shutdown();server.server_close()


def test_restarted_uncertain_operation_is_not_replayed(tmp_path):
    from catena_engine.executions import Executions
    receipts = Executions(tmp_path)
    payload={"request_id":"uncertain"}
    receipts.execute("owner",payload,lambda e:{"status":"completed"})
    path=receipts._path("owner","uncertain")
    import json
    value=json.loads(path.read_text());value["state"]="running";path.write_text(json.dumps(value))
    assert Executions(tmp_path).replay("owner",payload)[0]==409
