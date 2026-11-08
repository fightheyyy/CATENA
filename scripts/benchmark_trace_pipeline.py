#!/usr/bin/env python3
"""Opt-in local Trace data-plane acceptance. Uses only official containers.

Runs real Go ingress/auth, Collector, Kafka and ClickHouse. PostgreSQL auth is
enabled when BARENA_TEST_DATABASE_URL is supplied. No RSI/model calls.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import threading
import time

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project', default='catena-trace-study')
    parser.add_argument('--seconds', type=int, default=60)
    parser.add_argument('--output', default='artifacts/cloud-workspace/trace-pipeline')
    parser.add_argument('--resume-at-12-partitions', action='store_true', help='resume an existing isolated project after partition expansion; retain earlier logs')
    args = parser.parse_args()
    if not args.project.startswith('catena-trace-'):
        parser.error('use a dedicated catena-trace-* Compose project')
    if not os.environ.get('BARENA_TEST_DATABASE_URL'):
        parser.error('BARENA_TEST_DATABASE_URL is required for real PostgreSQL authentication')
    out = (ROOT / args.output).resolve()
    out.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env.update(CATENA_OTEL_LOAD='1', CATENA_OTEL_LOAD_SECONDS=str(args.seconds),
               CATENA_OTEL_LOAD_RPS='10', CATENA_OTEL_LOAD_BATCH='100', CATENA_OTEL_LOAD_TENANTS='20',
               CATENA_OTEL_LOAD_RESULTS=str(out / 'results.jsonl'),
               CATENA_OTEL_TEST_DSN=f"http://default:catena@127.0.0.1:{env.get('CATENA_CLICKHOUSE_PORT', '8123')}/catena?dial_timeout=5s")
    if env.get('CATENA_CLICKHOUSE_PASSWORD', 'catena') != 'catena':
        parser.error('this local benchmark requires its public fixture password catena; do not use production data')
    compose = ['docker', 'compose', '-p', args.project, '-f', str(ROOT / 'deploy/catena-mvp1/compose.yml'),
               '-f', str(ROOT / 'deploy/catena-mvp1/compose.telemetry-test.yml')]
    run_id = str(time.time_ns())
    stop = threading.Event()

    def command(cmd, *, check=True):
        p = subprocess.run(cmd, cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE,
                           stderr=subprocess.STDOUT, timeout=240)
        with (out / 'commands.log').open('a') as f:
            f.write(f"\n{time.time()} {' '.join(cmd)}\n{p.stdout}\n")
        if check and p.returncode:
            raise RuntimeError(f"command failed ({p.returncode}): {cmd}; see {out}/commands.log")
        return p.stdout

    def ids(service):
        return command(compose + ['ps', '-q', service]).split()

    def monitor():
        with (out / 'resources.jsonl').open('a') as f:
            while not stop.is_set():
                row = {'timestamp': time.time()}
                for name in ['cpu.stat', 'memory.current']:
                    row[name] = Path('/sys/fs/cgroup/' + name).read_text().strip()
                p = subprocess.run(['docker', 'stats', '--no-stream', '--format', '{{json .}}'],
                                   text=True, capture_output=True, timeout=20)
                row['docker'] = [json.loads(s) for s in p.stdout.splitlines() if s.startswith('{')]
                f.write(json.dumps(row) + '\n')
                f.flush()
                stop.wait(3)

    def refresh_gateways(gateways):
        addresses = []
        for cid in ids('otel-gateway'):
            addresses.append('http://' + command(['docker', 'port', cid, '4318/tcp']).strip())
        env['CATENA_OTEL_TEST_GATEWAYS'] = ','.join(addresses)
        assert len(addresses) == gateways

    def scale(gateways, consumers):
        command(compose + ['up', '-d', '--scale', f'otel-gateway={gateways}', '--scale',
                          f'otel-consumer={consumers}', 'clickhouse', 'otel-gateway', 'otel-consumer'])
        refresh_gateways(gateways)

    def load(label, seconds=None, mode='', rps=10, batch=100, tenants=20, retries=0):
        current = env.copy()
        current.update(CATENA_OTEL_LOAD_RUN=f'{run_id}-{label}', CATENA_OTEL_LOAD_MODE=mode)
        current.update(CATENA_OTEL_LOAD_RPS=str(rps), CATENA_OTEL_LOAD_BATCH=str(batch), CATENA_OTEL_LOAD_TENANTS=str(tenants))
        if retries:
            current['CATENA_OTEL_LOAD_RETRIES'] = str(retries)
        else:
            current.pop('CATENA_OTEL_LOAD_RETRIES', None)
        if seconds is not None:
            current['CATENA_OTEL_LOAD_SECONDS'] = str(seconds)
        print(f"{time.strftime('%H:%M:%S')} {label}: {mode or 'load+verify'}", flush=True)
        start = time.time()
        p = subprocess.run(['go', 'test', '-mod=readonly', '-count=1', '-v', '-timeout=3m',
                            '-run', '^TestOTelDataPlaneLoad$', './internal/control'],
                           cwd=ROOT / 'backend', env=current, text=True,
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=200)
        (out / f'{label}-{mode or "load"}.log').write_text(p.stdout)
        with (out / 'phases.jsonl').open('a') as f:
            f.write(json.dumps({'phase': label, 'mode': mode, 'started': start, 'finished': time.time(),
                                'exit_code': p.returncode}) + '\n')
        if p.returncode:
            raise RuntimeError(f"Go load failed: {label}; see {out}/{label}-{mode or 'load'}.log")

    broker = None

    def kafka(*arguments):
        return command(['docker', 'exec', broker, '/opt/kafka/bin/' + arguments[0], *arguments[1:]])

    def offsets(label):
        text = kafka('kafka-consumer-groups.sh', '--bootstrap-server', 'kafka:9092',
                     '--group', 'catena-clickhouse-v1', '--describe')
        (out / f'offsets-{label}.txt').write_text(text)
        result = {}
        for line in text.splitlines():
            v = line.split()
            if len(v) >= 6 and v[0] == 'catena-clickhouse-v1' and v[2].isdigit():
                result[int(v[2])] = {'committed': int(v[3]) if v[3].isdigit() else -1,
                                    'end': int(v[4]) if v[4].isdigit() else 0}
        if not result:
            raise RuntimeError('consumer offsets could not be read')
        return result

    thread = threading.Thread(target=monitor, daemon=True)
    thread.start()
    try:
        scale(2 if args.resume_at_12_partitions else 1, 2 if args.resume_at_12_partitions else 1)
        # Bind-mounted configuration changes do not restart Collectors on up.
        # Explicitly reload both roles before relying on new offset settings.
        command(compose + ['restart', 'otel-gateway', 'otel-consumer'])
        refresh_gateways(2 if args.resume_at_12_partitions else 1)
        broker = ids('kafka')[0]
        description = kafka('kafka-topics.sh', '--bootstrap-server', 'kafka:9092', '--describe',
                            '--topic', 'catena-traces-v1')
        count = 12 if args.resume_at_12_partitions else 6
        if f'PartitionCount: {count}' not in description and f'PartitionCount:{count}' not in description:
            raise RuntimeError(f'expected {count} partitions; Kafka partitions cannot shrink')
        if not args.resume_at_12_partitions:
            load('p6-g1-c1')
            scale(2, 1)
            load('p6-g2-c1')
            scale(2, 2)
            load('p6-g2-c2')
            kafka('kafka-topics.sh', '--bootstrap-server', 'kafka:9092', '--alter', '--topic',
                  'catena-traces-v1', '--partitions', '12')
        load('p12-g2-c2')
        load('tenant-cardinality', seconds=20, rps=50, batch=1, tenants=1000)

        # Wait until every previously stored record is committed, so unrelated
        # delayed auto-commits cannot masquerade as loss during the outage.
        for _ in range(20):
            before = offsets('before-clickhouse-outage')
            if all(v['committed'] == v['end'] for v in before.values()):
                break
            time.sleep(1)
        else:
            raise RuntimeError('baseline Kafka backlog did not drain')
        ch = ids('clickhouse')[0]
        command(['docker', 'stop', '--time', '5', ch])
        try:
            load('clickhouse-outage', seconds=10, mode='enqueue')
            during = offsets('clickhouse-down')
            if {k:v['committed'] for k,v in before.items()} != {k:v['committed'] for k,v in during.items()}:
                raise RuntimeError('consumer committed offsets while ClickHouse was down')
            if sum(v['end'] for v in during.values()) <= sum(v['end'] for v in before.values()):
                raise RuntimeError('outage did not enqueue durable Kafka records')
            consumers = ids('otel-consumer')
            command(['docker', 'kill', *consumers])
            command(['docker', 'start', *consumers])
            time.sleep(3)
            restarted = offsets('consumer-restarted-clickhouse-down')
            if {k:v['committed'] for k,v in before.items()} != {k:v['committed'] for k,v in restarted.items()}:
                raise RuntimeError('restarted consumer committed pending records before storage')
        finally:
            command(['docker', 'start', ch])
        recovery_started = time.time()
        for _ in range(30):
            p = subprocess.run(['docker', 'exec', ch, 'clickhouse-client', '--password', 'catena',
                                '--query', 'SELECT 1'], capture_output=True, timeout=5)
            if p.returncode == 0:
                break
            time.sleep(1)
        load('clickhouse-outage', seconds=10, mode='verify')
        recovery = {'clickhouse_recovery_wall_seconds': time.time()-recovery_started,
                    'pending_messages': sum(v['end']-v['committed'] for v in during.values()),
                    'commits_held_while_down': True, 'consumer_killed_while_pending': True}
        (out / 'reliability.json').write_text(json.dumps(recovery, indent=2))
        # A broker/controller is forcibly killed; RF=3, minISR=2 still permits
        # acknowledged writes. The original broker volume is retained.
        failed_broker = ids('kafka-3')[0]
        command(['docker', 'kill', failed_broker])
        try:
            load('broker-unavailable', seconds=20, retries=8)
        finally:
            command(['docker', 'start', failed_broker])
        offsets('final')
        (out / 'reliability.json').write_text(json.dumps(recovery, indent=2))
        print('All data-plane studies passed; evidence:', out, flush=True)
    finally:
        stop.set()
        thread.join(timeout=25)
        command(compose + ['logs', '--no-color', '--tail', '200', 'otel-consumer', 'otel-gateway'], check=False)


if __name__ == '__main__':
    main()
