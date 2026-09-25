#!/usr/bin/env bash
# =============================================================================
# scripts/promoter-secrets.sh — create the secrets a KlubHub Promoter stack
# needs under .local/promoter/secrets/ (gitignored). Existing files are NEVER
# overwritten: replacing the KEK makes every sealed value unreadable, and KEK
# rotation is a deliberate procedure (docs/SECURITY.md), not a re-run.
#
# Creates: postgres_password, app_db_password, relay_db_password,
#          migrate_database_url, database_url, relay_database_url,
#          kek (base64, 32 bytes), kek_id, nats/{auth.conf,*.creds}
#
# Back up kek + kek_id OFFLINE (password manager / sealed envelope). Lose the
# KEK and personal/financial data is unrecoverable by design.
#
# Usage: scripts/promoter-secrets.sh [--dir .local/promoter/secrets]
# =============================================================================
set -euo pipefail
cd "$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"

DIR=".local/promoter/secrets"
[[ "${1:-}" == "--dir" ]] && DIR="$2"
mkdir -p "$DIR"
chmod 700 "$DIR"
git check-ignore -q "$DIR" || { echo "Refusing: $DIR is not gitignored." >&2; exit 1; }

rand() { openssl rand -base64 48 | tr -d '/+=\n' | cut -c1-40; }
create() { # file, value
    if [[ -e "$DIR/$1" ]]; then
        echo "  keep    $1"
    else
        umask 077
        printf '%s' "$2" > "$DIR/$1"
        echo "  create  $1"
    fi
}

echo "Promoter secrets in $DIR"
create postgres_password "$(rand)"
create app_db_password "$(rand)"
create relay_db_password "$(rand)"
create kek "$(openssl rand -base64 32)"
create kek_id "kek-$(date +%Y%m%d)"

OWNER_PW="$(cat "$DIR/postgres_password")"
create migrate_database_url "postgres://klubhub_owner:${OWNER_PW}@promoter-db:5432/promoter?sslmode=disable"
create database_url "postgres://klubhub_app:$(cat "$DIR/app_db_password")@promoter-db:5432/promoter?sslmode=disable"
create relay_database_url "postgres://klubhub_relay:$(cat "$DIR/relay_db_password")@promoter-db:5432/promoter?sslmode=disable"

if [[ -e "$DIR/nats/auth.conf" ]]; then
    echo "  keep    nats/"
else
    infra/promoter/nats/generate-creds.sh --out-dir "$DIR/nats"
fi

echo
echo "Done. Back up $DIR/kek and $DIR/kek_id offline before storing any data."
