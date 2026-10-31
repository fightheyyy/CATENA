import pytest
@pytest.fixture
def turn_payload():
    return {"request_id":"operation-1","run_id":"job-1","prompt":"Inspect the supplied evidence.","timeout_ms":5000,"model":{"provider":"openai","base_url":"http://127.0.0.1:9999/v1","model":"fixture","api_key":"sk-test-secret-value"}}
