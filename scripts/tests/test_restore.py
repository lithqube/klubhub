#!/usr/bin/env python3
"""Executable fail-closed restore contract.

These tests pin the regressions identified in security finding SR-01:
  (a) restore.sh references the current stack's application service (app),
      not the obsolete api/frontend names. Passing --app-service with a
      non-existent service must abort before any container is touched.
  (b) DROP DATABASE and CREATE DATABASE are executed OUTSIDE a psql
      transaction (single-statement -c calls, no --single-transaction),
      because DROP/CREATE DATABASE raise SQLSTATE 25001 in a transaction.
      The SQL replay must use --single-transaction so a bad dump rolls
      back without leaving a partially-replayed database.
  (c) Every post-quiesce failure (stop / drop / create / replay / sync)
      leaves the application stopped. There is no automatic restart
      onto a possibly-corrupt DB; restart is opt-in and only after the
      SQL replay AND storage sync both succeed.

Boundary isolation: docker and aws are replaced with a shell-stub
binary on PATH so the test is hermetic and exercises exactly the
orchestration decisions in restore.sh. The end-to-end real Docker
compose + Garage round-trip lives in restore_drill.py.
"""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'restore.sh'
SUPPORT = Path(__file__).resolve().parents[1] / 'restore_support.py'
BACKUP = Path(__file__).resolve().parents[1] / 'backup.sh'

# A Python stub masquerading as `docker` or `aws`. It records every
# invocation to a log and lets each test inject one of the boundary
# failures called out by the audit.
STUB = r"""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path

argv = sys.argv[1:]
log_path = os.environ['CALLS']
fail = os.environ.get('FAIL', '')

def record():
    with open(log_path, 'a') as f:
        f.write(json.dumps([Path(sys.argv[0]).name] + argv) + '\n')

# `aws` stub
if Path(sys.argv[0]).name == 'aws':
    record()
    if fail == 'storage' and ('sync' in argv or 'cp' in argv):
        sys.exit(19)
    if fail == 'listing' and 'list-objects-v2' in argv:
        sys.exit(19)
    if fail == 'deletion' and 'delete-object' in argv:
        sys.exit(19)
    if 'list-objects-v2' in argv:
        print(json.dumps({'Contents': [{'Key': k} for k in
            json.loads(os.environ.get('DEST_KEYS', '[]'))]}))
    if 'cp' in argv and '--recursive' in argv:
        import shutil
        source = Path(argv[2])
        destination = os.environ.get('DEST_ROOT')
        if destination:
            for p in source.rglob('*'):
                if p.is_file():
                    target = Path(destination) / p.relative_to(source)
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(p, target)
    if 'delete-object' in argv and os.environ.get('DEST_ROOT'):
        (Path(os.environ['DEST_ROOT']) / argv[argv.index('--key') + 1]).unlink()
    if fail == 'bucket' and 'head-bucket' in argv:
        sys.exit(19)
    if 'sync' in argv and argv[2].startswith('s3://'):
        marker = Path(argv[3]) / 'metadata-marker'
        marker.write_bytes(b'sr01 real backup object')
        if sys.platform == 'darwin':
            import subprocess
            subprocess.run(['/usr/bin/xattr', '-w', 'com.apple.comment',
                            'sr01', str(marker)], check=True)
    sys.exit(0)

# `docker` stub
record()

# `docker inspect <id>` -- returns running/stopped for the supplied id
if argv and argv[0] == 'inspect':
    target = argv[-1]
    state = 'running'
    try:
        for line in open(log_path).read().splitlines():
            cmd = json.loads(line)
            svc = target.replace('id-', '')
            if cmd[-1] == svc and 'stop' in cmd and fail != 'stop-noop':
                state = 'stopped'
            if cmd[-1] == svc and 'start' in cmd:
                state = 'running'
    except FileNotFoundError:
        pass
    project = 'wrong-project' if 'wrong' in fail else 'restore-test'
    print(json.dumps([{
        'Id': 'id-' + target.replace('id-', ''),
        'State': {'Running': state == 'running'},
        'Config': {'Labels': {
            'com.docker.compose.project': project,
            'com.docker.compose.service': target.replace('id-', ''),
        }},
    }]))
elif 'config' in argv and '--format' in argv:
    print(json.dumps({'name': 'restore-test', 'services': {
        'db': {'environment': {'POSTGRES_USER': 'klubhub', 'POSTGRES_DB': 'klubhub'}},
        'storage': {},
        'app': {},
    }}))
elif 'ps' in argv and any(a.startswith('-') and 'q' in a for a in argv):
    print('id-' + argv[-1])
elif 'port' in argv:
    print('127.0.0.1:39999')
elif 'stop' in argv:
    if fail == 'stop':
        sys.exit(17)
elif 'start' in argv:
    if fail == 'start':
        sys.exit(17)
elif 'up' in argv:
    if fail == 'up':
        sys.exit(17)
elif 'exec' in argv:
    rest = argv[argv.index('exec') + 1:]
    if any(s in ('api', 'frontend') for s in rest):
        sys.exit('no such service: api')
    if any('DROP DATABASE' in s for s in rest):
        if '--single-transaction' in rest:
            sys.exit('SQLSTATE 25001: DROP DATABASE cannot run inside a transaction block')
        if fail == 'drop':
            sys.exit(18)
    elif any('CREATE DATABASE' in s for s in rest):
        if fail == 'create':
            sys.exit(18)
    elif '-f' in rest or '--file' in rest:
        sys.stdin.read()
        if fail == 'replay':
            sys.exit(18)
    else:
        if fail == 'maintenance' and '-d' in rest and rest[rest.index('-d') + 1] == 'postgres':
            sys.exit('database does not exist')
        db = rest[rest.index('-d') + 1] if '-d' in rest else '?'
        print(db + '|klubhub')
"""


class Restore(unittest.TestCase):
    def setUp(self):
        scratch = Path(os.environ.get(
            'TMPDIR',
            str(Path.home() / '.hermes' / 'cache' / 'scratch'),
        ))
        scratch.mkdir(parents=True, exist_ok=True)
        self.tmp = tempfile.TemporaryDirectory(dir=scratch)
        self.root = Path(self.tmp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ('docker', 'aws'):
            p = self.bin / name
            p.write_text(STUB)
            p.chmod(0o755)
        self.compose = self.root / 'compose.yml'
        self.compose.write_text('name: restore-test\nservices: {}\n')
        self.archive = self.root / 'backup.tar.gz'
        with tarfile.open(self.archive, 'w:gz') as t:
            for name, data in [
                ('work-test/db.sql', b'CREATE TABLE marker(id int);\n'),
                ('work-test/storage/marker', b'original'),
            ]:
                i = tarfile.TarInfo(name)
                i.size = len(data)
                t.addfile(i, io.BytesIO(data))
            i = tarfile.TarInfo('work-test/storage')
            i.type = tarfile.DIRTYPE
            t.addfile(i)
        self.sha = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        self.calls = self.root / 'calls'
        self.env = dict(
            os.environ,
            PATH=str(self.bin) + ':' + os.environ['PATH'],
            CALLS=str(self.calls),
            S3_ACCESS_KEY='fixture',
            S3_SECRET_KEY='fixture',
            S3_ENDPOINT='http://127.0.0.1:39999',
            S3_BUCKET='fixture',
        )

    def tearDown(self):
        self.tmp.cleanup()

    def run_restore(self, fail='', extra=(), checksum=None, env_override=None):
        env = dict(self.env if env_override is None else env_override, FAIL=fail)
        p = subprocess.run(
            [
                'bash', str(SCRIPT), '-f', str(self.compose),
                '--project', 'restore-test',
                '--archive-sha256', self.sha if checksum is None else checksum,
                '--yes',
                '--confirm-target', 'restore-test/klubhub/fixture',
                *extra,
                str(self.archive),
            ],
            env=env, text=True, capture_output=True,
        )
        calls = (
            [json.loads(l) for l in self.calls.read_text().splitlines()]
            if self.calls.exists() else []
        )
        return p, calls

    # (a)+(b) happy path: correct service name, DROP/CREATE outside txn,
    # replay inside a single transaction with ON_ERROR_STOP.
    def test_current_stack_services_and_transaction_boundaries(self):
        p, calls = self.run_restore(extra=['--restart-on-success'])
        self.assertEqual(p.returncode, 0, p.stderr + p.stdout)
        stop = [c for c in calls if c[0] == 'docker' and 'stop' in c]
        self.assertEqual(stop[0][-1], 'app', calls)
        self.assertFalse(any('api' in c or 'frontend' in c for c in calls),
                         f'obsolete service names referenced: {calls}')
        drop = [c for c in calls if c[0] == 'docker' and any('DROP DATABASE' in x for x in c)]
        create = [c for c in calls if c[0] == 'docker' and any('CREATE DATABASE' in x for x in c)]
        self.assertEqual(len(drop), 1, calls)
        self.assertEqual(len(create), 1, calls)
        self.assertNotIn('--single-transaction', drop[0])
        self.assertNotIn('--single-transaction', create[0])
        replay = [c for c in calls if c[0] == 'docker' and 'psql' in c and any(x in ('-f', '--file') for x in c) and ('--single-transaction' in c or 'ON_ERROR_STOP' in c)]
        self.assertIn('--single-transaction', replay[0])
        self.assertTrue(any('ON_ERROR_STOP' in x for x in replay[0]))
        self.assertTrue(any('start' in c for c in calls))

    # (c) Failure paths must NEVER auto-restart onto a possibly-corrupt DB.
    def test_each_post_stop_failure_never_restarts(self):
        for failure in ('stop', 'drop', 'create', 'replay', 'storage', 'start'):
            with self.subTest(failure=failure):
                self.calls.unlink(missing_ok=True)
                p, calls = self.run_restore(fail=failure, extra=['--restart-on-success'])
                self.assertNotEqual(p.returncode, 0, p.stderr)
                self.assertTrue(any('stop' in c for c in calls), p.stderr)
                self.assertIn('remain stopped', p.stderr)
                if failure != 'start':
                    self.assertFalse(
                        any('start' in c or 'up' in c for c in calls),
                        f'{failure} must not auto-restart; got {calls}',
                    )

    def test_preflight_failure_leaves_running_services_untouched(self):
        for failure in ('wrong', 'bucket'):
            with self.subTest(failure=failure):
                self.calls.unlink(missing_ok=True)
                p, calls = self.run_restore(fail=failure)
                self.assertNotEqual(p.returncode, 0, p.stderr)
                self.assertFalse(any('stop' in c or 'start' in c for c in calls))

    def test_checksum_mismatch_aborts_before_stop(self):
        p, calls = self.run_restore(checksum='0' * 64)
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse(any('stop' in c for c in calls))

    def test_unknown_application_service_rejected_before_stop(self):
        p, calls = self.run_restore(extra=['--app-service', 'api'])
        self.assertNotEqual(p.returncode, 0, p.stderr)
        self.assertFalse(any('stop' in c for c in calls))

    def test_obsolete_api_or_frontend_name_aborts_before_stop(self):
        # Audit-observed "Real Compose dry-run exits 1: no such service: api".
        for bogus in ('api', 'frontend'):
            with self.subTest(bogus=bogus):
                self.calls.unlink(missing_ok=True)
                p, calls = self.run_restore(extra=['--app-service', bogus])
                self.assertNotEqual(p.returncode, 0, p.stderr + p.stdout)
                self.assertFalse(any('stop' in c for c in calls))

    def test_missing_secrets_abort_before_stop(self):
        env = dict(self.env)
        env.pop('S3_SECRET_KEY')
        self.calls.unlink(missing_ok=True)
        p, calls = self.run_restore(env_override=env)
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse(any('stop' in l for l in calls))

    def test_success_without_restart_policy_leaves_stopped(self):
        p, calls = self.run_restore()
        self.assertEqual(p.returncode, 0, p.stderr + p.stdout)
        self.assertFalse(any('start' in c or 'up' in c for c in calls))

    def test_maintenance_db_unreachable_does_not_stop_apps(self):
        p, calls = self.run_restore(fail='maintenance')
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse(any('stop' in c for c in calls))

    def test_backup_omits_macos_metadata_entries(self):
        output = self.root / 'actual-backup'
        env = dict(self.env)
        env.pop('COPYFILE_DISABLE', None)  # Native macOS tar must be exercised.
        p = subprocess.run(
            ['bash', str(BACKUP), '-f', str(self.compose), str(output)],
            env=env, capture_output=True, text=True,
        )
        self.assertEqual(p.returncode, 0, p.stderr + p.stdout)
        local, = output.glob('*.tar.gz')
        checksum = Path(str(local) + '.sha256').read_text().split()[0]
        preflight = subprocess.run(
            ['python3', str(SUPPORT), 'archive', str(local), checksum,
             str(self.root / 'extracted')], capture_output=True, text=True,
        )
        self.assertEqual(preflight.returncode, 0, preflight.stderr)
        root = Path(preflight.stdout.strip())
        self.assertEqual((root / 'storage/metadata-marker').read_bytes(),
                         b'sr01 real backup object')
        with tarfile.open(local, 'r:gz') as t:
            names = t.getnames()
        self.assertFalse(any('/._' in n or n.startswith('._') for n in names), names)

    def test_backup_produces_archive_with_checksum_sidecar(self):
        output = self.root / 'backups'
        p = subprocess.run(
            ['bash', str(BACKUP), '-f', str(self.compose), str(output)],
            env=self.env, capture_output=True, text=True,
        )
        self.assertEqual(p.returncode, 0, p.stderr + p.stdout)
        self.assertIn('--project', p.stdout)
        self.assertIn('-f', p.stdout)
        self.assertIn('S3_ENDPOINT', p.stdout)
        self.assertIn('scripts/RESTORE.md', p.stdout)
        archives = list(output.glob('*.tar.gz'))
        self.assertEqual(len(archives), 1, p.stderr + p.stdout)
        sidecar = Path(str(archives[0]) + '.sha256')
        self.assertTrue(sidecar.is_file(), 'backup must publish an integrity sidecar')
        self.assertEqual(
            sidecar.read_text().split()[0],
            hashlib.sha256(archives[0].read_bytes()).hexdigest(),
        )
        with tarfile.open(archives[0], 'r:gz') as t:
            self.assertTrue(any(n.endswith('/db.sql') for n in t.getnames()))

    def test_unconditional_overwrite_and_exact_nested_pruning(self):
        destination = self.root / 'destination'
        destination.mkdir()
        (destination / 'marker').write_bytes(b'XXXXXXXX')
        os.utime(destination / 'marker', (2000000000, 2000000000))
        (destination / 'marker-extra').write_bytes(b'extra')
        (destination / 'nested').mkdir()
        (destination / 'nested/extra').write_bytes(b'extra')
        self.env.update(DEST_ROOT=str(destination),
                        DEST_KEYS=json.dumps(['marker', 'marker-extra', 'nested/extra']))
        p, calls = self.run_restore(extra=['--restart-on-success'])
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual((destination / 'marker').read_bytes(), b'original')
        self.assertEqual([p.relative_to(destination).as_posix() for p in
                          destination.rglob('*') if p.is_file()], ['marker'])
        self.assertTrue(any('cp' in c and '--recursive' in c for c in calls))

    def test_empty_archive_deletes_every_destination_key(self):
        with tarfile.open(self.archive, 'w:gz') as t:
            data = b'CREATE TABLE marker(id int);'
            member = tarfile.TarInfo('work-test/db.sql')
            member.size = len(data)
            t.addfile(member, io.BytesIO(data))
            member = tarfile.TarInfo('work-test/storage')
            member.type = tarfile.DIRTYPE
            t.addfile(member)
        self.sha = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        self.env['DEST_KEYS'] = json.dumps(['one', 'nested/two'])
        p, calls = self.run_restore()
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(sorted(c[c.index('--key') + 1] for c in calls
                                if 'delete-object' in c), ['nested/two', 'one'])

    def test_pruning_failures_never_restart(self):
        self.env['DEST_KEYS'] = json.dumps(['extra'])
        for failure in ('listing', 'deletion'):
            with self.subTest(failure=failure):
                self.calls.unlink(missing_ok=True)
                p, calls = self.run_restore(fail=failure, extra=['--restart-on-success'])
                self.assertNotEqual(p.returncode, 0)
                self.assertFalse(any('start' in c for c in calls))
                self.assertIn('remain stopped', p.stderr)

    def test_archive_traversal_rejected(self):
        with tarfile.open(self.archive, 'w:gz') as t:
            i = tarfile.TarInfo('../escape')
            i.size = 1
            t.addfile(i, io.BytesIO(b'x'))
        self.sha = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        p, calls = self.run_restore()
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse(any('stop' in c for c in calls))


if __name__ == '__main__':
    unittest.main(verbosity=2)