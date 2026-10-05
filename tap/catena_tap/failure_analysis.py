"""Conservative, reproducible issue candidates from canonical coding-agent turns.

These signals describe observable calls. They never infer task success or root cause.
Raw prompts, tool arguments, and tool results are deliberately absent from output.
"""

from __future__ import annotations

import hashlib
import json
import re
from collections import defaultdict
from typing import Any

SCHEMA = "catena.failure_scan.v1"
SIGNALS = ("unrecovered_tool_error", "repeated_failed_call", "repeated_identical_call")
_EXIT_CODE = re.compile(r"(?m)^Exit code:\s*(-?\d+)\s*$")
_NESTED_TOOL = re.compile(r"\btools\.([a-zA-Z][a-zA-Z0-9_]*)\s*\(")
_PASSIVE_TOOLS = {"wait", "wait_agent", "list_agents", "write_stdin", "sleep", "view_image", "mcp__codex_app__wait_threads"}


def _tool_name(node: dict[str, Any]) -> str:
    attributes = node.get("attributes") or {}
    value = attributes.get("gen_ai.tool.name") or node.get("name") or "unknown"
    name = str(value).strip().lower()[:80]
    if name == "exec" and isinstance(node.get("input"), str):
        # Some Codex traces expose the functions.exec wrapper, so a repeated
        # poll can look like a repeated execution. Unwrap only unambiguous JS.
        nested = set(_NESTED_TOOL.findall(node["input"]))
        if len(nested) == 1:
            return next(iter(nested)).lower()[:80]
    return name


def _fingerprint(value: Any) -> str:
    encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, default=str).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()[:16]


def _failed(node: dict[str, Any]) -> bool:
    if node.get("state") == "error":
        return True
    output = node.get("output")
    if not isinstance(output, str):
        return False
    # Codex's older result envelope may say state=ok while carrying the shell
    # process exit status in this exact line. A nonzero exit is tool evidence,
    # not a verdict on the user's task.
    return any(int(match.group(1)) != 0 for match in _EXIT_CODE.finditer(output[:8192]))


def scan_graph(graph: dict[str, Any]) -> list[dict[str, Any]]:
    if graph.get("schema_version") != "catena.coding_agent.event_graph.v1":
        raise ValueError("unsupported canonical graph schema")
    runtime, session_id = graph.get("runtime"), graph.get("session_id")
    if not isinstance(runtime, str) or not isinstance(session_id, str):
        raise ValueError("canonical graph lacks runtime or session ID")
    result = []
    source = graph.get("source") or {}
    source_file = str(source.get("path", "")) if isinstance(source, dict) else ""
    for trace in graph.get("traces", []):
        if not isinstance(trace, dict) or not isinstance(trace.get("trace_id"), str):
            raise ValueError("canonical graph contains an invalid trace")
        tools = [node for node in trace.get("nodes", []) if isinstance(node, dict) and node.get("kind") == "tool"]
        errors: dict[str, list[dict[str, Any]]] = defaultdict(list)
        successful: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for node in tools:
            name = _tool_name(node)
            if _failed(node):
                errors[name].append(node)
            elif node.get("state") == "ok":
                successful[name].append(node)
        findings = []
        for name, failed in sorted(errors.items()):
            # A later successful call of the same tool is only observed recovery,
            # not proof the user's task was completed.
            last_error = max(_order(node) for node in failed)
            if not any(_order(node) > last_error for node in successful[name]):
                findings.append(_finding("unrecovered_tool_error", name, failed))
        streak: list[dict[str, Any]] = []
        streak_key: tuple[str, str] | None = None

        def flush() -> None:
            if not streak or streak_key is None:
                return
            name = streak_key[0]
            if name not in _PASSIVE_TOOLS and len(streak) >= 3:
                findings.append(_finding("repeated_identical_call", name, streak))
            failed_streak: list[dict[str, Any]] = []
            for call in streak:
                if _failed(call):
                    failed_streak.append(call)
                else:
                    if len(failed_streak) >= 2:
                        findings.append(_finding("repeated_failed_call", name, failed_streak))
                    failed_streak = []
            if len(failed_streak) >= 2:
                findings.append(_finding("repeated_failed_call", name, failed_streak))

        for node in tools:
            key = (_tool_name(node), _fingerprint(node.get("input")))
            if key != streak_key:
                flush()
                streak, streak_key = [], key
            streak.append(node)
        flush()
        result.append({
            "runtime": runtime,
            "session_id": session_id,
            "source_file": source_file,
            "trace_id": trace["trace_id"],
            "turn_id": str(trace.get("turn_id", "")),
            "turn_state": trace.get("state", "unknown"),
            "tool_calls": len(tools),
            "signals": findings,
        })
    return result


def _order(node: dict[str, Any]) -> tuple[int, str]:
    try:
        timestamp = int(node.get("end_time_unix_nano") or node.get("start_time_unix_nano") or 0)
    except (TypeError, ValueError):
        timestamp = 0
    return timestamp, str(node.get("key", ""))


def _finding(signal: str, tool: str, nodes: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "signal": signal,
        "tool": tool,
        "observed_calls": len(nodes),
        "node_keys": [str(node.get("key", "")) for node in nodes[:8]],
        "interpretation": "review_required",
    }


def summarize(turns: list[dict[str, Any]], skipped: list[dict[str, str]] | None = None) -> dict[str, Any]:
    unique = {(turn["runtime"], turn["session_id"], turn["trace_id"]): turn for turn in turns}
    ordered = sorted(unique.values(), key=lambda turn: (turn["runtime"], turn["session_id"], turn["trace_id"]))
    groups: dict[tuple[str, str], list[str]] = defaultdict(list)
    for turn in ordered:
        for signal in turn["signals"]:
            groups[(signal["signal"], signal["tool"])].append(turn["trace_id"])
    return {
        "schema": SCHEMA,
        "eligible_turns": len(ordered),
        "sessions": len({(turn["runtime"], turn["session_id"]) for turn in ordered}),
        "turns_with_signals": sum(bool(turn["signals"]) for turn in ordered),
        "incomplete_turns": sum(turn["turn_state"] in {"aborted", "incomplete"} for turn in ordered),
        "turns_without_tool_calls": sum(turn["tool_calls"] == 0 for turn in ordered),
        "groups": [
            {"signal": signal, "tool": tool, "affected_turns": len(set(ids)), "eligible_turns": len(ordered),
             "sample_trace_ids": sorted(set(ids))[:5], "status": "hypothesis"}
            for (signal, tool), ids in sorted(groups.items())
        ],
        "turns": ordered,
        "skipped_sources": skipped or [],
    }


def score_labels(report: dict[str, Any], labels: list[dict[str, str]]) -> dict[str, Any]:
    """Estimate population rates from random samples stratified by prediction."""
    predictions = {turn["trace_id"]: bool(turn["signals"]) for turn in report["turns"]}
    confusion = {"tp": 0, "fp": 0, "tn": 0, "fn": 0}
    seen = set()
    uncertain = 0
    sampled_positive = 0
    sampled_negative = 0
    for row in labels:
        trace_id, label = row.get("trace_id"), row.get("label")
        if trace_id in seen or trace_id not in predictions or label not in {"actionable", "normal", "uncertain"}:
            raise ValueError("labels must contain unique known trace IDs and actionable/normal/uncertain values")
        seen.add(trace_id)
        sampled_positive += int(predictions[trace_id])
        sampled_negative += int(not predictions[trace_id])
        if label == "uncertain":
            uncertain += 1
            continue
        confusion[("t" if predictions[trace_id] == (label == "actionable") else "f") +
                  ("p" if predictions[trace_id] else "n")] += 1
    if not seen:
        raise ValueError("at least one reviewed label is required")
    tp, fp, tn, fn = (confusion[key] for key in ("tp", "fp", "tn", "fn"))
    population_positive = sum(predictions.values())
    population_negative = len(predictions) - population_positive
    # An unresolved label can be correlated with either outcome. Suppress
    # estimated rates instead of silently excluding it from a denominator.
    estimable = uncertain == 0 and sampled_positive > 0 and sampled_negative > 0
    weighted_fp = fp * population_positive / sampled_positive if estimable else 0
    weighted_tn = tn * population_negative / sampled_negative if estimable else 0
    weighted_tp = tp * population_positive / sampled_positive if estimable else 0
    weighted_fn = fn * population_negative / sampled_negative if estimable else 0
    return {
        "reviewed_turns": len(seen), "uncertain_turns": uncertain, "eligible_turns": len(predictions),
        "sampled_flagged": sampled_positive, "sampled_unflagged": sampled_negative, **confusion,
        "precision": tp / (tp + fp) if estimable and tp + fp else None,
        "recall": weighted_tp / (weighted_tp + weighted_fn) if estimable and weighted_tp + weighted_fn else None,
        "false_positive_rate": weighted_fp / (weighted_fp + weighted_tn) if estimable and weighted_fp + weighted_tn else None,
        "note": "Rates use inverse sampling weights for the two prediction strata. They describe actionable issue labels, not proven task failure; uncertainty or incomplete review suppresses rates.",
    }
