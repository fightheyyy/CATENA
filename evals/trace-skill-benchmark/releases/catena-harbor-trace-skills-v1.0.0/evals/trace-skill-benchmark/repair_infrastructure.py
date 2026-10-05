"""Resume missing trials after setup failures or explicit provider rate limits."""
import argparse
import asyncio
import json
import os
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from harbor.job import Job
from harbor.models.job.config import JobConfig
from harbor.models.trial.config import AgentConfig, EnvironmentConfig, TaskConfig
from run_study import ROOT, REPO, collect, dump, frozen, summaries

async def main(run):
    out=REPO/'.local/trace-skill-benchmark'/run;receipt=out/'study.json'
    state=json.loads(receipt.read_text(encoding='utf-8'))
    if state['status']=='running': raise RuntimeError('Wait until the main schedule finishes')
    if frozen()!=state['frozen_sha256']: raise RuntimeError('Suite changed')
    image=subprocess.check_output(['docker','image','inspect','catena/trace-skill-eval:1','--format','{{.Id}}'],text=True).strip()
    if image!=state['image_digest']: raise RuntimeError('Container image changed')
    tasks={t['id']:t for t in json.loads((ROOT/'manifest.json').read_text(encoding='utf-8'))['tasks']}
    keyfile=Path(os.environ.get('CATENA_EVAL_KEY_FILE',str(Path(os.environ['LOCALAPPDATA'])/'DuMemEval/cliproxyapi-7.3.2/client-key.txt')))
    credentials={'OPENAI_API_KEY':keyfile.read_text().strip(),'OPENAI_BASE_URL':'http://host.docker.internal:8317/v1'}
    excluded=state.setdefault('excluded_infrastructure_attempts',[])
    for i,entry in enumerate(list(state['trials'])):
        if entry['valid']: continue
        base=f'{entry["task"]}-{entry["arm"]}-{entry["repeat"]}'
        candidates=sorted((out/'jobs').glob(base+'*/**/result.json'))
        candidates=[p for p in candidates if p.parent.name.startswith(entry['task']+'__')]
        original=next((p for p in candidates if p.parent.name==entry['trial_name']),None)
        if original is None: raise RuntimeError('Missing current attempt evidence')
        raw=json.loads(original.read_text(encoding='utf-8'))
        exception_type=(raw.get('exception_info') or {}).get('exception_type') or entry.get('exception_type')
        # A Harbor trial can enter the agent phase and still fail before it
        # receives a usable rollout when the upstream proxy returns 429. This
        # is a transient provider failure, not an agent outcome. Retry it one
        # at a time and retain the original attempt in the exclusion record.
        provider_transient=exception_type == 'ApiRateLimitError'
        setup_failure=raw.get('agent_execution') is None and bool(raw.get('exception_info'))
        if not (setup_failure or provider_transient):
            raise RuntimeError('Refusing to retry an attempt that produced model evidence')
        artifact=str(original.relative_to(out))
        if not any(e.get('original_artifact')==artifact for e in excluded):
            reason='provider rate limit before a usable rollout' if provider_transient else 'environment/setup failure before agent execution'
            excluded.append({**entry,'reason':reason,'original_artifact':artifact})
        t=tasks[entry['task']];arm=entry['arm'];rep=entry['repeat']
        # Reuse a completed valid retry if a previous repair was interrupted
        # before writing the receipt. Failed retries keep their artifacts;
        # a later invocation gets a fresh job name rather than reusing 429.
        recovered=None
        for candidate in candidates:
            if candidate==original: continue
            found=collect(candidate.parent.parent,t,arm,rep,'ab')
            if found['valid']:
                recovered=found;break
        if recovered is not None:
            recovered['infrastructure_retry']=True
            recovered['execution_phase']='quota_resume'
            state['trials'][i]=recovered;dump(receipt,state)
            continue
        retry_kind='provider' if provider_transient else 'infra'
        retry_index=1
        while (out/'jobs'/(base+f'-{retry_kind}-retry-{retry_index}')).exists():
            retry_index+=1
        name=base+f'-{retry_kind}-retry-{retry_index}'
        cfg=JobConfig(job_name=name,jobs_dir=out/'jobs',n_concurrent_trials=1,n_attempts=1,quiet=True,
          environment=EnvironmentConfig(type='docker',delete=True),
          agents=[AgentConfig(name='codex',model_name='gpt-5.5',skills=[ROOT/'skills'/t['skill']] if arm=='with' else [],kwargs={'version':'0.160.0','reasoning_effort':'low','web_search':'disabled'},env=credentials)],
          tasks=[TaskConfig(path=ROOT/'tasks'/t['id'])],extra_instructions=[f'Use the installed ${t["skill"]} Skill for this task.'] if arm=='with' else [])
        jobdir=out/'jobs'/name
        if not list(jobdir.glob('*/result.json')):
            job=await Job.create(cfg);await job.run()
        repaired=collect(jobdir,t,arm,rep,'ab');repaired['infrastructure_retry']=True
        repaired['execution_phase']='quota_resume'
        state['trials'][i]=repaired
        dump(receipt,state)
        print(json.dumps({'task':t['id'],'arm':arm,'repeat':rep,'valid':repaired['valid'],'passed':repaired['passed']}),flush=True)
        if not repaired['valid']:
            # Avoid spending the remaining schedule on a provider that is
            # cooling down, and never retry a genuine verifier failure.
            raise RuntimeError('Repair produced invalid evidence; retained attempt and stopped for inspection')
    state['summaries']=summaries(state['trials']);state['tasks_unchanged']=frozen()==state['frozen_sha256']
    state['status']='completed' if len(state['trials'])==168 and all(t['valid'] for t in state['trials']) and state['tasks_unchanged'] else 'invalid'
    state['finished_at']=datetime.now(timezone.utc).isoformat()
    artifacts=[p for p in (out/'jobs').glob('**/result.json') if p.parent.name.startswith(tuple(t['id']+'__' for t in tasks.values()))]
    selected={t['trial_name'] for t in state['trials']}
    excluded=[]
    for artifact in artifacts:
        raw=json.loads(artifact.read_text(encoding='utf-8'))
        if raw.get('trial_name') in selected: continue
        exception=(raw.get('exception_info') or {}).get('exception_type')
        excluded.append({'exception_type':exception,'reason':'provider rate limit' if exception=='ApiRateLimitError' else 'environment/setup failure','original_artifact':str(artifact.relative_to(out))})
    state['excluded_infrastructure_attempts']=excluded
    state['total_harbor_attempts']=len(artifacts)
    dump(receipt,state)
    public={**state,'trials':[{k:v for k,v in t.items() if k!='trial_name'} for t in state['trials']],
            'excluded_infrastructure_attempts':[{k:v for k,v in t.items() if k!='original_artifact'} for t in excluded]}
    dump(ROOT/'results'/f'{run}.json',public)
    print(json.dumps({'status':state['status'],'excluded_infrastructure_attempts':len(excluded),'valid':sum(t['valid'] for t in state['trials'])}))
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('run');asyncio.run(main(p.parse_args().run))
