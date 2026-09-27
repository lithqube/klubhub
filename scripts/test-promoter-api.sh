#!/usr/bin/env bash
# =============================================================================
# scripts/test-promoter-api.sh — HTTP-level API tests for KlubHub Promoter.
#
# Starts a disposable Postgres (docker, random host port, unique name),
# applies the Promoter migrations with `promoter migrate` (the same goose
# provider the binary and pgtest use), creates the instance organisation with
# `promoter bootstrap`, starts `promoter serve` on a free port and runs the
# Bruno collection in tests/bruno-promoter/. Everything is torn down on exit.
#
# All credentials (DB passwords, KEK, owner password, setup token) are
# generated per run, live only in this process's environment and a temp
# directory outside the repo, and are removed on exit.
#
# Usage: scripts/test-promoter-api.sh [extra bru run args, e.g. --bail]
#        pnpm nx run api:test-api
#
# Env:   KEEP_LOGS=1           keep the temp dir (API log, bru results)
#        PROMOTER_TEST_PG_IMAGE  Postgres image (default postgres:16-alpine)
# =============================================================================
set -euo pipefail

ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
COLLECTION="$ROOT/tests/bruno-promoter"
PG_IMAGE="${PROMOTER_TEST_PG_IMAGE:-postgres:16-alpine}"

BRU="$(command -v bru || true)"
[[ -z "$BRU" && -x "$HOME/.local/bin/bru" ]] && BRU="$HOME/.local/bin/bru"
[[ -n "$BRU" ]] || { echo "bru CLI not found (npm i -g @usebruno/cli)" >&2; exit 1; }
command -v docker >/dev/null || { echo "docker is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/klubhub-promoter-api.XXXXXX")"
chmod 700 "$WORK"
CONTAINER="klubhub-promoter-apitest-$$-$(openssl rand -hex 3)"
API_PID=""

cleanup() {
    local code=$?
    if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
        kill "$API_PID" 2>/dev/null || true
        wait "$API_PID" 2>/dev/null || true
    fi
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    if [[ $code -ne 0 && -f "$WORK/api.log" ]]; then
        echo "---- last 40 lines of the API log ----" >&2
        tail -n 40 "$WORK/api.log" >&2 || true
    fi
    if [[ "${KEEP_LOGS:-}" == "1" ]]; then
        # Secrets never touch disk except the setup log; drop it anyway.
        rm -f "$WORK/bootstrap.out"
        echo "Logs kept in $WORK"
    else
        rm -rf "$WORK"
    fi
    exit $code
}
trap cleanup EXIT INT TERM

rand() { openssl rand -base64 48 | tr -d '/+=\n' | cut -c1-32; }
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'; }

OWNER_PW="$(rand)"
APP_PW="$(rand)"
RELAY_PW="$(rand)"

echo "==> Starting disposable Postgres ($PG_IMAGE) as $CONTAINER"
docker run -d --rm --name "$CONTAINER" \
    -e POSTGRES_USER=klubhub_owner -e POSTGRES_PASSWORD="$OWNER_PW" -e POSTGRES_DB=promoter \
    -p 127.0.0.1::5432 "$PG_IMAGE" >/dev/null
for _ in $(seq 1 60); do
    # The init-time server listens on the unix socket only, so a TCP probe
    # succeeds only once the final server is up.
    if docker exec "$CONTAINER" pg_isready -U klubhub_owner -d promoter -h 127.0.0.1 >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
PG_PORT="$(docker port "$CONTAINER" 5432/tcp | head -n1 | awk -F: '{print $NF}')"
[[ -n "$PG_PORT" ]] || { echo "could not read the Postgres port" >&2; exit 1; }

echo "==> Building the promoter binary"
(cd "$ROOT/api" && go build -o "$WORK/promoter" ./cmd/promoter)

API_PORT="$(free_port)"
BASE_URL="http://127.0.0.1:$API_PORT"

export PROMOTER_MIGRATE_DATABASE_URL="postgres://klubhub_owner:${OWNER_PW}@127.0.0.1:${PG_PORT}/promoter?sslmode=disable"
export PROMOTER_DATABASE_URL="postgres://klubhub_app:${APP_PW}@127.0.0.1:${PG_PORT}/promoter?sslmode=disable"
export PROMOTER_APP_DB_PASSWORD="$APP_PW"
export PROMOTER_RELAY_DB_PASSWORD="$RELAY_PW"
export PROMOTER_KEK="$(openssl rand -base64 32)"
export PROMOTER_KEK_ID="kek-apitest"
export PROMOTER_PUBLIC_ORIGIN="$BASE_URL"
export PROMOTER_AUTH_PROVIDER=local
export PROMOTER_BIND_ADDRESS=127.0.0.1
export PROMOTER_PORT="$API_PORT"
export PROMOTER_LOG_LEVEL="${PROMOTER_LOG_LEVEL:-warn}"
unset PROMOTER_NATS_SERVERS PROMOTER_SERVE_FRONTEND

echo "==> Applying migrations"
for _ in $(seq 1 10); do
    if "$WORK/promoter" migrate >"$WORK/migrate.log" 2>&1; then break; fi
    sleep 1
done
grep -q 'migrations applied' "$WORK/migrate.log" || { cat "$WORK/migrate.log" >&2; exit 1; }

OWNER_EMAIL="owner-$(openssl rand -hex 4)@example.org"
echo "==> Bootstrapping the instance organisation"
"$WORK/promoter" bootstrap --org-name "API Test Collective" --slug api-test \
    --timezone Europe/Berlin --currency EUR --owner-email "$OWNER_EMAIL" --owner-name "API Owner" \
    >"$WORK/bootstrap.out" 2>&1 || { cat "$WORK/bootstrap.out" >&2; exit 1; }
SETUP_TOKEN="$(sed -n 's/.*#token=\([^[:space:]]*\).*/\1/p' "$WORK/bootstrap.out" | head -n1)"
rm -f "$WORK/bootstrap.out"
[[ -n "$SETUP_TOKEN" ]] || { echo "bootstrap printed no setup token" >&2; exit 1; }

echo "==> Starting the promoter API on $BASE_URL"
"$WORK/promoter" serve >"$WORK/api.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 60); do
    if curl -fsS "$BASE_URL/api/v1/health" >/dev/null 2>&1; then break; fi
    kill -0 "$API_PID" 2>/dev/null || { echo "API exited during start" >&2; exit 1; }
    sleep 0.5
done
curl -fsS "$BASE_URL/api/v1/health" >/dev/null || { echo "API did not become healthy" >&2; exit 1; }

echo "==> Running Bruno collection"
OWNER_PASSWORD="$(rand)Aa1!"
set +e
(cd "$COLLECTION" && "$BRU" run -r --env local --disable-cookies \
    --env-var "baseUrl=$BASE_URL" \
    --env-var "origin=$BASE_URL" \
    --env-var "ownerEmail=$OWNER_EMAIL" \
    --env-var "ownerPassword=$OWNER_PASSWORD" \
    --env-var "setupToken=$SETUP_TOKEN" \
    --reporter-skip-all-headers \
    --reporter-json "$WORK/results.json" "$@")
status=$?
set -e
exit $status
