import json,time
from pathlib import Path
root=Path(__file__).parent
with (root/"launches.log").open("a") as f: f.write("launch\n")
(root/"result.json").write_text(json.dumps({"state":"running"}))
print("progress: started",flush=True)
time.sleep(12)
(root/"result.json").write_text('{"state": "completed", "value": 42}')
print("completed",flush=True)
