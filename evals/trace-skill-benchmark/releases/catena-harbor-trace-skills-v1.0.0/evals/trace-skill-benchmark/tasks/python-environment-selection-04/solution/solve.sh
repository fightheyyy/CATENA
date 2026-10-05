#!/bin/bash
set -euxo pipefail
cd /workspace/case
apps/delta/.venv/bin/python report.py
