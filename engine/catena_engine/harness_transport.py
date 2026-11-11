"""Bounded, non-redirecting private capability transport."""

import json
import urllib.error
import urllib.request

from .contracts import OptimizationError


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def private_post(url, token, value, timeout, limit):
    request = urllib.request.Request(
        url,
        data=json.dumps(value, ensure_ascii=False).encode(),
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.build_opener(NoRedirect).open(request, timeout=timeout) as response:
            raw = response.read(limit + 1)
    except (OSError, ValueError):
        raise OptimizationError("private Harness request failed") from None
    if len(raw) > limit:
        raise OptimizationError("private Harness reply exceeded its size bound")
    try:
        return json.loads(raw)
    except ValueError:
        raise OptimizationError("private Harness reply was invalid") from None
