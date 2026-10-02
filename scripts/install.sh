#!/usr/bin/env bash
# =============================================================================
# KlubHub one-line installer — production stack for KlubHub DJ, KlubHub
# Promoter, or both, on one machine with Docker.
#
#   curl -fsSL <raw url of this file> | bash
#   curl -fsSL <raw url of this file> | bash -s -- --promoter --yes \
#     --org-name "My Collective" --owner-email me@example.com --owner-name Me
#
# What it does (safe to re-run: it updates, never overwrites secrets or data):
#   1. checks Docker (Compose v2), git, curl, openssl, python3 and free ports
#   2. fetches the KlubHub scripts and Compose files at one commit into
#      ~/klubhub (or --dir) and picks the images built from that same commit
#   3. DJ: private secrets + Garage storage, then the production stack
#   4. Promoter: secrets, encryption key and NATS credentials (nsc runs in a
#      container, nothing to install), the stack, and your organisation with a
#      one-time owner sign-up link
#   5. saves your settings and writes <dir>/klubhub for status, logs, stop,
#      start, config, update and backup reminders
#
# Everything binds to 127.0.0.1 unless you pass --bind. Reach it from other
# devices over a VPN (e.g. Tailscale) or a reverse proxy you control; see
# docs/SELF-HOSTING.md. The whole script is a function called on the last
# line, so a truncated download never runs half an install.
# =============================================================================
set -euo pipefail

REPO_URL="${KLUBHUB_REPO_URL:-https://github.com/lithqube/klubhub.git}"
REGISTRY="ghcr.io/lithqube"
NATS_BOX_IMAGE="natsio/nats-box:0.20.0"
HISTORY=200 # commits searched for the newest published image
# Where ./klubhub fetches this installer again if its saved copy is missing.
INSTALLER_URL="${KLUBHUB_INSTALLER_URL:-https://raw.githubusercontent.com/lithqube/klubhub/main/scripts/install.sh}"

# ---------------------------------------------------------------- output ----
if [[ -t 1 ]]; then B=$'\033[1m'; D=$'\033[2m'; G=$'\033[32m'; Y=$'\033[33m'; R=$'\033[31m'; C=$'\033[36m'; N=$'\033[0m'
else B=""; D=""; G=""; Y=""; R=""; C=""; N=""; fi
step() { printf '\n%s==>%s %s%s%s\n' "$C" "$N" "$B" "$*" "$N"; }
ok()   { printf '  %s✓%s %s\n' "$G" "$N" "$*"; }
warn() { printf '  %s!%s %s\n' "$Y" "$N" "$*" >&2; }
die()  { printf '\n%s✗ %s%s\n' "$R" "$*" "$N" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
  cat <<'EOF'
KlubHub installer

Usage: install.sh [--dj] [--promoter] [options]
  No product flag: asks. With --yes and no product flag: installs both.
  Settings are saved; re-running (or ./klubhub update) keeps them unless you
  pass new ones.

Products
  --dj                     KlubHub DJ (tracklists, social scheduler, press kit, gigs)
  --promoter               KlubHub Promoter (events, guest lists, offline door)
  --both                   both products

Install
  --dir DIR                install directory (default: ~/klubhub)
  --ref REF                branch, tag or commit to install (default: main)
  --dj-tag TAG             DJ image tag (default: newest image built from the
                           installed commit or its history)
  --promoter-tag TAG       Promoter image tag (same default)
  --emulate-arm64          allow the arm64-only DJ image on x86 via QEMU (slow)
  --check                  only check this machine and show what would be
                           installed (commit, images, ports); starts nothing
  -y, --yes                accept defaults, never prompt

Network (both products)
  --bind ADDR              host address to publish on (default: 127.0.0.1;
                           0.0.0.0 = all interfaces, only behind a firewall/VPN)

DJ configuration
  --dj-port PORT           UI + API port (default: 8080)
  --dj-url URL             URL you open DJ at, for CORS (default: http://127.0.0.1:<port>)
  --s3-port PORT           storage (S3) port (default: 39000)
  --s3-url URL             storage URL browsers use for images/downloads
                           (default: http://127.0.0.1:<s3-port>)
  --ra-import              enable Resident Advisor import
  --dj-env KEY=VALUE       any other docker-compose.prod.yml variable (repeatable),
                           e.g. SPOTIFY_CLIENT_ID=… LOG_LEVEL=debug

Promoter configuration
  --promoter-port PORT     UI + API port (default: 8090)
  --origin URL             URL you open Promoter at; door phones need the same
                           (default: http://localhost:<port>)
  --promoter-env KEY=VALUE any other docker-compose.promoter.yml variable (repeatable),
                           e.g. PROMOTER_LOG_LEVEL=debug
  --org-name NAME          organisation name        --slug SLUG      URL name
  --owner-email EMAIL      first owner's email      --owner-name NAME
  --timezone TZ            IANA timezone (default: this machine's)
  --currency CODE          ISO currency (default: EUR)

  --log-level LEVEL        debug | info | warn | error for both products
  -h, --help               this help

Every option also reads KLUBHUB_<OPTION> from the environment, e.g.
KLUBHUB_DIR, KLUBHUB_BIND, KLUBHUB_OWNER_EMAIL.
EOF
}

# ------------------------------------------------------------- prompting ----
TTY=""
if [[ -r /dev/tty ]] && { : </dev/tty; } 2>/dev/null; then TTY=/dev/tty; fi
ask() { # var, question, default — keeps a value already given by flag/env
  local __v="$1" q="$2" def="${3:-}" ans=""
  if [[ -n "${!__v:-}" ]]; then return; fi
  if [[ "$YES" == 1 || -z "$TTY" ]]; then printf -v "$__v" '%s' "$def"; return; fi
  if [[ -n "$def" ]]; then printf '  %s %s[%s]%s: ' "$q" "$D" "$def" "$N" >"$TTY"; else printf '  %s: ' "$q" >"$TTY"; fi
  IFS= read -r ans <"$TTY" || true
  printf -v "$__v" '%s' "${ans:-$def}"
}
confirm() { # question, default y|n
  local q="$1" def="${2:-n}" ans=""
  if [[ "$YES" == 1 || -z "$TTY" ]]; then [[ "$def" == y ]]; return; fi
  printf '  %s %s[%s]%s ' "$q" "$D" "$([[ $def == y ]] && echo Y/n || echo y/N)" "$N" >"$TTY"
  IFS= read -r ans <"$TTY" || true
  ans="${ans:-$def}"; [[ "$ans" =~ ^[Yy] ]]
}

# ------------------------------------------------------------- arguments ----
# Compose variables given on the command line; merged over saved settings.
DJ_SET=(); PROMOTER_SET=()
set_var() { # dj|promoter, KEY=VALUE
  local kv="$2" key="${2%%=*}"
  [[ "$kv" == *=* && "$key" =~ ^[A-Z][A-Z0-9_]*$ ]] || die "Expected KEY=VALUE with an upper-case KEY, got: $kv"
  # CORS_ORIGIN feeds the API's Host/Origin boundary, which only accepts an
  # exact origin: never let an un-normalised value through any route.
  if [[ "$1" == dj && "$key" == CORS_ORIGIN ]]; then normalize_origin "CORS_ORIGIN" "${kv#*=}"; kv="CORS_ORIGIN=$NORM_ORIGIN"; fi
  if [[ "$1" == dj ]]; then DJ_SET+=("$kv"); else PROMOTER_SET+=("$kv"); fi
}
need_val() { [[ $# -ge 2 && -n "$2" && "$2" != --* ]] || die "Option $1 needs a value."; }
valid_port() { [[ "$2" =~ ^[0-9]+$ && "$2" -ge 1 && "$2" -le 65535 ]] || die "$1 must be a port number (1–65535)."; }
# origin_normalize LABEL VALUE -> sets NORM_ORIGIN, or sets ORIGIN_ERR and
# returns 1. The DJ API accepts CORS_ORIGIN only as an exact serialized origin:
# scheme://host[:port], lowercase, no trailing slash, path, query, fragment or
# userinfo, and (like browsers' Origin header) no default port.
origin_normalize() {
  local label="$1" raw="$2" scheme rest host port="" lower
  NORM_ORIGIN=""; ORIGIN_ERR=""
  [[ "$raw" != *[[:space:][:cntrl:]\\]* ]] || { ORIGIN_ERR="$label must be a URL like https://dj.example[:port]: no spaces or backslashes."; return 1; }
  [[ "$raw" =~ ^([A-Za-z][A-Za-z0-9+.-]*)://(.*)$ ]] || { ORIGIN_ERR="$label must be a URL like https://dj.example[:port], got: $raw"; return 1; }
  scheme="$(printf '%s' "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')"; rest="${BASH_REMATCH[2]}"
  [[ "$scheme" == http || "$scheme" == https ]] || { ORIGIN_ERR="$label must start with http:// or https://, got scheme '$scheme'."; return 1; }
  [[ "$rest" != *@* ]] || { ORIGIN_ERR="$label must not contain credentials (user:password@); it is an origin, not a login URL."; return 1; }
  rest="${rest%/}" # one trailing slash is tolerated, nothing more
  [[ "$rest" != */* && "$rest" != *\?* && "$rest" != *\#* ]] || { ORIGIN_ERR="$label must be an origin only (scheme://host[:port]): no path, query or fragment."; return 1; }
  if [[ "$rest" =~ ^(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?)(:([0-9]+))?$ ]]; then
    host="${BASH_REMATCH[1]}"; port="${BASH_REMATCH[4]}"
  else
    ORIGIN_ERR="$label has an invalid host or port: https://dj.example[:port] expected."; return 1
  fi
  if [[ -n "$port" ]]; then
    [[ "${#port}" -le 5 && "$((10#$port))" -ge 1 && "$((10#$port))" -le 65535 ]] || { ORIGIN_ERR="$label port must be 1–65535."; return 1; }
    port="$((10#$port))"
    if [[ ( "$scheme" == http && "$port" == 80 ) || ( "$scheme" == https && "$port" == 443 ) ]]; then port=""; fi
  fi
  lower="$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')"
  NORM_ORIGIN="$scheme://$lower${port:+:$port}"
}
normalize_origin() { origin_normalize "$@" || die "$ORIGIN_ERR"; } # sets NORM_ORIGIN; no subshell, so die exits
valid_url() { [[ "$2" =~ ^https?://[^[:space:]/]+(/[^[:space:]]*)?$ ]] || die "$1 must be a URL like https://host[:port], got: $2"; }

parse_args() {
  DJ="${KLUBHUB_DJ:-}"; PROMOTER="${KLUBHUB_PROMOTER:-}"; YES="${KLUBHUB_YES:-0}"
  DIR="${KLUBHUB_DIR:-$HOME/klubhub}"; REF="${KLUBHUB_REF:-}"
  DJ_TAG="${KLUBHUB_DJ_TAG:-}"; PROMOTER_TAG="${KLUBHUB_PROMOTER_TAG:-}"
  ORIGIN="${KLUBHUB_ORIGIN:-}"; ORG_NAME="${KLUBHUB_ORG_NAME:-}"; SLUG="${KLUBHUB_SLUG:-}"
  OWNER_EMAIL="${KLUBHUB_OWNER_EMAIL:-}"; OWNER_NAME="${KLUBHUB_OWNER_NAME:-}"
  TZNAME="${KLUBHUB_TIMEZONE:-}"; CURRENCY="${KLUBHUB_CURRENCY:-}"; EMULATE="${KLUBHUB_EMULATE_ARM64:-0}"
  CHECK="${KLUBHUB_CHECK:-0}"
  [[ -z "${KLUBHUB_BIND:-}" ]]           || { set_var dj "API_BIND=$KLUBHUB_BIND"; set_var dj "S3_BIND=$KLUBHUB_BIND"; set_var promoter "PROMOTER_BIND=$KLUBHUB_BIND"; }
  [[ -z "${KLUBHUB_DJ_PORT:-}" ]]        || set_var dj "API_PORT=$KLUBHUB_DJ_PORT"
  [[ -z "${KLUBHUB_DJ_URL:-}" ]]         || { normalize_origin KLUBHUB_DJ_URL "$KLUBHUB_DJ_URL"; set_var dj "CORS_ORIGIN=$NORM_ORIGIN"; }
  [[ -z "${KLUBHUB_S3_PORT:-}" ]]        || set_var dj "S3_PORT=$KLUBHUB_S3_PORT"
  [[ -z "${KLUBHUB_S3_URL:-}" ]]         || set_var dj "S3_PUBLIC_ENDPOINT=$KLUBHUB_S3_URL"
  [[ -z "${KLUBHUB_PROMOTER_PORT:-}" ]]  || set_var promoter "PROMOTER_HOST_PORT=$KLUBHUB_PROMOTER_PORT"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dj) DJ=1 ;; --promoter) PROMOTER=1 ;; --both) DJ=1; PROMOTER=1 ;;
      --dir) need_val "$@"; DIR="$2"; shift ;;
      --ref) need_val "$@"; REF="$2"; shift ;;
      --dj-tag) need_val "$@"; DJ_TAG="$2"; shift ;;
      --promoter-tag) need_val "$@"; PROMOTER_TAG="$2"; shift ;;
      --bind) need_val "$@"; set_var dj "API_BIND=$2"; set_var dj "S3_BIND=$2"; set_var promoter "PROMOTER_BIND=$2"; shift ;;
      --dj-port) need_val "$@"; valid_port "$1" "$2"; set_var dj "API_PORT=$2"; shift ;;
      --dj-url) need_val "$@"; normalize_origin "$1" "$2"; set_var dj "CORS_ORIGIN=$NORM_ORIGIN"; shift ;;
      --s3-port) need_val "$@"; valid_port "$1" "$2"; set_var dj "S3_PORT=$2"; shift ;;
      --s3-url) need_val "$@"; valid_url "$1" "$2"; set_var dj "S3_PUBLIC_ENDPOINT=$2"; shift ;;
      --ra-import) set_var dj "FEATURE_RA_IMPORT=true" ;;
      --dj-env) need_val "$@"; set_var dj "$2"; shift ;;
      --promoter-port) need_val "$@"; valid_port "$1" "$2"; set_var promoter "PROMOTER_HOST_PORT=$2"; shift ;;
      --origin) need_val "$@"; valid_url "$1" "$2"; ORIGIN="$2"; shift ;;
      --promoter-env) need_val "$@"; set_var promoter "$2"; shift ;;
      --log-level) need_val "$@"; [[ "$2" =~ ^(debug|info|warn|error)$ ]] || die "--log-level: debug, info, warn or error."
                   set_var dj "LOG_LEVEL=$2"; set_var promoter "PROMOTER_LOG_LEVEL=$2"; shift ;;
      --org-name) need_val "$@"; ORG_NAME="$2"; shift ;;
      --slug) need_val "$@"; SLUG="$2"; shift ;;
      --owner-email) need_val "$@"; OWNER_EMAIL="$2"; shift ;;
      --owner-name) need_val "$@"; OWNER_NAME="$2"; shift ;;
      --timezone) need_val "$@"; TZNAME="$2"; shift ;;
      --currency) need_val "$@"; CURRENCY="$2"; shift ;;
      --emulate-arm64) EMULATE=1 ;;
      --check) CHECK=1 ;;
      -y|--yes) YES=1 ;;
      -h|--help) usage; exit 0 ;;
      *) usage >&2; die "Unknown option: $1" ;;
    esac
    shift
  done
  case "$DIR" in /*) ;; *) DIR="$PWD/$DIR" ;; esac
}

choose_products() {
  if [[ -z "$DJ" && -z "$PROMOTER" && -f "$DIR/.local/installer.env" ]]; then
    DJ="$(saved KLUBHUB_DJ)"; PROMOTER="$(saved KLUBHUB_PROMOTER)" # re-run: same products
  fi
  if [[ "${DJ:-0}" != 1 && "${PROMOTER:-0}" != 1 ]]; then
    if [[ "$YES" == 1 ]]; then
      DJ=1; PROMOTER=1
    elif [[ -z "$TTY" ]]; then
      die "No terminal to ask in. Pass --dj, --promoter or --both (add --yes to accept defaults)."
    else
      printf '\n  Which KlubHub do you want to run?\n' >"$TTY"
      printf '    1) DJ        — tracklist images, social scheduler, press kit, gigs\n' >"$TTY"
      printf '    2) Promoter  — events, guest lists, offline door app, reports\n' >"$TTY"
      printf '    3) Both\n' >"$TTY"
      local c=""; ask c "Choose 1, 2 or 3" 3
      case "$c" in 1) DJ=1 ;; 2) PROMOTER=1 ;; 3) DJ=1; PROMOTER=1 ;; *) die "Please choose 1, 2 or 3." ;; esac
    fi
  fi
  DJ="${DJ:-0}"; PROMOTER="${PROMOTER:-0}"
}

# ------------------------------------------------------ saved settings ----
# .local/installer.env: products, ref, tags, origin.
# .local/dj.env / .local/promoter.env: Compose variables (may hold API keys
# you pass with --dj-env, so all three are private to your user).
saved() { [[ -f "$DIR/.local/installer.env" ]] && sed -n "s/^$1=//p" "$DIR/.local/installer.env" | tail -1; return 0; }
env_file_merge() { # file, KEY=VALUE... — later values win, one line per key
  local f="$1" kv key; shift
  (umask 077; touch "$f")
  for kv in "$@"; do
    key="${kv%%=*}"
    grep -v "^$key=" "$f" >"$f.tmp" || true
    printf '%s=%s\n' "$key" "${kv#*=}" >>"$f.tmp"
    mv "$f.tmp" "$f"; chmod 600 "$f"
  done
}
env_get() { [[ -f "$1" ]] && sed -n "s/^$2=//p" "$1" | tail -1; return 0; }
with_env() { # env file, command... — exports the file's variables for one command
  local f="$1"; shift
  ( if [[ -f "$f" ]]; then while IFS= read -r line; do [[ "$line" == *=* ]] && export "${line?}"; done <"$f"; fi; "$@" )
}

load_settings() {
  # Keep a product installed earlier when this run only touches the other one.
  DJ_KEEP=0; PROMOTER_KEEP=0
  if [[ "$(saved KLUBHUB_DJ)" == 1 && "$DJ" != 1 ]]; then DJ_KEEP=1; fi
  if [[ "$(saved KLUBHUB_PROMOTER)" == 1 && "$PROMOTER" != 1 ]]; then PROMOTER_KEEP=1; fi
  local old_dj_port; old_dj_port="$(env_get "$DIR/.local/dj.env" API_PORT)"
  if [[ ${#DJ_SET[@]} -gt 0 ]]; then env_file_merge "$DIR/.local/dj.env" "${DJ_SET[@]}"; fi
  if [[ ${#PROMOTER_SET[@]} -gt 0 ]]; then env_file_merge "$DIR/.local/promoter.env" "${PROMOTER_SET[@]}"; fi
  DJ_PORT="$(env_get "$DIR/.local/dj.env" API_PORT)"; DJ_PORT="${DJ_PORT:-8080}"
  S3_PORT_V="$(env_get "$DIR/.local/dj.env" S3_PORT)"; S3_PORT_V="${S3_PORT_V:-39000}"
  P_PORT="$(env_get "$DIR/.local/promoter.env" PROMOTER_HOST_PORT)"; P_PORT="${P_PORT:-8090}"
  DJ_HOST="$(probe_host "$(env_get "$DIR/.local/dj.env" API_BIND)")"
  P_HOST="$(probe_host "$(env_get "$DIR/.local/promoter.env" PROMOTER_BIND)")"
  # Local URLs follow the ports (also after a port change); custom URLs you
  # passed (--dj-url, --s3-url, --origin) are never rewritten.
  if [[ "$DJ" == 1 ]]; then
    follow_port "$DIR/.local/dj.env" CORS_ORIGIN "http://127.0.0.1:$DJ_PORT"
    follow_port "$DIR/.local/dj.env" S3_PUBLIC_ENDPOINT "http://127.0.0.1:$S3_PORT_V"
    check_saved_cors_origin "${old_dj_port:-8080}"
  fi
  if [[ "$PROMOTER" == 1 ]]; then
    ORIGIN="${ORIGIN:-$(saved KLUBHUB_ORIGIN)}"
    if [[ "$ORIGIN" =~ ^http://localhost:[0-9]+$ ]]; then ORIGIN="http://localhost:$P_PORT"; fi
  fi
  return 0
}
follow_port() { # env file, key, local default
  local cur; cur="$(env_get "$1" "$2")"
  if [[ -z "$cur" || "$cur" =~ ^http://127\.0\.0\.1:[0-9]+$ ]]; then env_file_merge "$1" "$2=$3"; fi
}
check_saved_cors_origin() { # previous DJ port — repair/flag a saved CORS_ORIGIN the API would reject
  local cur fixed port; cur="$(env_get "$DIR/.local/dj.env" CORS_ORIGIN)"
  [[ -n "$cur" ]] || return 0
  if ! origin_normalize CORS_ORIGIN "$cur"; then
    warn "Saved CORS_ORIGIN '$cur' is not a bare origin; the API ignores it and refuses saves from that host. Fix: --dj-url https://dj.example"
    return 0
  fi
  fixed="$NORM_ORIGIN"
  if [[ "$fixed" != "$cur" ]]; then
    env_file_merge "$DIR/.local/dj.env" "CORS_ORIGIN=$fixed"; warn "Saved CORS_ORIGIN '$cur' normalized to '$fixed' (the API wants an exact origin)."
  fi
  # A custom origin that still names the old direct port after a port change.
  port="${fixed##*:}"
  if [[ "$fixed" != "http://127.0.0.1:$DJ_PORT" && "$1" != "$DJ_PORT" && "$fixed" =~ :[0-9]+$ && "$port" == "$1" ]] && ! loopback_host "$(origin_host "$fixed")"; then
    warn "CORS_ORIGIN $fixed still names port $1 but DJ now listens on $DJ_PORT. If you open DJ directly (no proxy), re-run with --dj-url ${fixed%:*}:$DJ_PORT"
  fi
}
probe_host() { case "${1:-}" in ""|0.0.0.0|"::") echo 127.0.0.1 ;; *) echo "$1" ;; esac; }
# ---- Host/Origin boundary (api/internal/platform/http/origin.go) ------------
# Unsafe /api/v1 requests (POST/PUT/PATCH/DELETE) are accepted only when the
# request Host is localhost, 127.0.0.1, [::1] or the host of CORS_ORIGIN (ports
# ignored); an Origin header, when sent, must equal the request origin or
# CORS_ORIGIN exactly. GET/HEAD/OPTIONS (health probes, page loads) are exempt.
loopback_host() { case "$1" in localhost|127.0.0.1|"[::1]"|"::1") return 0 ;; esac; return 1; }
origin_host() { # origin -> host without scheme and port
  local h="${1#*://}"; h="${h%/}"
  if [[ "$h" == \[* ]]; then h="${h%%]*}]"; else h="${h%%:*}"; fi
  printf '%s' "$h"
}
dj_published_beyond_loopback() {
  case "$(env_get "$DIR/.local/dj.env" API_BIND)" in ""|127.0.0.1|localhost|"::1"|"[::1]") return 1 ;; esac
  return 0
}
dj_has_public_url() { # a CORS_ORIGIN whose host is not loopback
  local o; o="$(env_get "$DIR/.local/dj.env" CORS_ORIGIN)"
  [[ -n "$o" ]] && ! loopback_host "$(origin_host "$o")"
}
cors_exposure_warning() {
  dj_published_beyond_loopback || return 0
  ! dj_has_public_url || return 0
  warn "DJ is published on $(env_get "$DIR/.local/dj.env" API_BIND), but CORS_ORIGIN is still $(env_get "$DIR/.local/dj.env" CORS_ORIGIN)."
  warn "Saving from any other device (POST/PUT/PATCH/DELETE) will fail with 403 until CORS_ORIGIN is the exact address you open DJ at."
  warn "Re-run with --dj-url http://<lan-ip-or-hostname>:$DJ_PORT (or https://dj.example behind a proxy), or: ./klubhub set dj CORS_ORIGIN=<that URL>"
}
cors_rule_lines() {
  local o h; o="$(env_get "$DIR/.local/dj.env" CORS_ORIGIN)"; h="$(origin_host "$o")"
  printf '    CORS_ORIGIN=%s\n' "$o"
  if loopback_host "$h"; then
    printf '    Allowed Host for saves: localhost, 127.0.0.1, [::1] (any port): loopback only\n'
  else
    printf '    Allowed Host for saves: localhost, 127.0.0.1, [::1], %s (any port)\n' "$h"
  fi
  printf '    A browser Origin must equal the address you opened or CORS_ORIGIN exactly (scheme://host[:port], no trailing slash).\n'
}
offer_public_url() { # interactive only: --yes never prompts and never invents a hostname
  [[ "$DJ" == 1 && "$YES" != 1 && "$CHECK" != 1 && -n "$TTY" ]] || return 0
  dj_published_beyond_loopback || return 0
  ! dj_has_public_url || return 0
  printf '\n  DJ is published beyond this machine. The API accepts saves only from localhost or from the\n  host of CORS_ORIGIN, so enter the exact address you will open DJ at (e.g. http://192.168.1.5:%s\n  or https://dj.example). Leave blank to skip.\n' "$DJ_PORT" >"$TTY"
  local tries=0 pub_url=""
  while [[ $tries -lt 3 ]]; do
    pub_url=""; ask pub_url "Public URL for DJ" ""
    [[ -n "$pub_url" ]] || return 0
    if origin_normalize "Public URL" "$pub_url"; then
      env_file_merge "$DIR/.local/dj.env" "CORS_ORIGIN=$NORM_ORIGIN"; ok "CORS_ORIGIN set to $NORM_ORIGIN"; return 0
    fi
    warn "$ORIGIN_ERR"; tries=$((tries + 1))
  done
  return 0
}


# ------------------------------------------------------------- preflight ----
pkg_hint() {
  if have brew; then echo "brew install $*"
  elif have apt-get; then echo "sudo apt-get install -y $*"
  elif have dnf; then echo "sudo dnf install -y $*"
  elif have apk; then echo "sudo apk add $*"
  else echo "your package manager: $*"; fi
}

preflight() {
  step "Checking this machine"
  OS="$(uname -s)"; ARCH="$(uname -m)"
  case "$OS" in Linux|Darwin) ok "$OS $ARCH" ;; *) die "Unsupported system: $OS. Use Linux or macOS (on Windows, run this inside WSL 2)." ;; esac

  local missing=() t
  for t in curl git openssl; do have "$t" || missing+=("$t"); done
  if [[ "$DJ" == 1 ]] && ! have python3; then missing+=(python3); fi
  [[ ${#missing[@]} -eq 0 ]] || die "Missing: ${missing[*]}. Install with: $(pkg_hint "${missing[@]}") — then run this again."
  ok "curl, git, openssl$([[ $DJ == 1 ]] && echo ", python3")"

  if ! have docker; then
    if [[ "$OS" == Linux ]] && confirm "Docker is not installed. Install it now with Docker's official script (needs sudo)?" n; then
      curl -fsSL https://get.docker.com | sh
      sudo usermod -aG docker "$USER" 2>/dev/null || true
      die "Docker installed. Log out and back in (so your user can run docker), then run this installer again."
    fi
    die "Docker is required: Docker Desktop or OrbStack on macOS, Docker Engine on Linux — https://docs.docker.com/get-docker/"
  fi
  docker info >/dev/null 2>&1 || die "Docker is installed but not running, or your user can't use it. Start Docker Desktop / OrbStack, or on Linux: sudo systemctl start docker (and add yourself to the docker group)."
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required (the 'docker compose' command). Update Docker."
  DOCKER_ARCH="$(docker version --format '{{.Server.Arch}}' 2>/dev/null || echo unknown)"
  ok "Docker $(docker version --format '{{.Server.Version}}' 2>/dev/null) ($DOCKER_ARCH), Compose $(docker compose version --short 2>/dev/null)"

  if [[ "$DJ" == 1 && "$DOCKER_ARCH" != arm64 && "$DOCKER_ARCH" != aarch64 ]]; then
    if docker run --rm --platform linux/arm64 alpine:3.20 true >/dev/null 2>&1; then
      warn "KlubHub DJ images are arm64-only; this $DOCKER_ARCH machine runs them through emulation (slower)."
    elif [[ "$EMULATE" == 1 ]] || confirm "KlubHub DJ images are arm64-only and this $DOCKER_ARCH machine can't run them yet. Enable arm64 emulation (QEMU; runs one privileged container)?" n; then
      docker run --privileged --rm tonistiigi/binfmt --install arm64 >/dev/null
      docker run --rm --platform linux/arm64 alpine:3.20 true >/dev/null 2>&1 || die "arm64 emulation could not be enabled."
      ok "arm64 emulation enabled"
    else
      die "KlubHub DJ needs an arm64 machine (Apple Silicon, Raspberry Pi 5, Graviton/Ampere servers) or --emulate-arm64. Promoter runs anywhere: re-run with --promoter."
    fi
  fi
}

port_busy() { # port
  if have lsof; then lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1
  elif have ss; then ss -ltn "sport = :$1" 2>/dev/null | grep -q LISTEN
  else (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; fi
}
check_ports() { # compose project, "label port"...
  local project="$1" item; shift
  # Our own running stack holds these ports; re-running as an update is fine.
  [[ -z "$(docker compose -p "$project" ps -q 2>/dev/null)" ]] || return 0
  for item in "$@"; do
    if port_busy "${item##* }"; then die "Port ${item##* } (${item% *}) is already in use. Stop whatever uses it, or pick another port (see --help), then run this again."; fi
  done
}

# ------------------------------------------------------------------ fetch ----
fetch_repo() {
  step "Fetching KlubHub ($REF) into $DIR"
  if [[ -d "$DIR/.git" ]]; then
    [[ -z "$(git -C "$DIR" status --porcelain --untracked-files=no)" ]] || die "$DIR has local changes to tracked files. Commit or discard them first; your secrets are not affected."
  elif [[ -e "$DIR" && -n "$(ls -A "$DIR" 2>/dev/null)" ]]; then
    die "$DIR exists and is not a KlubHub install. Choose another --dir."
  else
    mkdir -p "$DIR"; git -C "$DIR" init -q; git -C "$DIR" remote add origin "$REPO_URL"
  fi
  git -C "$DIR" fetch -q --depth "$HISTORY" origin "$REF" || die "Could not fetch '$REF' from $REPO_URL."
  git -C "$DIR" -c advice.detachedHead=false checkout -q --detach FETCH_HEAD
  COMMIT="$(git -C "$DIR" rev-parse --short=7 HEAD)"
  ok "commit $COMMIT"
  mkdir -p "$DIR/.local"; chmod 700 "$DIR/.local"
}

registry_token() { # repo — anonymous pull token, no docker login needed
  curl -fsS "https://ghcr.io/token?scope=repository:lithqube/$1:pull" 2>/dev/null | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
}
image_exists() { # repo, tag
  local token; token="$(registry_token "$1")"; [[ -n "$token" ]] || return 1
  curl -fsS -o /dev/null -H "Authorization: Bearer $token" \
    -H "Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.oci.image.manifest.v1+json" \
    "https://ghcr.io/v2/lithqube/$1/manifests/$2"
}
image_tags() { # repo — every published tag, one per line
  local token; token="$(registry_token "$1")"; [[ -n "$token" ]] || return 1
  curl -fsS -H "Authorization: Bearer $token" "https://ghcr.io/v2/lithqube/$1/tags/list?n=10000" 2>/dev/null \
    | sed -e 's/.*"tags":\[//' -e 's/\].*//' | tr ',' '\n' | tr -d '" '
}
resolve_tag() { # var, image repo, flag name
  # CI publishes an image only when that product's files change, tagged
  # sha-<short commit>. Use the newest one built from the installed commit
  # or its history, so scripts and image always come from the same line.
  local __v="$1" repo="$2" flag="$3" tag="${!1:-}" tags c n
  if [[ -n "$tag" ]]; then
    image_exists "$repo" "$tag" || die "Image $REGISTRY/$repo:$tag is not published. Check the tag passed with $flag."
  else
    tags="$(image_tags "$repo")" || die "Could not reach the image registry (ghcr.io). Check your internet connection."
    for c in $(git -C "$DIR" rev-list --max-count="$HISTORY" HEAD); do
      for n in 7 8 9 10; do
        if grep -qx "sha-${c:0:$n}" <<<"$tags"; then tag="sha-${c:0:$n}"; break 2; fi
      done
    done
    [[ -n "$tag" ]] || die "No published $repo image found for commit $COMMIT or its last $HISTORY ancestors. If it was just pushed, CI may still be building it: wait a few minutes and re-run, or pass $flag <tag>."
  fi
  printf -v "$__v" '%s' "$tag"
  if [[ "$tag" == "sha-$COMMIT" ]]; then ok "image $REGISTRY/$repo:$tag"
  else ok "image $REGISTRY/$repo:$tag $D(newest build for this commit's history)$N"; fi
}

# --------------------------------------------------------------------- DJ ----
install_dj() {
  step "KlubHub DJ"
  resolve_tag DJ_TAG klubhub-dj-api --dj-tag
  check_ports klubhub-dj-prod "UI/API $DJ_PORT" "storage $S3_PORT_V"
  (cd "$DIR" && IMAGE_TAG="$DJ_TAG" with_env "$DIR/.local/dj.env" bash scripts/setup.sh prod) \
    || die "DJ setup failed (see above). Re-running is safe."
  wait_http "http://$DJ_HOST:$DJ_PORT/api/v1/health" "DJ" 300
}

# --------------------------------------------------------------- Promoter ----
nsc_shim() { # a temporary `nsc` that runs inside nats-box; its keystore stays in .local
  NSC_HOME_DIR="$DIR/.local/promoter/nsc"; mkdir -p "$NSC_HOME_DIR"; chmod 700 "$NSC_HOME_DIR"
  SHIM_DIR="$(mktemp -d)"
  cat >"$SHIM_DIR/nsc" <<EOF
#!/usr/bin/env bash
exec docker run --rm -i -u "$(id -u):$(id -g)" -e HOME=/nsc -v "$NSC_HOME_DIR:/nsc" "$NATS_BOX_IMAGE" nsc "\$@"
EOF
  chmod +x "$SHIM_DIR/nsc"
  docker pull -q "$NATS_BOX_IMAGE" >/dev/null
}

slugify() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -e 's/[^a-z0-9][^a-z0-9]*/-/g' -e 's/^-//' -e 's/-$//' | cut -c1-40; }
local_tz() {
  local tz=""
  if [[ "${TZ:-}" == */* ]]; then tz="$TZ"
  elif [[ -L /etc/localtime ]]; then tz="$(readlink /etc/localtime | sed -n 's|.*/zoneinfo/||p')"
  elif have timedatectl; then tz="$(timedatectl show -p Timezone --value 2>/dev/null || true)"; fi
  printf '%s' "${tz:-UTC}"
}
promoter_compose() {
  (cd "$DIR" && PROMOTER_TAG="$PROMOTER_TAG" PROMOTER_PUBLIC_ORIGIN="$ORIGIN" \
    with_env "$DIR/.local/promoter.env" docker compose --env-file /dev/null -f docker-compose.promoter.yml "$@")
}

install_promoter() {
  step "KlubHub Promoter"
  resolve_tag PROMOTER_TAG klubhub-promoter-api --promoter-tag
  check_ports klubhub-promoter "UI/API $P_PORT"
  ask ORIGIN "URL you'll open Promoter at (door phones need the same)" "http://localhost:$P_PORT"
  valid_url "Promoter URL" "$ORIGIN"

  if [[ -e "$DIR/.local/promoter/secrets/nats/auth.conf" ]]; then
    ok "secrets and NATS credentials already exist (kept)"
  else
    nsc_shim
    (cd "$DIR" && PATH="$SHIM_DIR:$PATH" bash scripts/promoter-secrets.sh) | sed 's/^/    /'
    [[ ${PIPESTATUS[0]} -eq 0 ]] || die "Creating Promoter secrets failed."
    rm -rf "$SHIM_DIR"
    ok "secrets, encryption key (KEK) and NATS credentials created in .local/promoter"
  fi

  promoter_compose up -d --quiet-pull || die "Starting Promoter failed. Logs: $DIR/klubhub logs promoter"
  wait_http "http://$P_HOST:$P_PORT/api/v1/health" "Promoter" 240

  if [[ -e "$DIR/.local/promoter/bootstrapped" ]]; then ok "organisation already set up"; return; fi
  printf '\n  %sYour organisation%s %s(the first owner gets a one-time sign-up link)%s\n' "$B" "$N" "$D" "$N"
  ask ORG_NAME "Collective / organisation name" "My Collective"
  ask SLUG "Short name for URLs" "$(slugify "$ORG_NAME")"
  ask OWNER_NAME "Your name" "Owner"
  ask OWNER_EMAIL "Your email" ""
  ask TZNAME "Timezone" "$(local_tz)"
  ask CURRENCY "Currency" "EUR"
  [[ -n "$OWNER_EMAIL" ]] || die "An owner email is required (pass --owner-email)."
  local out
  # The image's default entrypoint supervises the server; CLI commands run the
  # Go binary directly (like the promoter-migrate service does).
  if out="$(promoter_compose run --rm -T --no-deps --entrypoint /promoter promoter bootstrap \
      --org-name "$ORG_NAME" --slug "$SLUG" --timezone "$TZNAME" --currency "$CURRENCY" \
      --owner-email "$OWNER_EMAIL" --owner-name "$OWNER_NAME" 2>&1)"; then
    SETUP_LINK="$(printf '%s\n' "$out" | grep -o 'http[^ ]*/setup#token=[^ ]*' | head -1 || true)"
    : >"$DIR/.local/promoter/bootstrapped"
    ok "organisation \"$ORG_NAME\" created"
  elif grep -q "already has an organisation" <<<"$out"; then
    : >"$DIR/.local/promoter/bootstrapped"; ok "organisation already set up"
  else
    printf '%s\n' "$out" | sed 's/^/    /' >&2; die "Creating the organisation failed. Fix the values and re-run."
  fi
}

wait_http() { # url, label, seconds
  local url="$1" label="$2" deadline=$(( $(date +%s) + $3 ))
  printf '  waiting for %s to be ready ' "$label"
  until curl -fsS -o /dev/null "$url" 2>/dev/null; do
    [[ $(date +%s) -lt $deadline ]] || { echo; die "$label did not become ready in $3 s. Logs: $DIR/klubhub logs"; }
    printf '.'; sleep 3
  done
  printf ' %sready%s\n' "$G" "$N"
}

# ------------------------------------------------------------ management ----
save_installer() { # keep a copy for ./klubhub set/update (curl | bash leaves no file)
  local src="${BASH_SOURCE[0]:-}" dst="$DIR/.local/install.sh"
  if [[ -f "$src" && "$src" -ef "$dst" ]]; then return 0
  elif [[ -f "$src" ]]; then cp "$src" "$dst.tmp"
  else curl -fsSL "$INSTALLER_URL" -o "$dst.tmp" 2>/dev/null || { rm -f "$dst.tmp"; return 0; }
  fi
  if bash -n "$dst.tmp" 2>/dev/null; then chmod 700 "$dst.tmp"; mv "$dst.tmp" "$dst"; else rm -f "$dst.tmp"; fi
}

write_settings() {
  if [[ "$DJ_KEEP" == 1 ]]; then DJ=1; DJ_TAG="${DJ_TAG:-$(saved KLUBHUB_DJ_TAG)}"; fi
  if [[ "$PROMOTER_KEEP" == 1 ]]; then PROMOTER=1; PROMOTER_TAG="${PROMOTER_TAG:-$(saved KLUBHUB_PROMOTER_TAG)}"; ORIGIN="${ORIGIN:-$(saved KLUBHUB_ORIGIN)}"; fi
  (umask 077; {
    echo "# Written by the KlubHub installer; reused by ./klubhub and re-runs."
    echo "KLUBHUB_DJ=$DJ"; echo "KLUBHUB_PROMOTER=$PROMOTER"; echo "KLUBHUB_REF=$REF"
    echo "KLUBHUB_DJ_TAG=${DJ_TAG:-}"; echo "KLUBHUB_PROMOTER_TAG=${PROMOTER_TAG:-}"; echo "KLUBHUB_ORIGIN=${ORIGIN:-}"
    echo "KLUBHUB_INSTALLER_URL=$INSTALLER_URL"
  } >"$DIR/.local/installer.env")
  save_installer
  cat >"$DIR/klubhub" <<'EOF'
#!/usr/bin/env bash
# KlubHub control, written by scripts/install.sh.
#   ./klubhub status | logs [dj|promoter] | stop | start | restart | config
#             | set dj|promoter KEY=VALUE… | update [ref] | backup-info
set -euo pipefail
cd "$(dirname "$0")"
v() { sed -n "s/^$1=//p" .local/installer.env | tail -1; }
DJ="$(v KLUBHUB_DJ)"; PROMOTER="$(v KLUBHUB_PROMOTER)"
with_env() { local f="$1"; shift; ( if [[ -f "$f" ]]; then while IFS= read -r l; do [[ "$l" == *=* ]] && export "${l?}"; done <"$f"; fi; "$@" ); }
dj()       { IMAGE_TAG="$(v KLUBHUB_DJ_TAG)" with_env .local/dj.env \
             docker compose --env-file /dev/null -p klubhub-dj-prod -f docker-compose.prod.yml "$@"; }
promoter() { PROMOTER_TAG="$(v KLUBHUB_PROMOTER_TAG)" PROMOTER_PUBLIC_ORIGIN="$(v KLUBHUB_ORIGIN)" with_env .local/promoter.env \
             docker compose --env-file /dev/null -f docker-compose.promoter.yml "$@"; }
each() { if [[ "$DJ" == 1 ]]; then dj "$@"; fi; if [[ "$PROMOTER" == 1 ]]; then promoter "$@"; fi; }
products() { local f=(); [[ "$DJ" == 1 ]] && f+=(--dj); [[ "$PROMOTER" == 1 ]] && f+=(--promoter); echo "${f[@]}"; }
installer() { # the saved copy, else the one in this checkout, else download it
  if [[ -f .local/install.sh ]]; then exec bash .local/install.sh "$@"
  elif [[ -f scripts/install.sh ]]; then exec bash scripts/install.sh "$@"
  else curl -fsSL "$(v KLUBHUB_INSTALLER_URL)" | exec bash -s -- "$@"; fi
}
case "${1:-status}" in
  status)  each ps ;;
  logs)    case "${2:-}" in dj) dj logs -f --tail 200 ;; promoter) promoter logs -f --tail 200 ;; *) each logs --tail 100 ;; esac ;;
  stop)    each stop ;;
  start)   each up -d ;;
  restart) each up -d --force-recreate ;;
  config)  sed 's/^/  /' .local/installer.env
           for p in dj promoter; do [[ -f .local/$p.env ]] || continue
             echo "  [$p]"; sed -E 's/^([A-Z0-9_]*(SECRET|KEY|TOKEN|PASSWORD)[A-Z0-9_]*)=.*/\1=********/; s/^/    /' ".local/$p.env"; done ;;
  set)     [[ "${2:-}" =~ ^(dj|promoter)$ && $# -ge 3 ]] || { echo "usage: ./klubhub set dj|promoter KEY=VALUE…" >&2; exit 2; }
           p="$2"; shift 2; flags=(); for kv in "$@"; do flags+=("--$p-env" "$kv"); done
           # shellcheck disable=SC2046
           installer $(products) --dir "$PWD" --yes "${flags[@]}" ;;
  update)  # shellcheck disable=SC2046
           installer $(products) --dir "$PWD" --ref "${2:-$(v KLUBHUB_REF)}" --yes ;;
  backup-info)
    echo "Back up these folders (encrypted, offline) together with your data backups:"
    if [[ "$DJ" == 1 ]]; then echo "  $PWD/secrets/prod       DJ secrets and storage configuration"; fi
    if [[ "$PROMOTER" == 1 ]]; then echo "  $PWD/.local/promoter    Promoter secrets, encryption key (KEK) and NATS keys"; fi
    echo "  $PWD/.local/*.env       your settings (may include API keys you passed)"
    echo "Losing the Promoter KEK makes guest and finance data unreadable, by design." ;;
  *) echo "usage: ./klubhub status | logs [dj|promoter] | stop | start | restart | config | set dj|promoter KEY=VALUE… | update [ref] | backup-info" >&2; exit 2 ;;
esac
EOF
  chmod +x "$DIR/klubhub"
}

summary() {
  step "Done"
  if [[ "$DJ" == 1 ]]; then printf '  %sKlubHub DJ%s        %s\n' "$B" "$N" "$(env_get "$DIR/.local/dj.env" CORS_ORIGIN)"; fi
  if [[ "$PROMOTER" == 1 ]]; then
    printf '  %sKlubHub Promoter%s  %s\n' "$B" "$N" "$ORIGIN"
    if [[ -n "${SETUP_LINK:-}" ]]; then
      printf '\n  %sOpen this one-time link within 24 hours to set your owner password:%s\n    %s\n' "$Y" "$N" "$SETUP_LINK"
      printf '  Then add an authenticator app under Account security.\n'
    fi
  fi
  printf '\n  %sBack up your keys now%s (offline: a password manager or an encrypted drive):\n' "$Y" "$N"
  if [[ "$DJ" == 1 ]]; then printf '    %s/secrets/prod\n' "$DIR"; fi
  if [[ "$PROMOTER" == 1 ]]; then printf '    %s/.local/promoter   %s(holds the encryption key: lose it and guest data is gone)%s\n' "$DIR" "$D" "$N"; fi
  printf '\n  Manage it with %s%s/klubhub%s status | logs | config | set | update | backup-info\n' "$B" "$DIR" "$N"
  local bind; bind="$(env_get "$DIR/.local/promoter.env" PROMOTER_BIND)$(env_get "$DIR/.local/dj.env" API_BIND)"
  if [[ -z "$bind" || "$bind" == 127.0.0.1* ]]; then
    printf '  Everything listens on 127.0.0.1 only. For other devices use a VPN (e.g. Tailscale)\n  or a reverse proxy with --dj-url/--s3-url/--origin: docs/SELF-HOSTING.md\n\n'
  else
    printf '  %sPublished beyond this machine.%s KlubHub has no public sign-up protection of its own:\n  keep it behind a firewall, VPN or authenticating proxy.\n\n' "$Y" "$N"
  fi
}

check_only() {
  step "Would install"
  if [[ "$DJ" == 1 ]]; then
    resolve_tag DJ_TAG klubhub-dj-api --dj-tag
    check_ports klubhub-dj-prod "UI/API $DJ_PORT" "storage $S3_PORT_V"
    ok "DJ on $(env_get "$DIR/.local/dj.env" CORS_ORIGIN) (port $DJ_PORT free or ours)"
  fi
  if [[ "$PROMOTER" == 1 ]]; then
    resolve_tag PROMOTER_TAG klubhub-promoter-api --promoter-tag
    check_ports klubhub-promoter "UI/API $P_PORT"
    ok "Promoter on ${ORIGIN:-http://localhost:$P_PORT} (port $P_PORT free or ours)"
  fi
  printf '\n  Everything looks ready. Run again without --check to install.\n\n'
}

main() {
  parse_args "$@"
  printf '%sKlubHub installer%s\n' "$B" "$N"
  choose_products
  REF="${REF:-$(saved KLUBHUB_REF)}"; REF="${REF:-main}"
  preflight
  fetch_repo
  load_settings
  if [[ "$CHECK" == 1 ]]; then check_only; return; fi
  if [[ "$DJ" == 1 ]]; then install_dj; fi
  if [[ "$PROMOTER" == 1 ]]; then install_promoter; fi
  write_settings
  summary
}

# Run only when executed (also as `curl | bash`, where BASH_SOURCE is empty);
# sourcing just defines the functions so tests can exercise them.
(return 0 2>/dev/null) || main "$@"
