#!/bin/bash
set -eu
cd /workspace/case
python3 -m venv .venv
.venv/bin/python -m pip install --no-index vendor/atlascalc-2.0.0-py3-none-any.whl

