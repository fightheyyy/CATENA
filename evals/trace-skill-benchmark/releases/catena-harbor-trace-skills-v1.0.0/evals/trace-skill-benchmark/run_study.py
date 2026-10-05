"""Harbor native Codex A/B runner with frozen tasks, paired scheduling and resume."""
import argparse
import asyncio
import hashlib
import json
import os
import random
import re
import statistics
import subprocess
from datetime import datetime, timezone
from pathlib import Path

from harbor.job import Job
from harbor.models.job.config import JobConfig
from harbor.models.trial.config import AgentConfig, EnvironmentConfig, TaskConfig

ROOT=Path(__file__).resolve().parent
REPO=ROOT.parents[1]
ERRORS={
 'powershell-execution':r'ParserError|Missing file specification after redirection|Unexpected token',
 'path-discovery':r'Cannot find path|FileNotFoundError|No such file or directory',
 'reliable-file-editing':r'Failed to delete file|Failed to find expected|Permission denied|patch.*failed',
 'python-environment-selection':r'ModuleNotFoundError|No module named',
 'git-context-isolation':r'not a git repository',
 'long-command-lifecycle':r'command timed out|Process timed out|timeout exceeded',
 'secret-output-control':r'(?!)',
}

def frozen():
    expected=json.loads((ROOT/'frozen-sha256.json').read_text())
    current={k:hashlib.sha256((ROOT/k).read_bytes()).hexdigest() for k in expected}
    if current!=expected: raise RuntimeError('Frozen task or Skill hash mismatch')
    return hashlib.sha256(json.dumps(expected,sort_keys=True).encode()).hexdigest()
def dump(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    temp=path.with_suffix('.tmp');temp.write_text(json.dumps(value,ensure_ascii=False,indent=2),encoding='utf-8');temp.replace(path)

def native_metrics(agent,skill):
    files=list((agent/'sessions').rglob('*.jsonl'))
    if len(files)!=1: raise ValueError('Missing or ambiguous native Codex session')
    calls=[];pending={};answer='';input_messages=[]
    for line in files[0].read_text(encoding='utf-8').splitlines():
        e=json.loads(line);p=e.get('payload',{})
        if e.get('type')!='response_item': continue
        typ=p.get('type')
        if typ=='message' and p.get('role') in ['user','developer','system']:
            input_messages.append(''.join(x.get('text','') for x in p.get('content',[]) if isinstance(x,dict)))
        if typ in ['function_call','custom_tool_call']:
            raw=p.get('arguments',p.get('input',''))
            try: args=json.loads(raw)
            except (ValueError,TypeError): args={}
            pending[p['call_id']]={'tool':p.get('name',''),'input':raw,'command':args.get('cmd',args.get('command',raw)) if isinstance(args,dict) else raw}
        elif typ in ['function_call_output','custom_tool_call_output'] and p.get('call_id') in pending:
            c=pending.pop(p['call_id']);out=p.get('output','');out=out if isinstance(out,str) else json.dumps(out)
            m=re.search(r'(?:Process exited with code|Exit code:)\s*(-?\d+)',out)
            c.update(output=out,exit_code=int(m[1]) if m else None);calls.append(c)
        elif typ=='message' and p.get('role')=='assistant' and p.get('phase')!='commentary':
            answer=''.join(x.get('text','') for x in p.get('content',[]) if x.get('type')=='output_text') or answer
    errors=[]
    for i,c in enumerate(calls):
        if re.search(ERRORS[skill],c['output'],re.I):
            if skill=='python-environment-selection' and c['exit_code']==0: continue
            errors.append(i)
    retries=sum(calls[i]['tool']==calls[i+1]['tool'] and calls[i]['input']==calls[i+1]['input'] for i in errors if i+1<len(calls))
    failures=sum(c['exit_code'] not in [None,0] or bool(re.search(r'Failed to delete file|Failed to find expected',c['output'],re.I)) for c in calls)
    loaded=any('SKILL.md' in str(c['command']) and skill in str(c['command']) for c in calls)
    guidance=(ROOT/'skills'/skill/'SKILL.md').read_text(encoding='utf-8').strip().splitlines()[-1]
    context_verified=any(guidance in message for message in input_messages)
    policy=any('operation not permitted by policy' in c['output'].lower() or 'approval required' in c['output'].lower() for c in calls)
    return {'native_session_verified':True,'final_answer_present':bool(answer),'tool_calls':len(calls),
            'failed_tool_calls':failures,'recurrence_errors':len(errors),'blind_retries':retries,
            'skill_read_observed':loaded,'skill_context_verified':context_verified,'policy_rejection':policy}

def collect(jobdir,task,arm,repeat,mode):
    paths=list(jobdir.glob('*/result.json'))
    if len(paths)!=1: raise ValueError('Expected exactly one Harbor trial')
    p=paths[0];r=json.loads(p.read_text());verifier=p.parent/'verifier'
    assessment=json.loads((verifier/'assessment.json').read_text()) if (verifier/'assessment.json').exists() else {}
    rewards=(r.get('verifier_result') or {}).get('rewards') or {}
    agent=r.get('agent_result') or {};execution=r.get('agent_execution') or {}
    seconds=None
    if execution.get('started_at') and execution.get('finished_at'):
        seconds=(datetime.fromisoformat(execution['finished_at'])-datetime.fromisoformat(execution['started_at'])).total_seconds()
    entry={'task':task['id'],'skill':task['skill'],'arm':arm,'repeat':repeat,'trial_name':r.get('trial_name'),
           'valid':not r.get('exception_info') and bool(rewards) and bool(assessment),
           'passed':rewards.get('reward')==1,'functional_passed':assessment.get('functional_passed',False),
           'safety_violation':assessment.get('safety_violation',False),'checks':assessment.get('checks',{}),
           'input_tokens':agent.get('n_input_tokens'),'output_tokens':agent.get('n_output_tokens'),
           'cached_input_tokens':agent.get('n_cache_tokens'),'duration_seconds':seconds,
           'exception_type':(r.get('exception_info') or {}).get('exception_type')}
    if mode=='ab' and entry['valid']:
        try:
            entry.update(native_metrics(p.parent/'agent',task['skill']))
            entry['valid']=entry['final_answer_present'] and not entry['policy_rejection']
            entry['valid']=entry['valid'] and entry['skill_context_verified']==(arm=='with')
            usage=[]
            for line in (p.parent/'agent/codex.txt').read_text(errors='replace').splitlines():
                try:
                    e=json.loads(line)
                    if e.get('type')=='turn.completed': usage.append(e.get('usage',{}))
                except ValueError: pass
            entry['usage_verified']=bool(usage) and sum(u.get('input_tokens',0) for u in usage)==entry['input_tokens'] and sum(u.get('output_tokens',0) for u in usage)==entry['output_tokens']
            entry['valid']=entry['valid'] and entry['usage_verified']
            entry['cached_input_tokens']=sum(u.get('cached_input_tokens',0) for u in usage)
        except Exception as error:
            entry['valid']=False;entry['evidence_error']=type(error).__name__
    return entry

def summaries(entries):
    result={}
    for arm in ['without','with']:
        rows=[r for r in entries if r['arm']==arm and r['valid']]
        result[arm]={'valid':len(rows),'passed':sum(r['passed'] for r in rows),'functional_passed':sum(r['functional_passed'] for r in rows),
          'recurrence_errors':sum(r.get('recurrence_errors',0) for r in rows),
          'trials_with_recurrence':sum(r.get('recurrence_errors',0)>0 for r in rows),
          'failed_tool_calls':sum(r.get('failed_tool_calls',0) for r in rows),
          'tool_calls':sum(r.get('tool_calls',0) for r in rows),'blind_retries':sum(r.get('blind_retries',0) for r in rows),
          'safety_violations':sum(r['safety_violation'] for r in rows),
          'input_tokens':sum(r.get('input_tokens') or 0 for r in rows),'output_tokens':sum(r.get('output_tokens') or 0 for r in rows),
          'cached_input_tokens':sum(r.get('cached_input_tokens') or 0 for r in rows),
          'duration_seconds':sum(r.get('duration_seconds') or 0 for r in rows),
          'median_duration_seconds':statistics.median([r['duration_seconds'] for r in rows if r.get('duration_seconds') is not None]) if rows else None,
          'skill_reads_observed':sum(r.get('skill_read_observed',False) for r in rows),
          'skill_contexts_verified':sum(r.get('skill_context_verified',False) for r in rows)}
    return result

async def main(args):
    digest=frozen();manifest=json.loads((ROOT/'manifest.json').read_text())
    tasks=manifest['tasks']
    if args.task: tasks=[t for t in tasks if t['id']==args.task]
    if args.skill: tasks=[t for t in tasks if t['skill']==args.skill]
    if not tasks: raise ValueError('No tasks selected')
    out=REPO/'.local/trace-skill-benchmark'/args.run
    out.mkdir(parents=True,exist_ok=True)
    receipt=out/'study.json'
    previous=json.loads(receipt.read_text()) if receipt.exists() else None
    if previous and previous['frozen_sha256']!=digest: raise RuntimeError('Cannot resume changed suite')
    state=previous or {'schema':'catena.trace_skill_study.v1','run':args.run,'mode':args.mode,'started_at':datetime.now(timezone.utc).isoformat(),'frozen_sha256':digest,'model':'gpt-5.5','codex_version':'0.160.0','harbor_version':'0.23.0','environment':manifest['environment'],'repeats':args.repeats,'status':'running','trials':[]}
    state['status']='running';state['repeats']=args.repeats
    image=subprocess.check_output(['docker','image','inspect','catena/trace-skill-eval:1','--format','{{.Id}}'],text=True).strip()
    if state.get('image_digest') and state['image_digest']!=image: raise RuntimeError('Container image changed on resume')
    state['image_digest']=image
    state['expected_rollouts']=len(tasks)*args.repeats*(2 if args.mode=='ab' else 1)
    dump(receipt,state)
    credentials={}
    if args.mode=='ab':
        key=Path(os.environ.get('CATENA_EVAL_KEY_FILE',str(Path(os.environ['LOCALAPPDATA'])/'DuMemEval/cliproxyapi-7.3.2/client-key.txt'))).read_text().strip()
        credentials={'OPENAI_API_KEY':key,'OPENAI_BASE_URL':'http://host.docker.internal:8317/v1'}
    semaphore=asyncio.Semaphore(args.concurrency)
    lock=asyncio.Lock()
    identities={(r['task'],r['arm'],r['repeat']) for r in state['trials']}
    async def run_one(t,arm,rep):
        ident=(t['id'],arm,rep)
        if ident in identities: return
        name=f'{t["id"]}-{arm}-{rep}'
        jobdir=out/'jobs'/name
        try:
            if not list(jobdir.glob('*/result.json')):
                agent=AgentConfig(name='oracle') if args.mode=='oracle' else AgentConfig(name='codex',model_name='gpt-5.5',skills=[ROOT/'skills'/t['skill']] if arm=='with' else [],kwargs={'version':'0.160.0','reasoning_effort':'low','web_search':'disabled'},env=credentials)
                cfg=JobConfig(job_name=name,jobs_dir=out/'jobs',n_concurrent_trials=1,n_attempts=1,quiet=True,
                  environment=EnvironmentConfig(type='docker',delete=True),agents=[agent],tasks=[TaskConfig(path=ROOT/'tasks'/t['id'])],
                  extra_instructions=[f'Use the installed ${t["skill"]} Skill for this task.'] if arm=='with' else [])
                job=await Job.create(cfg);await job.run()
            entry=collect(jobdir,t,arm,rep,args.mode)
        except Exception as e:
            entry={'task':t['id'],'skill':t['skill'],'arm':arm,'repeat':rep,'valid':False,'passed':False,'exception_type':type(e).__name__}
        async with lock:
            state['trials'].append(entry);identities.add(ident)
            state['summaries']=summaries(state['trials']);dump(receipt,state)
            print(json.dumps({'finished':len(state['trials']),'expected':state['expected_rollouts'],'task':t['id'],'arm':arm,'repeat':rep,'valid':entry['valid'],'passed':entry['passed'],'error':entry.get('exception_type')}),flush=True)
    pairs=[(t,r) for r in range(1,args.repeats+1) for t in tasks]
    random.Random(431558).shuffle(pairs)
    async def pair(t,rep):
        async with semaphore:
            arms=['oracle'] if args.mode=='oracle' else (['without','with'] if (tasks.index(t)+rep)%2 else ['with','without'])
            for arm in arms: await run_one(t,arm,rep)
    await asyncio.gather(*(pair(t,r) for t,r in pairs))
    state['tasks_unchanged']=frozen()==digest
    state['status']='completed' if len(state['trials'])==state['expected_rollouts'] and all(r['valid'] for r in state['trials']) and state['tasks_unchanged'] else 'invalid'
    if args.mode=='oracle': state['oracle_all_passed']=all(r['passed'] for r in state['trials'])
    state['finished_at']=datetime.now(timezone.utc).isoformat();state['summaries']=summaries(state['trials']);dump(receipt,state)
    if args.mode=='ab':
        public={**state,'trials':[{k:v for k,v in r.items() if k!='trial_name'} for r in state['trials']]}
        dump(ROOT/'results'/f'{args.run}.json',public)
    print(json.dumps({'status':state['status'],'finished':len(state['trials']),'oracle_all_passed':state.get('oracle_all_passed')}),flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--mode',choices=['oracle','ab'],required=True);p.add_argument('--run',required=True)
    p.add_argument('--repeats',type=int,default=3);p.add_argument('--concurrency',type=int,default=3);p.add_argument('--task');p.add_argument('--skill')
    asyncio.run(main(p.parse_args()))
