import concurrent.futures
import threading
import time

from conftest import FakeModel, factory_for
from test_server import call

from catena_engine.server import create_server


def test_busy_turn_does_not_block_health_or_harbor(monkeypatch, turn_payload):
    entered = threading.Event()

    class SlowModel(FakeModel):
        async def get_response(self, *args, **kwargs):
            entered.set()
            return await super().get_response(*args, **kwargs)

    monkeypatch.setenv("CATENA_ENGINE_MAX_CONCURRENT_TURNS", "1")
    monkeypatch.delenv("CATENA_HARBOR_WORKER_URL", raising=False)
    server = create_server("127.0.0.1:0", model_factory=factory_for(SlowModel(delay=0.6)))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_port}"
    try:
        with concurrent.futures.ThreadPoolExecutor() as pool:
            first = pool.submit(call, base + "/v1/turn", turn_payload)
            assert entered.wait(2)
            started = time.monotonic()
            status, body = call(base + "/v1/turn", turn_payload)
            assert status == 429
            assert body["error"]["code"] == "runtime_busy"
            assert call(base + "/healthz")[0] == 200
            assert call(base + "/v1/harbor/status", {"job_id": "test"})[0] == 503
            assert time.monotonic() - started < 0.5
            assert first.result()[1]["result"]["status"] == "completed"
        # The slot must be released after the previous request.
        assert call(base + "/v1/turn", turn_payload)[1]["result"]["status"] == "completed"
    finally:
        server.shutdown()
        server.server_close()
