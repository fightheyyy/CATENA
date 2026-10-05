#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -m venv .venv
.venv/bin/python -m pip install --no-index vendor/couriercalc-2.0.0-py3-none-any.whl
.venv/bin/python report.py
