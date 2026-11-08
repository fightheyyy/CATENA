#!/usr/bin/env python3
"""Pinned-topology 60s Collector capacity staircase, with explicit rejection accounting."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import threading
import time

ROOT=Path(__file__).resolve().parents[1]


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--targets',default='3000,5000,10000,20000')
    args=parser.parse_args()
    targets=[int(x) for x in args.targets.split(',')]
    if any(x<100 or x%100 for x in targets):parser.error('targets must be positive multiples of 100 Span/s')
    env=os.environ.copy()
    if not env.get('BARENA_TEST_DATABASE_URL'):parser.error('an isolated BARENA_TEST_DATABASE_URL is required')
    env['GOMAXPROCS']='4'
    run=str(time.time_ns());out=ROOT/'artifacts/cloud-workspace/trace-capacity'/run;out.mkdir(parents=True)
    compose=['docker','compose','-p','catena-trace-study','-f',str(ROOT/'deploy/catena-mvp1/compose.yml'),'-f',str(ROOT/'deploy/catena-mvp1/compose.telemetry-test.yml')]
    def cmd(argv,data=None,cwd=ROOT):
        p=subprocess.run(argv,cwd=cwd,env=env,input=data,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=240)
        with (out/'commands.log').open('a') as f:f.write(f"\n{time.time()} {' '.join(argv)}\n{p.stdout}")
        if p.returncode:raise RuntimeError(str(argv)+': '+p.stdout[-1200:])
        return p.stdout.strip()
    cmd(['go','test','-mod=readonly','-c','-o',str(out/'capacity.test'),'./internal/control'],cwd=ROOT/'backend')
    cmd(compose+['up','-d','clickhouse']);ch=cmd(compose+['ps','-q','clickhouse'])
    for _ in range(30):
        p=subprocess.run(['docker','exec',ch,'clickhouse-client','--password','catena','-q','SELECT 1'],capture_output=True)
        if not p.returncode:break
        time.sleep(1)
    def sql(db,text):return cmd(['docker','exec','-i',ch,'clickhouse-client','--password','catena','--database',db,'--multiquery'],data=text)
    stop=threading.Event()
    def monitor():
        with (out/'resources.jsonl').open('w') as f:
            while not stop.is_set():
                r={'timestamp':time.time()}
                for name in ['cpu.stat','memory.current','memory.stat']:r[name]=Path('/sys/fs/cgroup/'+name).read_text()
                f.write(json.dumps(r)+'\n');f.flush();stop.wait(.5)
    thread=threading.Thread(target=monitor);thread.start()
    try:
        for target in targets:
            db=f'catena_capacity_{run}_{target}';sql('default','CREATE DATABASE '+db)
            for f in sorted((ROOT/'deploy/catena-mvp1/telemetry/migrations').glob('*.sql')):sql(db,f.read_text())
            config=out/'consumer.yaml';config.write_text((ROOT/'deploy/catena-mvp1/telemetry/consumer.yaml').read_text().replace('database: catena','database: '+db));config.chmod(0o644)
            override=out/'override.json';override.write_text(json.dumps({'services':{'otel-consumer':{'volumes':[str(config)+':/etc/otelcol/config.yaml:ro']}}}))
            active=compose+['-f',str(override)]
            cmd(active+['up','-d','--scale','otel-gateway=2','--scale','otel-consumer=2','clickhouse','otel-gateway','otel-consumer'])
            cmd(active+['restart','otel-consumer'])
            gateways=['http://'+cmd(['docker','port',cid,'4318/tcp']) for cid in cmd(active+['ps','-q','otel-gateway']).split()]
            child=env.copy();child.update(CATENA_TRACE_CAPACITY='1',CATENA_CAPACITY_RPS=str(target//100),CATENA_CAPACITY_RUN=run+'-'+str(target),CATENA_CAPACITY_DSN=f'http://default:catena@127.0.0.1:8123/{db}?dial_timeout=5s',CATENA_CAPACITY_GATEWAYS=','.join(gateways),CATENA_CAPACITY_RESULT=str(out/(str(target)+'.json')))
            print(time.strftime('%H:%M:%S'),'target',target,flush=True)
            with (out/(str(target)+'.log')).open('w') as log:
                p=subprocess.run([str(out/'capacity.test'),'-test.v','-test.count=1','-test.timeout=5m','-test.run=^TestTraceCollectorCapacity$'],cwd=ROOT/'backend',env=child,stdout=log,stderr=subprocess.STDOUT,timeout=330)
            if p.returncode:raise RuntimeError('capacity trial failed: '+str(target)+'; see '+str(out/(str(target)+'.log')))
            r=json.loads((out/(str(target)+'.json')).read_text());print('finished',target,'ACK spans',r['acknowledged_spans'],'window stored',r['sampled_window_stored_spans_per_second'],'P95 ms',r['e2e_p95_ms'],flush=True)
            # Independent sink-window audit: active visibility polling can
            # time out under pressure. QueryFinish records actual INSERT
            # completion, rather than assuming a Kafka ACK is already stored.
            sql('default','SYSTEM FLUSH LOGS')
            text=sql('default',"SELECT toUnixTimestamp64Micro(event_time_microseconds)/1000000.0 AS at, written_rows, query_duration_ms FROM system.query_log WHERE type='QueryFinish' AND query_kind='Insert' AND has(databases, '"+db+"') ORDER BY event_time_microseconds FORMAT JSONEachRow")
            (out/(str(target)+'-insert-completions.jsonl')).write_text(text+'\n')
            inserts=[json.loads(x) for x in text.splitlines() if x.strip()]
            total=sum(int(x['written_rows']) for x in inserts)
            audit={'window_insert_completed_spans':sum(int(x['written_rows']) for x in inserts if r['started']<=float(x['at'])<=r['started']+60),
                   'total_insert_rows':total,'warm_insert_rows':sum(int(x['written_rows']) for x in inserts if float(x['at'])<r['started']),
                   'expected_unique_total':10000+r['verified_acknowledged_spans']+r['unacknowledged_but_stored_spans']}
            audit['physical_equals_unique']=total==audit['expected_unique_total']
            (out/(str(target)+'-backend.json')).write_text(json.dumps(audit,indent=2))
            broker=cmd(compose+['ps','-q','kafka'])
            for _ in range(30):
                text=cmd(['docker','exec',broker,'/opt/kafka/bin/kafka-consumer-groups.sh','--bootstrap-server','kafka:9092','--group','catena-clickhouse-v1','--describe'])
                rows=[x.split() for x in text.splitlines() if x.startswith('catena-clickhouse-v1')]
                if rows and all(x[5]=='0' for x in rows):break
                time.sleep(1)
            else:raise RuntimeError('backlog did not commit; do not change Consumer database')
    finally:
        stop.set();thread.join();cmd(compose+['stop'])
    print('All capacity trials completed:',out,flush=True)


if __name__=='__main__':main()
