"""Write private OV config from deployment credentials without printing secrets."""
import json
import os
from pathlib import Path
import secrets

root = Path(__file__).resolve().parents[2]
directory = Path(os.getenv("CATENA_OPENVIKING_CONFIG_DIR", str(root / ".local" / "openviking")))
directory.mkdir(parents=True, exist_ok=True)
config_file = directory / "ov.conf"
existing = json.loads(config_file.read_text(encoding="utf-8-sig")) if config_file.exists() else {}
llm_key = os.getenv("CATENA_LLM_API_KEY") or existing.get("vlm", {}).get("api_key")
if not llm_key:
    raise SystemExit("Set CATENA_LLM_API_KEY to your OpenAI-compatible model key first.")
service_key = os.getenv("CATENA_OPENVIKING_API_KEY") or existing.get("server", {}).get("root_api_key") or secrets.token_hex(32)
config = {
    "storage": {"workspace": "/app/.openviking/data", "vectordb": {"name": "context", "backend": "local"}, "agfs": {"backend": "local"}},
    "server": {"host": "0.0.0.0", "port": 1933, "auth_mode": "trusted", "root_api_key": service_key},
    "memory": {"link_enabled": True},
    "embedding": {"dense": {"provider": "openai", "api_base": "http://catena-embedding:8080/v1", "api_key": "local", "model": "BAAI/bge-small-zh-v1.5", "dimension": 512, "encoding_format": "float", "max_input_tokens": 512}},
    "vlm": {"provider": "openai", "api_base": os.getenv("CATENA_LLM_BASE_URL", existing.get("vlm", {}).get("api_base", "http://host.docker.internal:8317/v1")), "api_key": llm_key, "model": os.getenv("CATENA_LLM_MODEL", existing.get("vlm", {}).get("model", "gpt-5.5"))},
}
config_file.write_text(json.dumps(config, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
env_file = directory / "service.env"
env_file.write_text(f"CATENA_OPENVIKING_API_KEY={service_key}\n", encoding="utf-8")
for private_file in (config_file, env_file):
    private_file.chmod(0o600)
print(f"Configuration saved in {directory}. No credentials printed.")
