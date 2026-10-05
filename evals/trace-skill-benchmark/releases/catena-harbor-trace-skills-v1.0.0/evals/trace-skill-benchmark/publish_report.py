"""Publish only verified complete A/B results and evidence-bounded resume wording."""
import argparse
import hashlib
import json
import random
import subprocess
from pathlib import Path
from run_study import ROOT, REPO, summaries

def main(run):
    path=ROOT/'results'/f'{run}.json'
    report=json.loads(path.read_text(encoding='utf-8'))
    expected_hashes=json.loads((ROOT/'frozen-sha256.json').read_text(encoding='utf-8'))
    actual_hashes={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for area in ['tasks','skills'] for p in (ROOT/area).rglob('*') if p.is_file()}
    if actual_hashes!=expected_hashes: raise ValueError('Added, deleted or changed frozen task/Skill files')
    image=subprocess.check_output(['docker','image','inspect','catena/trace-skill-eval:1','--format','{{.Id}}'],text=True).strip()
    if image!=report.get('image_digest'): raise ValueError('Container image changed')
    trials=report['trials'];expected=168
    identities={(t['task'],t['arm'],t['repeat']) for t in trials}
    if report['status']!='completed' or len(trials)!=expected or len(identities)!=expected or not all(t['valid'] for t in trials) or not report['tasks_unchanged']:
        raise ValueError('Cannot publish an incomplete or invalid acceptance study')
    oracle=json.loads((REPO/'.local/trace-skill-benchmark/oracle-v4/study.json').read_text(encoding='utf-8'))
    if not oracle.get('oracle_all_passed') or len(oracle['trials'])!=28 or oracle['frozen_sha256']!=report['frozen_sha256']:
        raise ValueError('Reference-solution gate does not match the frozen suite')
    report['oracle_validation']={'valid':28,'passed':28,'same_frozen_suite':True}
    report['execution_phases']={phase:sum(t.get('execution_phase','initial_batch')==phase for t in trials) for phase in ['initial_batch','quota_resume']}
    totals=summaries(trials)
    report['summaries']=totals
    skills=sorted({t['skill'] for t in trials})
    report['by_skill']={s:summaries([t for t in trials if t['skill']==s]) for s in skills}
    tasks=sorted({t['task'] for t in trials})
    paired=[]
    for task in tasks:
        a=[t['passed'] for t in trials if t['task']==task and t['arm']=='without']
        b=[t['passed'] for t in trials if t['task']==task and t['arm']=='with']
        if len(a)!=3 or len(b)!=3: raise ValueError('Unbalanced task repeats')
        paired.append(sum(b)/3-sum(a)/3)
    rng=random.Random(431558)
    bootstrap=sorted(sum(rng.choices(paired,k=len(paired)))/len(paired)*100 for _ in range(5000))
    report['pass_rate_difference_task_cluster_bootstrap_95ci_pp']=[bootstrap[125],bootstrap[4874]]
    path.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
    lines=['# Trace-derived Skill A/B acceptance results','',
      'Measured target: Harbor 0.23.0 native Codex CLI 0.160.0, GPT-5.5, low reasoning effort. Linux Docker / PowerShell 7 adaptation.','',
      'Seven manually authored Skills derived from audited traces; 28 anonymous reconstruction/mutation/transfer tasks. Three repeats per task per arm, 168 valid rollouts. These are not strict historical held-out tasks.','',
      f'Infrastructure attempts: {report.get("total_harbor_attempts",168)} total completed Harbor attempts, including {len(report.get("excluded_infrastructure_attempts",[]))} retained exclusions. Only the 168 valid logical trials enter the effect metrics. {report["execution_phases"]["quota_resume"]} trials were resumed after the proxy quota recovered; this later cohort is recorded and can affect timing and model-provider behavior.','',
      '| Metric | Without | With |','|---|---:|---:|']
    for label,key in [('Verifier passes','passed'),('Functional passes','functional_passed'),('Recurrence errors','recurrence_errors'),('Trials with recurrence','trials_with_recurrence'),('Failed tool calls (raw)','failed_tool_calls'),('Native tool calls','tool_calls'),('Exact blind retries','blind_retries'),('Synthetic secret output violations','safety_violations'),('Input tokens, including cached','input_tokens'),('Cached input tokens (subset)','cached_input_tokens'),('Output tokens','output_tokens'),('Cumulative agent seconds','duration_seconds'),('Median agent seconds','median_duration_seconds')]:
        vals=[totals[a][key] for a in ['without','with']]
        lines.append(f'| {label} | {vals[0]:,.3f} | {vals[1]:,.3f} |' if isinstance(vals[0],float) else f'| {label} | {vals[0]:,} | {vals[1]:,} |')
    lines+=['','| Skill | Without passes / 12 | With passes / 12 | Without recurrence | With recurrence | Without secret violations | With secret violations |','|---|---:|---:|---:|---:|---:|---:|']
    for s in skills:
        a=report['by_skill'][s]['without'];b=report['by_skill'][s]['with']
        lines.append(f'| {s} | {a["passed"]} | {b["passed"]} | {a["recurrence_errors"]} | {b["recurrence_errors"]} | {a["safety_violations"]} | {b["safety_violations"]} |')
    lines+=['','## Failed verifier checks','', '| Task | Arm | Repeat | Failed checks |','|---|---|---:|---|']
    for t in trials:
        if not t['passed']:
            failed=', '.join(k for k,v in t.get('checks',{}).items() if not v)
            lines.append(f'| {t["task"]} | {t["arm"]} | {t["repeat"]} | {failed} |')
    a,b=totals['without'],totals['with'];diff=(b['passed']-a['passed'])/84*100
    ci=report['pass_rate_difference_task_cluster_bootstrap_95ci_pp']
    rate_metrics={}
    for key in ['recurrence_errors','failed_tool_calls','tool_calls','input_tokens','output_tokens','duration_seconds']:
        rate_metrics[key]={'without_per_trial':a[key]/84,'with_per_trial':b[key]/84,'change_percent':(b[key]/a[key]-1)*100 if a[key] else None}
    report['per_trial_comparisons']=rate_metrics
    path.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
    lines+=['',f'Observed pass rate: {a["passed"]}/84 ({a["passed"]/84:.1%}) → {b["passed"]}/84 ({b["passed"]/84:.1%}), difference {diff:+.2f} percentage points. Task-cluster bootstrap interval: [{ci[0]:.2f}, {ci[1]:.2f}] percentage points. This is descriptive uncertainty on the constructed task set, not population coverage or proof of significance.','',
      '## Resume wording supported by this run','',
      f'从 3,021 条真实 Agent Trace 中核对重复工具问题，构建 7 项操作 Skill 与 28 个匿名重建任务，集成 Harbor 驱动真实 Codex 完成 168 次 with/without 对照；独立 verifier 成功率为 {a["passed"]/84:.1%} → {b["passed"]/84:.1%}，同类错误调用 {a["recurrence_errors"]} → {b["recurrence_errors"]}，并回收逐次执行轨迹、token 与耗时。','',
      'Do not describe this as automatic Skill generation, original Windows replay, production-wide improvement, or evaluation of Claude Code. All seven interventions are reported, including neutral or negative results. Tokens are usage rather than billing cost. Timing excludes setup and is affected by concurrency and cache. The shared image lacks rg. Finite lifecycle tasks simulate local delayed jobs; server_alive checks availability after Codex exits, so that task tests runtime lifecycle as well as instructions.','',
      f'Machine-readable report: `{path.name}`. Freeze digest: `{report["frozen_sha256"]}`. Container image: `{report.get("image_digest")}`.']
    (ROOT/'results'/f'{run}.md').write_text('\n'.join(lines)+'\n',encoding='utf-8')
    print(json.dumps({'valid_rollouts':168,'without_passed':a['passed'],'with_passed':b['passed'],'without_recurrence':a['recurrence_errors'],'with_recurrence':b['recurrence_errors'],'pass_difference_pp':diff,'ci_pp':ci}))
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('run');main(p.parse_args().run)
