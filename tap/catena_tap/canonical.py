"""Catena Coding Agent Canonical Event Graph helpers."""

from __future__ import annotations

import json
from datetime import datetime, timezone
from typing import Any, Dict, Iterable, List

SCHEMA_VERSION = "catena.coding_agent.event_graph.v1"
CANONICAL_STATES = {"ok", "error", "retry", "aborted", "incomplete"}


def timestamp_to_unix_nano(value: Any, fallback: int = 1) -> str:
    if isinstance(value, datetime):
        parsed = value
    elif isinstance(value, str) and value:
        try:
            parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        except ValueError:
            return str(max(1, fallback))
    elif isinstance(value, (int, float)):
        # Claude timestamps are strings, but accepting epoch seconds makes the
        # contract helper useful to tests without weakening parser semantics.
        return str(max(1, int(value * 1_000_000_000)))
    else:
        return str(max(1, fallback))
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return str(max(1, int(parsed.timestamp() * 1_000_000_000)))


def ordered_unique(values: Iterable[Any]) -> List[str]:
    seen: set[str] = set()
    result: List[str] = []
    for value in values:
        if not isinstance(value, str) or not value or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def canonical_json(value: Dict[str, Any]) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
