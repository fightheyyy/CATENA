import concurrent.futures
import threading

from test_server import call

from catena_engine.server import create_server


def test_rsi_is_authenticated_and_shares_analysis_capacity(turn_payload):
    started, release = threading.Event(), threading.Event()

    def optimize(payload):
        started.set()
        assert release.wait(2)
        return {"schema": "catena.rsi_result.v1", "status": "completed"}

    server = create_server("127.0.0.1:0", token="private", rsi_handler=optimize)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_port}"
    try:
        assert call(base + "/v1/rsi/adapters")[0] == 401
        assert call(base + "/v1/rsi/optimize", {})[0] == 401
        assert call(base + "/v1/rsi/adapters", token="private")[1]["adapters"][0]["name"] == "harness2"
        with concurrent.futures.ThreadPoolExecutor() as pool:
            future = pool.submit(call, base + "/v1/rsi/optimize", {}, "private")
            assert started.wait(2)
            assert call(base + "/v1/turn", turn_payload, "private")[0] == 429
            assert call(base + "/v1/rsi/optimize", {}, "private")[0] == 429
            assert call(base + "/healthz")[0] == 200
            release.set()
            assert future.result()[0] == 200
        assert call(base + "/v1/rsi/optimize", {}, "private")[0] == 200
    finally:
        release.set()
        server.shutdown()
        server.server_close()


def test_rsi_failure_is_safe_and_releases_capacity():
    def broken(payload):
        raise RuntimeError("SECRET_KEY")
    server = create_server("127.0.0.1:0", rsi_handler=broken)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_port}"
    try:
        status, body = call(base + "/v1/rsi/optimize", {})
        assert status == 500 and "SECRET_KEY" not in str(body)
        assert call(base + "/v1/rsi/optimize", {})[0] == 500  # Not a leaked slot/429.
    finally:
        server.shutdown()
        server.server_close()
