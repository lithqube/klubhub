#!/usr/bin/env bash
# =============================================================================
# scripts/with-secrets.sh — run a command with secrets injected by Infisical.
#
# Local-dev overlay for Infisical secrets management (KlubHub plan D7; adopted from OpenSchild ADR-025). If the
# Infisical CLI is installed AND this repo is linked (.infisical.json present),
# the command is wrapped in `infisical run --env=<env> -- …` so secrets are
# pulled from Infisical at runtime. Otherwise it falls back to running the
# command as-is — it then reads the gitignored .env file, which is today's
# local default. This keeps every command working whether or not a dev has
# adopted Infisical yet (the CLI is not a hard dependency), and keeps the
# OSS self-hosted tier free of any Infisical requirement.
#
# Env:
#   INFISICAL_ENV       target Infisical environment slug (default: dev)
#   INFISICAL_DISABLE   set to 1 to force the .env fallback
#
# Usage:
#   ./scripts/with-secrets.sh pnpm dev
#   ./scripts/with-secrets.sh pnpm nx e2e promoter-e2e
#   INFISICAL_ENV=staging ./scripts/with-secrets.sh -- env | grep S3_
#   ./scripts/with-secrets.sh --check        # report whether injection is active
# =============================================================================
set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

INFISICAL_ENV="${INFISICAL_ENV:-dev}"

MODE="run"
case "${1:-}" in
    --check) MODE="check"; shift ;;
    -h | --help)
        sed -n '3,24p' "${BASH_SOURCE[0]}"
        exit 0
        ;;
esac
[[ "${1:-}" == "--" ]] && shift

active=false
reason=""
if [[ "${INFISICAL_DISABLE:-}" == "1" ]]; then
    reason="INFISICAL_DISABLE=1 set"
elif ! command -v infisical >/dev/null 2>&1; then
    reason="infisical CLI not installed (https://infisical.com/docs/cli/overview)"
elif [[ ! -f "$REPO_ROOT/.infisical.json" ]]; then
    reason="no .infisical.json (copy .infisical.example.json, then run 'infisical init')"
else
    active=true
fi

if [[ "$MODE" == "check" ]]; then
    if [[ "$active" == "true" ]]; then
        printf '\033[32m✓\033[0m Infisical injection ACTIVE — env=%s\n' "$INFISICAL_ENV"
    else
        printf '\033[36mℹ\033[0m Infisical injection INACTIVE — falling back to .env (%s)\n' "$reason"
    fi
    exit 0
fi

if [[ $# -eq 0 ]]; then
    echo "with-secrets.sh: no command given. Try --help." >&2
    exit 2
fi

if [[ "$active" == "true" ]]; then
    exec infisical run --env="$INFISICAL_ENV" -- "$@"
else
    printf '\033[36mℹ\033[0m Infisical inactive (%s) — running with local .env\n' "$reason" >&2
    exec "$@"
fi
