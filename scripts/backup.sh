#!/usr/bin/env bash
# KlubHub DJ — Backup Script
# Dumps PostgreSQL and mirrors MinIO data into a timestamped archive.
# Usage: bash scripts/backup.sh [output-dir]
# Requires: docker compose stack running (all four services healthy)
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

# 2. MinIO mirror
echo "[backup] Mirroring MinIO storage..."
mkdir -p "${WORK_DIR}/storage"
docker compose -f "${PROJECT_ROOT}/docker-compose.yml" exec -T storage \
  mc mirror /data "${WORK_DIR}/storage/" 2>/dev/null || true
# Fallback: copy via docker cp if mc mirror fails
if [ ! -d "${WORK_DIR}/storage" ] || [ -z "$(ls -A "${WORK_DIR}/storage" 2>/dev/null)" ]; then
  CONTAINER_ID=$(docker compose -f "${PROJECT_ROOT}/docker-compose.yml" ps -q storage)
  docker cp "${CONTAINER_ID}:/data/." "${WORK_DIR}/storage/"
fi

# 3. Archive
echo "[backup] Creating archive: ${ARCHIVE}"
tar -czf "${ARCHIVE}" -C "${OUTPUT_DIR}" "work-${TIMESTAMP}"
rm -rf "${WORK_DIR}"

echo "[backup] Complete: ${ARCHIVE}"
echo "[backup] Size: $(du -sh "${ARCHIVE}" | cut -f1)"
