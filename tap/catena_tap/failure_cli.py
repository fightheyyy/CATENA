"""Offline, privacy-conscious failure scan and human-label evaluation."""

from __future__ import annotations

import argparse
import csv
import json
import random
import subprocess
from pathlib import Path

from .claude_graph import parse_claude_transcript
from .failure_analysis import scan_graph, score_labels, summarize


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="python -m catena_tap.failure_cli")
    commands = parser.add_subparsers(dest="command", required=True)
    scan = commands.add_parser("scan", help="scan canonical graphs or local coding-agent history")
    scan.add_argument("--graph-dir", type=Path, help="directory of canonical graph JSON files")
    scan.add_argument("--codex-root", type=Path, help="Codex sessions directory")
    scan.add_argument("--claude-root", type=Path, help="Claude transcripts directory")
    scan.add_argument("--codex-bundle", type=Path, help="built Tap Codex parser")
    scan.add_argument("--max-files", type=int, default=0, help="0 scans all matching files")
    scan.add_argument("--max-file-mib", type=int, default=32)
    scan.add_argument("--output", type=Path, required=True)
    sample = commands.add_parser("sample", help="create a balanced, reproducible labeling sheet")
    sample.add_argument("report", type=Path)
    sample.add_argument("--per-class", type=int, default=30)
    sample.add_argument("--seed", type=int, default=17)
    sample.add_argument("--output", type=Path, required=True)
    score = commands.add_parser("score", help="score a fully reviewed labeling sheet")
    score.add_argument("report", type=Path)
    score.add_argument("labels", type=Path)
    score.add_argument("--output", type=Path, required=True)
    return parser


def _sources(args: argparse.Namespace) -> list[tuple[str, Path]]:
    sources = []
    for kind, root, pattern in (
        ("graph", args.graph_dir, "*.json"),
        ("codex", args.codex_root, "*.jsonl"),
        ("claude", args.claude_root, "*.jsonl"),
    ):
        if root:
            if not root.is_dir():
                raise ValueError(f"{kind} directory does not exist: {root}")
            sources.extend((kind, path) for path in root.rglob(pattern) if path.is_file())
    if not sources:
        raise ValueError("provide at least one source directory with matching files")
    sources.sort(key=lambda item: (item[0], str(item[1])))
    return sources[:args.max_files] if args.max_files > 0 else sources


def _graph(kind: str, path: Path, bundle: Path | None) -> dict:
    if kind == "graph":
        return json.loads(path.read_text(encoding="utf-8"))
    if kind == "claude":
        return parse_claude_transcript(path)
    if bundle is None or not bundle.is_file():
        raise ValueError("Codex analysis requires --codex-bundle pointing to the built Tap parser")
    result = subprocess.run(["node", str(bundle), "import", str(path)], capture_output=True, timeout=180, check=False)
    if result.returncode:
        raise ValueError(f"Codex parser exited with status {result.returncode}")
    return json.loads(result.stdout)


def _write_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def _scan(args: argparse.Namespace) -> None:
    if args.max_file_mib < 1:
        raise ValueError("max-file-mib must be positive")
    turns = []
    skipped = []
    for kind, path in _sources(args):
        if path.stat().st_size > args.max_file_mib * 1024 * 1024:
            skipped.append({"source": path.name, "reason": "file_size_limit"})
            continue
        try:
            turns.extend(scan_graph(_graph(kind, path, args.codex_bundle)))
        except (OSError, ValueError, subprocess.TimeoutExpired, json.JSONDecodeError) as error:
            skipped.append({"source": path.name, "reason": type(error).__name__})
    report = summarize(turns, skipped)
    _write_json(args.output, report)
    print(f"{report['sessions']} sessions, {report['eligible_turns']} turns, "
          f"{report['turns_with_signals']} flagged turns, {len(skipped)} skipped sources")


def _sample(args: argparse.Namespace) -> None:
    if args.per_class < 1:
        raise ValueError("per-class must be positive")
    report = json.loads(args.report.read_text(encoding="utf-8"))
    rng = random.Random(args.seed)
    flagged = [turn for turn in report["turns"] if turn["signals"]]
    quiet = [turn for turn in report["turns"] if not turn["signals"]]
    chosen = rng.sample(flagged, min(args.per_class, len(flagged))) + rng.sample(quiet, min(args.per_class, len(quiet)))
    rng.shuffle(chosen)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", encoding="utf-8", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=["trace_id", "turn_id", "runtime", "source_file", "signal_count", "label"])
        writer.writeheader()
        for turn in chosen:
            writer.writerow({"trace_id": turn["trace_id"], "turn_id": turn.get("turn_id", ""),
                             "runtime": turn["runtime"],
                             "source_file": turn.get("source_file", ""),
                             "signal_count": len(turn["signals"]), "label": ""})
    print(f"{len(chosen)} turns selected; review each source Trace and label actionable, normal, or uncertain")


def _score(args: argparse.Namespace) -> None:
    report = json.loads(args.report.read_text(encoding="utf-8"))
    with args.labels.open(encoding="utf-8", newline="") as stream:
        labels = list(csv.DictReader(stream))
    result = score_labels(report, labels)
    _write_json(args.output, result)
    print(f"{result['reviewed_turns']} reviewed turns; FPR={result['false_positive_rate']}")


def main() -> None:
    args = _parser().parse_args()
    try:
        {"scan": _scan, "sample": _sample, "score": _score}[args.command](args)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        raise SystemExit(str(error)) from error


if __name__ == "__main__":
    main()
