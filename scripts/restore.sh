#!/usr/bin/env bash
# KlubHub DJ — Restore Script
# Restores PostgreSQL and Garage S3 storage from a backup archive produced by backup.sh.
# Usage: bash scripts/restore.sh <backup-archive.tar.gz>
# WARNING: This DROPS and recreates the klubhub database. All current data will be lost.
# Requires: docker compose stack running (db and storage healthy)
# Note: Uses aws s3 CLI with --endpoint-url for Garage S3 API (port 39000)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ARCHIVE="${1:?Usage: $0 <backup-archive.tar.gz>}"

if [ ! -f "${ARCHIVE}" ]; then
  echo "[restore] ERROR: Archive not found: ${ARCHIVE}" >&2
  exit 1
fi

echo "[restore] WARNING: This will DROP and recreate the klubhub database."
echo "[restore] Archive: ${ARCHIVE}"
read -r -p "[restore] Type 'yes' to continue: " CONFIRM
if [ "${CONFIRM}" != "yes" ]; then
  echo "[restore] Aborted."
  exit 0
fi

WORK_DIR=$(mktemp -d)
trap 'rm -rf "${WORK_DIR}"' EXIT

echo "[restore] Extracting archive..."
tar -xzf "${ARCHIVE}" -C "${WORK_DIR}" --strip-components=1

# 1. Restore PostgreSQL
echo "[restore] Restoring PostgreSQL..."
docker compose -f "${PROJECT_ROOT}/docker-compose.yml" exec -T db \
  psql -U klubhub -c "DROP DATABASE IF EXISTS klubhub;" postgres
docker compose -f "${PROJECT_ROOT}/docker-compose.yml" exec -T db \
  psql -U klubhub -c "CREATE DATABASE klubhub;" postgres
docker compose -f "${PROJECT_ROOT}/docker-compose.yml" exec -T db \
  psql -U klubhub -d klubhub < "${WORK_DIR}/db.sql"

# 2. Restore Garage S3 storage (using aws s3 CLI with endpoint-url)
echo "[restore] Restoring Garage S3 storage..."

AWS_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:39000}"
AWS_ACCESS_KEY="${S3_ACCESS_KEY}"
AWS_SECRET_KEY="${S3_SECRET_KEY}"
S3_BUCKET="${S3_BUCKET:-klubhub}"

# Use aws s3 cli with endpoint-url for Garage; --delete removes objects not in backup
aws s3 sync "${WORK_DIR}/storage/" "s3://${S3_BUCKET}" \
  --endpoint-url "${AWS_ENDPOINT}" \
  --access-key "${AWS_ACCESS_KEY}" \
  --secret-key "${AWS_SECRET_KEY}" \
  --delete \
  --no-verify-ssl 2>/dev/null || true

echo "[restore] Complete. Restart the api service to re-run migrations if needed:"
echo "  docker compose restart api"
