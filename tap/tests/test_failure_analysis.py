from __future__ import annotations

import json
from pathlib import Path

import pytest

from catena_tap.failure_analysis import scan_graph, score_labels, summarize


def graph_with_tools(tools):
    return {
        "schema_version": "catena.coding_agent.event_graph.v1",
        "runtime": "codex", "session_id": "session-1",
        "traces": [{"trace_id": "trace-1", "turn_id": "turn-1", "state": "ok", "nodes": tools}],
    }


def tool(key, name, state, content, time):
    return {"key": key, "kind": "tool", "state": state, "input": content,
            "end_time_unix_nano": str(time), "attributes": {"gen_ai.tool.name": name}}


def test_unrecovered_error_is_a_hypothesis_without_raw_contents():
    turns = scan_graph(graph_with_tools([tool("secret-call", "shell", "error", "sk-secret", 1)]))
    report = summarize(turns)
    assert report["eligible_turns"] == 1
    assert report["turns_with_signals"] == 1
    assert report["groups"][0]["status"] == "hypothesis"
    assert "sk-secret" not in json.dumps(report)


def test_observed_recovery_and_identical_polling_are_distinct():
    turns = scan_graph(graph_with_tools([
        tool("a", "shell", "error", "run", 1),
        tool("b", "shell", "ok", "run", 2),
        tool("c", "read", "ok", "poll", 3),
        tool("d", "read", "ok", "poll", 4),
        tool("e", "read", "ok", "poll", 5),
    ]))
    signals = [item["signal"] for item in turns[0]["signals"]]
    assert signals == ["repeated_identical_call"]


def test_codex_nonzero_exit_in_result_envelope_is_tool_evidence():
    failed = tool("a", "shell", "ok", "run", 1)
    failed["output"] = "Wall time: 1.2 seconds\nExit code: 124\nOutput:\nretry"
    report = summarize(scan_graph(graph_with_tools([failed])))
    assert report["groups"][0]["signal"] == "unrecovered_tool_error"
    assert "retry" not in json.dumps(report)


def test_repeated_errors_and_deduped_source_counts():
    graph = graph_with_tools([tool("a", "shell", "error", "x", 1), tool("b", "shell", "error", "x", 2)])
    turns = scan_graph(graph)
    report = summarize(turns + turns)
    assert report["eligible_turns"] == 1
    assert {item["signal"] for item in report["groups"]} == {"unrecovered_tool_error", "repeated_failed_call"}


def test_different_failed_inputs_and_passive_polls_do_not_form_repeat_signal():
    report = summarize(scan_graph(graph_with_tools([
        tool("a", "shell", "error", "command-a", 1),
        tool("b", "shell", "error", "command-b", 2),
        tool("c", "wait_agent", "ok", "same", 3),
        tool("d", "wait_agent", "ok", "same", 4),
        tool("e", "wait_agent", "ok", "same", 5),
    ])))
    assert [item["signal"] for item in report["groups"]] == ["unrecovered_tool_error"]


def test_exec_wrapper_unwraps_passive_write_stdin_polls():
    poll = 'const r = await tools.write_stdin({session_id: 7, chars: "", yield_time_ms: 30000}); text(r.output);'
    report = summarize(scan_graph(graph_with_tools([
        tool("a", "exec", "ok", poll, 1),
        tool("b", "exec", "ok", poll, 2),
        tool("c", "exec", "ok", poll, 3),
    ])))
    assert report["groups"] == []


def test_edit_between_failed_validation_runs_breaks_the_retry_streak():
    report = summarize(scan_graph(graph_with_tools([
        tool("a", "shell", "error", "validate", 1),
        tool("edit", "apply_patch", "ok", "fix", 2),
        tool("b", "shell", "error", "validate", 3),
        tool("edit-2", "apply_patch", "ok", "fix again", 4),
        tool("c", "shell", "ok", "validate", 5),
    ])))
    assert report["groups"] == []


def test_score_requires_actual_labels_and_uses_negative_denominator():
    flagged = scan_graph(graph_with_tools([tool("a", "shell", "error", "x", 1)]))[0]
    quiet = {**flagged, "trace_id": "trace-2", "signals": []}
    report = summarize([flagged, quiet])
    result = score_labels(report, [{"trace_id": "trace-1", "label": "normal"},
                                   {"trace_id": "trace-2", "label": "normal"}])
    assert result["fp"] == 1 and result["tn"] == 1
    assert result["false_positive_rate"] == 0.5
    with pytest.raises(ValueError):
        score_labels(report, [{"trace_id": "trace-1", "label": ""}])


def test_balanced_sample_uses_population_weights_and_abstains_on_uncertain():
    flagged = scan_graph(graph_with_tools([tool("a", "shell", "error", "x", 1)]))[0]
    turns = [{**flagged, "trace_id": f"flag-{index}"} for index in range(2)]
    turns += [{**flagged, "trace_id": f"quiet-{index}", "signals": []} for index in range(10)]
    report = summarize(turns)
    labels = [{"trace_id": "flag-0", "label": "normal"}, {"trace_id": "quiet-0", "label": "normal"}]
    assert score_labels(report, labels)["false_positive_rate"] == pytest.approx(2 / 12)
    labels[0]["label"] = "uncertain"
    assert score_labels(report, labels)["false_positive_rate"] is None


def test_real_golden_graphs_are_accepted_without_declaring_task_failure():
    golden = Path(__file__).resolve().parents[1] / "fixtures" / "golden"
    turns = []
    for runtime in ("codex", "claude"):
        turns.extend(scan_graph(json.loads((golden / f"{runtime}.canonical.json").read_text(encoding="utf-8"))))
    report = summarize(turns)
    assert report["eligible_turns"] > 0
    assert all(group["status"] == "hypothesis" for group in report["groups"])
