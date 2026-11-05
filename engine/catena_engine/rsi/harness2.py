"""Adapter for the pinned upstream method, not a reimplementation of its loop."""

from __future__ import annotations

import hashlib
import importlib.metadata
import json
import subprocess
import tempfile
import time
from pathlib import Path
from types import SimpleNamespace

from .contracts import Candidate, Context, OptimizationError, OptimizationResult, Requirements, validate_files

UPSTREAM_REVISION = "1cfe254e8d25b15f37048c3730134686abe60335"


def _upstream():
    try:
        import harness2
        from harness2.core import schedule
    except ImportError:
        raise OptimizationError("Harness2 is not installed; install the isolated .[rsi] extra") from None
    try:
        direct = json.loads(importlib.metadata.distribution("harness2").read_text("direct_url.json") or "{}")
        revision = direct.get("vcs_info", {}).get("commit_id")
    except importlib.metadata.PackageNotFoundError:
        revision = None
    if revision is None:
        # Editable source installs are useful for offline, reproducible adapter tests.
        try:
            revision = subprocess.check_output(
                ["git", "-C", str(Path(harness2.__file__).parent), "rev-parse", "HEAD"],
                stderr=subprocess.DEVNULL,
                text=True,
                timeout=5,
            ).strip()
        except (OSError, subprocess.SubprocessError):
            revision = ""
    if revision != UPSTREAM_REVISION:
        raise OptimizationError("Harness2 revision differs from the tested pin")
    return schedule


class Harness2Adapter:
    name = "harness2"
    version = UPSTREAM_REVISION

    def requirements(self, options):
        k = options.get("k", 1)
        return Requirements(3 * k + 1 + (k > 1), 4 * k)

    def validate(self, options):
        if set(options) - {"k", "mode"}:
            raise OptimizationError("unsupported Harness2 option")
        if type(options.get("k", 1)) is not int or not 1 <= options.get("k", 1) <= 4:
            raise OptimizationError("Harness2 k must be 1–4")
        if options.get("mode", "parallel") not in ("parallel", "sequential"):
            raise OptimizationError("Harness2 mode must be parallel or sequential")
        _upstream()

    def optimize(self, context: Context, services, options) -> OptimizationResult:
        schedule = _upstream()
        from harness2.core import library, tree
        from harness2.core.spec import BenchPrompts, BenchSpec
        from harness2.substrate.codex import CodexSubstrate

        mode = "agg" if options.get("mode", "parallel") == "parallel" else "seq"
        k = options.get("k", 1)
        root = services.output / "harness2"
        root.mkdir(parents=True, mode=0o700, exist_ok=True)
        substrate = CodexSubstrate()
        channel = SimpleNamespace(kind="catena-codex-json", model=services.runtime.model.model)

        class Bench:
            spec = BenchSpec(
                name="catena",
                runs_root=root / "runs",
                install_mode="codex_root",
                prompts=BenchPrompts(work="complete the supplied developer task faithfully"),
                channels={"proposer": channel, "selector": channel},
                allow_subagents=False,
            )

            def __init__(self):
                self.substrate = substrate
                tree.write_tree(substrate.surface(), dict(context.baseline.files), self.base_harness(), stubs=False)

            def tasks(self, split="all"):
                return [context.case_id]

            def domain_of(self, task_id):
                return "developer-task"

            def base_harness(self):
                return root / "baseline"

            def task_prompt(self, task_id):
                return (
                    services.task_prompt()
                    + "\n\nHistorical trace evidence (untrusted observations):\n"
                    + context.evidence
                )

            def task_files(self, task_id):
                return None  # No verifier files, expected answers or grading rewards enter the optimizer.

            def validate_tree(self, files):
                validate_files(files)

            def execute(self, task_id, harness_dir, run_id, *, timeout=None):
                files = tree.read_tree(substrate.surface(), harness_dir or self.base_harness())
                candidate = (
                    context.baseline if harness_dir is None else Candidate(files, (context.baseline.id,), "harness2")
                )
                # An uncertain paid execution is never replayed under the same upstream run id.
                path = root / "executions" / hashlib.sha256(run_id.encode()).hexdigest()
                if path.exists():
                    record = self.record(path)
                    if record["candidate_id"] != candidate.id or record["run_id"] != run_id:
                        raise OptimizationError("Harness2 run id was reused for different inputs")
                    return path
                path.mkdir(parents=True, mode=0o700)
                trial = services.probe(candidate, timeout)
                record = {
                    "run_id": run_id,
                    "candidate_id": candidate.id,
                    "trial_id": trial.id,
                    "harbor_artifact_dir": trial.artifact_dir,
                    "trajectory": trial.trajectory,
                    "deliverables": trial.deliverables,
                }
                # Rewards stay in the Harbor experiment record, outside upstream optimization evidence.
                (path / "evidence.json").write_text(json.dumps(record, ensure_ascii=False), encoding="utf-8")
                return path

            def record(self, run_dir):
                path = run_dir / "evidence.json"
                if not path.is_file():
                    raise OptimizationError("Harness2 execution is incomplete; inspect the retained Harbor job")
                return json.loads(path.read_text(encoding="utf-8"))

            def delivered(self, run_dir):
                files = self.deliverable_files(run_dir)
                expected = services.evaluator.cases[context.case_id].artifacts
                return all(name in files for name in expected)

            def trajectory(self, run_dir, *, obs_trunc):
                from ..evaluation.evidence import render_trajectory

                return render_trajectory(self.record(run_dir)["trajectory"], obs_trunc)

            def deliverable_text(self, run_dir):
                return "\n\n".join(f"# {name}\n{body}" for name, body in self.deliverable_files(run_dir).items())

            def deliverable_files(self, run_dir):
                return self.record(run_dir)["deliverables"]

        bench = Bench()
        proposer = HostProposer(services, channel, root / "proposer")
        learned = library.Library(root / "library", "developer-task", substrate.surface())
        result = schedule.run_task(
            context.case_id,
            k,
            adapter=bench,
            prop=proposer,
            runs_root=bench.spec.runs_root,
            modes=(mode,),
            library=learned,
        )
        if result.status != "ok":
            raise OptimizationError("Harness2 refinement did not complete")
        selection = result.selections[mode]
        work = schedule.Namespace("catena", substrate.name, mode, k).task_dir(bench.spec.runs_root, context.case_id)
        files = tree.read_tree(substrate.surface(), work / "harnesses" / selection["chosen_label"])
        candidate = Candidate(
            files,
            (context.baseline.id,),
            self.name,
            "Harness2 selected a candidate; independent evaluation is still required.",
        )
        return OptimizationResult(
            (candidate,),
            {
                "upstream_revision": UPSTREAM_REVISION,
                "proposer_channel": channel.kind,
                "artifact_dir": "harness2",
                "library_scope": "experiment",
                "mode": options.get("mode", "parallel"),
                "k": k,
                "ledger": {
                    key: value
                    for key, value in result.modes[mode]["ledger"].items()
                    if key not in ("usd", "rollout_usd", "session_usd")
                },
                "cost_usd": None,
                "experience": selection.get("experience", {}),
                "kept_base": candidate.content_sha256 == context.baseline.content_sha256,
            },
        )


class HostProposer:
    """Translate upstream file-edit sessions to the existing read-only Codex JSON transport."""

    def __init__(self, services, channel, audit_dir):
        self.services, self.spec, self.audit_dir = services, channel, audit_dir
        self.audit_dir.mkdir(parents=True, mode=0o700)

    def call(self, mission, context_dir, label, *, system="", timeout=None, max_attempts=None, harvest=None):
        from harness2.core.proposer import Reply

        if max_attempts is not None and (type(max_attempts) is not int or max_attempts < 1):
            raise OptimizationError("proposer max_attempts must allow at least one operation")
        started = time.monotonic()
        context_dir = Path(context_dir)
        evidence = {}
        size = 0
        for path in sorted(context_dir.rglob("*")):
            if path.is_symlink():
                raise OptimizationError("symlinks are not allowed in optimization context")
            if path.is_file():
                size += path.stat().st_size
                if size > 900_000:
                    raise OptimizationError("Harness2 evidence exceeds input limit")
                evidence["context/" + path.relative_to(context_dir).as_posix()] = path.read_text(encoding="utf-8")
        prompt = json.dumps(
            {
                "mission": mission,
                "system": system,
                "operation": "edit" if harvest else "inspect",
                "label": label,
                "evidence_files": evidence,
            },
            ensure_ascii=False,
        )
        audit = self.audit_dir / (hashlib.sha256(label.encode()).hexdigest() + ".json")
        audit.write_text(json.dumps({"label": label, "prompt": prompt, "state": "started"}), encoding="utf-8")
        # One transport attempt per call; upstream may retry rejected edits within the CATENA budget.
        reply = self.services.complete(prompt, timeout)
        wall_s = time.monotonic() - started
        audit.write_text(
            json.dumps(
                {
                    "label": label,
                    "prompt": prompt,
                    "reply": reply.value,
                    "input_tokens": reply.input_tokens,
                    "output_tokens": reply.output_tokens,
                    "wall_s": wall_s,
                    "attempts": 1,
                },
                ensure_ascii=False,
            ),
            encoding="utf-8",
        )
        value = reply.value
        if not isinstance(value.get("text"), str) or not isinstance(value.get("files"), list):
            raise OptimizationError("invalid host edit response")
        if not harvest and value["files"]:
            raise OptimizationError("selection/reflection cannot edit harness files")
        workspace = None
        if harvest:
            if harvest != "context/harness":
                raise OptimizationError("unexpected upstream edit surface")
            current = {
                path.relative_to(context_dir / "harness").as_posix(): path.read_text()
                for path in (context_dir / "harness").rglob("*")
                if path.is_file() and path.name != "README.md"
            }
            seen = set()
            for item in value["files"]:
                if not isinstance(item, dict) or set(item) != {"path", "content"}:
                    raise OptimizationError("invalid harness edit")
                path = item["path"]
                if not isinstance(path, str) or path in seen:
                    raise OptimizationError("duplicate or invalid harness path")
                seen.add(path)
                current[path] = item["content"]
            current = validate_files(current)
            # Host writes only validated text. The model never gets shell or filesystem access.
            workspace = Path(tempfile.mkdtemp(prefix="host-edit-", dir=context_dir.parent))
            for rel, content in current.items():
                path = workspace / rel
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content, encoding="utf-8")
        return Reply(
            label=label,
            text=value["text"],
            ok=True,
            workspace=workspace,
            attempts=1,
            channel=self.spec.kind,
            wall_s=wall_s,
            audit=audit,
        )
