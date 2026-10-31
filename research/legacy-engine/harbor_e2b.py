"""Harbor's E2B provider with bounded lifetimes and cleanup evidence."""
from __future__ import annotations

import json
from pathlib import Path, PurePosixPath

from e2b import AsyncSandbox
from harbor.environments.e2b import E2BEnvironment
from harbor.models.task.config import NetworkMode


class CatenaE2BEnvironment(E2BEnvironment):
    async def _create_sandbox(self):
        self._sandbox = await AsyncSandbox.create(
            template=self._template_name, metadata={"environment_name": self.environment_name,
                                                   "session_id": self.session_id, "platform": "catena"},
            envs=self._startup_env(), timeout=1200,
            allow_internet_access=self.network_policy.network_mode != NetworkMode.NO_NETWORK,
            network=self._sandbox_create_network_options())
        self._created_sandbox_id = self._sandbox.sandbox_id
        self._record({"sandbox_id": self._created_sandbox_id, "timeout_seconds": 1200, "created": True,
                      "kill_confirmed": False})

    def _record(self, values: dict) -> None:
        root = self.trial_paths.trial_dir
        root.mkdir(parents=True, exist_ok=True)
        path = root / "catena-e2b-lifecycle.json"
        existing = json.loads(path.read_text(encoding="utf-8")) if path.exists() else {}
        path.write_text(json.dumps({**existing, **values}, indent=2), encoding="utf-8")

    async def stop(self, delete: bool):
        # Harbor's stock provider suppresses kill exceptions. Keep that cleanup
        # evidence visible so a successful answer cannot hide an orphan sandbox.
        if self._sandbox is not None:
            try:
                await self._stop_sandbox()
                self._record({"kill_confirmed": True})
            except Exception as error:
                self._record({"kill_confirmed": False, "cleanup_error": type(error).__name__})
                raise
            finally:
                self._sandbox = None

    async def upload_file(self, source_path: Path | str, target_path: str):
        if self._sandbox is None:
            raise RuntimeError("sandbox is not running")
        await self._sandbox.files.write(target_path, Path(source_path).read_bytes(), user="root")

    async def upload_dir(self, source_dir: Path | str, target_dir: str):
        if self._sandbox is None:
            raise RuntimeError("sandbox is not running")
        from e2b import WriteEntry

        root = Path(source_dir)
        files = [WriteEntry(path=str(PurePosixPath(target_dir) / p.relative_to(root).as_posix()), data=p.read_bytes())
                 for p in root.rglob("*") if p.is_file()]
        for offset in range(0, len(files), self._UPLOAD_BATCH_SIZE):
            await self._sandbox.files.write_files(files[offset:offset + self._UPLOAD_BATCH_SIZE], user="root")
