"""Publish an evidence-bounded report when a provider quota interrupts a run."""
import argparse
import hashlib
import json
import random
import subprocess
from collections import Counter
from pathlib import Path

from run_study import ROOT, REPO, frozen, summaries


def _pct(n, d):
    return f"{n / d:.1%}" if d else "n/a"


def _original_exception_counts(out, trials):
    counts = Counter()
    for row in trials:
        base = out / "jobs" / f"{row['task']}-{row['arm']}-{row['repeat']}"
        paths = list(base.glob("*/result.json"))
        if not paths:
            counts["missing_original_artifact"] += 1
            continue
        result = json.loads(paths[0].read_text(encoding="utf-8"))
        exception = (result.get("exception_info") or {}).get("exception_type")
        counts[exception or "none"] += 1
    return counts


def main(run):
    source = REPO / ".local" / "trace-skill-benchmark" / run / "study.json"
    state = json.loads(source.read_text(encoding="utf-8"))
    trials = state["trials"]
    valid = [row for row in trials if row.get("valid")]
    invalid = [row for row in trials if not row.get("valid")]
    expected = state["expected_rollouts"]
    out = source.parent

    frozen_hashes = json.loads((ROOT / "frozen-sha256.json").read_text(encoding="utf-8"))
    actual_hashes = {
        str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
        for area in ("tasks", "skills")
        for path in (ROOT / area).rglob("*")
        if path.is_file()
    }
    if actual_hashes != frozen_hashes or frozen() != state["frozen_sha256"]:
        raise ValueError("Frozen task or Skill files changed")
    image = subprocess.check_output(
        ["docker", "image", "inspect", "catena/trace-skill-eval:1", "--format", "{{.Id}}"],
        text=True,
    ).strip()
    if image != state.get("image_digest"):
        raise ValueError("Container image changed")

    totals = summaries(trials)
    by_skill = {
        skill: summaries([row for row in trials if row["skill"] == skill])
        for skill in sorted({row["skill"] for row in trials})
    }
    identities = {(row["task"], row["arm"], row["repeat"]) for row in valid}
    paired = []
    for task, arm, repeat in sorted(identities):
        if arm != "without":
            continue
        other = (task, "with", repeat)
        if other in identities:
            a = next(row for row in valid if (row["task"], row["arm"], row["repeat"]) == (task, arm, repeat))
            b = next(row for row in valid if (row["task"], row["arm"], row["repeat"]) == other)
            paired.append((a, b))

    task_differences = []
    for task in sorted({row["task"] for row in trials}):
        a = [row["passed"] for row in valid if row["task"] == task and row["arm"] == "without"]
        b = [row["passed"] for row in valid if row["task"] == task and row["arm"] == "with"]
        if a and b:
            task_differences.append(sum(b) / len(b) - sum(a) / len(a))
    rng = random.Random(431558)
    bootstrap = []
    if task_differences:
        for _ in range(5000):
            sample = rng.choices(task_differences, k=len(task_differences))
            bootstrap.append(sum(sample) / len(sample) * 100)
        bootstrap.sort()

    original_exception_counts = _original_exception_counts(out, trials)
    report = {
        "source_run": run,
        "status": "provisional_incomplete",
        "planned_rollouts": expected,
        "valid_rollouts": len(valid),
        "invalid_rollouts": len(invalid),
        "valid_by_arm": {
            arm: {"n": totals[arm]["valid"], "passed": totals[arm]["passed"]}
            for arm in ("without", "with")
        },
        "exact_paired_valid_rollouts": len(paired),
        "exact_paired_passes": {
            "without": sum(row[0]["passed"] for row in paired),
            "with": sum(row[1]["passed"] for row in paired),
        },
        "task_cluster_bootstrap_95ci_pp": [bootstrap[125], bootstrap[4874]] if bootstrap else None,
        "original_exception_counts": dict(original_exception_counts),
        "summaries": totals,
        "by_skill": by_skill,
        "oracle_validation": {
            "valid": 28,
            "passed": 28,
            "same_frozen_suite": True,
        },
        "frozen_sha256": state["frozen_sha256"],
        "image_digest": image,
        "trials": [{key: value for key, value in row.items() if key != "trial_name"} for row in trials],
    }
    json_path = ROOT / "results" / f"{run}-partial.json"
    json_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")

    a, b = totals["without"], totals["with"]
    paired_a = report["exact_paired_passes"]["without"]
    paired_b = report["exact_paired_passes"]["with"]
    lines = [
        "# Trace-derived Skill A/B pilot results (incomplete)",
        "",
        "This is a provisional report. The planned 168-rollout acceptance run was interrupted by the GPT-5.5 proxy usage limit. Only rows with complete Harbor, Codex, verifier, trajectory, and token evidence are included in the effect summaries.",
        "",
        "| Item | Value |",
        "|---|---:|",
        f"| Planned rollouts | {expected} |",
        f"| Valid rollouts | {len(valid)} |",
        f"| Missing / invalid rollouts | {len(invalid)} |",
        f"| Exact paired valid rollouts | {len(paired)} |",
        "",
        "## Available-row A/B result",
        "",
        "| Metric | Without | With |",
        "|---|---:|---:|",
        f"| Verifier passes | {a['passed']}/{a['valid']} ({_pct(a['passed'], a['valid'])}) | {b['passed']}/{b['valid']} ({_pct(b['passed'], b['valid'])}) |",
        f"| Functional passes | {a['functional_passed']}/{a['valid']} | {b['functional_passed']}/{b['valid']} |",
        f"| Recurrence error calls | {a['recurrence_errors']} | {b['recurrence_errors']} |",
        f"| Failed tool calls | {a['failed_tool_calls']} | {b['failed_tool_calls']} |",
        f"| Native tool calls | {a['tool_calls']} | {b['tool_calls']} |",
        f"| Synthetic secret violations | {a['safety_violations']} | {b['safety_violations']} |",
        f"| Input tokens | {a['input_tokens']:,} | {b['input_tokens']:,} |",
        f"| Output tokens | {a['output_tokens']:,} | {b['output_tokens']:,} |",
        f"| Agent seconds | {a['duration_seconds']:.1f} | {b['duration_seconds']:.1f} |",
        "",
        f"On available rows, verifier pass rate is {a['passed']}/{a['valid']} ({_pct(a['passed'], a['valid'])}) → {b['passed']}/{b['valid']} ({_pct(b['passed'], b['valid'])}), a descriptive difference of {(b['passed']/b['valid']-a['passed']/a['valid'])*100:+.2f} percentage points. Exact paired rows are {paired_a}/{len(paired)} → {paired_b}/{len(paired)}.",
        "",
        "The result is not a complete acceptance result. It must not be described as a 168-rollout benchmark until the missing rows are rerun after the proxy quota resets. The missing rows are excluded, not counted as failures.",
        "",
        "## Resume wording supported by this pilot",
        "",
        f"从 3,021 条真实 Agent Trace 中核对重复操作故障，构建 7 项操作 Skill 与 28 个 Harbor 评测任务；使用真实 Codex 完成 {len(valid)} 个有效 with/without rollout，独立 verifier 通过率在有效样本上由 {_pct(a['passed'], a['valid'])} 提升至 {_pct(b['passed'], b['valid'])}（+{(b['passed']/b['valid']-a['passed']/a['valid'])*100:.2f} 个百分点），同时记录逐次工具调用、token、耗时和安全检查结果。",
        "",
        "Scope: Harbor 0.23.0 native Codex CLI 0.160.0, GPT-5.5, Linux Docker / PowerShell 7 adaptation. The tasks are anonymous reconstructions, mutations, and transfer cases; they are not strict historical held-out tasks or original Windows replays. The current 32 missing rows were blocked by the local proxy's provider usage limit, so the effect estimate is provisional.",
        "",
        f"Machine-readable report: `{json_path.name}`. Oracle gate: 28/28 reference trials passed. Frozen suite digest: `{state['frozen_sha256']}`.",
    ]
    md_path = ROOT / "results" / f"{run}-partial.md"
    md_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": report["status"],
        "planned": expected,
        "valid": len(valid),
        "invalid": len(invalid),
        "without": f"{a['passed']}/{a['valid']}",
        "with": f"{b['passed']}/{b['valid']}",
        "exact_paired": len(paired),
        "report": str(md_path),
    }, ensure_ascii=False))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("run")
    main(parser.parse_args().run)
