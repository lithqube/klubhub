#!/usr/bin/env bash
# =============================================================================
# scripts/test-dj-api.sh — HTTP-level API tests for the KlubHub DJ API.
#
# Starts a disposable Postgres (docker, random host port, unique name) and a
# tiny in-memory S3 stub (scripts/test-dj-api-s3-stub.py), builds ./cmd/api,
# waits for /api/v1/health, seeds one gig + one tracklist + one venue + one
# contact, then runs the Bruno collection in tests/bruno/. Everything is torn
# down on exit.
#
# The DJ API has no auth layer (single-user self-hosted), so the Bruno
# collection exercises the unauthenticated endpoints directly.
#
# Usage: scripts/test-dj-api.sh [extra bru run args, e.g. --bail]
#        pnpm nx run api:test-dj-api
#
# Env:   KEEP_LOGS=1           keep the temp dir (API log, bru results)
#        DJ_TEST_PG_IMAGE        Postgres image (default postgres:16-alpine)
#        DJ_TEST_LOG_LEVEL       API log level (default warn)
# =============================================================================
set -euo pipefail

ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
COLLECTION="$ROOT/tests/bruno"
PG_IMAGE="${DJ_TEST_PG_IMAGE:-postgres:16-alpine}"

BRU="$(command -v bru || true)"
[[ -z "$BRU" && -x "$HOME/.local/bin/bru" ]] && BRU="$HOME/.local/bin/bru"
[[ -n "$BRU" ]] || { echo "bru CLI not found (npm i -g @usebruno/cli)" >&2; exit 1; }
command -v docker >/dev/null || { echo "docker is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/klubhub-dj-api.XXXXXX")"
chmod 700 "$WORK"
CONTAINER="klubhub-dj-apitest-$$-$(openssl rand -hex 3)"
API_PID=""
S3_PID=""

cleanup() {
    local code=$?
    for pid in "$API_PID" "$S3_PID"; do
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
    done
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    if [[ $code -ne 0 && -f "$WORK/api.log" ]]; then
        echo "---- last 40 lines of the API log ----" >&2
        tail -n 40 "$WORK/api.log" >&2 || true
    fi
    if [[ "${KEEP_LOGS:-}" == "1" ]]; then
        echo "Logs kept in $WORK"
    else
        rm -rf "$WORK"
    fi
    exit $code
}
trap cleanup EXIT INT TERM

free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'; }

echo "==> Starting disposable Postgres ($PG_IMAGE) as $CONTAINER"
docker run -d --rm --name "$CONTAINER" \
    -e POSTGRES_USER=klubhub -e POSTGRES_PASSWORD=klubhub -e POSTGRES_DB=klubhub \
    -p 127.0.0.1::5432 "$PG_IMAGE" >/dev/null
for _ in $(seq 1 60); do
    # The init-time server listens on the unix socket only, so a TCP probe
    # succeeds only once the final server is up.
    if docker exec "$CONTAINER" pg_isready -U klubhub -d klubhub -h 127.0.0.1 >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
PG_PORT="$(docker port "$CONTAINER" 5432/tcp | head -n1 | awk -F: '{print $NF}')"
[[ -n "$PG_PORT" ]] || { echo "could not read the Postgres port" >&2; exit 1; }

echo "==> Starting in-memory S3 stub"
S3_PORT="$(free_port)"
python3 "$ROOT/scripts/test-dj-api-s3-stub.py" "$S3_PORT" >"$WORK/s3.log" 2>&1 &
S3_PID=$!
for _ in $(seq 1 20); do
    if curl -fsS "http://127.0.0.1:$S3_PORT/klubhub?location=" >/dev/null 2>&1; then break; fi
    kill -0 "$S3_PID" 2>/dev/null || { echo "S3 stub exited during start" >&2; exit 1; }
    sleep 0.5
done

echo "==> Building the DJ API binary"
(cd "$ROOT/api" && go build -o "$WORK/api" ./cmd/api)

API_PORT="$(free_port)"
BASE_URL="http://127.0.0.1:$API_PORT"

# The API runs its own migrations at boot (migrations.RunMigrations), so no
# separate migrate step is needed. S3_REGION is set so minio-go skips the
# bucket-location lookup when presigning URLs.
export DATABASE_URL="postgres://klubhub:klubhub@127.0.0.1:${PG_PORT}/klubhub?sslmode=disable"
export S3_ENDPOINT="http://127.0.0.1:$S3_PORT"
export S3_ACCESS_KEY="dj-apitest"
export S3_SECRET_KEY="dj-apitest-secret"
export S3_REGION="garage"
export S3_BUCKET="klubhub"
export ICAL_SECRET="test-secret"
export BIND_ADDRESS="127.0.0.1"
export PORT="$API_PORT"
export LOG_LEVEL="${DJ_TEST_LOG_LEVEL:-warn}"
unset SERVE_FRONTEND TOKEN_ENCRYPTION_KEY

echo "==> Starting the DJ API on $BASE_URL"
"$WORK/api" >"$WORK/api.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 60); do
    if curl -fsS "$BASE_URL/api/v1/health" >/dev/null 2>&1; then break; fi
    kill -0 "$API_PID" 2>/dev/null || { echo "API exited during start" >&2; exit 1; }
    sleep 0.5
done
curl -fsS "$BASE_URL/api/v1/health" >/dev/null || { echo "API did not become healthy" >&2; exit 1; }

echo "==> Seeding fixtures (gig, tracklist, venue, contact)"
GIG_ID="$(curl -fsS -X POST "$BASE_URL/api/v1/gigs" \
    -H 'Content-Type: application/json' \
    -d '{"date":"2026-12-01T20:00:00Z","venue":"Seed Arena","city":"Berlin","country":"DE","event_name":"Seed Party","promoter_name":"Seed Promoter","promoter_email":"seed@example.org","promoter_phone":"+49 000","fee_amount":500.5,"fee_currency":"EUR","set_length_minutes":120,"notes":"seeded by test-dj-api.sh"}' \
    | jq -r '.id')"
TRACKLIST_ID="$(curl -fsS -X POST "$BASE_URL/api/v1/tracklists/upload" \
    -F "file=@$ROOT/api/testdata/rekordbox_sample.txt" \
    | jq -r '.tracklist.id')"
VENUE_ID="$(curl -fsS -X POST "$BASE_URL/api/v1/venues" \
    -H 'Content-Type: application/json' \
    -d '{"name":"Seed Venue","city":"Berlin","country":"DE","tech_contact_name":"Seed Tech","tech_contact_email":"tech@example.org","tech_contact_phone":"+49 111","notes":"seeded by test-dj-api.sh"}' \
    | jq -r '.id')"
CONTACT_ID="$(curl -fsS -X POST "$BASE_URL/api/v1/contacts" \
    -H 'Content-Type: application/json' \
    -d '{"name":"Seed Contact","email":"contact@example.org","phone":"+49 222","type":"promoter","notes":"seeded by test-dj-api.sh"}' \
    | jq -r '.id')"
for v in "$GIG_ID" "$TRACKLIST_ID" "$VENUE_ID" "$CONTACT_ID"; do
    [[ "$v" =~ ^[0-9a-f-]{36}$ ]] || { echo "seeding failed (got '$v')" >&2; exit 1; }
done

echo "==> Running Bruno collection"
set +e
(cd "$COLLECTION" && "$BRU" run -r --env local --disable-cookies \
    --env-var "baseUrl=$BASE_URL" \
    --env-var "calendarSecret=test-secret" \
    --env-var "gigId=$GIG_ID" \
    --env-var "tracklistId=$TRACKLIST_ID" \
    --env-var "venueId=$VENUE_ID" \
    --env-var "contactId=$CONTACT_ID" \
    --reporter-skip-all-headers \
    --reporter-json "$WORK/results.json" "$@")
status=$?
set -e
exit $status
