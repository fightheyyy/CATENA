#!/bin/bash
set -euxo pipefail
cd /workspace/case
.venv/bin/python report.py
