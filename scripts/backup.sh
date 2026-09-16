#!/usr/bin/env bash
# KlubHub DJ — Backup Script
#
# Dumps PostgreSQL and mirrors Garage S3 storage into a single timestamped
# tar.gz archive. Fails closed: every step returns non-zero on error, the
# offending step is named, and partial work directories are cleaned up.
#
# Usage:
#   bash scripts/backup.sh [output-dir]
#   bash scripts/backup.sh -f docker-compose.prod.yml [output-dir]
#
# Environment (required for the storage step):
#   S3_ENDPOINT      default: http://127.0.0.1:39000
#   S3_ACCESS_KEY    required
#   S3_SECRET_KEY    required
#   S3_BUCKET        default: klubhub
#
# The script uses AWS_* env vars (set below) instead of inline
# `--access-key`/`--secret-key` CLI flags so the secret never appears in
# `ps` output, and never passes `--no-verify-ssl`.
#
# Requires: docker compose stack running (db healthy, storage healthy);
# aws CLI on PATH; the `garage` binary is NOT required (we hit S3 only).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ---- Arg parsing ---------------------------------------------------------
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"
OUTPUT_DIR="${PROJECT_ROOT}/backups"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file)
      COMPOSE_FILE="$2"
      shift 2
      ;;
    -h|--help)
      sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    -*)
      echo "[backup] ERROR: unknown flag: $1" >&2
      exit 2
      ;;
    *)
      OUTPUT_DIR="$1"
      shift
      ;;
  esac
done

if [[ ! -f "${COMPOSE_FILE}" ]]; then
  echo "[backup] ERROR: compose file not found: ${COMPOSE_FILE}" >&2
  exit 1
fi

TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
WORK_DIR="${OUTPUT_DIR}/work-${TIMESTAMP}"
ARCHIVE="${OUTPUT_DIR}/klubhub-backup-${TIMESTAMP}.tar.gz"

mkdir -p "${OUTPUT_DIR}"
mkdir -p "${WORK_DIR}"

# Cleanup trap: remove the partial work dir on any failure so we never leave
# half-written backups lying around. Use a script-scoped variable (see
# verifying-infra-as-code skill pitfall D: trap-on-local-var under set -u).
cleanup() {
  local rc=$?
  if [[ ${rc} -ne 0 && -d "${WORK_DIR}" ]]; then
    echo "[backup] ERROR: backup failed (exit ${rc}); removing ${WORK_DIR}" >&2
    rm -rf "${WORK_DIR}"
  fi
}
trap cleanup EXIT

echo "[backup] Starting KlubHub DJ backup — ${TIMESTAMP}"
echo "[backup] Compose file: ${COMPOSE_FILE}"
echo "[backup] Output dir:  ${OUTPUT_DIR}"

# ---- Resolve required secrets early (fail fast) --------------------------
# Use explicit checks (not just `: ${VAR:?...}`) so `set -e` actually
# aborts on a missing secret — `: ${VAR:?...}` prints the message and
# continues with rc=0 in interactive shells.
if [[ -z "${S3_ACCESS_KEY:-}" ]]; then
  echo "[backup] ERROR: S3_ACCESS_KEY env var is required" >&2
  exit 1
fi
if [[ -z "${S3_SECRET_KEY:-}" ]]; then
  echo "[backup] ERROR: S3_SECRET_KEY env var is required" >&2
  exit 1
fi

# Set AWS_* for the aws CLI (avoids inline --access-key/--secret-key flags).
export AWS_ACCESS_KEY_ID="${S3_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${S3_SECRET_KEY}"
# Don't let a stale AWS_PROFILE from the user's env leak in.
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN

S3_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:39000}"
S3_BUCKET="${S3_BUCKET:-klubhub}"

# ---- 1. PostgreSQL dump --------------------------------------------------
echo "[backup] Step 1/3: Dumping PostgreSQL..."
if ! docker compose -f "${COMPOSE_FILE}" exec -T db \
    pg_dump -U klubhub -d klubhub --no-password \
    > "${WORK_DIR}/db.sql"; then
  echo "[backup] ERROR: pg_dump failed" >&2
  exit 1
fi
if [[ ! -s "${WORK_DIR}/db.sql" ]]; then
  echo "[backup] ERROR: pg_dump produced an empty file" >&2
  exit 1
fi

# ---- 2. Garage S3 mirror -------------------------------------------------
echo "[backup] Step 2/3: Mirroring Garage S3 storage (s3://${S3_BUCKET} via ${S3_ENDPOINT})..."
mkdir -p "${WORK_DIR}/storage"

# `--only-show-errors` keeps stdout clean for the archive; we still bail on
# non-zero exit so an unreachable Garage is a real failure, not `|| true`.
if ! aws s3 sync "s3://${S3_BUCKET}" "${WORK_DIR}/storage/" \
    --endpoint-url "${S3_ENDPOINT}" \
    --only-show-errors; then
  echo "[backup] ERROR: aws s3 sync failed against ${S3_ENDPOINT}" >&2
  exit 1
fi

# An empty bucket is fine but verify the dir exists (sanity).
if [[ ! -d "${WORK_DIR}/storage" ]]; then
  echo "[backup] ERROR: storage mirror directory is missing" >&2
  exit 1
fi

# ---- 3. Archive ----------------------------------------------------------
echo "[backup] Step 3/3: Creating archive: ${ARCHIVE}"
# `--owner=0 --group=0 --numeric-owner` so the archive extracts cleanly
# regardless of which UID the restorer runs as.
if ! tar --owner=0 --group=0 --numeric-owner \
       -czf "${ARCHIVE}" -C "${OUTPUT_DIR}" "work-${TIMESTAMP}"; then
  echo "[backup] ERROR: tar archive creation failed" >&2
  exit 1
fi

# Clear the success-flag so the cleanup trap leaves the work dir alone, then
# remove it ourselves.
trap - EXIT
rm -rf "${WORK_DIR}"

ARCHIVE_SIZE=$(du -sh "${ARCHIVE}" | cut -f1)
echo "[backup] Complete: ${ARCHIVE} (${ARCHIVE_SIZE})"
echo "[backup] To restore: bash scripts/restore.sh ${ARCHIVE}"