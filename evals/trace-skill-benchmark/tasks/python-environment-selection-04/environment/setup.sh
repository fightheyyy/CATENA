#!/bin/bash
set -eu
cd /workspace/case
python3 -m venv apps/delta/.venv
apps/delta/.venv/bin/python -m pip install --no-index vendor/deltacalc-2.0.0-py3-none-any.whl

