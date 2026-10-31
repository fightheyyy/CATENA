"""Invoke the live platform engine experiment endpoint with local credentials."""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import uuid
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--resume-job")
    parser.add_argument("--environment", choices=["docker", "e2b"], default="docker")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    root = repo / ".local/harbor-study"
    key_file = Path(os.environ["LOCALAPPDATA"]) / "DuMemEval/cliproxyapi-7.3.2/client-key.txt"
    request = {"schema": "catena.engine_turn_request.v1", "request_id": uuid.uuid4().hex, "environment": args.environment,
               "run_id": "resume-" + args.resume_job if args.resume_job else uuid.uuid4().hex, "role": "inspector", "timeout_ms": 900000,
               "prompt": f"调用 Harbor 使用 {args.environment} 环境跑 rg-missing-search 的无 Skill/有 Skill 对照，等待全部结果，核对实验有效性并用中文总结。缺少凭证则明确报告阻塞，不切换环境。",
               "model": {"provider": "openai", "base_url": "http://host.docker.internal:8317/v1",
                         "model": "gpt-5.5", "api_key": key_file.read_text().strip()}}
    code = (
        "import os,json,urllib.request\n"
        f"payload={request!r}\n"
        "request=urllib.request.Request('http://127.0.0.1:8790/v1/experiment',data=json.dumps(payload).encode(),"
        "headers={'Content-Type':'application/json','Authorization':'Bearer '+os.environ['CATENA_ENGINE_TOKEN']})\n"
        "with urllib.request.urlopen(request,timeout=930) as response: print(response.read().decode())\n"
    )
    result = subprocess.run(["docker", "exec", "-i", "catena-catena-engine-1", "python", "-"],
                            input=code, text=True, encoding="utf-8", capture_output=True, timeout=960)
    if result.returncode:
        print(result.stderr.replace(request["model"]["api_key"], "[redacted]")[:2000])
        raise SystemExit(result.returncode)
    response = json.loads(result.stdout)
    target = "platform-response.json" if args.environment == "docker" else "platform-response-e2b.json"
    (root / target).write_text(json.dumps(response, ensure_ascii=False, indent=2), encoding="utf-8")
    outcome = response["result"]
    print(json.dumps({"status": outcome["status"], "tools": outcome.get("tool_events"),
                      "summary": outcome.get("assistant")}, ensure_ascii=True))


if __name__ == "__main__":
    main()
