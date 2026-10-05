#!/bin/bash
set -eu
cd /workspace/case
python3 -m venv environments/current
environments/current/bin/python -m pip install --no-index vendor/beaconcalc-2.0.0-py3-none-any.whl
python3 -m venv environments/old

