#!/usr/bin/env bash
# KlubHub DJ — Release-Day Smoke Check (Plan G.1)
#
# Runs on a fresh Linux host with only `docker` and `curl` available.
# Pulls the v1.0.0 production compose, fills in the secrets directory,
# brings the stack up, and verifies health, backup, and restore paths.
#
# Expected to be run by the release engineer ONCE per release candidate,
# then archived as `release-day-logs/<date>-<commit>.tar.gz` for
# traceability. NOT for inclusion in the published image.

set -euo pipefail

KH_VER="${KH_VER:-v1.0.1}"
KH_DIR="${KH_DIR:-./klubhub-release-check}"
KH_SECRETS="${KH_SECRETS:-$KH_DIR/secrets}"
KH_LOGS="${KH_LOGS:-$KH_DIR/logs}"

log() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"; }

log "KlubHub DJ release-day smoke check — target version $KH_VER"
log "Working directory: $KH_DIR"
mkdir -p "$KH_DIR" "$KH_SECRETS" "$KH_LOGS"

# 1. Pull the production compose from the v1.0.0 tag.
log "Pulling docker-compose.prod.yml and .env.example from v$KH_VER"
curl -fsSL "https://raw.githubusercontent.com/lithqube/klubhub-dj/$KH_VER/docker-compose.prod.yml" \
  -o "$KH_DIR/docker-compose.prod.yml"
curl -fsSL "https://raw.githubusercontent.com/lithqube/klubhub-dj/$KH_VER/.env.example" \
  -o "$KH_DIR/.env.example"
cp "$KH_DIR/.env.example" "$KH_DIR/.env"

# 2. Generate secrets.
log "Generating secrets in $KH_SECRETS"
for key in postgres_password garage_rpc_secret garage_admin_token \
           token_encryption_key ical_secret; do
  openssl rand -hex 32 > "$KH_SECRETS/$key"
  chmod 600 "$KH_SECRETS/$key"
done
: > "$KH_SECRETS/s3_access_key"
: > "$KH_SECRETS/s3_secret_key"

# Fill the .env from the secrets directory (operator may override).
{
  echo "BIND_ADDRESS=127.0.0.1"
  echo "CORS_ORIGIN=http://127.0.0.1:3000"
  echo "NUXT_PUBLIC_API_BASE=http://api:8080"
  echo "S3_PUBLIC_ENDPOINT=http://127.0.0.1:39000"
  echo "IMAGE_TAG=$KH_VER"
} >> "$KH_DIR/.env"

# 3. Pull and bring the stack up.
log "Pulling images"
(cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml pull)
log "Bringing stack up"
(cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml up -d)

# 4. Wait for the API to become healthy.
log "Waiting for api health endpoint"
for i in $(seq 1 60); do
  status=$(curl -fsS -o /dev/null -w "%{http_code}" \
    "http://127.0.0.1:8080/api/v1/health" 2>/dev/null || echo 000)
  if [ "$status" = "200" ] || [ "$status" = "503" ]; then
    log "Health endpoint reachable (HTTP $status)"
    break
  fi
  sleep 2
done
if [ "$i" -ge 60 ]; then
  log "ERROR: api did not become reachable in 120s. Capturing logs."
  (cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml logs > "$KH_LOGS/timeout.log" 2>&1)
  exit 1
fi

# 5. Print full health JSON.
curl -fsS "http://127.0.0.1:8080/api/v1/health" | tee "$KH_LOGS/health.json"
echo

# 6. Bootstrap Garage (idempotent).
log "Bootstrapping Garage storage"
bash scripts/garage-bootstrap.sh -f "$KH_DIR/docker-compose.prod.yml" -p klubhub | tee "$KH_LOGS/garage-bootstrap.log"

# Capture the S3 keys from the bootstrap output and write to the secrets
# directory, then restart the API so it picks them up.
S3_ACCESS_KEY_OUT=$(grep -E 'S3_ACCESS_KEY=' "$KH_LOGS/garage-bootstrap.log" | head -1 | cut -d= -f2-)
S3_SECRET_KEY_OUT=$(grep -E 'S3_SECRET_KEY=' "$KH_LOGS/garage-bootstrap.log" | head -1 | cut -d= -f2-)
if [ -n "$S3_ACCESS_KEY_OUT" ] && [ -n "$S3_SECRET_KEY_OUT" ]; then
  printf '%s' "$S3_ACCESS_KEY_OUT" > "$KH_SECRETS/s3_access_key"
  printf '%s' "$S3_SECRET_KEY_OUT" > "$KH_SECRETS/s3_secret_key"
  chmod 600 "$KH_SECRETS/s3_access_key" "$KH_SECRETS/s3_secret_key"
  log "S3 keys written to $KH_SECRETS; restarting api"
  (cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml restart api)
  sleep 5
fi

# 7. Run a backup.
log "Running backup"
bash scripts/backup.sh -f "$KH_DIR/docker-compose.prod.yml" -p klubhub "$KH_DIR/backups" \
  | tee "$KH_LOGS/backup.log"

# 8. Run a restore drill (against the backup we just made).
BACKUP_FILE=$(find "$KH_DIR/backups" -name 'klubhub-backup-*.tar.gz' -print -quit)
if [ -n "$BACKUP_FILE" ]; then
  log "Running restore drill against $BACKUP_FILE"
  bash scripts/restore.sh -f "$KH_DIR/docker-compose.prod.yml" -p klubhub "$BACKUP_FILE" \
    | tee "$KH_LOGS/restore-drill.log"
else
  log "No backup file produced; skipping restore drill"
fi

# 9. Final state capture.
log "Capturing final state"
(cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml ps > "$KH_LOGS/ps-final.log" 2>&1)
(cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml logs --tail 200 > "$KH_LOGS/logs-final.log" 2>&1)

# 10. Cleanup — bring stack down (DO NOT remove volumes).
log "Bringing stack down (volumes preserved)"
(cd "$KH_DIR" && docker compose --project-directory "$KH_DIR" -f docker-compose.prod.yml down)

log "Smoke check complete. Logs in $KH_LOGS"
log "Archive for posterity: tar -czf release-day-logs/$KH_VER.tar.gz -C $KH_DIR logs"
