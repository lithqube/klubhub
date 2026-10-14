# Trusted PostgreSQL + Garage restore

`restore.sh` drops the selected database and replaces the selected S3 bucket,
including deleting keys absent from the backup. Run only with owner approval,
against an explicitly selected Compose file/project. Never infer the target from
an archive filename or copy a drill command into a production terminal.

## Before destruction

- Quiesce **every writer** during backup to obtain a consistent database/object
  snapshot: `backup.sh` itself does not stop applications. Take a separate safety
  backup of the target before a planned restore.
- Trust the SQL producer. A checksum detects corruption, not malicious SQL or
  provenance; SQL replay is privileged. Obtain the expected SHA-256 through a
  trusted channel (or use a trusted `.sha256` sidecar beside the archive).
- Resolve `docker compose -f COMPOSE -p PROJECT config --services` and verify the
  actual project/container labels and DB/user/bucket with the owner. Default
  service names are `app`, `db`, `storage`. Repeat `--app-service SERVICE` for
  every writer; use `--db-service` / `--storage-service` for renamed services.
- Load `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_ENDPOINT`, and `S3_BUCKET` via a secure
  environment/secret manager. Never put credentials in command arguments, logs,
  documentation, or chat. For Garage, set `AWS_DEFAULT_REGION=garage` (or the
  region in its configuration). Endpoint must be loopback and match the actual
  published storage container port 3900; host ports may be dynamically allocated.
- Require Docker Compose, AWS CLI and Python 3. Keep backups and temporary
  extraction on protected storage; set `TMPDIR` to a protected scratch directory.

## Interactive restore (safe default)

Replace uppercase placeholders deliberately; this is a template, not a ready
production command. The script will display `PROJECT/DATABASE/BUCKET` and require
that exact text before stopping applications or dropping anything:

```sh
bash scripts/restore.sh -f COMPOSE --project PROJECT \
  --archive-sha256 TRUSTED_SHA256 ARCHIVE.tar.gz
```

For explicitly approved automation only, add `--yes --confirm-target
PROJECT/DATABASE/BUCKET`. A bare `--yes` does not bypass confirmation. Preflight
validates the archive layout, checksum, target configuration, real container
ownership/state, maintenance DB access and S3 bucket access before stopping apps.
An archive must contain nonempty `db.sql` and `storage/`, at root or under a
single `work-*` directory. Traversal, links, special entries, ambiguous layouts
and duplicate entries are rejected. Native macOS backup tar suppresses metadata
sidecars rather than weakening these checks.

## Success and stopped/recoverable failure

Success means SQL replay and unconditional bucket upload/exact-key pruning
succeeded, **not** that business data or application health is verified.
`aws s3 sync` is deliberately not used for restore: local-to-S3 sync skips
same-size destinations with equal/newer mtimes even when their bytes differ.
Recursive `aws s3 cp` overwrites every archived object; a paginated JSON listing
and checked individual exact-key deletes remove only keys absent from the
archive (all keys for an empty archive). Upload/list/delete failure prevents
restart. By default applications remain stopped.
Validate database rows, object key sets and bytes, then start only approved
writers using the exact same Compose file/project. `--restart-on-success` is
opt-in and restarts only applications that were running before preflight; it
never starts applications that were already stopped.

A preflight failure leaves running services unchanged. Any failure after
quiescing attempts to leave all selected apps stopped, including replay/sync or
restart failures. Check their actual state; a CRITICAL stop error requires manual
intervention. Do not start against partial data. Preserve the archive and logs,
fix the cause, and rerun the **full** restore to the same approved target (DROP
and CREATE use the maintenance database, so rerunning works with a missing DB).
Verify the complete DB and object snapshot before manually restarting writers.
Temporary extraction is removed; the archive is not removed.

## Disposable real acceptance drill

```sh
TMPDIR="$HOME/.hermes/cache/scratch" PYTHONDONTWRITEBYTECODE=1 \
  python3 scripts/tests/test_restore.py
TMPDIR="$HOME/.hermes/cache/scratch" PYTHONDONTWRITEBYTECODE=1 \
  python3 scripts/tests/test_restore_drill.py
PYTHONDONTWRITEBYTECODE=1 python3 scripts/tests/restore_drill.py
```

The live command creates one new full-UUID project with real PostgreSQL 16 and
Garage 2.2; no fallback/stub path exists. All published host ports are ephemeral
and bound to 127.0.0.1. Configuration follows Garage's official quick-start;
manual layout/key provisioning is required for 2.2 (automatic provisioning in
the current quick-start requires 2.3). It runs the actual `backup.sh`, verifies
its exact archive, overwrites one object, deletes another, adds an extraneous
key, and drops **only its own fixture database** before the real restore.
A test-only Docker wrapper injects another, same-length different-body PUT at
application stop, after restore extraction/preflight/confirmation. Real AWS
head-object readback proves its destination mtime is newer than the extracted
file. Exact final bytes must still match the archive.
It asserts DB rows, key set, object SHA-256/bytes, and stopped-app policy; any
inequality exits 1. Cleanup removes only its UUID project's resources and reads
back container/volume/network absence.

Evidence lives in `~/.hermes/cache/scratch/finance-hardening-evidence/restore/
work-UUID/`. Each exclusive-create `canonical-summary.json` records retained
archive identity/checksum, source manifest, full original command stdout/stderr
paths and true return codes, before/damaged/after readbacks, and cleanup checks.
The copied sources and `source-hashes.json` bind that run to the uncommitted
artifact. The summary never overwrites or relabels historical runs. Managed
scratch may be pruned after inactivity: export the entire protected evidence
folder to durable storage before relying on it long term.

Evidence folders/files are private (0700/0600). `garage.toml` and the original
`key-create-private.stdout` contain **disposable credentials** and must not be
published; the canonical summary marks that command private and contains no
credential values. Reports must reference paths, not paste these files.
