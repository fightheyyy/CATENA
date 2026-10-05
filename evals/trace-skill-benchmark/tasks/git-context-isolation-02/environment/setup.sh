#!/bin/bash
set -eu
cd /workspace/case
git -C apps/beacon/vendor/helper init -q -b main
git -C apps/beacon/vendor/helper config user.name Fixture
git -C apps/beacon/vendor/helper config user.email fixture@example.invalid
git -C apps/beacon/vendor/helper add .
git -C apps/beacon/vendor/helper commit -qm initial
git -C apps/beacon init -q -b main
git -C apps/beacon config user.name Fixture
git -C apps/beacon config user.email fixture@example.invalid
git -C apps/beacon add .
git -C apps/beacon commit -qm initial

