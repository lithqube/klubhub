#!/usr/bin/env bash
# KlubHub DJ — Garage Storage Bootstrap
#
# Idempotently provisions a single-node Garage S3 cluster for KlubHub DJ:
#   1. Assigns + applies a layout (only if no layout exists yet)
#   2. Creates the configured bucket (only if it does not exist yet)
#   3. Creates a bucket-scoped access key (only if it does not exist yet)
#   4. Grants the key read+write+owner on the bucket
#
# The script reads secrets from env (never CLI flags) so they don't leak via
# `ps`, and exits non-zero on the first failed step. Idempotent re-runs are
# safe — running it again after a successful bootstrap is a no-op.
#
# Required env:
#   GARAGE_ADMIN_TOKEN       admin token matching garage.toml `[admin]` section
#   S3_BUCKET                bucket to create (default: klubhub)
#   S3_ACCESS_KEY_NAME       key name to create  (default: klubhub-app-key)
#
# Optional env:
#   COMPOSE_FILE             compose file (default: docker-compose.yml)
#   GARAGE_CONTAINER_NAME    container name      (default: klubhub-storage)
#   GARAGE_LAYOUT_ZONE       zone label          (default: dc1)
#   GARAGE_LAYOUT_CAPACITY   capacity per node   (default: 1G)
#
# Outputs on stdout (so callers can capture):
#   S3_ACCESS_KEY=<id>
#   S3_SECRET_KEY=<secret>
#
# Usage:
#   bash scripts/garage-bootstrap.sh                     # default compose
#   bash scripts/garage-bootstrap.sh -f compose.prod.yml # prod compose
#   eval "$(bash scripts/garage-bootstrap.sh)"           # populate env from output
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ---- Arg parsing ---------------------------------------------------------
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file) COMPOSE_FILE="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "[garage-bootstrap] ERROR: unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ ! -f "${COMPOSE_FILE}" ]]; then
  echo "[garage-bootstrap] ERROR: compose file not found: ${COMPOSE_FILE}" >&2
  exit 1
fi

GARAGE_CONTAINER_NAME="${GARAGE_CONTAINER_NAME:-klubhub-storage}"
S3_BUCKET="${S3_BUCKET:-klubhub}"
S3_ACCESS_KEY_NAME="${S3_ACCESS_KEY_NAME:-klubhub-app-key}"
GARAGE_LAYOUT_ZONE="${GARAGE_LAYOUT_ZONE:-dc1}"
GARAGE_LAYOUT_CAPACITY="${GARAGE_LAYOUT_CAPACITY:-1G}"

# ---- Pre-flight checks -----------------------------------------------------
if [[ -z "${GARAGE_ADMIN_TOKEN:-}" ]]; then
  echo "[garage-bootstrap] ERROR: GARAGE_ADMIN_TOKEN env var is required" >&2
  exit 1
fi

# Confirm the garage container is up.
if ! docker compose -f "${COMPOSE_FILE}" ps --status running --services 2>/dev/null \
     | grep -qx "storage"; then
  echo "[garage-bootstrap] ERROR: storage service is not running in ${COMPOSE_FILE}" >&2
  echo "[garage-bootstrap]        start it first: docker compose -f ${COMPOSE_FILE} up -d storage" >&2
  exit 1
fi
if ! docker inspect --format '{{.Name}}' "${GARAGE_CONTAINER_NAME}" >/dev/null 2>&1; then
  echo "[garage-bootstrap] ERROR: container ${GARAGE_CONTAINER_NAME} not found" >&2
  echo "[garage-bootstrap]        set GARAGE_CONTAINER_NAME to match your compose service" >&2
  exit 1
fi

# `garage` CLI subcommands need GARAGE_ADMIN_TOKEN set inside the container too.
# We wrap every `garage` invocation in this helper so the token is always
# present and we surface non-zero exits with the failing step named.
garage() {
  if ! docker exec \
      -e GARAGE_ADMIN_TOKEN="${GARAGE_ADMIN_TOKEN}" \
      "${GARAGE_CONTAINER_NAME}" /garage "$@"; then
    echo "[garage-bootstrap] ERROR: garage $* failed" >&2
    exit 1
  fi
}

echo "[garage-bootstrap] Compose:  ${COMPOSE_FILE}"
echo "[garage-bootstrap] Container: ${GARAGE_CONTAINER_NAME}"
echo "[garage-bootstrap] Bucket:   ${S3_BUCKET}"
echo "[garage-bootstrap] Key name: ${S3_ACCESS_KEY_NAME}"

# ---- 1. Layout -----------------------------------------------------------
# `garage status` shows the current layout. If the capacity column has any
# non-zero entries, a layout is already in place and we skip assign/apply.
echo "[garage-bootstrap] Step 1/4: Checking cluster layout..."
STATUS_JSON="$(docker exec -e GARAGE_ADMIN_TOKEN="${GARAGE_ADMIN_TOKEN}" \
  "${GARAGE_CONTAINER_NAME}" /garage status --no-decorate 2>/dev/null || true)"

# The `--no-decorate` output is a JSON array of nodes. Each entry has a
# `capacity` field with `available` and `allocated` keys; we just need to
# know whether ANY node has a non-zero layout version, which we infer from
# the `roles` field being non-empty. Empty roles == unassigned node.
NEEDS_LAYOUT=true
if [[ -n "${STATUS_JSON}" ]] && \
   printf '%s' "${STATUS_JSON}" | python3 -c "
import json,sys
try:
    nodes = json.load(sys.stdin)
except Exception:
    sys.exit(0)
sys.exit(0 if any(n.get('role') not in (None,'') and n.get('role') != [] for n in nodes) else 1)
" 2>/dev/null; then
  NEEDS_LAYOUT=false
fi

if [[ "${NEEDS_LAYOUT}" == "true" ]]; then
  echo "[garage-bootstrap] No layout found; querying node id..."
  # `garage node id` prints a single uuid; capture the first non-empty line.
  NODE_ID="$(docker exec -e GARAGE_ADMIN_TOKEN="${GARAGE_ADMIN_TOKEN}" \
    "${GARAGE_CONTAINER_NAME}" /garage node id 2>/dev/null | awk 'NF{print; exit}')"
  if [[ -z "${NODE_ID}" ]]; then
    echo "[garage-bootstrap] ERROR: could not determine node id from garage" >&2
    exit 1
  fi
  echo "[garage-bootstrap] Assigning layout to node ${NODE_ID} (zone=${GARAGE_LAYOUT_ZONE} cap=${GARAGE_LAYOUT_CAPACITY})..."
  garage layout assign -z "${GARAGE_LAYOUT_ZONE}" -c "${GARAGE_LAYOUT_CAPACITY}" "${NODE_ID}"
  garage layout apply --version 1
else
  echo "[garage-bootstrap] Layout already applied; skipping assign/apply."
fi

# ---- 2. Bucket -----------------------------------------------------------
echo "[garage-bootstrap] Step 2/4: Ensuring bucket '${S3_BUCKET}'..."
# `garage bucket info <name>` exits 0 with a non-empty body if the bucket
# exists; non-zero + "NoSuchBucket" if not.
if docker exec -e GARAGE_ADMIN_TOKEN="${GARAGE_ADMIN_TOKEN}" \
   "${GARAGE_CONTAINER_NAME}" /garage bucket info "${S3_BUCKET}" >/dev/null 2>&1; then
  echo "[garage-bootstrap] Bucket already exists; skipping create."
else
  garage bucket create "${S3_BUCKET}"
fi

# ---- 3. Key --------------------------------------------------------------
# Garage prints "Key ID: ... Secret: ..." for `key create`. Capture both
# (idempotency: if the key already exists we exit non-zero — callers should
# treat that as "already provisioned" and look up the secret elsewhere, e.g.
# in their secret manager).
echo "[garage-bootstrap] Step 3/4: Creating key '${S3_ACCESS_KEY_NAME}'..."
KEY_OUTPUT="$(docker exec -e GARAGE_ADMIN_TOKEN="${GARAGE_ADMIN_TOKEN}" \
  "${GARAGE_CONTAINER_NAME}" /garage key create "${S3_ACCESS_KEY_NAME}" 2>&1)" || {
  # If the key already exists, Garage exits non-zero with an error containing
  # "already exists". Treat that as success and just reflow the grant step.
  if printf '%s' "${KEY_OUTPUT}" | grep -qi 'already exists\|Key already'; then
    echo "[garage-bootstrap] Key already exists; skipping create."
    KEY_CREATED=false
  else
    echo "${KEY_OUTPUT}" >&2
    echo "[garage-bootstrap] ERROR: garage key create failed" >&2
    exit 1
  fi
}

if [[ "${KEY_CREATED:-true}" == "true" ]]; then
  KEY_ID="$(printf '%s\n' "${KEY_OUTPUT}" | awk -F': *' '/^Key ID:/{print $2; exit}')"
  SECRET="$(printf '%s\n' "${KEY_OUTPUT}" | awk -F': *' '/^Secret key:|^Secret:/{print $2; exit}')"
  if [[ -z "${KEY_ID:-}" || -z "${SECRET:-}" ]]; then
    echo "[garage-bootstrap] ERROR: could not parse key id / secret from garage output:" >&2
    echo "${KEY_OUTPUT}" >&2
    exit 1
  fi
  # Emit machine-readable lines for eval-capture.
  echo "S3_ACCESS_KEY=${KEY_ID}"
  echo "S3_SECRET_KEY=${SECRET}"
fi

# ---- 4. Allow ------------------------------------------------------------
echo "[garage-bootstrap] Step 4/4: Granting key access on bucket..."
garage bucket allow \
  --read --write --owner \
  --bucket "${S3_BUCKET}" \
  --key "${S3_ACCESS_KEY_NAME}"

echo "[garage-bootstrap] Complete."
echo "[garage-bootstrap] Verify: docker exec ${GARAGE_CONTAINER_NAME} /garage bucket info ${S3_BUCKET}"