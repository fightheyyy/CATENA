#!/bin/bash
set -euxo pipefail
cd /workspace/case
environments/current/bin/python report.py
