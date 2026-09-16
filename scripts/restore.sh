#!/usr/bin/env bash
# KlubHub DJ — Restore Script
#
# Restores PostgreSQL and Garage S3 storage from a backup archive produced
# by backup.sh. Validates the archive up front, quiesces the api and
# frontend so no requests race the restore, drops+recreates the database,
# replays the SQL with `psql -v ON_ERROR_STOP=1`, mirrors the storage back,
# and brings services back up.
#
# Usage:
#   bash scripts/restore.sh <backup-archive.tar.gz>
#   bash scripts/restore.sh -f docker-compose.prod.yml <backup-archive.tar.gz>
#
# WARNING: This DROPS and recreates the klubhub database and replaces every
# object in the configured S3 bucket. All current data will be lost.
#
# Environment (required for the storage step):
#   S3_ENDPOINT      default: http://127.0.0.1:39000
#   S3_ACCESS_KEY    required
#   S3_SECRET_KEY    required
#   S3_BUCKET        default: klubhub
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ---- Arg parsing ---------------------------------------------------------
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"
ARCHIVE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file)
      COMPOSE_FILE="$2"
      shift 2
      ;;
    -h|--help)
      sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    --yes)
      CONFIRM="yes"
      shift
      ;;
    -*)
      echo "[restore] ERROR: unknown flag: $1" >&2
      exit 2
      ;;
    *)
      if [[ -z "${ARCHIVE}" ]]; then
        ARCHIVE="$1"
      else
        echo "[restore] ERROR: unexpected extra argument: $1" >&2
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "${ARCHIVE}" ]]; then
  echo "[restore] ERROR: missing archive argument" >&2
  echo "[restore] Usage: $0 [-f compose.yml] [--yes] <backup-archive.tar.gz>" >&2
  exit 2
fi
if [[ ! -f "${ARCHIVE}" ]]; then
  echo "[restore] ERROR: archive not found: ${ARCHIVE}" >&2
  exit 1
fi
if [[ ! -f "${COMPOSE_FILE}" ]]; then
  echo "[restore] ERROR: compose file not found: ${COMPOSE_FILE}" >&2
  exit 1
fi

echo "[restore] Compose file: ${COMPOSE_FILE}"
echo "[restore] Archive:      ${ARCHIVE}"

if [[ "${CONFIRM:-}" != "yes" ]]; then
  echo "[restore] WARNING: This will DROP and recreate the klubhub database and"
  echo "[restore]          REPLACE every object in the configured S3 bucket."
  read -r -p "[restore] Type 'yes' to continue: " CONFIRM
fi
if [[ "${CONFIRM}" != "yes" ]]; then
  echo "[restore] Aborted."
  exit 0
fi

# ---- Resolve required secrets -------------------------------------------
if [[ -z "${S3_ACCESS_KEY:-}" ]]; then
  echo "[restore] ERROR: S3_ACCESS_KEY env var is required" >&2
  exit 1
fi
if [[ -z "${S3_SECRET_KEY:-}" ]]; then
  echo "[restore] ERROR: S3_SECRET_KEY env var is required" >&2
  exit 1
fi
export AWS_ACCESS_KEY_ID="${S3_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${S3_SECRET_KEY}"
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN

S3_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:39000}"
S3_BUCKET="${S3_BUCKET:-klubhub}"

# ---- Extract archive AND validate BEFORE stopping services ----------------------------------------------------
# We extract into a tempdir and validate the layout there. If the archive is
# malformed, we abort now WITHOUT touching the running stack. Stopping api
# + frontend for a bad archive would leave the user with a stopped service
# for no reason.
WORK_DIR=$(mktemp -d)
# Use script-scoped var so trap fires after set -e sees the live binding
# (verifying-infra-as-code pitfall D).
RESTORE_WORK_DIR="${WORK_DIR}"
cleanup() {
  local rc=$?
  if [[ -d "${RESTORE_WORK_DIR}" ]]; then
    rm -rf "${RESTORE_WORK_DIR}"
  fi
  if [[ ${rc} -ne 0 ]]; then
    echo "[restore] ERROR: restore failed (exit ${rc})" >&2
  fi
}
trap cleanup EXIT

echo "[restore] Extracting archive..."
if ! tar --no-same-owner --no-same-permissions \
       -xzf "${ARCHIVE}" -C "${WORK_DIR}"; then
  echo "[restore] ERROR: tar extract failed (corrupt archive?)" >&2
  exit 1
fi

# Layout: backup.sh creates `work-<ts>/{db.sql,storage/}`. We accept either
# that exact path (preferred) or a top-level `{db.sql,storage/}` (in case the
# operator stripped the `work-*` prefix before archiving).
EXTRACT_ROOT=""
if [[ -f "${WORK_DIR}/db.sql" && -d "${WORK_DIR}/storage" ]]; then
  EXTRACT_ROOT="${WORK_DIR}"
else
  # Glob match for the `work-*` subdirectory. Shellcheck SC2144 wants a
  # for-loop here — we only ever expect at most one matching dir because
  # each backup.sh run names its work dir with a unique timestamp.
  for candidate in "${WORK_DIR}"/work-*; do
    if [[ -d "${candidate}" ]] && \
       [[ -f "${candidate}/db.sql" ]] && \
       [[ -d "${candidate}/storage" ]]; then
      EXTRACT_ROOT="${candidate}"
      break
    fi
  done
fi
if [[ -z "${EXTRACT_ROOT}" ]]; then
  echo "[restore] ERROR: archive missing required db.sql + storage/ entries" >&2
  echo "[restore]        expected either top-level db.sql+storage/ or work-*/{db.sql,storage/}" >&2
  exit 1
fi

DB_DUMP="${EXTRACT_ROOT}/db.sql"
STORAGE_SRC="${EXTRACT_ROOT}/storage"

# Sanity: dump must be non-empty.
if [[ ! -s "${DB_DUMP}" ]]; then
  echo "[restore] ERROR: ${DB_DUMP} is empty" >&2
  exit 1
fi

# ---- Quiesce the application stack --------------------------------------
# Stop api + frontend first so nothing races the database drop. Storage is
# left running because we're about to write to it.
echo "[restore] Stopping api + frontend..."
docker compose -f "${COMPOSE_FILE}" stop api frontend

# ---- 1. Restore PostgreSQL -----------------------------------------------
echo "[restore] Step 1/2: Restoring PostgreSQL..."

# Use psql with ON_ERROR_STOP=1 and a single transaction so the restore
# either succeeds end-to-end or rolls back cleanly. We connect to the
# `postgres` maintenance DB to drop/recreate the target DB without conflict.
docker compose -f "${COMPOSE_FILE}" exec -T db \
  psql -U klubhub -v ON_ERROR_STOP=1 --single-transaction \
    -c "DROP DATABASE IF EXISTS klubhub;" \
    -c "CREATE DATABASE klubhub;" postgres

if ! docker compose -f "${COMPOSE_FILE}" exec -T db \
    psql -U klubhub -d klubhub -v ON_ERROR_STOP=1 --single-transaction \
    < "${DB_DUMP}"; then
  echo "[restore] ERROR: psql restore failed; database may be in a partial state" >&2
  # Best-effort: bring services back up so the user isn't stuck with a
  # stopped stack on a failed restore. Errors here are deliberately ignored.
  docker compose -f "${COMPOSE_FILE}" up -d api frontend 2>/dev/null || true
  exit 1
fi

# ---- 2. Restore Garage S3 storage ----------------------------------------
echo "[restore] Step 2/2: Restoring Garage S3 storage (s3://${S3_BUCKET} via ${S3_ENDPOINT})..."

# `--delete` drops any objects in the bucket that are NOT in the backup.
# This is the right semantic for "restore": the bucket should match the
# backup exactly afterwards.
if ! aws s3 sync "${STORAGE_SRC}/" "s3://${S3_BUCKET}" \
    --endpoint-url "${S3_ENDPOINT}" \
    --delete \
    --only-show-errors; then
  echo "[restore] ERROR: aws s3 sync failed against ${S3_ENDPOINT}" >&2
  docker compose -f "${COMPOSE_FILE}" up -d api frontend 2>/dev/null || true
  exit 1
fi

# ---- Restart the application stack --------------------------------------
echo "[restore] Restarting api + frontend..."
docker compose -f "${COMPOSE_FILE}" up -d api frontend

# Clear the failure trap; we're done.
trap - EXIT
rm -rf "${RESTORE_WORK_DIR}"

echo "[restore] Complete. Verify with: docker compose -f ${COMPOSE_FILE} ps"
echo "[restore]         and: curl -fs http://127.0.0.1:8080/api/v1/health"