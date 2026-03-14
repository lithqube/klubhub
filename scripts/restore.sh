#!/usr/bin/env bash
# KlubHub DJ — Restore Script
# Restores PostgreSQL and MinIO from a backup archive produced by backup.sh.
# Usage: bash scripts/restore.sh <backup-archive.tar.gz>
# WARNING: This DROPS and recreates the klubhub database. All current data will be lost.
# Requires: docker compose stack running (db and storage healthy)
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

# 2. Restore MinIO (copy files back into the storage container volume)
echo "[restore] Restoring MinIO storage..."
CONTAINER_ID=$(docker compose -f "${PROJECT_ROOT}/docker-compose.yml" ps -q storage)
docker cp "${WORK_DIR}/storage/." "${CONTAINER_ID}:/data/"

echo "[restore] Complete. Restart the api service to re-run migrations if needed:"
echo "  docker compose restart api"
