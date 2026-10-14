#!/usr/bin/env bash
# KlubHub DJ — Restore Script (SR-01 hardened)
#
# Destructive PostgreSQL + Garage S3 restore. See scripts/RESTORE.md.
#
# Hardening contract (security finding SR-01):
#   - Resolves the application service from the active compose (defaults to
#     `app`; the obsolete api/frontend names are never assumed).
#   - Executes DROP DATABASE / CREATE DATABASE as their OWN single-statement
#     psql invocations (no --single-transaction) because both are forbidden
#     inside a transaction block (SQLSTATE 25001).
#   - Replays the SQL dump with --single-transaction + ON_ERROR_STOP so a
#     bad dump rolls back atomically without leaving a partial database.
#   - On ANY post-quiesce failure (stop / drop / create / replay / sync)
#     leaves the application stopped. Auto-restart requires the explicit
#     --restart-on-success flag and only fires AFTER replay + sync succeed.
#   - All sensitive preflight (archive validation, configuration parse,
#     secret availability, container ownership, S3 endpoint parity) is
#     delegated to scripts/restore_support.py so the bash stays thin.
#
# Usage:
#   bash scripts/restore.sh -f docker-compose.yml --project NAME \
#       --yes --confirm-target NAME/DATABASE/BUCKET \
#       [--app-service APP] [--db-service DB] [--storage-service STORAGE] \
#       [--archive-sha256 HEX] [--restart-on-success] ARCHIVE.tar.gz
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUPPORT="${SCRIPT_DIR}/restore_support.py"
[[ -f "$SUPPORT" ]] || { echo "[restore] ERROR: $SUPPORT is required" >&2; exit 1; }

COMPOSE_FILE="" PROJECT="" ARCHIVE="" EXPECTED_SHA="" CONFIRM_TARGET=""
DB_SERVICE=db STORAGE_SERVICE=storage APPS_REQUESTED=""
YES=false RESTART=false
QUIESCED=false SUCCESS=false WORK_DIR=""

fail() { echo "[restore] ERROR: $*" >&2; exit 1; }
need_value() { [[ $# -ge 2 && -n "$2" && "$2" != -* ]] || fail "missing value for $1"; }
usage() {
  cat <<USAGE
Usage: restore.sh -f COMPOSE --project PROJECT [--yes --confirm-target PROJECT/DATABASE/BUCKET]
                  [--app-service APP] [--db-service DB] [--storage-service STORAGE]
                  [--archive-sha256 SHA256] [--restart-on-success] ARCHIVE.tar.gz
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file)            need_value "$@"; COMPOSE_FILE="$2"; shift 2 ;;
    -p|--project)         need_value "$@"; PROJECT="$2"; shift 2 ;;
    --archive-sha256)     need_value "$@"; EXPECTED_SHA="$2"; shift 2 ;;
    --confirm-target)     need_value "$@"; CONFIRM_TARGET="$2"; shift 2 ;;
    --app-service)        need_value "$@"; APPS_REQUESTED="${APPS_REQUESTED:+${APPS_REQUESTED} }$2"; shift 2 ;;
    --db-service)         need_value "$@"; DB_SERVICE="$2"; shift 2 ;;
    --storage-service)    need_value "$@"; STORAGE_SERVICE="$2"; shift 2 ;;
    --yes)                YES=true; shift ;;
    --restart-on-success) RESTART=true; shift ;;
    -h|--help)            usage; exit 0 ;;
    -*)                   fail "unknown flag: $1" ;;
    *)
      [[ -z "$ARCHIVE" ]] || fail "extra archive argument: $1"
      ARCHIVE="$1"; shift
      ;;
  esac
done

[[ -n "$PROJECT" ]]    || fail "--project is required (lowercase Compose project name)"
[[ -n "$COMPOSE_FILE" ]] || fail "-f/--file COMPOSE is required"
[[ -f "$COMPOSE_FILE" ]] || fail "compose file not found: $COMPOSE_FILE"
[[ -n "$ARCHIVE" ]]    || fail "archive path is required"
[[ -f "$ARCHIVE" ]]    || fail "archive not found: $ARCHIVE"

for cmd in docker aws python3; do
  command -v "$cmd" >/dev/null || fail "$cmd is required on PATH"
done
[[ -n "${S3_ACCESS_KEY:-}" && -n "${S3_SECRET_KEY:-}" && -n "${S3_ENDPOINT:-}" && -n "${S3_BUCKET:-}" ]] \
  || fail "S3_ACCESS_KEY, S3_SECRET_KEY, S3_ENDPOINT and S3_BUCKET are required"
[[ "$S3_BUCKET" =~ ^[a-z0-9][a-z0-9.-]+[a-z0-9]$ ]] || fail "invalid S3_BUCKET"

export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN
export AWS_EC2_METADATA_DISABLED=true

DC=(docker compose -p "$PROJECT" -f "$COMPOSE_FILE")

APPS_IDS=()
RUNNING_APPS=()
APPS=()

verify_stopped() {
  local i svc id
  for ((i=0; i<${#APPS[@]}; i++)); do
    svc="${APPS[$i]}"
    id="${APPS_IDS[$i]}"
    docker inspect "$id" | python3 "$SUPPORT" inspect "$PROJECT" "$svc" stopped >/dev/null \
      || { echo "[restore] ERROR: $svc did not quiesce" >&2; return 1; }
  done
}

cleanup() {
  local rc=$?
  trap - EXIT INT TERM
  if [[ "$QUIESCED" == true && "$SUCCESS" != true ]]; then
    # Stop may have partially succeeded, or replay/sync may have partially
    # failed. Retry stop, never start against an unverified or partial
    # restore -- that would open the stack onto a possibly corrupt DB.
    if [[ "${#APPS[@]}" -gt 0 ]] && ! "${DC[@]}" stop "${APPS[@]}"; then
      echo "[restore] CRITICAL: docker compose stop failed; stop manually." >&2
    elif [[ "${#APPS[@]}" -gt 0 ]] && ! verify_stopped; then
      echo "[restore] CRITICAL: application did not actually stop; stop manually." >&2
    fi
    echo "[restore] ERROR: restore failed (exit $rc); application services must remain stopped. Fix the cause and rerun the full restore; no automatic restart." >&2
  elif [[ $rc -ne 0 ]]; then
    echo "[restore] ERROR: preflight failed (exit $rc); running services were not changed." >&2
  fi
  [[ -z "$WORK_DIR" ]] || rm -rf "$WORK_DIR"
  exit "$rc"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ---- Preflight: archive, secrets, configuration, container ownership ------

# Sidecar is integrity protection, not proof of provenance. Only trusted dumps.
if [[ -z "$EXPECTED_SHA" && -f "${ARCHIVE}.sha256" ]]; then
  read -r EXPECTED_SHA _ < "${ARCHIVE}.sha256"
fi
[[ -n "$EXPECTED_SHA" ]] || fail "provide trusted --archive-sha256 or ARCHIVE.sha256 sidecar"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/klubhub-restore.XXXXXXXX")"
EXTRACT_ROOT="$(python3 "$SUPPORT" archive "$ARCHIVE" "$EXPECTED_SHA" "$WORK_DIR/payload")"

"${DC[@]}" config --format json > "$WORK_DIR/config.json"
python3 "$SUPPORT" target "$WORK_DIR/config.json" "$PROJECT" "$DB_SERVICE" "$STORAGE_SERVICE" "$APPS_REQUESTED" > "$WORK_DIR/target"
{ read -r APP_NAMES; read -r DB_USER; read -r DATABASE; } < "$WORK_DIR/target"
read -r -a APPS <<< "$APP_NAMES"

# Verify actual containers and their ownership, not just YAML service names.
for service in "$DB_SERVICE" "$STORAGE_SERVICE" "${APPS[@]}"; do
  id="$("${DC[@]}" ps -aq "$service")"
  [[ -n "$id" && "$id" != *$'\n'* ]] || fail "$service must have exactly one existing container"
  if [[ "$service" == "$DB_SERVICE" || "$service" == "$STORAGE_SERVICE" ]]; then
    docker inspect "$id" | python3 "$SUPPORT" inspect "$PROJECT" "$service" running >/dev/null
  else
    docker inspect "$id" | python3 "$SUPPORT" inspect "$PROJECT" "$service" either >/dev/null
    APPS_IDS+=("$id")
  fi
done

binding="$("${DC[@]}" port "$STORAGE_SERVICE" 3900)"
python3 "$SUPPORT" endpoint "$S3_ENDPOINT" "$binding"

# Use the maintenance DB so recovery also works if a previous CREATE failed.
actual="$("${DC[@]}" exec -T "$DB_SERVICE" psql -X --no-password -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 -At \
              -c 'SELECT current_database() || chr(124) || current_user;')"
[[ "$actual" == "postgres|$DB_USER" ]] || fail "maintenance DB/user differs from selected target"

aws s3api head-bucket --bucket "$S3_BUCKET" --endpoint-url "$S3_ENDPOINT" >/dev/null

TARGET="$PROJECT/$DATABASE/$S3_BUCKET"
echo "[restore] Target: $TARGET; services: ${APPS[*]}; S3: $S3_ENDPOINT"
echo "[restore] WARNING: database will be dropped and bucket contents replaced. Only restore trusted SQL."

if [[ "$YES" == true ]]; then
  [[ "$CONFIRM_TARGET" == "$TARGET" ]] || fail "--yes requires exact --confirm-target PROJECT/DATABASE/BUCKET"
else
  read -r -p "[restore] Type '$TARGET' to confirm destruction: " answer || fail "confirmation input unavailable"
  [[ "$answer" == "$TARGET" ]] || fail "confirmation did not match; aborted"
fi

# Snapshot which apps are currently running so --restart-on-success can be
# conservative: never start an app that was already stopped before preflight.
for ((i=0; i<${#APPS[@]}; i++)); do
  svc="${APPS[$i]}"
  if docker inspect "${APPS_IDS[$i]}" \
       | python3 -c 'import json,sys; rows=json.load(sys.stdin); print("running" if rows[0]["State"]["Running"] else "stopped")' \
       | grep -q '^running$'; then
    RUNNING_APPS+=("$svc")
  fi
done

# ---- Quiesce -------------------------------------------------------------
# Set QUIESCED before stop so partial-stop and signal failures also get safe cleanup.
QUIESCED=true
"${DC[@]}" stop "${APPS[@]}"
verify_stopped || fail "application stop did not quiesce every selected container"

# ---- Drop + recreate the database, OUTSIDE a transaction ----------------
# DROP/CREATE DATABASE are forbidden inside a transaction block (SQLSTATE
# 25001). Each statement must be its own single-statement psql call with no
# --single-transaction wrapper. -c "DROP DATABASE IF EXISTS ... WITH (FORCE)"
# terminates other backends first so a stuck S3 cannot stall recovery.
"${DC[@]}" exec -T "$DB_SERVICE" psql -X --no-password -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS \"$DATABASE\" WITH (FORCE);"
"${DC[@]}" exec -T "$DB_SERVICE" psql -X --no-password -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 \
  -c "CREATE DATABASE \"$DATABASE\" OWNER \"$DB_USER\";"

# ---- Replay the SQL dump, INSIDE a single transaction --------------------
# -f - is required: psql -1 does not wrap plain stdin in a transaction.
# ON_ERROR_STOP=1 + --single-transaction means a single bad statement rolls
# back the entire replay; nothing partial remains on disk.
"${DC[@]}" exec -T "$DB_SERVICE" psql -X --no-password -U "$DB_USER" -d "$DATABASE" \
  -v ON_ERROR_STOP=1 --single-transaction -f - < "$EXTRACT_ROOT/db.sql"

# ---- Mirror storage back ------------------------------------------------
# sync compares size/mtime and can skip corrupt same-size newer objects.
# cp always PUTs every archived file; pruning uses exact keys, no comparator.
aws s3 cp "$EXTRACT_ROOT/storage/" "s3://$S3_BUCKET/" --endpoint-url "$S3_ENDPOINT" --recursive --only-show-errors
python3 "$SUPPORT" prune-storage "$EXTRACT_ROOT/storage" "$S3_BUCKET" "$S3_ENDPOINT"

# ---- Restart on success, only if explicitly requested --------------------
# Start existing containers only; preserve applications already stopped
# before preflight. Recovery reruns therefore cannot silently reopen a
# damaged stack. Refuse to start if any service failed to actually stop
# earlier (defensive, in case verify_stopped was bypassed).
if [[ "$RESTART" == true && "${#RUNNING_APPS[@]}" -gt 0 ]]; then
  "${DC[@]}" start "${RUNNING_APPS[@]}"
  for svc in "${RUNNING_APPS[@]}"; do
    id="$("${DC[@]}" ps -aq "$svc")"
    docker inspect "$id" | python3 "$SUPPORT" inspect "$PROJECT" "$svc" running >/dev/null
  done
fi

SUCCESS=true
echo "[restore] Complete: SQL replay and storage sync succeeded. Verify application health and business data separately."
if [[ "$RESTART" != true ]]; then
  echo "[restore] Application remains stopped by policy; start it explicitly after verification."
fi