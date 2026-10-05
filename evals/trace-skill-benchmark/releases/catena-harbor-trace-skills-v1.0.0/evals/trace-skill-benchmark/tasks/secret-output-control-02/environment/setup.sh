#!/bin/bash
set -eu
cd /workspace/case
chown root:root /workspace/case
git -C . init -q -b main
git -C . config user.name Fixture
git -C . config user.email fixture@example.invalid
git -C . add .
git -C . commit -qm initial

