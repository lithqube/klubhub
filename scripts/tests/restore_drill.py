#!/usr/bin/env python3
"""Real PostgreSQL + Garage destructive-loss drill in a new UUID project.

Requires Docker Compose and AWS CLI. No stub/fallback or retained live stack.
Only newly created fixture resources are destroyed. Evidence is retained under
Hermes managed scratch; credential-bearing original output is private (0600).
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import uuid

REPO = Path(__file__).resolve().parents[2]
SCRATCH = Path.home() / '.hermes/cache/scratch/finance-hardening-evidence/restore'
FLAGS = ('equality_db', 'equality_keys', 'equality_bodies')


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def require_equal(summary):
    if not all(summary.get(key) is True for key in FLAGS):
        raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check-equality', type=Path,
                        help='Pure exit-status probe using the live equality gate')
    args = parser.parse_args()
    if args.check_equality:
        require_equal(json.loads(args.check_equality.read_text()))
        return
    os.umask(0o077)
    identity = uuid.uuid4().hex
    project = 'restore-drill-' + identity
    work = SCRATCH / ('work-' + identity)
    work.mkdir(parents=True, exist_ok=False)
    compose = work / 'compose.yml'
    compose.write_text(f'''name: {project}
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: klubhub
      POSTGRES_DB: klubhub
      POSTGRES_HOST_AUTH_METHOD: trust
    healthcheck:
      test: [CMD-SHELL, "pg_isready -U klubhub -d klubhub"]
      interval: 1s
      timeout: 3s
      retries: 30
    ports: ["127.0.0.1::5432"]
    volumes: ["db-data:/var/lib/postgresql/data"]
  storage:
    image: dxflrs/garage:v2.2.0
    environment:
      GARAGE_CONFIG_FILE: /etc/garage.toml
    volumes:
      - ./garage.toml:/etc/garage.toml:ro
      - garage-meta:/data/meta
      - garage-data:/data/storage
    healthcheck:
      test: [CMD, /garage, status]
      interval: 1s
      timeout: 5s
      retries: 30
    ports: ["127.0.0.1::3900"]
  app:
    image: postgres:16-alpine
    command: [sleep, infinity]
volumes:
  db-data:
  garage-meta:
  garage-data:
''')
    (work / 'garage.toml').write_text(f'''metadata_dir = "/data/meta"
data_dir = "/data/storage"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "{os.urandom(32).hex()}"
[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
''')
    sources = [REPO / 'scripts' / name for name in
               ('backup.sh', 'restore.sh', 'restore_support.py', 'RESTORE.md')]
    sources += sorted((REPO / 'scripts/tests').glob('*.py'))
    manifest = {str(p.relative_to(REPO)): digest(p) for p in sources}
    for source in sources:
        target = work / 'source' / source.relative_to(REPO)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
    (work / 'source-hashes.json').write_text(json.dumps(manifest, indent=2) + '\n')
    summary: dict = dict(uuid=identity, project=project, compose_file=str(compose),
                   source_manifest=str(work / 'source-hashes.json'), storage_kind='garage',
                   documentation='https://garagehq.deuxfleurs.fr/documentation/quick-start/',
                   commands=[], equal=False, cleanup_verified=False)
    dc = ['docker', 'compose', '-p', project, '-f', str(compose)]
    env = dict(os.environ, TMPDIR=str(work), COMPOSE_PROJECT_NAME=project,
               AWS_EC2_METADATA_DISABLED='true', AWS_DEFAULT_REGION='garage')
    for key in ('AWS_PROFILE', 'AWS_DEFAULT_PROFILE', 'AWS_SESSION_TOKEN'):
        env.pop(key, None)

    def run(label, command, *, check=True, private=False):
        result = subprocess.run(command, env=env, capture_output=True, text=True,
                                timeout=300)
        (work / (label + '.stdout')).write_text(result.stdout)
        (work / (label + '.stderr')).write_text(result.stderr)
        record = dict(label=label, command=command, rc=result.returncode,
                      stdout=str(work / (label + '.stdout')),
                      stderr=str(work / (label + '.stderr')), private=private)
        summary['commands'].append(record)
        print(f'{label}: rc={result.returncode}', flush=True)
        if check and result.returncode:
            raise RuntimeError(f'{label} failed: rc={result.returncode}; see retained output')
        return result

    def sql(label, statement, database='klubhub'):
        return run(label, dc + ['exec', '-T', 'db', 'psql', '-X', '--no-password',
                   '-U', 'klubhub', '-d', database, '-v', 'ON_ERROR_STOP=1', '-At',
                   '-c', statement]).stdout

    bucket = 'drill-' + identity
    attempted_up = False
    rc = 1
    try:
        attempted_up = True
        run('up', dc + ['up', '-d', '--wait', '--wait-timeout', '120'])
        endpoint = 'http://' + run('s3-binding', dc + ['port', 'storage', '3900']).stdout.strip()
        summary['s3_endpoint'] = endpoint
        summary['db_binding'] = run('db-binding', dc + ['port', 'db', '5432']).stdout.strip()
        garage = dc + ['exec', '-T', 'storage', '/garage']
        node = run('node', garage + ['node', 'id', '-q']).stdout.strip().split('@')[0]
        run('layout-assign', garage + ['layout', 'assign', '-z', 'dc1', '-c', '1G', node])
        run('layout-apply', garage + ['layout', 'apply', '--version', '1'])
        run('bucket-create', garage + ['bucket', 'create', bucket])
        credentials = run('key-create-private', garage + ['key', 'create', 'drill-key'],
                          private=True).stdout
        access_match = re.search(r'^Key ID:\s*(GK[0-9a-f]+)', credentials, re.M)
        secret_match = re.search(r'^Secret key:\s*([0-9a-f]+)', credentials, re.M)
        if access_match is None or secret_match is None:
            raise RuntimeError('Garage credentials missing; see private original output')
        access, secret = access_match.group(1), secret_match.group(1)
        env.update(AWS_ACCESS_KEY_ID=access, AWS_SECRET_ACCESS_KEY=secret,
                   S3_ACCESS_KEY=access, S3_SECRET_KEY=secret,
                   S3_ENDPOINT=endpoint, S3_BUCKET=bucket)
        run('bucket-allow', garage + ['bucket', 'allow', bucket, '--read', '--write',
                                    '--key', 'drill-key'])
        aws = ['aws', '--endpoint-url', endpoint]

        def put(label, key, body):
            source = work / (label + '.body')
            source.write_bytes(body)
            run(label, aws + ['s3', 'cp', str(source), f's3://{bucket}/{key}', '--only-show-errors'])

        def storage(label):
            listing = json.loads(run(label + '-keys', aws + ['s3api', 'list-objects-v2',
                '--bucket', bucket, '--output', 'json']).stdout)
            keys = sorted(row['Key'] for row in listing.get('Contents', []))
            root = work / label
            root.mkdir()
            for i, key in enumerate(keys):
                target = root / key
                target.parent.mkdir(parents=True, exist_ok=True)
                run(f'{label}-body-{i}', aws + ['s3', 'cp', f's3://{bucket}/{key}',
                                             str(target), '--only-show-errors'])
            return {key: digest(root / key) for key in keys}

        sql('seed-db', "CREATE TABLE drill_marker(id int PRIMARY KEY, payload text); "
            f"INSERT INTO drill_marker VALUES(1, '{identity} alpha'), (2, '{identity} beta');")
        put('seed-changed', 'changed.txt', b'original changed ' + identity.encode())
        put('seed-missing', 'data/missing.bin', b'original missing\x00' + identity.encode())
        query = 'SELECT id, payload FROM drill_marker ORDER BY id;'
        before_db = sql('before-db', query)
        before_storage = storage('before-storage')
        run('backup', ['bash', str(REPO / 'scripts/backup.sh'), '-f', str(compose), str(work / 'backups')])
        archive, = (work / 'backups').glob('*.tar.gz')
        sha = digest(archive)
        assert Path(str(archive) + '.sha256').read_text().split()[0] == sha
        extracted = Path(run('archive-preflight', ['python3', str(REPO / 'scripts/restore_support.py'),
            'archive', str(archive), sha, str(work / 'archive-readback')]).stdout.strip())
        backup_storage = {p.relative_to(extracted / 'storage').as_posix(): digest(p)
                          for p in (extracted / 'storage').rglob('*') if p.is_file()}
        assert backup_storage == before_storage
        summary.update(archive=str(archive), archive_sha256=sha,
                       before_storage=before_storage, before_db=before_db)
        put('damage-changed', 'changed.txt', b'DAMAGED')
        run('damage-delete', aws + ['s3', 'rm', f's3://{bucket}/data/missing.bin', '--only-show-errors'])
        put('damage-extra', 'extraneous.txt', b'EXTRANEOUS')
        damaged = storage('damaged-storage')
        assert damaged['changed.txt'] != before_storage['changed.txt']
        assert 'data/missing.bin' not in damaged and 'extraneous.txt' in damaged
        summary['damaged_storage'] = damaged
        sql('damage-drop-db', 'DROP DATABASE klubhub WITH (FORCE);', 'postgres')
        missing_db = sql('damage-db-readback', "SELECT count(*) FROM pg_database WHERE datname='klubhub';",
                         'postgres')
        assert missing_db.strip() == '0'
        # Test-only Docker wrapper: inject a same-length corrupt PUT exactly at
        # stop, after extraction/preflight/confirmation, using the real AWS CLI.
        wrapper_dir = work / 'wrapper-bin'
        wrapper_dir.mkdir()
        real_docker = shutil.which('docker')
        real_aws = shutil.which('aws')
        if real_docker is None or real_aws is None:
            raise RuntimeError('real Docker and AWS CLI are required')
        wrapper = wrapper_dir / 'docker'
        wrapper.write_text('''#!/usr/bin/env python3
import datetime, json, os, pathlib, subprocess, sys, time
work = pathlib.Path(os.environ['DRILL_WORK'])
args = sys.argv[1:]
if 'stop' in args and not (work / 'same-size-witness.json').exists():
    sources = list(work.glob('klubhub-restore.*/payload/*/storage/changed.txt'))
    if len(sources) != 1:
        raise SystemExit('expected one extracted archived object')
    source = sources[0]
    original = source.read_bytes()
    corrupt = b'X' * len(original)
    if corrupt == original:
        raise SystemExit('corruption must differ')
    body = work / 'late-corruption.body'
    body.write_bytes(corrupt)
    time.sleep(2)
    aws = [os.environ['DRILL_AWS'], '--endpoint-url', os.environ['S3_ENDPOINT']]
    subprocess.run(aws + ['s3', 'cp', str(body), 's3://' + os.environ['S3_BUCKET'] + '/changed.txt', '--only-show-errors'], check=True)
    result = subprocess.run(aws + ['s3api', 'head-object', '--bucket', os.environ['S3_BUCKET'], '--key', 'changed.txt'], check=True, capture_output=True, text=True)
    head = json.loads(result.stdout)
    remote = datetime.datetime.fromisoformat(head['LastModified'].replace('Z', '+00:00')).timestamp()
    witness = dict(source_mtime=source.stat().st_mtime, destination_mtime=remote,
                   source_size=len(original), destination_size=head['ContentLength'],
                   different_bytes=corrupt != original, after_confirmation=True)
    (work / 'same-size-witness.json').write_text(json.dumps(witness, indent=2) + '\\n')
    if remote <= witness['source_mtime'] or head['ContentLength'] != len(original):
        raise SystemExit('same-size/newer witness not established')
os.execv(os.environ['DRILL_DOCKER'], [os.environ['DRILL_DOCKER']] + args)
''')
        wrapper.chmod(0o700)
        env.update(PATH=str(wrapper_dir) + ':' + env['PATH'], DRILL_WORK=str(work),
                   DRILL_DOCKER=real_docker, DRILL_AWS=real_aws)
        restore = run('restore', ['bash', str(REPO / 'scripts/restore.sh'), '-f', str(compose),
            '--project', project, '--yes', '--confirm-target', f'{project}/klubhub/{bucket}',
            '--archive-sha256', sha, str(archive)])
        summary['restore_rc'] = restore.returncode
        summary['same_size_corruption_witness'] = json.loads(
            (work / 'same-size-witness.json').read_text())
        after_db = sql('after-db', query)
        after_storage = storage('after-storage')
        summary.update(after_db=after_db, after_storage=after_storage,
                       equality_db=after_db == before_db,
                       equality_keys=sorted(after_storage) == sorted(backup_storage),
                       equality_bodies=after_storage == backup_storage)
        require_equal(summary)
        apps = json.loads(run('app-state', dc + ['ps', '-a', '--format', 'json', 'app']).stdout)
        if isinstance(apps, dict):
            apps = [apps]
        assert len(apps) == 1 and apps[0]['State'] == 'exited'
        assert manifest == {str(p.relative_to(REPO)): digest(p) for p in sources}
        summary['equal'] = True
        rc = 0
    except (Exception, SystemExit) as error:
        summary['error'] = str(error)
        print(f'DRILL FAILED: {type(error).__name__}: {error}', flush=True)
    finally:
        if attempted_up:
            try:
                run('cleanup', dc + ['down', '-v', '--remove-orphans'])
                selector = ['--filter', 'label=com.docker.compose.project=' + project]
                for kind, command in (
                    ('containers', ['docker', 'ps', '-aq'] + selector),
                    ('volumes', ['docker', 'volume', 'ls', '-q'] + selector),
                    ('networks', ['docker', 'network', 'ls', '-q'] + selector)):
                    assert not run('cleanup-readback-' + kind, command).stdout.strip()
                summary['cleanup_verified'] = True
            except Exception as error:
                summary['cleanup_error'] = str(error)
                rc = 1
        summary['exit_code'] = rc
        # Exclusive create: never overwrite/relabel historical drill evidence.
        with (work / 'canonical-summary.json').open('x') as output:
            json.dump(summary, output, indent=2)
            output.write('\n')
        ledger = {p.relative_to(work).as_posix(): digest(p)
                  for p in sorted(work.rglob('*')) if p.is_file()}
        with (work / 'ledger-sha256.json').open('x') as output:
            json.dump(ledger, output, indent=2)
            output.write('\n')
        print(f'canonical evidence: {work / "canonical-summary.json"}', flush=True)
    raise SystemExit(rc)


if __name__ == '__main__':
    main()
