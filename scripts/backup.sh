#!/usr/bin/env bash
# KlubHub DJ — Backup Script
# Dumps PostgreSQL and mirrors Garage S3 storage into a timestamped archive.
# Usage: bash scripts/backup.sh [output-dir]
# Requires: docker compose stack running (all four services healthy)
# Note: Uses aws s3 CLI with --endpoint-url for Garage S3 API (port 39000)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUTPUT_DIR="${1:-${PROJECT_ROOT}/backups}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
WORK_DIR="${OUTPUT_DIR}/work-${TIMESTAMP}"
ARCHIVE="${OUTPUT_DIR}/klubhub-backup-${TIMESTAMP}.tar.gz"

mkdir -p "${WORK_DIR}"

echo "[backup] Starting KlubHub DJ backup — ${TIMESTAMP}"

# 1. PostgreSQL dump
echo "[backup] Dumping PostgreSQL..."
docker compose -f "${PROJECT_ROOT}/docker-compose.yml" exec -T db \
  pg_dump -U klubhub -d klubhub --no-password \
  > "${WORK_DIR}/db.sql"

# 2. Garage S3 storage backup (using aws s3 CLI with endpoint-url)
echo "[backup] Mirroring Garage S3 storage..."
mkdir -p "${WORK_DIR}/storage"

AWS_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:39000}"
AWS_ACCESS_KEY="${S3_ACCESS_KEY}"
AWS_SECRET_KEY="${S3_SECRET_KEY}"
S3_BUCKET="${S3_BUCKET:-klubhub}"

# Use aws s3 cli with endpoint-url for Garage
aws s3 sync "s3://${S3_BUCKET}" "${WORK_DIR}/storage/" \
  --endpoint-url "${AWS_ENDPOINT}" \
  --access-key "${AWS_ACCESS_KEY}" \
  --secret-key "${AWS_SECRET_KEY}" \
  --no-verify-ssl 2>/dev/null || true

# 3. Archive
echo "[backup] Creating archive: ${ARCHIVE}"
tar -czf "${ARCHIVE}" -C "${OUTPUT_DIR}" "work-${TIMESTAMP}"
rm -rf "${WORK_DIR}"

echo "[backup] Complete: ${ARCHIVE}"
echo "[backup] Size: $(du -sh "${ARCHIVE}" | cut -f1)"
