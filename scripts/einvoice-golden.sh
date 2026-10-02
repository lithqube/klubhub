#!/usr/bin/env bash
# Golden checks for e-invoice export against the real generator.
#
# Starts the pinned e-invoice sidecar (the exact image docker-compose.prod.yml
# runs) and a veraPDF REST server, then runs the Go golden tests: every
# scenario in all three formats must pass the in-process EN 16931 / XRechnung
# rule engine, and the Factur-X PDFs must be valid PDF/A-3b.
#
#   bash scripts/einvoice-golden.sh
#
# Needs docker, curl and go. Both containers bind to 127.0.0.1 on free ports
# and are removed on exit.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
image="$(sed -n 's/^[[:space:]]*image: \(gflohr\/e-invoice-eu:[^[:space:]]*\)$/\1/p' "$root/docker-compose.prod.yml" | head -n1)"
[ -n "$image" ] || { echo "no e-invoice image in docker-compose.prod.yml" >&2; exit 1; }

sidecar="einvoice-golden-$$"
verapdf="verapdf-golden-$$"
cleanup() { docker rm -f "$sidecar" "$verapdf" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$sidecar" -p 127.0.0.1::3000 "$image" >/dev/null
docker run -d --name "$verapdf" -p 127.0.0.1::8080 verapdf/rest >/dev/null

hostport() { docker port "$1" "$2/tcp" | head -n1 | sed 's/.*://'; }
einvoice_url="http://127.0.0.1:$(hostport "$sidecar" 3000)"
verapdf_url="http://127.0.0.1:$(hostport "$verapdf" 8080)"

wait_for() { # url
  for _ in $(seq 1 60); do
    curl -s -o /dev/null "$1" && return 0
    sleep 1
  done
  echo "timed out waiting for $1" >&2
  return 1
}
wait_for "$einvoice_url/api/schema/invoice"
wait_for "$verapdf_url/api/info"

cd "$root/api"
EINVOICE_URL="$einvoice_url" VERAPDF_URL="$verapdf_url" SKIP_INTEGRATION=1 \
  go test -count=1 -run 'Golden' ./internal/einvoice/... ./internal/finance/...
