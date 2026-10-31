"""Install a validated harness into a disposable Harbor Codex workspace."""

from __future__ import annotations

import shlex
import tempfile
from pathlib import Path, PurePosixPath

from harbor.agents.installed.codex import Codex, CodexOptions

from ..rsi.contracts import validate_files

# Credentials are in-process references, never AgentConfig/JobConfig fields.
CREDENTIALS: dict[str, dict[str, str]] = {}


class RSICodexOptions(CodexOptions):
    credential_ref: str = ""
    harness_files: dict[str, str] = {}


class RSICodex(Codex):
    options_model = RSICodexOptions

    def __init__(self, *args, credential_ref: str, harness_files: dict, **kwargs):
        credentials = CREDENTIALS[credential_ref]
        super().__init__(*args, credential_ref=credential_ref, harness_files=harness_files,
                         extra_env={"OPENAI_API_KEY": credentials["api_key"],
                                    "OPENAI_BASE_URL": credentials["base_url"]}, **kwargs)
        self.harness_files = validate_files(harness_files)

    async def run(self, instruction, environment, context):
        probe = await environment.exec("pwd", timeout_sec=10)
        workspace = PurePosixPath((probe.stdout or "").strip())
        if probe.return_code or not workspace.is_absolute() or ".." in workspace.parts:
            raise ValueError("Harbor task workspace is unavailable")
        with tempfile.TemporaryDirectory(prefix="catena-rsi-install-") as directory:
            for rel, content in self.harness_files.items():
                destination = workspace / rel
                result = await environment.exec("mkdir -p " + shlex.quote(str(destination.parent)), timeout_sec=10)
                if result.return_code:
                    raise ValueError("harness installation failed")
                source = Path(directory) / "file"
                source.write_text(content, encoding="utf-8")
                await environment.upload_file(source, str(destination))
        await super().run(instruction, environment, context)
