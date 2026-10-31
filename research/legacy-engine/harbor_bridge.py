"""Private bounded transport for durable control-plane experiment records."""
import os

import httpx


async def harbor_request(path: str, payload: dict) -> tuple[int, dict]:
    url = os.environ.get("CATENA_HARBOR_WORKER_URL")
    token = os.environ.get("CATENA_HARBOR_WORKER_TOKEN")
    if not url or not token:
        return 503, {"error": "Harbor worker is not configured"}
    if path.endswith("submit"):
        if payload.get("case_id") != "rg-missing-search" or payload.get("environment") != "docker":
            return 400, {"error": "Unsupported Case or environment"}
        model = payload.get("model")
        if not isinstance(model, dict) or any(not model.get(k) for k in ("model", "base_url", "api_key")):
            return 400, {"error": "Model configuration is required"}
        endpoint = "/submit"
    elif path.endswith("lookup"):
        if not isinstance(payload.get("request_id"), str):
            return 400, {"error": "Request ID is required"}
        endpoint = "/lookup"
        payload = {"request_id": payload["request_id"]}
    elif path.endswith("evidence"):
        if not isinstance(payload.get("job_id"), str) or type(payload.get("trial_index")) is not int or not 0 <= payload["trial_index"] < 18:
            return 400, {"error": "Valid trial reference is required"}
        endpoint = "/evidence"
        payload = {"job_id": payload["job_id"], "trial_index": payload["trial_index"]}
    else:
        if not isinstance(payload.get("job_id"), str):
            return 400, {"error": "Job ID is required"}
        endpoint = "/status"
        payload = {"job_id": payload["job_id"]}
    try:
        async with httpx.AsyncClient(timeout=20) as client:
            response = await client.post(url + endpoint, json=payload,
                                         headers={"Authorization": "Bearer " + token})
            if response.status_code == 400:
                if endpoint == "/lookup":
                    return 404, {"error": "Experiment request not found"}
                return 409, {"error": "Harbor rejected this request; another experiment may be running"}
            response.raise_for_status()
            return 200, response.json()
    except Exception:
        return 503, {"error": "Harbor worker is unavailable"}
