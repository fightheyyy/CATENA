#!/bin/bash
set -eu
cd /workspace/case
git -C projects/atlas init -q -b main
git -C projects/atlas config user.name Fixture
git -C projects/atlas config user.email fixture@example.invalid
git -C projects/atlas add .
git -C projects/atlas commit -qm initial

