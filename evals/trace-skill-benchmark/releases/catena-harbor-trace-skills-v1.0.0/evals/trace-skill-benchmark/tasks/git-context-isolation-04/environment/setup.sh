#!/bin/bash
set -eu
cd /workspace/case
git -C project init -q -b main
git -C project config user.name Fixture
git -C project config user.email fixture@example.invalid
git -C project add .
git -C project commit -qm initial
git -C project rev-parse HEAD > /opt/fixture-git-head

