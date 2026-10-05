#!/bin/bash
set -euxo pipefail
cd /workspace/case
nohup python3 server.py > server.log 2>&1 &
python3 -c 'import urllib.request,time,pathlib;time.sleep(13);pathlib.Path("result.json").write_bytes(urllib.request.urlopen("http://127.0.0.1:8571/health").read())'
