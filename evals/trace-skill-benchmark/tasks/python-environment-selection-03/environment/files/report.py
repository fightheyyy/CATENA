import json,sys
from pathlib import Path
import couriercalc as lib
root=Path("/workspace/case")
items=json.loads((root/"data.json").read_text())
result={"total":lib.summarize(items),"version":lib.__version__}
(root/"result.json").write_text(json.dumps(result))
(root/"run-evidence.json").write_text(json.dumps({"version":lib.__version__,"interpreter":sys.executable}))
print(json.dumps(result))
