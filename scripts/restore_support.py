#!/usr/bin/env python3
"""Fail-closed preflight + boundary helpers for restore.sh (stdlib only).

Invoked by restore.sh as: python3 scripts/restore_support.py <command> [args...]
where <command> is one of:
  archive <tarball> <expected-sha256> <destination-dir>     validate+extract, print path
  target  <stack-file> <project> <db-svc> <storage-svc> <app-spec>
                                                           resolve services, print names
  inspect <project> <service> <required-state>              read docker inspect json from stdin,
                                                           print "running"|"stopped"
  endpoint <s3-url> <host:port binding>                     validate loopback endpoint matches binding

Any error prints "[restore] preflight ERROR: <message>" to stderr and exits 1.
"""
import hashlib
import json
import os
import re
import sys
from pathlib import Path, PurePosixPath
import shutil
import tarfile
import subprocess
from urllib.parse import urlparse


_IDENT = re.compile(r'^[A-Za-z_][A-Za-z0-9_]{0,62}$')
_PROJECT = re.compile(r'^[a-z0-9][a-z0-9_-]*$')
_S3_BUCKET = re.compile(r'^[a-z0-9][a-z0-9.-]+[a-z0-9]$')
_SHA256 = re.compile(r'^[0-9a-fA-F]{64}$')


def _fail(message):
    print('[restore] preflight ERROR: ' + str(message), file=sys.stderr)
    sys.exit(1)


def archive(source, expected, destination):
    if not _SHA256.fullmatch(expected or ''):
        _fail('expected archive SHA-256 must be 64 hex characters')
    if not os.path.isfile(source):
        _fail('archive not found')
    digest = hashlib.sha256()
    with open(source, 'rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            digest.update(block)
    if digest.hexdigest() != expected.lower():
        _fail('archive checksum mismatch')
    os.makedirs(destination, mode=0o700, exist_ok=True)
    with tarfile.open(source, 'r:gz') as t:
        members = t.getmembers()
        names = set()
        roots = set()
        total = 0
        for m in members:
            p = PurePosixPath(m.name)
            if p.is_absolute() or '..' in p.parts or not p.parts or '\\' in m.name:
                _fail('unsafe archive path')
            name = str(p)
            if name in names:
                _fail('duplicate archive entry')
            names.add(name)
            if not (m.isfile() or m.isdir()):
                _fail('links/special archive entries forbidden')
            total += m.size
            if total > 100 * 1024**3 or len(names) > 1000000:
                _fail('archive exceeds safety limits')
            if p.parts[0].startswith('work-'):
                roots.add(p.parts[0])
                relative = p.parts[1:]
            else:
                roots.add('')
                relative = p.parts
            if relative and not (relative == ('db.sql',) or relative[0] == 'storage'):
                _fail('unexpected archive layout')
        if len(roots) != 1:
            _fail('ambiguous archive roots')
        root = roots.pop()
        prefix = root + '/' if root else ''
        dump = next(
            (m for m in members if str(PurePosixPath(m.name)) == prefix + 'db.sql'),
            None,
        )
        storage_present = any(
            str(PurePosixPath(m.name)) == prefix + 'storage' and m.isdir()
            for m in members
        )
        if dump is None or not dump.isfile() or dump.size == 0 or not storage_present:
            _fail('archive missing nonempty db.sql + storage directory')
        for m in members:
            dest = Path(destination) / str(PurePosixPath(m.name))
            if m.isdir():
                dest.mkdir(parents=True, exist_ok=True, mode=0o700)
            else:
                dest.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
                src = t.extractfile(m)
                if src is None:
                    _fail('unreadable regular archive member')
                with src, open(dest, 'xb') as out:
                    shutil.copyfileobj(src, out)
                dest.chmod(0o600)
    print(str(Path(destination) / root))


def target(stack_path, project, db, storage, requested):
    if not _PROJECT.fullmatch(project or ''):
        _fail('explicit --project required (lowercase Compose project name)')
    c = json.loads(Path(stack_path).read_text())
    if c.get('name') != project:
        _fail('resolved Compose project mismatch')
    if db not in c['services'] or storage not in c['services'] or db == storage:
        _fail('DB/storage service missing or ambiguous in stack')
    apps = requested.split() if requested else (['app'] if 'app' in c['services'] else [])
    if not apps:
        _fail(
            'no application service resolved; pass --app-service to target the'
            ' correct service for this stack'
        )
    if len(set(apps)) != len(apps) or any(
        a not in c['services'] or a in (db, storage) for a in apps
    ):
        _fail('application service missing/unsafe; specify --app-service for each writer')
    for secret in c.get('secrets', {}).values():
        if 'file' in secret and not Path(secret['file']).is_file():
            _fail('required Compose secret file missing')
    env = c['services'][db].get('environment', {})
    user = env.get('POSTGRES_USER', 'klubhub')
    database = env.get('POSTGRES_DB', 'klubhub')
    for value in (user, database):
        if not isinstance(value, str) or not _IDENT.fullmatch(value):
            _fail('DB/user must be simple SQL identifiers')
    if database in ('postgres', 'template0', 'template1'):
        _fail('refusing maintenance database target')
    print(' '.join(apps))
    print(user)
    print(database)


def inspect(project, service, required='running'):
    rows = json.load(sys.stdin)
    if len(rows) != 1:
        _fail('target must be exactly one container')
    c = rows[0]
    labels = c['Config']['Labels']
    if (
        labels.get('com.docker.compose.project') != project
        or labels.get('com.docker.compose.service') != service
    ):
        _fail('actual container project/service mismatch')
    state = 'running' if c['State']['Running'] else 'stopped'
    if required != 'either' and state != required:
        _fail('target container must be ' + required)
    print(state)


def endpoint(url, binding):
    try:
        host, port = binding.rsplit(':', 1)
    except ValueError:
        _fail('malformed storage port binding')
    u = urlparse(url)
    if (
        u.scheme not in ('http', 'https')
        or u.hostname not in ('127.0.0.1', 'localhost')
        or u.username
        or u.password
        or u.path not in ('', '/')
        or u.query
        or u.fragment
    ):
        _fail('S3 endpoint must be explicit loopback URL without credentials/path')
    if host not in ('127.0.0.1', '0.0.0.0') or str(u.port) != port:
        _fail('S3 endpoint does not match target storage published port')


def prune_storage(root, bucket, url):
    """Remove only exact keys absent from the archive after unconditional PUTs.

    AWS CLI auto-pagination collects every page with JSON output. Individual
    delete-object calls fail on transport/API errors; no bulk Errors can hide
    inside a successful delete-objects response. Empty archives remove all keys.
    """
    archived = {p.relative_to(root).as_posix() for p in Path(root).rglob('*')
                if p.is_file()}
    aws = ['aws', '--endpoint-url', url, 's3api']
    result = subprocess.run(aws + ['list-objects-v2', '--bucket', bucket,
                                  '--output', 'json'], check=True,
                            capture_output=True, text=True)
    listing = json.loads(result.stdout)
    for row in listing.get('Contents', []):
        key = row['Key']
        if key not in archived:
            subprocess.run(aws + ['delete-object', '--bucket', bucket,
                                  '--key', key], check=True, stdout=subprocess.DEVNULL)


if __name__ == '__main__':
    try:
        command = sys.argv[1]
    except IndexError:
        _fail('missing subcommand')
    cmds = {
        'archive': archive,
        'target': target,
        'inspect': inspect,
        'endpoint': endpoint,
        'prune-storage': prune_storage,
    }
    if command not in cmds:
        _fail('unknown subcommand')
    try:
        cmds[command](*sys.argv[2:])
    except SystemExit:
        raise
    except Exception as error:  # noqa: BLE001
        _fail(str(error))