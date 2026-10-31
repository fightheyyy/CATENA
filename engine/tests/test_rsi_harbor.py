import asyncio
import json
import threading
from pathlib import Path

import pytest

pytest.importorskip("harbor")

from catena_engine.evaluation.harbor import HarborCase, HarborEvaluator, task_digest
from catena_engine.evaluation.harbor_agent import CREDENTIALS, RSICodex
from catena_engine.protocol import ModelConfig
from catena_engine.rsi.contracts import Candidate, OptimizationError


@pytest.fixture
def task(tmp_path):
    root = tmp_path / "task"
    root.mkdir()
    (root / "instruction.md").write_text("Inspect signed amounts.")
    (root / "task.toml").write_text('schema_version = "1.4"\n')
    (root / "tests").mkdir()
    (root / "tests/answer.txt").write_text("PRIVATE_GRADER_SENTINEL")
    return HarborCase.load(str(root), task_digest(root))


def evaluator(tmp_path, task):
    return HarborEvaluator(
        {"case": task},
        tmp_path / "jobs",
        ModelConfig("openai", "https://example.test/v1", "fixture-model", "FAKE_PRIVATE_KEY"),
    )


def test_real_harbor_config_keeps_credentials_out_and_reads_native_evidence(monkeypatch, tmp_path, task):
    from harbor.job import Job

    observed = []

    async def create(config):
        spec = config.agents[0]
        RSICodex.preflight(spec.kwargs)
        agent = RSICodex(logs_dir=tmp_path / "agent", model_name=spec.model_name, **spec.kwargs)
        assert agent.extra_env["OPENAI_API_KEY"] == "FAKE_PRIVATE_KEY"
        assert "reasoning_effort=low" in agent.build_cli_flags()
        serialized = config.model_dump_json()
        assert "FAKE_PRIVATE_KEY" not in serialized
        assert "PRIVATE_GRADER_SENTINEL" not in serialized
        observed.append(config)

        class Execution:
            async def run(self):
                directory = config.jobs_dir / config.job_name / "trial"
                (directory / "agent/sessions").mkdir(parents=True)
                (directory / "agent/sessions/native.jsonl").write_text(
                    json.dumps(
                        {
                            "type": "response_item",
                            "payload": {
                                "type": "message",
                                "role": "assistant",
                                "content": [{"type": "output_text", "text": "answer"}],
                            },
                        }
                    )
                    + "\n"
                )
                (directory / "result.json").write_text(
                    json.dumps(
                        {
                            "agent_result": {"n_input_tokens": 15, "n_output_tokens": 2},
                            "verifier_result": {"rewards": {"reward": 0}},
                            "exception_info": None,
                        }
                    )
                )

        return Execution()

    monkeypatch.setattr(Job, "create", create)
    host = evaluator(tmp_path, task)
    assert "PRIVATE_GRADER_SENTINEL" not in host.task_prompt("case")
    trial = host.probe("case", Candidate({"AGENTS.md": "Inspect first."}), 2)
    assert trial.valid and trial.answer == "answer" and trial.rewards == {"reward": 0}
    assert (trial.input_tokens, trial.output_tokens) == (15, 2)
    assert not CREDENTIALS
    assert observed[0].environment.delete is True
    assert observed[0].n_concurrent_trials == 1


def test_harbor_timeout_cancels_job_and_removes_credential_reference(monkeypatch, tmp_path, task):
    from harbor.job import Job

    cleaned = threading.Event()

    async def create(config):
        class Slow:
            async def run(self):
                try:
                    await asyncio.sleep(60)
                finally:
                    cleaned.set()

        return Slow()

    monkeypatch.setattr(Job, "create", create)
    with pytest.raises(TimeoutError):
        evaluator(tmp_path, task).probe("case", Candidate({"AGENTS.md": "Inspect."}), 0.05)
    assert cleaned.is_set() and not CREDENTIALS


def test_changed_task_is_rejected_before_harbor_runs(tmp_path, task):
    (task.path / "instruction.md").write_text("changed")
    with pytest.raises(OptimizationError, match="changed"):
        evaluator(tmp_path, task).probe("case", Candidate({"AGENTS.md": "Inspect."}), 2)
    assert not CREDENTIALS


def test_harness_is_installed_without_changing_verifier(monkeypatch, tmp_path):
    from harbor.agents.installed.codex import Codex

    captured = {}
    CREDENTIALS["test"] = {"api_key": "FAKE_PRIVATE_KEY", "base_url": "https://example.test/v1"}

    class Environment:
        async def exec(self, command, **kwargs):
            return type("Result", (), {"stdout": "/workspace/repo\n", "return_code": 0})()

        async def upload_file(self, source, target):
            captured[target] = Path(source).read_text()

    async def run(self, instruction, environment, context):
        captured["ran"] = True

    monkeypatch.setattr(Codex, "run", run)
    try:
        agent = RSICodex(
            logs_dir=tmp_path,
            model_name="fixture",
            credential_ref="test",
            harness_files={"AGENTS.md": "Inspect.", ".agents/skills/search/SKILL.md": "Search."},
        )
        asyncio.run(agent.run("Task", Environment(), None))
        assert captured == {
            "/workspace/repo/AGENTS.md": "Inspect.",
            "/workspace/repo/.agents/skills/search/SKILL.md": "Search.",
            "ran": True,
        }
    finally:
        CREDENTIALS.pop("test", None)
