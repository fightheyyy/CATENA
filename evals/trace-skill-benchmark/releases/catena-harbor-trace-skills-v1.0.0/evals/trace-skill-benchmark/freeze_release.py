"""Freeze Harbor tasks, Skills, results, and hashes after complete acceptance."""
import argparse
import csv
import hashlib
import json
import shutil
import subprocess
import zipfile
import tarfile
import importlib.metadata
import sys
import os
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

from run_study import ROOT, REPO, collect, frozen
import harbor


def sha256(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def main(run, version):
    result_path = ROOT / 'results' / f'{run}.json'
    result = json.loads(result_path.read_text(encoding='utf-8'))
    manifest = json.loads((ROOT / 'manifest.json').read_text(encoding='utf-8'))
    expected = {(task['id'], arm, repeat) for task in manifest['tasks']
                for arm in ('without', 'with') for repeat in (1, 2, 3)}
    actual = {(row['task'], row['arm'], row['repeat']) for row in result['trials']}
    if (result['status'] != 'completed' or actual != expected or len(result['trials']) != len(expected)
            or not all(row['valid'] for row in result['trials'])):
        raise RuntimeError('Complete, unique, valid 168-rollout result required')
    if frozen() != result['frozen_sha256']:
        raise RuntimeError('Frozen dataset changed')
    private = REPO / '.local' / 'trace-skill-benchmark' / run
    receipt = json.loads((private / 'study.json').read_text(encoding='utf-8'))
    tasks = {task['id']: task for task in manifest['tasks']}
    artifact_index={}
    for path in (private/'jobs').glob('*/*/result.json'):
        artifact_index.setdefault(path.parent.name,[]).append(path)
    # Re-read every selected native artifact instead of trusting the aggregate.
    evidence = []
    for row in receipt['trials']:
        candidates = artifact_index.get(row['trial_name'],[])
        if len(candidates) != 1:
            raise RuntimeError('Missing or ambiguous selected Harbor artifact')
        checked = collect(candidates[0].parent.parent, tasks[row['task']], row['arm'], row['repeat'], 'ab')
        for key in ('valid', 'passed', 'functional_passed', 'safety_violation', 'checks',
                    'input_tokens', 'output_tokens', 'cached_input_tokens', 'duration_seconds',
                    'native_session_verified', 'usage_verified', 'skill_context_verified',
                    'tool_calls', 'failed_tool_calls', 'recurrence_errors'):
            if checked.get(key) != row.get(key):
                raise RuntimeError(f'Artifact/receipt mismatch: {key}')
        selected = candidates[0].parent
        safe_hashes={}
        session_index=0
        for path in sorted(selected.rglob('*')):
            if not path.is_file(): continue
            relative=str(path.relative_to(selected)).replace('\\', '/')
            if relative.startswith('agent/sessions/'):
                session_index+=1
                relative=f'agent/native-session-{session_index:02d}.jsonl'
            safe_hashes[relative]=sha256(path)
        evidence.append({
            'task': row['task'], 'arm': row['arm'], 'repeat': row['repeat'],
            'execution_phase': row.get('execution_phase', 'initial_batch'),
            'artifact_hashes': safe_hashes,
        })

    release = ROOT / 'releases' / version
    if release.exists():
        raise RuntimeError('Release already exists; do not overwrite frozen evidence')
    dataset = release / 'evals' / 'trace-skill-benchmark'
    dataset.mkdir(parents=True)
    hashes = json.loads((ROOT / 'frozen-sha256.json').read_text(encoding='utf-8'))
    sources = [ROOT / relative for relative in hashes]
    sources += [ROOT / name for name in (
        'manifest.json', 'frozen-sha256.json', 'Dockerfile.base', 'README.md',
        'build_suite.py', 'verify.py', 'run_study.py', 'repair_infrastructure.py',
        'publish_report.py', 'validate_verifiers.py', 'freeze_release.py')]
    sources += [ROOT / 'results' / f'{run}{suffix}' for suffix in ('.json', '.md')]
    for source in sources:
        target = dataset / source.relative_to(ROOT)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
    for row in receipt['trials']:
        selected=artifact_index[row['trial_name']][0].parent
        target=release/'verifier-evidence'/row['task']/row['arm']/f'repeat-{row["repeat"]}.json'
        target.parent.mkdir(parents=True,exist_ok=True)
        shutil.copyfile(selected/'verifier'/'assessment.json',target)
    (release / 'docs').mkdir()
    shutil.copyfile(REPO / 'docs' / 'TRACE_SKILL_AUDIT_20261004.md',
                    release / 'docs' / 'TRACE_SKILL_AUDIT_20261004.md')
    # Capture the actual installed Harbor source, including any local Windows
    # compatibility fixes; a version label alone does not freeze those bytes.
    harbor_root=Path(harbor.__file__).parent
    for source in harbor_root.rglob('*'):
        if not source.is_file() or '__pycache__' in source.parts or source.suffix=='.pyc': continue
        target=release/'vendor'/'harbor'/source.relative_to(harbor_root)
        target.parent.mkdir(parents=True,exist_ok=True)
        shutil.copyfile(source,target)
    distribution=importlib.metadata.distribution('harbor')
    for source in distribution.files or []:
        if 'license' not in str(source).lower(): continue
        original=Path(distribution.locate_file(source))
        if original.is_file():
            target=release/'vendor'/'licenses'/original.name
            target.parent.mkdir(parents=True,exist_ok=True)
            shutil.copyfile(original,target)
    runtime={'python':sys.version.split()[0],
             'packages':{d.metadata['Name']:d.version for d in importlib.metadata.distributions() if d.metadata.get('Name')}}
    (release/'dependency-versions.json').write_text(json.dumps(runtime,indent=2),encoding='utf-8')
    (release / 'selected-evidence-sha256.json').write_text(
        json.dumps(evidence, ensure_ascii=False, indent=2), encoding='utf-8')
    fields = ('task', 'skill', 'arm', 'repeat', 'valid', 'passed', 'functional_passed',
              'safety_violation', 'tool_calls', 'failed_tool_calls', 'recurrence_errors',
              'blind_retries', 'input_tokens', 'cached_input_tokens', 'output_tokens',
              'duration_seconds', 'execution_phase')
    with (release / 'rollouts.csv').open('w', encoding='utf-8-sig', newline='') as stream:
        writer = csv.DictWriter(stream, fieldnames=fields)
        writer.writeheader()
        for row in result['trials']:
            writer.writerow({key: row.get(key, 'initial_batch' if key == 'execution_phase' else '') for key in fields})
    a,b=result['summaries']['without'],result['summaries']['with']
    ci=result['pass_rate_difference_task_cluster_bootstrap_95ci_pp']
    lines=['# Catena Harbor 实验冻结结果','',
           '7 项手工编写的 Skill × 28 个任务 × with/without 两组 × 每组每题 3 次，共 168 个有效 rollout。',
           '执行目标：真实 Codex CLI 0.160.0；模型：GPT-5.5；执行引擎：Harbor 0.23.0。','',
           '| 指标 | without | with |','|---|---:|---:|',
           f'| 验证通过 | {a["passed"]}/84（{a["passed"]/84:.1%}） | {b["passed"]}/84（{b["passed"]/84:.1%}） |',
           f'| 同类错误匹配次数 | {a["recurrence_errors"]} | {b["recurrence_errors"]} |',
           f'| 工具调用次数 | {a["tool_calls"]} | {b["tool_calls"]} |',
           f'| 合成秘密输出违规次数 | {a["safety_violations"]} | {b["safety_violations"]} |',
           f'| 输入 token（含缓存） | {a["input_tokens"]:,} | {b["input_tokens"]:,} |',
           f'| 输出 token | {a["output_tokens"]:,} | {b["output_tokens"]:,} |','',
           f'观察到通过率差异 {(b["passed"]-a["passed"])/84*100:+.2f} 个百分点。按任务聚类的 bootstrap 95% 区间为 [{ci[0]:.2f}, {ci[1]:.2f}] 个百分点。区间含零时不能声称提升有统计显著性。','',
           '| Skill | without 通过 /12 | with 通过 /12 |','|---|---:|---:|']
    for skill,totals in result['by_skill'].items():
        lines.append(f'| {skill} | {totals["without"]["passed"]} | {totals["with"]["passed"]} |')
    lines+=['','实验任务为匿名重建、变体和迁移用例，全部历史 Trace 曾参与设计，不属于严格历史留出集。执行环境为 Linux Docker 与 PowerShell 7，未还原原始 Windows 环境。',
            '原批次中 136 次有效，32 次在代理额度恢复后补跑；原批次使用并发执行，补跑使用单并发。模型标签和配置一致，但供应商后台版本未单独验证，耗时对比受执行时间、缓存和并发影响。',
            '失败试次按冻结 verifier 判定保留。基础设施异常与供应商限流记录单列，不进入通过率分母。工具错误匹配可能包含预期的探测失败，不能直接当作故障根因。',
            '', 'release-manifest.json 为冻结清单，rollouts.csv 为逐次结果，verifier-evidence/ 为 168 次验证明细。完整原始会话保存在本机私有实验目录，selected-evidence-sha256.json 保存其校验值。']
    (release/'RESULTS.zh-CN.md').write_text('\n'.join(lines)+'\n',encoding='utf-8')
    image = json.loads(subprocess.check_output(
        ['docker', 'image', 'inspect', 'catena/trace-skill-eval:1'], text=True))[0]
    snapshot = REPO / '.local' / 'trace-skill-benchmark' / 'images' / (image['Id'].split(':')[1] + '.tar')
    if not snapshot.exists():
        snapshot.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(['docker', 'image', 'save', '-o', str(snapshot),
                        'catena/trace-skill-eval:1'], check=True)
    with tarfile.open(snapshot,'r') as saved:
        image_manifest=json.load(saved.extractfile('manifest.json'))
        config=saved.extractfile(image_manifest[0]['Config']).read()
        config_digest=hashlib.sha256(config).hexdigest()
        image_digest=image['Id'].split(':')[1]
        # Docker's containerd image store reports an OCI index ID, whereas
        # legacy Docker reports the config ID. Validate either representation.
        matched=config_digest==image_digest
        pending=[] if matched else [image_digest]
        while pending:
            digest=pending.pop()
            blob=saved.extractfile('blobs/sha256/'+digest).read()
            if hashlib.sha256(blob).hexdigest()!=digest:
                raise RuntimeError('Saved OCI blob hash mismatch')
            descriptor=json.loads(blob)
            matched=matched or descriptor.get('config',{}).get('digest')=='sha256:'+config_digest
            pending.extend(m['digest'].split(':')[1] for m in descriptor.get('manifests',[]))
        if not matched:
            raise RuntimeError('Saved image config differs from the evaluated image')
    freeze = {
        'schema': 'catena.harbor_frozen_release.v1', 'version': version,
        'frozen_at_utc': datetime.now(timezone.utc).isoformat(),
        'source_run': run, 'task_count': 28, 'skill_count': 7, 'valid_rollouts': 168,
        'arms': {'without': 84, 'with': 84}, 'frozen_dataset_sha256': result['frozen_sha256'],
        'image_id': image['Id'], 'image_snapshot_sha256': sha256(snapshot),
        'image_snapshot_bytes': snapshot.stat().st_size,
        'target': {'harbor': result['harbor_version'], 'codex': result['codex_version'],
                   'model': result['model'], 'reasoning_effort': 'low', 'web_search': 'disabled'},
        'execution_phases': result['execution_phases'],
        'total_completed_harbor_attempts': result['total_harbor_attempts'],
        'excluded_attempts_by_type': dict(Counter(row['exception_type'] for row in result['excluded_infrastructure_attempts'])),
        'summaries': result['summaries'],
        'limitations': ['constructed tasks, not strict held-out data',
                        'Linux Docker / PowerShell adaptation, not original Windows replay',
                        '32 quota-resumed trials executed later and serially',
                        'GPT-5.5 label fixed; provider backend weights not independently verified'],
        'files': {str(path.relative_to(release)).replace('\\', '/'): sha256(path)
                  for path in release.rglob('*') if path.is_file()},
    }
    (release / 'release-manifest.json').write_text(
        json.dumps(freeze, ensure_ascii=False, indent=2), encoding='utf-8')
    guide = f'''# Catena Harbor dataset and result freeze: {version}

This snapshot contains 28 Harbor tasks, 7 corresponding Skills, and all 168 valid
Codex with/without results (84 per arm, 3 repeats per task). Original failed
provider/setup attempts are retained locally and excluded from effect metrics.

## Verify the snapshot

The release-manifest.json records SHA-256 for each dataset and result file.
selected-evidence-sha256.json records the original selected Harbor artifact hashes.
Native sessions and connection configurations remain private; the ZIP contains
only anonymous fixtures, Skill text, aggregate/individual metrics, and hashes.

## Reproduce

1. Use a Windows/PowerShell host with Docker Linux containers, Harbor
   {result['harbor_version']}, and Codex CLI {result['codex_version']}.
   dependency-versions.json lists the original Python environment. vendor/harbor
   contains the exact installed Harbor source; set PYTHONPATH to the vendor
   directory to reuse it with its matching installed dependencies.
2. Restore the saved image with docker image load, then confirm its image ID is
   {image['Id']}. The image tar is stored locally under
   .local/trace-skill-benchmark/images/{snapshot.name}, with the hash in the manifest.
3. Configure your own proxy at port 8317 and set CATENA_EVAL_KEY_FILE to your key file.
4. From the release root, run:
   python evals/trace-skill-benchmark/run_study.py --mode oracle --run oracle-new --repeats 1 --concurrency 1
   python evals/trace-skill-benchmark/run_study.py --mode ab --run ab-new --repeats 3 --concurrency 1
5. Keep this frozen release unchanged. New observations and task revisions need a
   new version. New stochastic model runs can produce different results.

All tasks and Skills retain the original pre-experiment hashes. The resumed cohort
is explicitly marked; timing across concurrent and serial cohorts is descriptive.
See evals/trace-skill-benchmark/results/{run}.md for outcomes and negative results.
'''
    (release / 'REPRODUCE.md').write_text(guide, encoding='utf-8')
    freeze['files']['REPRODUCE.md'] = sha256(release / 'REPRODUCE.md')
    (release / 'release-manifest.json').write_text(
        json.dumps(freeze, ensure_ascii=False, indent=2), encoding='utf-8')
    keyfile=Path(os.environ.get('CATENA_EVAL_KEY_FILE',str(Path(os.environ['LOCALAPPDATA'])/'DuMemEval/cliproxyapi-7.3.2/client-key.txt')))
    if keyfile.exists():
        private_key=keyfile.read_text().strip().encode()
        if private_key and any(private_key in path.read_bytes() for path in release.rglob('*') if path.is_file()):
            raise RuntimeError('Private proxy credential found in release; refusing to archive')
    archive = ROOT / 'releases' / f'{version}.zip'
    with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED) as zipped:
        for path in sorted(release.rglob('*')):
            if path.is_file():
                zipped.write(path, str(Path(version) / path.relative_to(release)))
    checksum = archive.with_suffix('.zip.sha256')
    checksum.write_text(f'{sha256(archive)}  {archive.name}\n', encoding='utf-8')
    with zipfile.ZipFile(archive) as zipped:
        if zipped.testzip() is not None:
            raise RuntimeError('Archive CRC verification failed')
        for relative, digest in freeze['files'].items():
            content = zipped.read(version + '/' + relative)
            if hashlib.sha256(content).hexdigest() != digest:
                raise RuntimeError('Archive content hash mismatch')
    print(json.dumps({'version': version, 'tasks': 28, 'skills': 7, 'valid_rollouts': 168,
                      'archive': str(archive), 'archive_bytes': archive.stat().st_size,
                      'archive_sha256': sha256(archive), 'files_verified': len(freeze['files'])}))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('run')
    parser.add_argument('--version', default='catena-harbor-trace-skills-v1.0.0')
    args = parser.parse_args()
    main(args.run, args.version)
