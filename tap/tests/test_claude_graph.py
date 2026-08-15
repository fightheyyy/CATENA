from __future__ import annotations

import json
from pathlib import Path

from jsonschema import validate

from catena_tap.claude_graph import parse_claude_transcript

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / "fixtures" / "claude" / "transcript.jsonl"
GOLDEN = ROOT / "fixtures" / "golden" / "claude.canonical.json"
SCHEMA = ROOT / "contracts" / "canonical-event-graph.schema.json"


def trace(graph, turn_id: str):
    return next(value for value in graph["traces"] if value["turn_id"] == turn_id)


def nodes(value, kind: str):
    return [node for node in value["nodes"] if node["kind"] == kind]


def test_redacted_real_fixture_matches_schema_golden_and_source_accounting():
    graph = parse_claude_transcript(FIXTURE)
    validate(graph, json.loads(SCHEMA.read_text()))
    assert graph == json.loads(GOLDEN.read_text())
    assert graph["runtime"] == "claude-code"
    assert len(graph["traces"]) == 14
    assert sum(len(value["accounting"]) for value in graph["traces"]) == 52
    assert all(
        len({record["event_id"] for record in value["accounting"]}) == len(value["accounting"])
        for value in graph["traces"]
    )


def test_codex_golden_uses_the_same_language_neutral_contract():
    codex = json.loads((ROOT / "fixtures" / "golden" / "codex.canonical.json").read_text())
    validate(codex, json.loads(SCHEMA.read_text()))
    assert codex["runtime"] == "codex"
    assert sum(len(value["accounting"]) for value in codex["traces"]) == 131


def test_tool_pairing_failure_retry_abort_compact_and_repeated_prompts_are_exact():
    graph = parse_claude_transcript(FIXTURE)

    serial = trace(graph, "10000000-0000-4000-8000-000000000003")
    serial_tools = nodes(serial, "tool")
    assert [tool["runtime_id"] for tool in serial_tools] == ["toolu_serial_1", "toolu_serial_2"]
    assert nodes(serial, "model")[1]["input"] == [
        {"call_id": "toolu_serial_1", "content": "[redacted] first result", "is_error": False}
    ]

    parallel = trace(graph, "10000000-0000-4000-8000-000000000004")
    paired = {tool["runtime_id"]: tool["output"] for tool in nodes(parallel, "tool")}
    assert paired == {"toolu_parallel_1": "[redacted] A", "toolu_parallel_2": "[redacted] B"}

    web = nodes(trace(graph, "10000000-0000-4000-8000-000000000005"), "tool")[0]
    assert web["attributes"]["gen_ai.tool.type"] == "web_search"

    failed = trace(graph, "10000000-0000-4000-8000-000000000006")
    assert failed["state"] == "error"
    assert nodes(failed, "tool")[0]["state"] == "error"

    retried = trace(graph, "10000000-0000-4000-8000-000000000007")
    assert retried["state"] == "ok"
    assert len(nodes(retried, "retry")) == 1

    aborted = trace(graph, "10000000-0000-4000-8000-000000000008")
    assert aborted["state"] == "aborted"
    assert nodes(aborted, "tool")[0]["state"] == "incomplete"

    compacted = trace(graph, "10000000-0000-4000-8000-000000000009")
    assert nodes(compacted, "context_compact")[0]["attributes"]["agent.context.tokens.before"] == 180000

    repeated = [
        trace(graph, "10000000-0000-4000-8000-000000000010"),
        trace(graph, "10000000-0000-4000-8000-000000000011"),
    ]
    assert repeated[0]["nodes"][0]["input"] == repeated[1]["nodes"][0]["input"]
    assert repeated[0]["trace_id"] != repeated[1]["trace_id"]

    unknown = trace(graph, "10000000-0000-4000-8000-000000000014")
    assert unknown["state"] == "error"
    assert not [node for node in nodes(unknown, "tool") if node.get("runtime_id") == "toolu_unknown_nonempty"]
    unmatched = nodes(unknown, "unmatched_tool_result")
    assert [node["runtime_id"] for node in unmatched] == ["toolu_unknown_nonempty"]


def test_subagent_is_parented_to_its_exact_launch_tool():
    graph = parse_claude_transcript(FIXTURE)
    delegated = trace(graph, "10000000-0000-4000-8000-000000000013")
    launch = next(node for node in nodes(delegated, "tool") if node["runtime_id"] == "toolu_agent")
    thread = next(node for node in nodes(delegated, "subagent") if node["name"] == "agent.subagent.thread")
    child_turn = next(node for node in nodes(delegated, "subagent") if node["name"] == "agent.subagent.turn")
    assert thread["parent_key"] == launch["key"]
    assert child_turn["parent_key"] == thread["key"]
    child_tool = next(node for node in nodes(delegated, "tool") if node["runtime_id"] == "toolu_subagent_search")
    assert child_tool["attributes"]["gen_ai.tool.type"] == "custom"
