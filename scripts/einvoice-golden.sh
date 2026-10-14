#!/usr/bin/env bash
# Golden checks for e-invoice export against the real generator.
#
# 1. Starts the pinned e-invoice sidecar (the exact image docker-compose.prod.yml
#    runs) and a veraPDF REST server, then runs the Go golden tests: every
#    scenario in all three formats must pass KlubHub's in-process EN 16931 /
#    XRechnung rule engine, and the Factur-X PDFs must be valid PDF/A-3b.
# 2. Keeps every file those tests generate and runs the OFFICIAL KoSIT validator
#    over them (the XML as generated, and the XML embedded in each Factur-X PDF)
#    with the XRechnung configuration: schema plus schematron. Any rejection, or
#    any file the validator did not look at, fails the run.
#
#   bash scripts/einvoice-golden.sh
#
# Needs docker, curl, go, unzip and pdfdetach (poppler: `brew install poppler`
# or `apt-get install poppler-utils`). The containers bind to 127.0.0.1 on free
# ports and are removed on exit. The KoSIT downloads are cached in
# $KOSIT_CACHE (default ~/.cache/klubhub-kosit) and checked against the SHA-256
# values below on every run.
set -euo pipefail

# --- pins: change them together, deliberately ---------------------------------
KOSIT_VERSION=1.6.3
KOSIT_JAR_URL="https://github.com/itplr-kosit/validator/releases/download/v${KOSIT_VERSION}/validator-${KOSIT_VERSION}-standalone.jar"
KOSIT_JAR_SHA256=799e64befca97d4080e03608c80b85dd5a5ecc5f4ae4f35d1116ec2855b9a7c9
XRECHNUNG_CONFIG_URL="https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/download/v2026-08-31/xrechnung-3.0.2-validator-configuration-2026-08-31.zip"
XRECHNUNG_CONFIG_SHA256=2530cd107c414511c5d0462ec10f886910395abfca820db82e83d70bf01221a8
JRE_IMAGE=eclipse-temurin@sha256:cff19e6215689161eb6162c11b86b0c60ddf802164f2eaf48d570f8fb79a36c5
VERAPDF_IMAGE=verapdf/rest@sha256:341359ac6af558f35f03c355a798fec57ba049dd04f0a503f300170e1696573b
# -------------------------------------------------------------------------------

root="$(cd "$(dirname "$0")/.." && pwd)"
image="$(sed -n 's/^[[:space:]]*image: \(gflohr\/e-invoice-eu:[^[:space:]]*\)$/\1/p' "$root/docker-compose.prod.yml" | head -n1)"
[ -n "$image" ] || { echo "no e-invoice image in docker-compose.prod.yml" >&2; exit 1; }

for tool in docker curl go unzip pdfdetach; do
  command -v "$tool" >/dev/null || {
    echo "missing '$tool'. pdfdetach comes with poppler: brew install poppler / apt-get install poppler-utils" >&2
    exit 1
  }
done

sha256_of() { if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

# fetch URL SHA256 DEST: download once, and never trust a file whose checksum differs.
fetch() {
  local url="$1" want="$2" dest="$3"
  if [ -f "$dest" ] && [ "$(sha256_of "$dest")" = "$want" ]; then return 0; fi
  curl -fsSL -o "$dest.part" "$url"
  local got
  got="$(sha256_of "$dest.part")"
  if [ "$got" != "$want" ]; then
    rm -f "$dest.part"
    echo "checksum mismatch for $url" >&2
    echo "  expected $want" >&2
    echo "  got      $got" >&2
    exit 1
  fi
  mv "$dest.part" "$dest"
}

sidecar="einvoice-golden-$$"
verapdf="verapdf-golden-$$"
work="$(mktemp -d)"
cleanup() {
  docker rm -f "$sidecar" "$verapdf" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

cache="${KOSIT_CACHE:-$HOME/.cache/klubhub-kosit}"
mkdir -p "$cache" "$work/out" "$work/report" "$work/cfg"
fetch "$KOSIT_JAR_URL" "$KOSIT_JAR_SHA256" "$cache/validator-${KOSIT_VERSION}.jar"
fetch "$XRECHNUNG_CONFIG_URL" "$XRECHNUNG_CONFIG_SHA256" "$cache/xrechnung-config.zip"
cp "$cache/validator-${KOSIT_VERSION}.jar" "$work/validator.jar"
unzip -oq "$cache/xrechnung-config.zip" -d "$work/cfg"

docker run -d --name "$sidecar" -p 127.0.0.1::3000 "$image" >/dev/null
docker run -d --name "$verapdf" -p 127.0.0.1::8080 "$VERAPDF_IMAGE" >/dev/null

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

echo "== Go golden tests (in-process rules, PDF/A-3b)"
cd "$root/api"
EINVOICE_URL="$einvoice_url" VERAPDF_URL="$verapdf_url" EINVOICE_OUT_DIR="$work/out" SKIP_INTEGRATION=1 \
  go test -count=1 -run 'Golden' ./internal/einvoice/... ./internal/finance/...

echo "== KoSIT validator ${KOSIT_VERSION} + XRechnung configuration"
# The XML inside each Factur-X PDF is what a recipient's software reads, so
# validate that, not just the copy we checked in-process.
shopt -s nullglob
pdfs=("$work"/out/*.pdf)
for pdf in "${pdfs[@]}"; do
  tmp="$(mktemp -d)"
  pdfdetach -saveall -o "$tmp" "$pdf" >/dev/null 2>&1 || true
  found=("$tmp"/*)
  if [ "${#found[@]}" -ne 1 ]; then
    echo "expected exactly one embedded file in $(basename "$pdf"), found ${#found[@]}" >&2
    exit 1
  fi
  mv "${found[0]}" "${pdf%.pdf}.embedded.xml"
  rm -rf "$tmp"
done

xmls=("$work"/out/*.xml)
[ "${#xmls[@]}" -gt 0 ] || { echo "the golden tests produced no files to validate" >&2; exit 1; }
names=()
for x in "${xmls[@]}"; do names+=("out/$(basename "$x")"); done

set +e
docker run --rm --user "$(id -u):$(id -g)" -v "$work":/k -w /k "$JRE_IMAGE" \
  java -jar validator.jar -s cfg/scenarios.xml -r cfg -o report -h "${names[@]}" >"$work/kosit.log" 2>&1
status=$?
set -e

summary="$(grep -E '^Acceptable:' "$work/kosit.log" || true)"
accepted="$(sed -n 's/^Acceptable: *\([0-9]*\).*/\1/p' <<<"$summary")"
rejected="$(sed -n 's/.*Rejected: *\([0-9]*\).*/\1/p' <<<"$summary")"
echo "KoSIT: ${summary:-no summary}  (${#xmls[@]} files given)"

if [ -z "$accepted" ] || [ -z "$rejected" ]; then
  echo "could not read the KoSIT result:" >&2
  tail -30 "$work/kosit.log" >&2
  exit 1
fi
if [ "$((accepted + rejected))" -ne "${#xmls[@]}" ]; then
  echo "KoSIT looked at $((accepted + rejected)) of ${#xmls[@]} files" >&2
  exit 1
fi
if [ "$status" -ne 0 ] || [ "$rejected" -ne 0 ]; then
  echo "KoSIT rejected files:" >&2
  grep -E 'REJECT' "$work/kosit.log" >&2 || tail -30 "$work/kosit.log" >&2
  exit 1
fi
echo "OK: the official validator accepted every generated file."
