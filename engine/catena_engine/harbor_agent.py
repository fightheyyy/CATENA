"""Harbor adapter for the frozen SDK search experiment (not Codex CLI)."""
from __future__ import annotations

import json
import os
import shlex
import time
from pathlib import Path

from agents import Agent, ModelSettings, OpenAIChatCompletionsModel, RunConfig, Runner, function_tool
from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext
from openai import AsyncOpenAI

# Credentials stay in the worker process, never in Harbor's serialized config.
JOB_MODELS: dict[str, dict] = {}


class CatenaSearchAgent(BaseAgent):
    def __init__(self, *args, credential_ref: str | None = None, **kwargs):
        super().__init__(*args, **kwargs)
        self.credential_ref = credential_ref

    @staticmethod
    def name() -> str:
        return "catena-sdk-search"

    def version(self) -> str:
        return "1.0.0"

    async def setup(self, environment: BaseEnvironment) -> None:
        probe = await environment.exec(
            "pwsh -NoProfile -NonInteractive -File /opt/read_only_shell.ps1 "
            "-Command 'Get-Command rg -ErrorAction SilentlyContinue'", cwd="/workspace/repo", timeout_sec=20,
        )
        if probe.return_code or (probe.stdout or "").strip():
            raise RuntimeError("absent-rg environment precondition failed")

    async def run(self, instruction: str, environment: BaseEnvironment, context: AgentContext) -> None:
        calls: list[dict] = []

        @function_tool
        async def shell(command: str) -> str:
            """Run literal read-only PowerShell pipelines in the fixture repository.

            Commands: rg, Get-Command, Get-ChildItem, Get-Content, Select-String,
            Select-Object, Format-List, ConvertTo-Json. Use relative paths only.
            Scripts, variables, expressions, writes and other executables are unsupported.
            """
            result = await environment.exec(
                "pwsh -NoProfile -NonInteractive -File /opt/read_only_shell.ps1 -Command " + shlex.quote(command),
                cwd="/workspace/repo", timeout_sec=20, user="catena-eval",
            )
            record = {"command": command, "exit_code": result.return_code,
                      "stdout": (result.stdout or "")[:24000], "stderr": (result.stderr or "")[:6000]}
            calls.append(record)
            return json.dumps(record, ensure_ascii=False)

        credentials = JOB_MODELS[self.credential_ref] if self.credential_ref else {
            "base_url": os.environ["CATENA_EVAL_BASE_URL"],
            "api_key": Path(os.environ["CATENA_EVAL_KEY_FILE"]).read_text().strip()}
        client = AsyncOpenAI(base_url=credentials["base_url"],
                             api_key=credentials["api_key"],
                             timeout=90, max_retries=0)
        try:
            started = time.monotonic()
            model = OpenAIChatCompletionsModel(model=self.model_name, openai_client=client)
            agent = Agent(name="CatenaSearchTarget", model=model, tools=[shell],
                          instructions="Inspect the read-only fixture with the shell tool. Return only the requested JSON.",
                          model_settings=ModelSettings(parallel_tool_calls=False))
            result = await Runner.run(agent, instruction, max_turns=10, run_config=RunConfig(tracing_disabled=True))
            usage = result.context_wrapper.usage
            context.n_input_tokens = usage.input_tokens
            context.n_output_tokens = usage.output_tokens
            context.n_cache_tokens = usage.input_tokens_details.cached_tokens
            context.metadata = {"command_count": len(calls), "environment": "Linux/PowerShell 7",
                                "duration_seconds": round(time.monotonic() - started, 3),
                                "missing_rg_errors": sum(c["exit_code"] != 0 and "rg" in c["command"]
                                                         and "not recognized" in c["stdout"] + c["stderr"] for c in calls),
                                "policy_rejections": sum("EVAL_POLICY" in c["stdout"] + c["stderr"] for c in calls)}
            self.logs_dir.mkdir(parents=True, exist_ok=True)
            answer = self.logs_dir / "agent.final.txt"
            answer.write_text(str(result.final_output), encoding="utf-8")
            (self.logs_dir / "tool-calls.json").write_text(json.dumps(calls, ensure_ascii=False, indent=2), encoding="utf-8")
            await environment.upload_file(answer, "/workspace/agent.final.txt")
        finally:
            await client.close()
