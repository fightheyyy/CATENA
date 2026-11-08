#!/usr/bin/env python3
"""Same-host archived Writer vs Collector ABBA study; no checkout reset."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
OLD_ADAPTER = '''package control
import("context";"testing")
func abOpen(ctx context.Context,dsn string)(*ClickHouseTraceStore,error){return OpenClickHouseTraceStore(ctx,dsn)}
func abHandler(t *testing.T,s Store,ch *ClickHouseTraceStore,c TraceIngestConfig)*ManagedHTTPHandler{t.Helper();h,e:=NewHTTPHandler(s,HTTPConfig{Auth:testAgentAuthConfig(),Traces:ch,Ingestion:c});if e!=nil{t.Fatal(e)};return h}
const abObserveSQL="SELECT trace_id, count() FROM catena_spans FINAL WHERE startsWith(owner_id, ?) GROUP BY trace_id"
const abIdentitySQL="SELECT owner_id, trace_id, groupArray(span_id) FROM catena_spans FINAL WHERE startsWith(owner_id, ?) GROUP BY owner_id, trace_id"
'''


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--old-ref', default='7e6c8b386f780fa2c03c4792f953b9c82704123d')
    p.add_argument('--seconds', type=int, default=60)
    p.add_argument('--output', default='artifacts/cloud-workspace/trace-ab')
    a = p.parse_args()
    # Preserve earlier evidence and avoid stale files when changing old refs.
    run_id = str(time.time_ns())
    out = (ROOT / a.output / run_id).resolve()
    out.mkdir(parents=True, exist_ok=False)
    env = os.environ.copy()
    if not env.get('BARENA_TEST_DATABASE_URL'):
        p.error('BARENA_TEST_DATABASE_URL must point to an isolated test database')
    env['GOMAXPROCS'] = '4'
    compose = ['docker', 'compose', '-p', 'catena-trace-study', '-f', str(ROOT/'deploy/catena-mvp1/compose.yml'),
               '-f', str(ROOT/'deploy/catena-mvp1/compose.telemetry-test.yml')]

    def cmd(argv, cwd=ROOT, data=None):
        r = subprocess.run(argv, cwd=cwd, env=env, input=data, text=True, stdout=subprocess.PIPE,
                           stderr=subprocess.STDOUT, timeout=240)
        with (out/'commands.log').open('a') as f:
            f.write(f"\n{time.time()} {' '.join(map(str,argv))}\n{r.stdout}")
        if r.returncode:
            raise RuntimeError(f'command failed: {argv}; see commands.log: {r.stdout[-1000:]}')
        return r.stdout

    commit = cmd(['git','rev-parse','--verify',a.old_ref+'^{commit}']).strip()
    archive = subprocess.check_output(['git','archive',commit,'backend'],cwd=ROOT)
    old = out/'old-source'
    old.mkdir(exist_ok=True)
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        tar.extractall(old, filter='data')
    shared = ROOT/'backend/internal/control/trace_ab_study_test.go'
    shutil.copyfile(shared, old/'backend/internal/control/trace_ab_study_test.go')
    (old/'backend/internal/control/trace_ab_adapter_test.go').write_text(OLD_ADAPTER)
    cmd(['go','test','-mod=readonly','-c','-o',str(out/'old.test'),'./internal/control'],old/'backend')
    cmd(['go','test','-mod=readonly','-c','-o',str(out/'new.test'),'./internal/control'],ROOT/'backend')
    manifest={'old_commit':commit,'old_writer_sha256':hashlib.sha256((old/'backend/internal/control/clickhouse_trace.go').read_bytes()).hexdigest(),'shared_harness_sha256':hashlib.sha256(shared.read_bytes()).hexdigest()}
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2))
    cmd(compose+['up','-d','clickhouse'])
    ch=cmd(compose+['ps','-q','clickhouse']).strip()

    def sql(query=None, db='default', data=None):
        argv=['docker','exec','-i',ch,'clickhouse-client','--password','catena','--database',db]
        return cmd(argv+(['--multiquery'] if data is not None else ['--query',query]),data=data)

    for _ in range(30):
        r=subprocess.run(['docker','exec',ch,'clickhouse-client','--password','catena','-q','SELECT 1'],capture_output=True)
        if r.returncode==0: break
        time.sleep(1)

    stop=threading.Event()
    def monitor():
        with (out/'resources.jsonl').open('a') as f:
            while not stop.is_set():
                row={'timestamp':time.time()}
                for name in ['cpu.stat','memory.current','memory.stat']:
                    row[name]=Path('/sys/fs/cgroup/'+name).read_text()
                f.write(json.dumps(row)+'\n');f.flush();stop.wait(.5)
    thread=threading.Thread(target=monitor);thread.start()
    config=out/'consumer.yaml';override=out/'override.json'

    def prepare(label,mode):
        db='catena_ab_'+run_id+'_'+label
        sql('CREATE DATABASE '+db)
        if mode=='collector':
            for f in sorted((ROOT/'deploy/catena-mvp1/telemetry/migrations').glob('*.sql')):
                sql(db=db,data=f.read_text())
            config.write_text((ROOT/'deploy/catena-mvp1/telemetry/consumer.yaml').read_text().replace('database: catena','database: '+db))
            config.chmod(0o644)
            override.write_text(json.dumps({'services':{'otel-consumer':{'volumes':[str(config)+':/etc/otelcol/config.yaml:ro']}}}))
            active=compose+['-f',str(override)]
            cmd(active+['up','-d','--scale','otel-gateway=1','--scale','otel-consumer=1','clickhouse','otel-gateway','otel-consumer'])
            # Mounted config contents may change without a Compose hash change.
            cmd(active+['restart','otel-consumer'])
            gateway=cmd(active+['ps','-q','otel-gateway']).strip()
            address='http://'+cmd(['docker','port',gateway,'4318/tcp']).strip()
        else:
            cmd(compose+['stop','otel-gateway','otel-consumer','kafka','kafka-2','kafka-3'])
            address=''
        return db,address

    def trial(label,mode,duration,fault=False):
        db,gateway=prepare(label,mode)
        child_env=env.copy();child_env.update(CATENA_TRACE_AB='1',CATENA_AB_DSN=f'http://default:catena@127.0.0.1:8123/{db}?dial_timeout=5s',CATENA_AB_MODE=mode,CATENA_AB_RUN=run_id+'-'+label,CATENA_AB_GATEWAY=gateway,CATENA_AB_SECONDS=str(duration),CATENA_AB_WARMUP='10',CATENA_AB_RESULT=str(out/(label+'.json')),CATENA_AB_MARKER=str(out/(label+'.started')))
        print(time.strftime('%H:%M:%S'),label,mode,flush=True)
        with (out/(label+'.log')).open('w') as log:
            process=subprocess.Popen([str(out/('old.test' if mode=='writer' else 'new.test')),'-test.v','-test.count=1','-test.timeout=5m','-test.run=^TestTraceWriterCollectorAB$'],cwd=(old/'backend' if mode=='writer' else ROOT/'backend'),env=child_env,stdout=log,stderr=subprocess.STDOUT)
            fault_data={}
            if fault:
                deadline=time.time()+120
                while not (out/(label+'.started')).exists():
                    if process.poll() is not None or time.time()>deadline:raise RuntimeError('warmup failed: '+label)
                    time.sleep(.1)
                started=json.loads((out/(label+'.started')).read_text())['started']
                time.sleep(max(0,started+5-time.time()))
                cmd(['docker','stop','--time','1',ch]);fault_data['down_at']=time.time()
                try:time.sleep(10)
                finally:
                    fault_data['restore_command_at']=time.time();cmd(['docker','start',ch])
                (out/(label+'-fault.json')).write_text(json.dumps(fault_data,indent=2))
            rc=process.wait(timeout=240)
        if rc:raise RuntimeError('trial failed: '+label+'; see '+str(out/(label+'.log')))
        if mode=='collector':
            broker=cmd(compose+['ps','-q','kafka']).strip()
            # Drain commits before switching the consumer database.
            for _ in range(15):
                text=cmd(['docker','exec',broker,'/opt/kafka/bin/kafka-consumer-groups.sh','--bootstrap-server','kafka:9092','--group','catena-clickhouse-v1','--describe'])
                rows=[s.split() for s in text.splitlines() if s.startswith('catena-clickhouse-v1')]
                if rows and all(v[5]=='0' for v in rows):break
                time.sleep(1)
            else:raise RuntimeError('Kafka backlog did not commit before database switch')
        print('passed',label,flush=True)

    try:
        for label,mode in [('a1','writer'),('b1','collector'),('b2','collector'),('a2','writer')]:
            trial(label,mode,a.seconds)
        trial('a_fault','writer',30,True)
        trial('b_fault','collector',30,True)
    finally:
        stop.set();thread.join()
        cmd(compose+['stop'])
    print('A/B complete; retained databases and evidence:',out,flush=True)


if __name__=='__main__':main()
