import pytest

from catena_engine.analysis import validate_report
from catena_engine.contracts import OptimizationError


def test_report_requires_real_references_and_separate_uncertainty():
    report = {"summary": "Observed a recoverable failure", "problems": [{"title": "Missing tool", "evidence": "Tool returned not found", "evidence_refs": ["owned-trace"], "hypothesis": "Tool unavailable", "uncertainty": "One trace only"}]}
    assert validate_report(report, ["owned-trace"]) == report
    report["problems"][0]["evidence_refs"] = ["invented-trace"]
    with pytest.raises(OptimizationError):
        validate_report(report, ["owned-trace"])
