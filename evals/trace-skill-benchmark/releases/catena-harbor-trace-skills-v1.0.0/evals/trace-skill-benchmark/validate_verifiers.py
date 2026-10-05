"""Reject wrong/extra-key answers, altered protected content, and output canaries."""
import hashlib
import json
import tempfile
from pathlib import Path
from verify import evaluate

ROOT=Path(__file__).resolve().parent
checks=0
with tempfile.TemporaryDirectory(prefix='catena-verifier-') as tmp:
    root=Path(tmp);logs=root/'logs';logs.mkdir()
    for task in (ROOT/'tasks').iterdir():
        spec=json.loads((task/'tests/check.json').read_text(encoding='utf-8'))
        answer=spec['answer'];minimal={'answer':answer}
        (root/'result.json').write_text(json.dumps(answer))
        assert evaluate(minimal,root,logs)['passed'];checks+=1
        for bad in [{}, {**answer,'unexpected_field':True}, None]:
            (root/'result.json').write_text(json.dumps(bad))
            assert not evaluate(minimal,root,logs)['passed'];checks+=1
    (root/'result.json').write_text('{"ok":true}')
    (root/'protected.txt').write_text('keep')
    spec={'answer':{'ok':True},'protected':{'protected.txt':hashlib.sha256(b'keep').hexdigest()}}
    assert evaluate(spec,root,logs)['passed'];checks+=1
    (root/'protected.txt').write_text('changed')
    assert not evaluate(spec,root,logs)['passed'];checks+=1
    spec={'answer':{'ok':True},'secret_canaries':['catena-synthetic-test-value']}
    for text,passed in [('key present, value hidden',True),('catena-synthetic-test-value',False)]:
        event={'type':'item.completed','item':{'type':'command_execution','aggregated_output':text}}
        (logs/'codex.txt').write_text(json.dumps(event)+'\n')
        result=evaluate(spec,root,logs)
        assert result['passed']==passed and result['functional_passed'];checks+=1
print(json.dumps({'tasks':28,'verifier_checks':checks,'status':'passed'}))
